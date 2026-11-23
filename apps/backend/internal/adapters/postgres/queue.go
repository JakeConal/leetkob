package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/domain"
)

// Worker lifecycle errors. They are distinct from verdicts: an exhausted
// worker failure never becomes a student verdict.
var (
	// ErrNoJobs reports an empty lease.
	ErrNoJobs = errors.New("postgres: no jobs available")
	// ErrStaleFence reports a commit from an expired or superseded lease.
	ErrStaleFence = errors.New("postgres: stale job fence")
	// ErrJobExhausted reports a job that reached its infrastructure retry
	// ceiling and is visible as failed.
	ErrJobExhausted = errors.New("postgres: job failed after infrastructure retries")
)

// LeasedJob is a job envelope under an active lease.
type LeasedJob struct {
	domain.Job
	LeaseExpiresAt time.Time
}

// JudgeOutcome is the stored verdict set committed by a worker.
type JudgeOutcome struct {
	SubmissionID string
	Verdict      string
	Tests        []TestOutcome
}

// TestOutcome is one per-test verdict and measured time.
type TestOutcome struct {
	TestID  string
	Verdict string
	TimeMs  int
}

// LeaseJobs claims up to max pending jobs for one worker, incrementing the
// attempt fence and binding a lease deadline. Already-leased work is not
// handed out again until its lease expires.
func (s *Store) LeaseJobs(ctx context.Context, worker string, max int, now time.Time, leaseFor time.Duration) ([]LeasedJob, error) {
	if max < 1 {
		return nil, nil
	}
	leased := make([]LeasedJob, 0, max)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, kind, submission_id, lab_id, generation, test_revision,
				constraint_revision, source_sha256
			FROM jobs
			WHERE (state = 'pending' AND available_at <= $1)
			   OR (state = 'leased' AND lease_expires_at <= $1)
			ORDER BY available_at, created_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED`, now, max)
		if err != nil {
			return err
		}
		type candidate struct {
			id, kind, submissionID, labID                string
			generation, testRevision, constraintRevision int
			sourceSha256                                 []byte
		}
		var candidates []candidate
		for rows.Next() {
			var item candidate
			if err := rows.Scan(&item.id, &item.kind, &item.submissionID, &item.labID,
				&item.generation, &item.testRevision, &item.constraintRevision, &item.sourceSha256); err != nil {
				rows.Close()
				return err
			}
			candidates = append(candidates, item)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, item := range candidates {
			expires := now.Add(leaseFor)
			var attempt int
			if err := tx.QueryRow(ctx, `
				UPDATE jobs
				SET state = 'leased', attempt = attempt + 1, lease_expires_at = $2
				WHERE id = $1
				RETURNING attempt`, item.id, expires).Scan(&attempt); err != nil {
				return err
			}
			leased = append(leased, LeasedJob{
				Job: domain.Job{
					Version: 1, JobID: item.id, Attempt: attempt,
					Kind: domain.JobKind(item.kind), SubmissionID: item.submissionID,
					LabID: item.labID, Generation: item.generation,
					TestRevision: item.testRevision, ConstraintRevision: item.constraintRevision,
					SourceSHA256: fmt.Sprintf("%x", item.sourceSha256),
				},
				LeaseExpiresAt: expires,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: lease jobs: %w", err)
	}
	return leased, nil
}

// CommitJudgeResult stores one verdict set and finishes the job, but only
// for the current attempt fence. A superseded or expired attempt cannot
// commit; the first successful commit is the only stored result.
func (s *Store) CommitJudgeResult(ctx context.Context, jobID string, attempt int, outcome JudgeOutcome, now time.Time) error {
	return s.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var submissionID string
		if err := tx.QueryRow(ctx, `
			UPDATE jobs
			SET state = 'done'
			WHERE id = $1 AND attempt = $2 AND state = 'leased' AND lease_expires_at > $3
			RETURNING submission_id`, jobID, attempt, now,
		).Scan(&submissionID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrStaleFence
			}
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE submissions
			SET state = 'judged', verdict = $2, judged_at = $3
			WHERE id = $1 AND state <> 'judged'`, submissionID, outcome.Verdict, now); err != nil {
			return err
		}
		for _, test := range outcome.Tests {
			if _, err := tx.Exec(ctx, `
				INSERT INTO submission_test_results (submission_id, test_id, verdict, time_ms)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (submission_id, test_id) DO NOTHING`,
				submissionID, test.TestID, test.Verdict, test.TimeMs,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// FailJob records one bounded infrastructure failure. Once the attempt
// ceiling is reached the job becomes visibly failed instead of returning
// to the pool, so infrastructure trouble never masquerades as a verdict.
// The failure state commits before ErrJobExhausted is reported.
func (s *Store) FailJob(ctx context.Context, jobID string, attempt int, now time.Time, maxFailures int) error {
	exhausted := false
	err := s.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `
			UPDATE jobs
			SET failure_count = failure_count + 1,
				state = CASE WHEN failure_count + 1 >= $4 THEN 'failed' ELSE 'pending' END,
				lease_expires_at = NULL,
				available_at = $3
			WHERE id = $1 AND attempt = $2 AND state = 'leased'
			RETURNING state`, jobID, attempt, now, maxFailures,
		).Scan(&state); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrStaleFence
			}
			return err
		}
		exhausted = state == "failed"
		return nil
	})
	if err != nil {
		return err
	}
	if exhausted {
		return ErrJobExhausted
	}
	return nil
}
