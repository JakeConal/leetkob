package judge

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/domain"
)

// Queue is the durable judge-job lease. Commit and failure both carry the
// attempt fence the lease returned.
type Queue interface {
	LeaseJudgeJobs(ctx context.Context, workerID string, max int, now, expires time.Time) ([]domain.Job, error)
	CommitVerdict(ctx context.Context, job domain.Job, result Result) error
	FailJudge(ctx context.Context, job domain.Job, reason string) error
}

// Executor produces one verdict set or an infrastructure error.
type Executor interface {
	Execute(ctx context.Context, job domain.Job) (Result, error)
}

// Worker leases judge jobs and calls Executor. It does not judge inside
// the API process.
type Worker struct {
	Queue    Queue
	Exec     Executor
	ID       string
	LeaseFor time.Duration
	Now      func() time.Time
	Sleep    time.Duration
	Log      *slog.Logger
}

// Run leases until ctx is cancelled. An empty queue waits and tries again.
func (w *Worker) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := w.Process(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			w.log().Error("judge batch failed", "error", err.Error())
		}
		if !sleep(ctx, w.sleep()) {
			return nil
		}
	}
}

// Process leases one batch, judges each job, and commits or records an
// infrastructure failure. It returns how many jobs were leased.
func (w *Worker) Process(ctx context.Context) (int, error) {
	if w == nil || w.Queue == nil || w.Exec == nil || w.ID == "" {
		return 0, errors.New("judge: worker is not configured")
	}
	now := w.now()
	jobs, err := w.Queue.LeaseJudgeJobs(ctx, w.ID, 1, now, now.Add(w.leaseFor()))
	if err != nil {
		return 0, err
	}
	for _, job := range jobs {
		if err := w.finish(ctx, job); err != nil {
			return len(jobs), err
		}
	}
	return len(jobs), nil
}

func (w *Worker) finish(ctx context.Context, job domain.Job) error {
	result, err := w.Exec.Execute(ctx, job)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		reason := "infrastructure failure"
		switch {
		case errors.Is(err, ErrCompileInfra):
			reason = ErrCompileInfra.Error()
		case errors.Is(err, ErrExecuteInfra):
			reason = ErrExecuteInfra.Error()
		case errors.Is(err, ErrNoTests):
			reason = ErrNoTests.Error()
		}
		if failErr := w.Queue.FailJudge(ctx, job, reason); failErr != nil && !errors.Is(failErr, ErrStaleAttempt) {
			return failErr
		}
		w.log().Info("infrastructure failure kept off the student verdict", "jobId", job.JobID, "attempt", job.Attempt)
		return nil
	}
	if err := w.Queue.CommitVerdict(ctx, job, result); err != nil {
		if errors.Is(err, ErrStaleAttempt) {
			w.log().Info("stale attempt did not commit", "jobId", job.JobID, "attempt", job.Attempt)
			return nil
		}
		return err
	}
	w.log().Info("verdict stored", "jobId", job.JobID, "attempt", job.Attempt, "verdict", result.Verdict)
	return nil
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now().UTC()
}

func (w *Worker) leaseFor() time.Duration {
	if w.LeaseFor > 0 {
		return w.LeaseFor
	}
	return 15 * time.Minute
}

func (w *Worker) sleep() time.Duration {
	if w.Sleep > 0 {
		return w.Sleep
	}
	return 200 * time.Millisecond
}

func (w *Worker) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
