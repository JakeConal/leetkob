package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JakeConal/leetkob/backend/internal/checking"
)

// FindingRepo stores checker findings for the current counted submission.
type FindingRepo struct {
	Store *Store
}

var _ checking.Store = (*FindingRepo)(nil)

// Load reads the submission, its lab constraints, and whether a finding set
// already exists for this revision tuple.
func (r *FindingRepo) Load(ctx context.Context, submissionID string) (checking.Submission, error) {
	if err := ctx.Err(); err != nil {
		return checking.Submission{}, err
	}
	if r == nil || r.Store == nil {
		return checking.Submission{}, errors.New("postgres: finding store is not configured")
	}
	var sub checking.Submission
	var displayName string
	err := r.Store.pool.QueryRow(ctx, `
		SELECT s.id::text, s.lab_id::text, s.author_user_id::text, s.source, s.source_sha256,
		       COALESCE(s.judge_verdict, ''), s.generation, s.test_revision, s.constraint_revision,
		       l.statement, u.display_name,
		       EXISTS (
		           SELECT 1 FROM counted_selections c
		           WHERE c.lab_id = s.lab_id AND c.author_user_id = s.author_user_id AND c.submission_id = s.id
		       ),
		       EXISTS (
		           SELECT 1 FROM findings f
		           WHERE f.submission_id = s.id
		             AND f.generation = s.generation
		             AND f.test_revision = s.test_revision
		             AND f.constraint_revision = s.constraint_revision
		       )
		FROM submissions s
		JOIN labs l ON l.id = s.lab_id
		JOIN users u ON u.id = s.author_user_id
		WHERE s.id = $1::uuid`, submissionID).Scan(
		&sub.ID, &sub.LabID, &sub.AuthorUserID, &sub.Source, &sub.SourceSHA256,
		&sub.JudgeVerdict, &sub.Generation, &sub.TestRevision, &sub.ConstraintRevision,
		&sub.Statement, &displayName, &sub.Counted, &sub.Checked,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return checking.Submission{}, checking.ErrNotFound
	}
	if err != nil {
		return checking.Submission{}, err
	}
	if displayName != "" {
		sub.RedactionTerms = append(sub.RedactionTerms, displayName)
	}
	if sub.AuthorUserID != "" {
		sub.RedactionTerms = append(sub.RedactionTerms, sub.AuthorUserID)
	}
	rows, err := r.Store.pool.Query(ctx, `
		SELECT constraint_id, body, deduction_hundredths, position
		FROM lab_constraints
		WHERE lab_id = $1::uuid AND revision = $2
		ORDER BY position, constraint_id`, sub.LabID, sub.ConstraintRevision)
	if err != nil {
		return checking.Submission{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var constraint checking.Constraint
		if err := rows.Scan(&constraint.ID, &constraint.Text, &constraint.DeductionHundredths, &constraint.Position); err != nil {
			return checking.Submission{}, err
		}
		sub.Constraints = append(sub.Constraints, constraint)
	}
	if err := rows.Err(); err != nil {
		return checking.Submission{}, err
	}
	return sub, nil
}

// Save writes the finding set only while the submission is still the counted
// AC work at the same generation and revisions. A mismatch writes nothing.
func (r *FindingRepo) Save(ctx context.Context, sub checking.Submission, findings []checking.Finding) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil || r.Store == nil {
		return errors.New("postgres: finding store is not configured")
	}
	return r.Store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		var verdict, hash string
		var generation, testRevision, constraintRevision int
		err := tx.QueryRow(ctx, `
			SELECT COALESCE(judge_verdict, ''), generation, test_revision, constraint_revision, source_sha256
			FROM submissions
			WHERE id = $1::uuid
			FOR UPDATE`, sub.ID).Scan(&verdict, &generation, &testRevision, &constraintRevision, &hash)
		if errors.Is(err, pgx.ErrNoRows) {
			return checking.ErrStale
		}
		if err != nil {
			return err
		}
		var counted string
		err = tx.QueryRow(ctx, `
			SELECT submission_id::text
			FROM counted_selections
			WHERE lab_id = $1::uuid AND author_user_id = $2::uuid
			FOR UPDATE`, sub.LabID, sub.AuthorUserID).Scan(&counted)
		if errors.Is(err, pgx.ErrNoRows) {
			return checking.ErrStale
		}
		if err != nil {
			return err
		}
		if counted != sub.ID || verdict != "AC" || generation != sub.Generation || testRevision != sub.TestRevision || constraintRevision != sub.ConstraintRevision || hash != sub.SourceSHA256 {
			return checking.ErrStale
		}
		var existing int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM findings
			WHERE submission_id = $1::uuid AND generation = $2 AND test_revision = $3 AND constraint_revision = $4`,
			sub.ID, generation, testRevision, constraintRevision).Scan(&existing); err != nil {
			return err
		}
		if existing > 0 {
			return checking.ErrAlreadyChecked
		}
		for _, finding := range findings {
			if err := insertFinding(ctx, tx, sub, finding); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertFinding(ctx context.Context, tx *Tx, sub checking.Submission, finding checking.Finding) error {
	switch finding.SourceKind {
	case checking.SourceHardRule, checking.SourceSemantic:
	default:
		return fmt.Errorf("postgres: unknown source kind %q", finding.SourceKind)
	}
	switch finding.Verdict {
	case checking.VerdictPass, checking.VerdictViolation, checking.VerdictUnsure:
	default:
		return fmt.Errorf("postgres: unknown verdict %q", finding.Verdict)
	}
	var cause any
	if finding.UnsureCause != "" {
		cause = finding.UnsureCause
	}
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO findings (
			submission_id, lab_id, constraint_id, generation, test_revision, constraint_revision,
			source_sha256, source_kind, verdict, unsure_cause, deduction_hundredths, required_review, revision)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 1)
		RETURNING id::text`,
		sub.ID, sub.LabID, finding.ConstraintID, sub.Generation, sub.TestRevision, sub.ConstraintRevision,
		sub.SourceSHA256, finding.SourceKind, string(finding.Verdict), cause, finding.DeductionHundredths, finding.RequiredReview,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return checking.ErrAlreadyChecked
		}
		return err
	}
	for i, evidence := range finding.Evidence {
		if err := tx.Exec(ctx, `
			INSERT INTO finding_evidence (finding_id, position, line, evidence_text)
			VALUES ($1::uuid, $2, $3, $4)`, id, i+1, evidence.Line, evidence.Text); err != nil {
			return err
		}
	}
	return nil
}
