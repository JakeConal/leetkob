package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/review"
)

// ErrReviewNotFound means the lab or finding is absent, or the caller is not
// a current member of the lab's class. The HTTP boundary conceals all three
// as the same 404 so another class's queue is never disclosed.
var ErrReviewNotFound = errors.New("postgres: review resource not found")

// StaleRevisionError reports a TA decision that targeted a superseded finding
// revision. Expected is the caller's value; Current is the durable revision.
type StaleRevisionError struct {
	Expected int
	Current  int
}

// Error implements error.
func (e *StaleRevisionError) Error() string {
	return fmt.Sprintf("postgres: stale finding revision %d (current %d)", e.Expected, e.Current)
}

// ReviewQueueQuery is one assigned-TA queue read. Limit is the already
// validated page size; callers fetch one extra row to detect a next page.
type ReviewQueueQuery struct {
	LabID   string
	ActorID string
	Limit   int
	Offset  int
}

// ReviewQueue is the current private queue for one lab.
type ReviewQueue struct {
	Items             []review.Finding
	ProcessingPending bool
	ReviewPending     int
}

// DecisionCommand is one revision-bound TA decision after authentication.
type DecisionCommand struct {
	LabID            string
	FindingID        string
	ActorID          string
	ExpectedRevision int
	Verdict          string
	Reason           string
	Evidence         *review.Evidence
	DecidedAt        time.Time
}

// reviewFindingColumns selects the TA-visible finding plus its current
// decision. d.finding_revision is bound to the finding revision, so a
// decision can only describe the revision it was recorded against.
const reviewFindingColumns = `
	f.id::text, f.submission_id::text, f.lab_id::text, f.constraint_id,
	f.constraint_revision, f.source_kind, f.checker_verdict, f.unsure_cause,
	f.evidence, f.deduction_cents, f.sampled, f.revision, f.created_at,
	s.source,
	d.verdict, d.reason, d.evidence, d.finding_revision,
	d.decided_by::text, d.decided_at`

