package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// JudgeTest is one trusted lab test input the worker must run.
type JudgeTest struct {
	ID             string
	Input          string
	ExpectedOutput string
}

// JudgeInput is the complete trusted input for one judge job: the stored
// source, its integrity hash and the lab's limits and tests. Only Source is
// untrusted; Tests and limits are TA-authored task data.
type JudgeInput struct {
	SubmissionID string
	LabID        string
	Source       string
	// SourceSHA256 is the lowercase hex digest stored at acceptance.
	SourceSHA256 string
	Generation   int
	CPUSeconds   int
	MemoryMiB    int
	Tests        []JudgeTest
}

// GetJudgeInput reads the submission source and its lab's immutable test
// inputs. The worker compares LabID, Generation and SourceSHA256 against the
// leased job envelope before executing anything, so a mismatched or corrupt
// row can never be judged as the job's work.
func (s *Store) GetJudgeInput(ctx context.Context, submissionID string) (JudgeInput, error) {
	var input JudgeInput
	err := s.pool.QueryRow(ctx, `
		SELECT s.lab_id, s.source, encode(s.source_sha256, 'hex'), s.generation,
			l.cpu_seconds, l.memory_mib
		FROM submissions s
		JOIN labs l ON l.id = s.lab_id
		WHERE s.id = $1`, submissionID,
	).Scan(&input.LabID, &input.Source, &input.SourceSHA256, &input.Generation,
		&input.CPUSeconds, &input.MemoryMiB)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JudgeInput{}, ErrSubmissionNotFound
		}
		return JudgeInput{}, fmt.Errorf("postgres: read judge input: %w", err)
	}
	input.SubmissionID = submissionID

	rows, err := s.pool.Query(ctx, `
		SELECT test_id, input, expected_output
		FROM lab_tests
		WHERE lab_id = $1
		ORDER BY position`, input.LabID)
	if err != nil {
		return JudgeInput{}, fmt.Errorf("postgres: read judge tests: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var test JudgeTest
		if err := rows.Scan(&test.ID, &test.Input, &test.ExpectedOutput); err != nil {
			return JudgeInput{}, fmt.Errorf("postgres: scan judge test: %w", err)
		}
		input.Tests = append(input.Tests, test)
	}
	if err := rows.Err(); err != nil {
		return JudgeInput{}, fmt.Errorf("postgres: iterate judge tests: %w", err)
	}
	if len(input.Tests) == 0 {
		return JudgeInput{}, fmt.Errorf("postgres: lab %s has no judge tests", input.LabID)
	}
	return input, nil
}

// ReleaseJob returns a lease to the pending pool without counting an
// infrastructure failure. It is used on graceful shutdown: the attempt fence
// stays as it was, so a result that somehow arrives later still cannot
// commit after another worker claims the next attempt.
func (s *Store) ReleaseJob(ctx context.Context, jobID string, attempt int, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'pending', lease_expires_at = NULL, available_at = $3
		WHERE id = $1 AND attempt = $2 AND state = 'leased'`,
		jobID, attempt, now)
	if err != nil {
		return fmt.Errorf("postgres: release job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrStaleFence
	}
	return nil
}
