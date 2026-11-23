package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JakeConal/leetkob/backend/internal/access"
	"github.com/JakeConal/leetkob/backend/internal/labs"
)

// CurrentMember reports a membership that has not been removed.
func (s *Store) CurrentMember(ctx context.Context, userID, classID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM memberships
			WHERE user_id = $1::uuid AND class_id = $2::uuid AND removed_at IS NULL
		)`, userID, classID).Scan(&ok)
	return ok, err
}

// FindReplay loads one completed creation record.
func (s *Store) FindReplay(ctx context.Context, actorID, operation, resource, key string) (labs.Replay, bool, error) {
	var replay labs.Replay
	err := s.pool.QueryRow(ctx, `
		SELECT actor_user_id::text, operation, resource, idempotency_key,
		       request_fingerprint, status_code, response_body
		FROM replays
		WHERE actor_user_id = $1::uuid AND operation = $2 AND resource = $3
		  AND idempotency_key = $4`,
		actorID, operation, resource, key).Scan(
		&replay.ActorID, &replay.Operation, &replay.Resource, &replay.Key,
		&replay.Fingerprint, &replay.Status, &replay.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return labs.Replay{}, false, nil
	}
	if err != nil {
		return labs.Replay{}, false, err
	}
	return replay, true, nil
}

// CreateLab writes the lab, its revision, tests, constraints and the
// replay body in one transaction. A unique replay key rolls the lab
// back and returns labs.ErrDuplicateReplay.
func (s *Store) CreateLab(ctx context.Context, lab labs.StoredLab, replay labs.Replay) error {
	return s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		if err := tx.Exec(ctx, `
			INSERT INTO labs (
				id, class_id, created_by, title, statement, opens_at, deadline,
				cpu_seconds, memory_mib, compile_seconds, wall_multiplier,
				max_processes, max_output_bytes, max_source_bytes, budget_cap_micros,
				current_revision, test_revision, constraint_revision, generation
			) VALUES (
				$1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7,
				$8, $9, $10, $11,
				$12, $13, $14, $15,
				$16, $17, $18, $19
			)`,
			lab.ID, lab.ClassID, lab.CreatedBy, lab.Title, lab.Statement, lab.OpensAt.UTC(), lab.Deadline.UTC(),
			lab.CPUSeconds, lab.MemoryMiB, lab.CompileSeconds, lab.WallMultiplier,
			lab.MaxProcesses, lab.MaxOutputBytes, lab.MaxSourceBytes, lab.BudgetCapMicros,
			lab.Revision, lab.TestRevision, lab.ConstraintRevision, lab.Generation); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `
			INSERT INTO lab_revisions (
				lab_id, revision, title, statement, opens_at, deadline,
				cpu_seconds, memory_mib, compile_seconds, wall_multiplier,
				max_processes, max_output_bytes, max_source_bytes,
				test_revision, constraint_revision, generation
			) VALUES (
				$1::uuid, $2, $3, $4, $5, $6,
				$7, $8, $9, $10,
				$11, $12, $13,
				$14, $15, $16
			)`,
			lab.ID, lab.Revision, lab.Title, lab.Statement, lab.OpensAt.UTC(), lab.Deadline.UTC(),
			lab.CPUSeconds, lab.MemoryMiB, lab.CompileSeconds, lab.WallMultiplier,
			lab.MaxProcesses, lab.MaxOutputBytes, lab.MaxSourceBytes,
			lab.TestRevision, lab.ConstraintRevision, lab.Generation); err != nil {
			return err
		}
		for position, test := range lab.Tests {
			if err := tx.Exec(ctx, `
				INSERT INTO lab_tests (lab_id, revision, test_id, position, input, expected_output, sample)
				VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)`,
				lab.ID, lab.TestRevision, test.ID, position+1, test.Input, test.ExpectedOutput, test.Sample); err != nil {
				return err
			}
		}
		for position, constraint := range lab.Constraints {
			if err := tx.Exec(ctx, `
				INSERT INTO lab_constraints (lab_id, revision, constraint_id, position, body, deduction_hundredths)
				VALUES ($1::uuid, $2, $3, $4, $5, $6)`,
				lab.ID, lab.ConstraintRevision, constraint.ID, position+1, constraint.Text, int(constraint.Deduction)); err != nil {
				return err
			}
		}
		if err := tx.Exec(ctx, `
			INSERT INTO replays (
				actor_user_id, operation, resource, idempotency_key,
				request_fingerprint, status_code, response_body
			) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::jsonb)`,
			replay.ActorID, replay.Operation, replay.Resource, replay.Key,
			replay.Fingerprint, replay.Status, string(replay.Body)); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return labs.ErrDuplicateReplay
			}
			return err
		}
		return nil
	})
}

// ListLabs returns labs of a class the user currently belongs to. A TA
// sees labs that have not opened. Anyone else sees a lab only once it
// has opened. Missing membership and a missing class are both not found.
func (s *Store) ListLabs(ctx context.Context, userID, role, classID string, now time.Time, limit int, cursor string) ([]labs.Summary, string, error) {
	member, err := s.CurrentMember(ctx, userID, classID)
	if err != nil {
		return nil, "", err
	}
	if !member {
		return nil, "", access.ErrNotFound
	}
	hasCursor := cursor != ""
	cursorTime := time.Unix(0, 0).UTC()
	cursorID := "00000000-0000-4000-8000-000000000000"
	if hasCursor {
		cursorTime, cursorID, err = labs.DecodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, class_id::text, title, opens_at, deadline, current_revision, created_at
		FROM labs
		WHERE class_id = $1::uuid
		  AND ($2::bool OR opens_at <= $3)
		  AND (
		    $4::bool = false
		    OR created_at > $5
		    OR (created_at = $5 AND id > $6::uuid)
		  )
		ORDER BY created_at, id
		LIMIT $7`,
		classID, role == access.RoleTA, now.UTC(), hasCursor, cursorTime, cursorID, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []labs.Summary{}
	for rows.Next() {
		var item labs.Summary
		if err := rows.Scan(&item.ID, &item.ClassID, &item.Title, &item.OpensAt, &item.Deadline, &item.Revision, &item.CreatedAt); err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		last := items[limit-1]
		next = labs.EncodeCursor(last.CreatedAt, last.ID)
		items = items[:limit]
	}
	return items, next, nil
}