// ListReviewQueue returns the assigned TA's private queue for one lab. Only
// current counted submissions are eligible; superseded work never appears or
// counts as an obligation.
//
// The pass sample is persisted before the read. Membership is a pure function
// of immutable finding IDs (see review.SampleIDs), so refreshing an unchanged
// queue returns exactly the same sample instead of redrawing it.
func (s *Store) ListReviewQueue(ctx context.Context, query ReviewQueueQuery) (ReviewQueue, error) {
	var queue ReviewQueue
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var classID string
		if err := tx.QueryRow(ctx,
			`SELECT class_id::text FROM labs WHERE id = $1 FOR UPDATE`, query.LabID,
		).Scan(&classID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrReviewNotFound
			}
			return err
		}
		member, err := classMembership(ctx, tx, classID, query.ActorID)
		if err != nil {
			return err
		}
		if !member {
			return ErrReviewNotFound
		}

		if err := ensureReviewSampling(ctx, tx, query.LabID); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `
			SELECT
				EXISTS (SELECT 1 FROM submissions
				        WHERE lab_id = $1 AND counted AND state <> 'judged')
			 OR EXISTS (SELECT 1 FROM jobs j
			            JOIN submissions s ON s.id = j.submission_id
			            WHERE j.lab_id = $1 AND s.counted AND j.kind = 'check'
			              AND j.state IN ('pending', 'leased'))`, query.LabID,
		).Scan(&queue.ProcessingPending); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM findings f
			JOIN submissions s ON s.id = f.submission_id
			LEFT JOIN decisions d ON d.finding_id = f.id AND d.finding_revision = f.revision
			WHERE f.lab_id = $1 AND s.counted AND d.id IS NULL
			  AND (f.checker_verdict <> 'pass' OR f.sampled)`, query.LabID,
		).Scan(&queue.ReviewPending); err != nil {
			return err
		}

		rows, err := tx.Query(ctx, `
			SELECT`+reviewFindingColumns+`
			FROM findings f
			JOIN submissions s ON s.id = f.submission_id
			LEFT JOIN decisions d ON d.finding_id = f.id AND d.finding_revision = f.revision
			WHERE f.lab_id = $1 AND s.counted
			ORDER BY s.received_at, f.constraint_id, f.id
			LIMIT $2 OFFSET $3`, query.LabID, query.Limit, query.Offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		queue.Items = []review.Finding{}
		for rows.Next() {
			finding, err := scanReviewFinding(rows)
			if err != nil {
				return err
			}
			queue.Items = append(queue.Items, finding)
		}
		return rows.Err()
	})
	if err != nil {
		return ReviewQueue{}, fmt.Errorf("postgres: list review queue: %w", err)
	}
	return queue, nil
}

// RecordReviewDecision binds one explicit decision to the current finding
// revision and appends it to the audit history. The finding row is locked,
// so of two TAs holding the same previous revision exactly one commits and
// the other observes a stale revision instead of overwriting.
//
// Checker provenance (verdict, source kind, evidence, deduction) is never
// updated here, and decisions rows are only ever inserted.
func (s *Store) RecordReviewDecision(ctx context.Context, command DecisionCommand) (review.Finding, error) {
	var updated review.Finding
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var revision int
		var source, classID string
		if err := tx.QueryRow(ctx, `
			SELECT f.revision, s.source, l.class_id::text
			FROM findings f
			JOIN submissions s ON s.id = f.submission_id
			JOIN labs l ON l.id = f.lab_id
			WHERE f.id = $1 AND f.lab_id = $2
			FOR UPDATE OF f`, command.FindingID, command.LabID,
		).Scan(&revision, &source, &classID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrReviewNotFound
			}
			return err
		}
		member, err := classMembership(ctx, tx, classID, command.ActorID)
		if err != nil {
			return err
		}
		if !member {
			return ErrReviewNotFound
		}
		if revision != command.ExpectedRevision {
			return &StaleRevisionError{Expected: command.ExpectedRevision, Current: revision}
		}

		if err := review.ValidateDecisionRequest(command.Verdict, command.Reason, command.Evidence); err != nil {
			return err
		}
		var canonical *review.Evidence
		if command.Evidence != nil {
			checked, err := review.CanonicalEvidence(source, *command.Evidence)
			if err != nil {
				return err
			}
			canonical = &checked
		}
		evidenceJSON, err := review.MarshalEvidence(canonical)
		if err != nil {
			return err
		}

		var newRevision int
		if err := tx.QueryRow(ctx, `
			UPDATE findings SET revision = revision + 1
			WHERE id = $1 AND revision = $2
			RETURNING revision`, command.FindingID, revision,
		).Scan(&newRevision); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &StaleRevisionError{Expected: command.ExpectedRevision, Current: revision}
			}
			return err
		}

		var evidenceParam any
		if evidenceJSON != nil {
			evidenceParam = string(evidenceJSON)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO decisions (id, finding_id, verdict, reason, evidence,
				finding_revision, decided_by, decided_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4::jsonb, $5, $6, $7)`,
			command.FindingID, command.Verdict, command.Reason, evidenceParam,
			newRevision, command.ActorID, command.DecidedAt,
		); err != nil {
			return err
		}

		updated, err = scanReviewFinding(tx.QueryRow(ctx, `
			SELECT`+reviewFindingColumns+`
			FROM findings f
			JOIN submissions s ON s.id = f.submission_id
			LEFT JOIN decisions d ON d.finding_id = f.id AND d.finding_revision = f.revision
			WHERE f.id = $1`, command.FindingID))
		return err
	})
	if err != nil {
		return review.Finding{}, err
	}
	return updated, nil
}

