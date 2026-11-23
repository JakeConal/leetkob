package checking

import (
	"context"
	"errors"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/domain"
)

// JobQueue leases check jobs created after an Accepted verdict.
type JobQueue interface {
	LeaseCheckJobs(ctx context.Context, workerID string, max int, now, expires time.Time) ([]domain.Job, error)
	CompleteCheck(ctx context.Context, job domain.Job) error
	FailCheck(ctx context.Context, job domain.Job, reason string) error
}

// Runner drains check jobs. It does not compile or execute student code.
type Runner struct {
	Queue    JobQueue
	Service  *Service
	ID       string
	LeaseFor time.Duration
	Now      func() time.Time
}

// Process leases one batch and stores findings for each current Accepted
// submission. A stale or already-checked submission still closes its job.
func (r *Runner) Process(ctx context.Context) (int, error) {
	if r == nil || r.Queue == nil || r.Service == nil || r.ID == "" {
		return 0, errors.New("checking: runner is not configured")
	}
	now := r.now()
	lease := r.LeaseFor
	if lease <= 0 {
		lease = 15 * time.Minute
	}
	jobs, err := r.Queue.LeaseCheckJobs(ctx, r.ID, 1, now, now.Add(lease))
	if err != nil {
		return 0, err
	}
	for _, job := range jobs {
		if err := r.finish(ctx, job); err != nil {
			return len(jobs), err
		}
	}
	return len(jobs), nil
}

func (r *Runner) finish(ctx context.Context, job domain.Job) error {
	_, err := r.Service.Run(ctx, job.SubmissionID)
	if err != nil && !errors.Is(err, ErrStale) && !errors.Is(err, ErrAlreadyChecked) {
		if failErr := r.Queue.FailCheck(ctx, job, err.Error()); failErr != nil && !errors.Is(failErr, ErrStale) {
			return failErr
		}
		return nil
	}
	if err := r.Queue.CompleteCheck(ctx, job); err != nil && !errors.Is(err, ErrStale) {
		return err
	}
	return nil
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}
