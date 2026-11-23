package postgres

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/review"
)

// ReviewRepo is the assigned-TA queue and the append-only decision record.
type ReviewRepo struct {
	Store *Store
}

var _ review.Store = (*ReviewRepo)(nil)

const decideOperation = "decide-finding"

type replayEnvelope struct {
	Kind     string          `json:"kind"`
	Finding  *review.Finding `json:"finding,omitempty"`
	Expected int             `json:"expected,omitempty"`
	Current  int             `json:"current,omitempty"`
}

// Access reads the user role and current class membership. It does not
// accept a role from the caller.
func (r *ReviewRepo) Access(ctx context.Context, labID, userID string) (review.Access, error) {
	if err := ctx.Err(); err != nil {
		return review.Access{}, err
	}
	if r == nil || r.Store == nil {
		return review.Access{}, errors.New("postgres: review store is not configured")
	}
	var access review.Access
	err := r.Store.pool.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM users WHERE id = $2::uuid),
			COALESCE((SELECT role FROM users WHERE id = $2::uuid), ''),
			EXISTS (SELECT 1 FROM labs WHERE id = $1::uuid),
			EXISTS (
				SELECT 1
				FROM labs l
				JOIN memberships m ON m.class_id = l.class_id
					AND m.user_id = $2::uuid
					AND m.removed_at IS NULL
				WHERE l.id = $1::uuid
			)`, labID, userID).Scan(&access.UserExists, &access.Role, &access.LabExists, &access.Member)
	if err != nil {
		return review.Access{}, err
	}
	return access, nil
}

// List returns the active finding set. RequiredReview is the stored flag.
func (r *ReviewRepo) List(ctx context.Context, labID string, limit int, cursor string) (review.Page, error) {
	if err := ctx.Err(); err != nil {
		return review.Page{}, err
	}
	if r == nil || r.Store == nil {
		return review.Page{}, errors.New("postgres: review store is not configured")
	}
	if limit < 1 {
		return review.Page{}, review.ErrInvalid
	}
	var page review.Page
	err := r.Store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		var labExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM labs WHERE id = $1::uuid)`, labID).Scan(&labExists); err != nil {
			return err
		}
		if !labExists {
			return review.ErrNotFound
		}
		if err := tx.QueryRow(ctx, `
			SELECT
				EXISTS (
					SELECT 1 FROM submissions s
					WHERE s.lab_id = $1::uuid AND s.state IN ('queued', 'judging')
				)
				OR EXISTS (
					SELECT 1
					FROM counted_selections c
					JOIN submissions s ON s.id = c.submission_id
					WHERE c.lab_id = $1::uuid
					  AND s.judge_verdict = 'AC'
					  AND NOT EXISTS (
					      SELECT 1 FROM findings f
					      WHERE f.submission_id = s.id
					        AND f.generation = s.generation
					        AND f.test_revision = s.test_revision
					        AND f.constraint_revision = s.constraint_revision
					  )
				)`, labID).Scan(&page.ProcessingPending); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM findings f
			JOIN submissions s ON s.id = f.submission_id
			JOIN counted_selections c ON c.submission_id = s.id AND c.lab_id = f.lab_id
			WHERE f.lab_id = $1::uuid
			  AND s.judge_verdict = 'AC'
			  AND f.generation = s.generation
			  AND f.test_revision = s.test_revision
			  AND f.constraint_revision = s.constraint_revision
			  AND f.required_review
			  AND NOT EXISTS (SELECT 1 FROM decisions d WHERE d.finding_id = f.id)`, labID).Scan(&page.ReviewPending); err != nil {
			return err
		}
		useCursor := cursor != ""
		var seq int64
		var pos int
		var cursorID string
		if useCursor {
			var err error
			seq, pos, cursorID, err = review.DecodeCursor(cursor)
			if err != nil {
				return err
			}
		}
		rows, err := tx.pg.Query(ctx, `
			SELECT f.id::text, f.submission_id::text, f.lab_id::text, f.constraint_id, f.constraint_revision,
			       f.revision, f.source_kind, f.verdict, COALESCE(f.unsure_cause, ''),
			       f.deduction_hundredths, f.required_review, s.source, s.server_seq, COALESCE(lc.position, 0)
			FROM findings f
			JOIN submissions s ON s.id = f.submission_id
			JOIN counted_selections c ON c.submission_id = s.id AND c.lab_id = f.lab_id
			LEFT JOIN lab_constraints lc
			  ON lc.lab_id = f.lab_id AND lc.revision = f.constraint_revision AND lc.constraint_id = f.constraint_id
			WHERE f.lab_id = $1::uuid
			  AND s.judge_verdict = 'AC'
			  AND f.generation = s.generation
			  AND f.test_revision = s.test_revision
			  AND f.constraint_revision = s.constraint_revision
			  AND (
			      NOT $2::boolean
			      OR (s.server_seq, COALESCE(lc.position, 0), f.id::text) > ($3::bigint, $4::integer, $5)
			  )
			ORDER BY s.server_seq, COALESCE(lc.position, 0), f.id::text
			LIMIT $6`, labID, useCursor, seq, pos, cursorID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		type queued struct {
			finding review.Finding
			seq     int64
			pos     int
		}
		var items []queued
		for rows.Next() {
			var item queued
			var deduction int
			if err := rows.Scan(
				&item.finding.ID, &item.finding.SubmissionID, &item.finding.LabID, &item.finding.ConstraintID,
				&item.finding.ConstraintRevision, &item.finding.Revision, &item.finding.SourceKind,
				&item.finding.CheckerVerdict, &item.finding.UnsureCause, &deduction, &item.finding.RequiredReview,
				&item.finding.Source, &item.seq, &item.pos,
			); err != nil {
				return err
			}
			item.finding.Deduction = review.Deduction(deduction)
			item.finding.Evidence = []review.Evidence{}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		more := len(items) > limit
		if more {
			items = items[:limit]
		}
		page.Items = make([]review.Finding, 0, len(items))
		for _, item := range items {
			evidence, err := loadEvidence(ctx, tx, item.finding.ID)
			if err != nil {
				return err
			}
			item.finding.Evidence = evidence
			decision, err := loadLatestDecision(ctx, tx, item.finding.ID, item.finding.Revision)
			if err != nil {
				return err
			}
			item.finding.Decision = decision
			page.Items = append(page.Items, item.finding)
		}
		if more && len(items) > 0 {
			last := items[len(items)-1]
			encoded := review.EncodeCursor(last.seq, last.pos, last.finding.ID)
			page.NextCursor = &encoded
		}
		return nil
	})
	if err != nil {
		return review.Page{}, err
	}
	if page.Items == nil {
		page.Items = []review.Finding{}
	}
	return page, nil
}

// Decide appends one TA decision when expectedRevision is current.
// The findings update changes only revision.
func (r *ReviewRepo) Decide(ctx context.Context, req review.DecisionRequest) (review.DecisionResult, error) {
	if err := ctx.Err(); err != nil {
		return review.DecisionResult{}, err
	}
	if r == nil || r.Store == nil {
		return review.DecisionResult{}, errors.New("postgres: review store is not configured")
	}
	if req.IdempotencyKey == "" || req.Fingerprint == "" || req.ActorID == "" {
		return review.DecisionResult{}, review.ErrInvalid
	}
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	} else {
		req.Now = req.Now.UTC()
	}
	var result review.DecisionResult
	err := r.Store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		access, err := accessInTx(ctx, tx, req.LabID, req.ActorID)
		if err != nil {
			return err
		}
		if !access.UserExists {
			return review.ErrNotFound
		}
		if !review.Allowed(access) {
			return review.ErrNotAssigned
		}
		if _, err := tx.pg.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, advisoryKey(req.ActorID, decideOperation, req.LabID, req.FindingID, req.IdempotencyKey)); err != nil {
			return err
		}
		if replayed, ok, err := readReplay(ctx, tx, req); err != nil || ok {
			if err != nil {
				return err
			}
			result = replayed
			return nil
		}
		current, err := loadActiveFinding(ctx, tx, req.LabID, req.FindingID)
		if err != nil {
			return err
		}
		next, err := review.ApplyDecision(current, req)
		if stale, ok := err.(*review.StaleRevision); ok {
			result = review.DecisionResult{Stale: &review.StaleRevision{Expected: stale.Expected, Current: stale.Current}}
			return writeReplay(ctx, tx, req, 409, replayEnvelope{Kind: "stale", Expected: stale.Expected, Current: stale.Current})
		}
		if err != nil {
			return err
		}
		var evidenceLine any
		var evidenceText any
		if next.Decision != nil && next.Decision.Evidence != nil {
			evidenceLine = next.Decision.Evidence.Line
			evidenceText = next.Decision.Evidence.Text
		}
		if _, err := tx.pg.Exec(ctx, `
			INSERT INTO decisions (
				finding_id, revision, verdict, reason, evidence_line, evidence_text, actor_user_id, created_at)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::uuid, $8)`,
			req.FindingID, req.ExpectedRevision, req.Verdict, req.Reason, evidenceLine, evidenceText, req.ActorID, req.Now.UTC(),
		); err != nil {
			return err
		}
		var kind, verdict, cause string
		var deduction int
		err = tx.QueryRow(ctx, `
			UPDATE findings
			SET revision = $2
			WHERE id = $1::uuid AND revision = $3
			RETURNING source_kind, verdict, deduction_hundredths, COALESCE(unsure_cause, '')`,
			req.FindingID, next.Revision, req.ExpectedRevision,
		).Scan(&kind, &verdict, &deduction, &cause)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("postgres: finding revision changed during decision")
		}
		if err != nil {
			return err
		}
		if kind != current.SourceKind || verdict != current.CheckerVerdict || review.Deduction(deduction) != current.Deduction || cause != current.UnsureCause {
			return errors.New("postgres: checker provenance changed")
		}
		result = review.DecisionResult{Finding: next}
		return writeReplay(ctx, tx, req, 200, replayEnvelope{Kind: "ok", Finding: &next})
	})
	if err != nil {
		return review.DecisionResult{}, err
	}
	return result, nil
}

func accessInTx(ctx context.Context, tx *Tx, labID, userID string) (review.Access, error) {
	var access review.Access
	err := tx.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM users WHERE id = $2::uuid),
			COALESCE((SELECT role FROM users WHERE id = $2::uuid), ''),
			EXISTS (SELECT 1 FROM labs WHERE id = $1::uuid),
			EXISTS (
				SELECT 1
				FROM labs l
				JOIN memberships m ON m.class_id = l.class_id
					AND m.user_id = $2::uuid
					AND m.removed_at IS NULL
				WHERE l.id = $1::uuid
			)`, labID, userID).Scan(&access.UserExists, &access.Role, &access.LabExists, &access.Member)
	return access, err
}