// ensureReviewSampling reconciles the persisted sample with the current
// counted pass pair set. It only writes flags; if nothing changed, an
// unchanged queue performs no finding update.
func ensureReviewSampling(ctx context.Context, tx pgx.Tx, labID string) error {
	rows, err := tx.Query(ctx, `
		SELECT f.id::text, f.sampled, f.required_review
		FROM findings f
		JOIN submissions s ON s.id = f.submission_id
		WHERE f.lab_id = $1 AND s.counted AND f.checker_verdict = 'pass'`, labID)
	if err != nil {
		return err
	}
	var passIDs []string
	sampled := map[string]bool{}
	required := map[string]bool{}
	for rows.Next() {
		var id string
		var isSampled, isRequired bool
		if err := rows.Scan(&id, &isSampled, &isRequired); err != nil {
			rows.Close()
			return err
		}
		passIDs = append(passIDs, id)
		sampled[id] = isSampled
		required[id] = isRequired
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	selected := review.SampledSet(passIDs)
	for _, id := range passIDs {
		want := selected[id]
		if sampled[id] == want && required[id] == want {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE findings SET sampled = $2, required_review = $2 WHERE id = $1`, id, want,
		); err != nil {
			return err
		}
	}

	// Violation and unsure findings are unconditional obligations. Persisting
	// the derived flag keeps required_review consistent for later readers.
	_, err = tx.Exec(ctx, `
		UPDATE findings f SET required_review = true
		WHERE f.lab_id = $1 AND f.checker_verdict <> 'pass' AND NOT f.required_review
		  AND EXISTS (SELECT 1 FROM submissions s WHERE s.id = f.submission_id AND s.counted)`,
		labID)
	return err
}

// classMembership reports whether userID currently belongs to classID.
func classMembership(ctx context.Context, tx pgx.Tx, classID, userID string) (bool, error) {
	var member bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM memberships WHERE class_id = $1 AND user_id = $2)`,
		classID, userID,
	).Scan(&member); err != nil {
		return false, err
	}
	return member, nil
}

// scanReviewFinding reads one joined finding row. Evidence is stored as JSON;
// a decision's finding_revision always equals the finding revision in this
// projection.
func scanReviewFinding(row pgx.Row) (review.Finding, error) {
	var finding review.Finding
	var evidenceJSON []byte
	var unsureCause *string
	var decisionVerdict, decisionReason, decidedBy *string
	var decisionEvidence []byte
	var decisionRevision *int
	var decidedAt *time.Time
	if err := row.Scan(
		&finding.ID, &finding.SubmissionID, &finding.LabID, &finding.ConstraintID,
		&finding.ConstraintRevision, &finding.SourceKind, &finding.CheckerVerdict, &unsureCause,
		&evidenceJSON, &finding.DeductionCents, &finding.Sampled, &finding.Revision,
		&finding.CreatedAt, &finding.Source,
		&decisionVerdict, &decisionReason, &decisionEvidence, &decisionRevision,
		&decidedBy, &decidedAt,
	); err != nil {
		return review.Finding{}, err
	}
	finding.UnsureCause = unsureCause
	finding.Evidence = []review.Evidence{}
	if len(evidenceJSON) > 0 {
		if err := json.Unmarshal(evidenceJSON, &finding.Evidence); err != nil {
			return review.Finding{}, fmt.Errorf("postgres: decode finding evidence: %w", err)
		}
	}
	if decisionVerdict != nil {
		decision := &review.Decision{Verdict: *decisionVerdict}
		if decisionReason != nil {
			decision.Reason = *decisionReason
		}
		if decisionRevision != nil {
			decision.Revision = *decisionRevision
		}
		if decidedBy != nil {
			decision.DecidedBy = *decidedBy
		}
		if decidedAt != nil {
			decision.DecidedAt = *decidedAt
		}
		if len(decisionEvidence) > 0 {
			var evidence review.Evidence
			if err := json.Unmarshal(decisionEvidence, &evidence); err != nil {
				return review.Finding{}, fmt.Errorf("postgres: decode decision evidence: %w", err)
			}
			decision.Evidence = &evidence
		}
		finding.Decision = decision
	}
	return finding, nil
}
