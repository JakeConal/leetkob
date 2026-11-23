package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrLabNotFound is returned when a lab row does not exist.
var ErrLabNotFound = errors.New("postgres: lab not found")

// ConstraintRecord is one TA-configured constraint of a lab revision.
type ConstraintRecord struct {
	ID             string
	Text           string
	DeductionCents int
}

// TestRecord is one lab test case; sample tests are student-visible.
type TestRecord struct {
	ID             string
	Input          string
	ExpectedOutput string
	Sample         bool
}

// LabRecord is the storage shape of an immutable lab input revision.
type LabRecord struct {
	ID          string
	ClassID     string
	Title       string
	Statement   string
	OpensAt     time.Time
	Deadline    time.Time
	Revision    int
	CPUSeconds  int
	MemoryMiB   int
	ModelNotice string
	CreatedBy   string
	Constraints []ConstraintRecord
	Tests       []TestRecord
}

// CreateLab persists a lab with its constraints and tests atomically.
// Any failure rolls back every row, so no partial lab is ever visible.
func (s *Store) CreateLab(ctx context.Context, lab LabRecord) error {
	return s.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO labs (id, class_id, title, statement, opens_at, deadline,
				revision, cpu_seconds, memory_mib, model_notice, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			lab.ID, lab.ClassID, lab.Title, lab.Statement, lab.OpensAt, lab.Deadline,
			lab.Revision, lab.CPUSeconds, lab.MemoryMiB, lab.ModelNotice, lab.CreatedBy,
		); err != nil {
			return fmt.Errorf("postgres: insert lab: %w", err)
		}
		for _, constraint := range lab.Constraints {
			if _, err := tx.Exec(ctx, `
				INSERT INTO lab_constraints (lab_id, constraint_id, text, deduction_cents, revision)
				VALUES ($1, $2, $3, $4, $5)`,
				lab.ID, constraint.ID, constraint.Text, constraint.DeductionCents, lab.Revision,
			); err != nil {
				return fmt.Errorf("postgres: insert constraint %s: %w", constraint.ID, err)
			}
		}
		for position, test := range lab.Tests {
			if _, err := tx.Exec(ctx, `
				INSERT INTO lab_tests (lab_id, test_id, input, expected_output, sample, position)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				lab.ID, test.ID, test.Input, test.ExpectedOutput, test.Sample, position,
			); err != nil {
				return fmt.Errorf("postgres: insert test %s: %w", test.ID, err)
			}
		}
		return nil
	})
}

// GetLab reads one lab with its constraints and tests.
func (s *Store) GetLab(ctx context.Context, id string) (LabRecord, error) {
	var lab LabRecord
	if err := s.pool.QueryRow(ctx, `
		SELECT id, class_id, title, statement, opens_at, deadline, revision,
			cpu_seconds, memory_mib, model_notice, created_by
		FROM labs WHERE id = $1`, id,
	).Scan(&lab.ID, &lab.ClassID, &lab.Title, &lab.Statement, &lab.OpensAt, &lab.Deadline,
		&lab.Revision, &lab.CPUSeconds, &lab.MemoryMiB, &lab.ModelNotice, &lab.CreatedBy,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LabRecord{}, ErrLabNotFound
		}
		return LabRecord{}, fmt.Errorf("postgres: read lab: %w", err)
	}

	constraintRows, err := s.pool.Query(ctx,
		`SELECT constraint_id, text, deduction_cents FROM lab_constraints
		 WHERE lab_id = $1 ORDER BY constraint_id`, id)
	if err != nil {
		return LabRecord{}, fmt.Errorf("postgres: read constraints: %w", err)
	}
	defer constraintRows.Close()
	for constraintRows.Next() {
		var constraint ConstraintRecord
		if err := constraintRows.Scan(&constraint.ID, &constraint.Text, &constraint.DeductionCents); err != nil {
			return LabRecord{}, fmt.Errorf("postgres: scan constraint: %w", err)
		}
		lab.Constraints = append(lab.Constraints, constraint)
	}
	if err := constraintRows.Err(); err != nil {
		return LabRecord{}, fmt.Errorf("postgres: iterate constraints: %w", err)
	}

	testRows, err := s.pool.Query(ctx,
		`SELECT test_id, input, expected_output, sample FROM lab_tests
		 WHERE lab_id = $1 ORDER BY position`, id)
	if err != nil {
		return LabRecord{}, fmt.Errorf("postgres: read tests: %w", err)
	}
	defer testRows.Close()
	for testRows.Next() {
		var test TestRecord
		if err := testRows.Scan(&test.ID, &test.Input, &test.ExpectedOutput, &test.Sample); err != nil {
			return LabRecord{}, fmt.Errorf("postgres: scan test: %w", err)
		}
		lab.Tests = append(lab.Tests, test)
	}
	return lab, testRows.Err()
}