func loadActiveFinding(ctx context.Context, tx *Tx, labID, findingID string) (review.Finding, error) {
	var finding review.Finding
	var deduction int
	err := tx.QueryRow(ctx, `
		SELECT f.id::text, f.submission_id::text, f.lab_id::text, f.constraint_id, f.constraint_revision,
		       f.revision, f.source_kind, f.verdict, COALESCE(f.unsure_cause, ''),
		       f.deduction_hundredths, f.required_review, s.source
		FROM findings f
		JOIN submissions s ON s.id = f.submission_id
		JOIN counted_selections c ON c.submission_id = s.id AND c.lab_id = f.lab_id
		WHERE f.id = $1::uuid AND f.lab_id = $2::uuid
		  AND s.judge_verdict = 'AC'
		  AND f.generation = s.generation
		  AND f.test_revision = s.test_revision
		  AND f.constraint_revision = s.constraint_revision
		FOR UPDATE OF f`, findingID, labID).Scan(
		&finding.ID, &finding.SubmissionID, &finding.LabID, &finding.ConstraintID, &finding.ConstraintRevision,
		&finding.Revision, &finding.SourceKind, &finding.CheckerVerdict, &finding.UnsureCause,
		&deduction, &finding.RequiredReview, &finding.Source,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return review.Finding{}, review.ErrNotFound
	}
	if err != nil {
		return review.Finding{}, err
	}
	finding.Deduction = review.Deduction(deduction)
	evidence, err := loadEvidence(ctx, tx, finding.ID)
	if err != nil {
		return review.Finding{}, err
	}
	finding.Evidence = evidence
	return finding, nil
}