// ReadLab loads a lab the user can currently see, with sample tests only.
// Hidden test rows are not selected. A student does not see a lab before
// it opens; that case is indistinguishable from a missing lab.
func (s *Store) ReadLab(ctx context.Context, userID, role, labID string, now time.Time) (labs.VisibleLab, bool, error) {
	var lab labs.VisibleLab
	var constraintRevision, testRevision int
	err := s.pool.QueryRow(ctx, `
		SELECT l.id::text, l.class_id::text, l.title, l.statement,
		       l.opens_at, l.deadline, l.current_revision,
		       l.cpu_seconds, l.memory_mib, l.constraint_revision, l.test_revision
		FROM labs l
		JOIN memberships m
		  ON m.class_id = l.class_id
		 AND m.user_id = $2::uuid
		 AND m.removed_at IS NULL
		WHERE l.id = $1::uuid
		  AND ($3::bool OR l.opens_at <= $4)`,
		labID, userID, role == access.RoleTA, now.UTC()).Scan(
		&lab.ID, &lab.ClassID, &lab.Title, &lab.Statement,
		&lab.OpensAt, &lab.Deadline, &lab.Revision,
		&lab.CPUSeconds, &lab.MemoryMiB, &constraintRevision, &testRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return labs.VisibleLab{}, false, nil
	}
	if err != nil {
		return labs.VisibleLab{}, false, err
	}
	constraints, err := s.constraints(ctx, lab.ID, constraintRevision)
	if err != nil {
		return labs.VisibleLab{}, false, err
	}
	samples, err := s.sampleTests(ctx, lab.ID, testRevision)
	if err != nil {
		return labs.VisibleLab{}, false, err
	}
	lab.Constraints = constraints
	lab.SampleTests = samples
	return lab, true, nil
}

func (s *Store) constraints(ctx context.Context, labID string, revision int) ([]labs.Constraint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT constraint_id, body, deduction_hundredths
		FROM lab_constraints
		WHERE lab_id = $1::uuid AND revision = $2
		ORDER BY position`, labID, revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []labs.Constraint{}
	for rows.Next() {
		var item labs.Constraint
		var deduction int
		if err := rows.Scan(&item.ID, &item.Text, &deduction); err != nil {
			return nil, err
		}
		item.Deduction = labs.Hundredths(deduction)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) sampleTests(ctx context.Context, labID string, revision int) ([]labs.Test, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT test_id, input, expected_output
		FROM lab_tests
		WHERE lab_id = $1::uuid AND revision = $2 AND sample = true
		ORDER BY position`, labID, revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []labs.Test{}
	for rows.Next() {
		var item labs.Test
		if err := rows.Scan(&item.ID, &item.Input, &item.ExpectedOutput); err != nil {
			return nil, err
		}
		item.Sample = true
		items = append(items, item)
	}
	return items, rows.Err()
}