func loadEvidence(ctx context.Context, tx *Tx, findingID string) ([]review.Evidence, error) {
	rows, err := tx.pg.Query(ctx, `
		SELECT line, evidence_text
		FROM finding_evidence
		WHERE finding_id = $1::uuid
		ORDER BY position`, findingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	evidence := []review.Evidence{}
	for rows.Next() {
		var item review.Evidence
		if err := rows.Scan(&item.Line, &item.Text); err != nil {
			return nil, err
		}
		evidence = append(evidence, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return evidence, nil
}

func loadLatestDecision(ctx context.Context, tx *Tx, findingID string, findingRevision int) (*review.TADecision, error) {
	var verdict, reason, actor string
	var line *int
	var text *string
	var decidedAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT verdict, reason, evidence_line, evidence_text, actor_user_id::text, created_at
		FROM decisions
		WHERE finding_id = $1::uuid
		ORDER BY revision DESC
		LIMIT 1`, findingID).Scan(&verdict, &reason, &line, &text, &actor, &decidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	decision := &review.TADecision{
		Verdict:   verdict,
		Reason:    reason,
		Revision:  findingRevision,
		DecidedBy: actor,
		DecidedAt: decidedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if line != nil && text != nil {
		decision.Evidence = &review.Evidence{Line: *line, Text: *text}
	}
	return decision, nil
}

func readReplay(ctx context.Context, tx *Tx, req review.DecisionRequest) (review.DecisionResult, bool, error) {
	var fingerprint string
	var body []byte
	err := tx.QueryRow(ctx, `
		SELECT request_fingerprint, response_body
		FROM replays
		WHERE actor_user_id = $1::uuid AND operation = $2 AND resource = $3 AND idempotency_key = $4`,
		req.ActorID, decideOperation, replayResource(req), req.IdempotencyKey).Scan(&fingerprint, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return review.DecisionResult{}, false, nil
	}
	if err != nil {
		return review.DecisionResult{}, false, err
	}
	if len(fingerprint) != len(req.Fingerprint) || subtle.ConstantTimeCompare([]byte(fingerprint), []byte(req.Fingerprint)) != 1 {
		return review.DecisionResult{Mismatch: true}, true, nil
	}
	var env replayEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return review.DecisionResult{}, false, err
	}
	if env.Kind == "stale" {
		return review.DecisionResult{
			Replay: true,
			Stale:  &review.StaleRevision{Expected: env.Expected, Current: env.Current},
		}, true, nil
	}
	if env.Finding == nil {
		return review.DecisionResult{}, false, errors.New("postgres: stored decision is empty")
	}
	return review.DecisionResult{Replay: true, Finding: *env.Finding}, true, nil
}

func writeReplay(ctx context.Context, tx *Tx, req review.DecisionRequest, status int, env replayEnvelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = tx.pg.Exec(ctx, `
		INSERT INTO replays (
			actor_user_id, operation, resource, idempotency_key, request_fingerprint, status_code, response_body)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::jsonb)`,
		req.ActorID, decideOperation, replayResource(req), req.IdempotencyKey, req.Fingerprint, status, string(payload))
	return err
}

func replayResource(req review.DecisionRequest) string {
	return req.LabID + "/" + req.FindingID
}

func advisoryKey(parts ...string) int64 {
	hash := fnv.New64a()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return int64(hash.Sum64() & 0x7fffffffffffffff)
}
