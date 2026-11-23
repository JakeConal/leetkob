// Package worker drains the durable judge queue in a separate process. It
// leases jobs with an increasing attempt fence, executes them only through
// the isolated sandbox, stores exactly one verdict set per submission/input
// revision, and treats infrastructure trouble as an operational retry or a
// visibly failed job — never as a student verdict.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/domain"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

// Defaults bound one worker process. The lease is deliberately longer than
// a normal compile-and-test run so healthy work is not duplicated; an
// expired lease still permits recovery by another worker, and the fence
// keeps only one committed result.
const (
	DefaultBatch           = 1
	DefaultLeaseFor        = 30 * time.Minute
	DefaultMaxFailures     = 3
	DefaultPollInterval    = 2 * time.Second
	DefaultShutdownTimeout = 30 * time.Second
)

// Queue is the durable lease boundary implemented by the PostgreSQL store.
type Queue interface {
	LeaseJobs(ctx context.Context, worker string, max int, now time.Time, leaseFor time.Duration) ([]postgres.LeasedJob, error)
	CommitJudgeResult(ctx context.Context, jobID string, attempt int, outcome postgres.JudgeOutcome, now time.Time) error
	FailJob(ctx context.Context, jobID string, attempt int, now time.Time, maxFailures int) error
	ReleaseJob(ctx context.Context, jobID string, attempt int, now time.Time) error
}

// Inputs reads the trusted judge input for one submission.
type Inputs interface {
	GetJudgeInput(ctx context.Context, submissionID string) (postgres.JudgeInput, error)
}

// Judge is the isolated execution boundary. The real implementation is
// internal/sandbox.Controller; tests inject a fake without Docker.
type Judge interface {
	Judge(ctx context.Context, req sandbox.JudgeRequest) (sandbox.JudgeResult, error)
}

// Clock supplies server time for lease stamps and result commits.
type Clock interface {
	Now() time.Time
}

// Options configures one worker. Zero values receive the documented
// defaults; Queue, Inputs and Judge are required.
type Options struct {
	Queue           Queue
	Inputs          Inputs
	Judge           Judge
	Clock           Clock
	Logger          *slog.Logger
	WorkerID        string
	Batch           int
	LeaseFor        time.Duration
	MaxFailures     int
	PollInterval    time.Duration
	ShutdownTimeout time.Duration
}

// Worker leases and judges durable jobs one batch at a time.
type Worker struct {
	queue           Queue
	inputs          Inputs
	judge           Judge
	clock           Clock
	logger          *slog.Logger
	workerID        string
	batch           int
	leaseFor        time.Duration
	maxFailures     int
	pollInterval    time.Duration
	shutdownTimeout time.Duration
}

// New validates the injected boundaries and returns a worker.
func New(options Options) (*Worker, error) {
	if options.Queue == nil {
		return nil, errors.New("worker: queue is required")
	}
	if options.Inputs == nil {
		return nil, errors.New("worker: inputs are required")
	}
	if options.Judge == nil {
		return nil, errors.New("worker: judge is required")
	}
	if options.Clock == nil {
		options.Clock = domain.SystemClock{}
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.WorkerID == "" {
		options.WorkerID = "worker"
	}
	if options.Batch < 1 {
		options.Batch = DefaultBatch
	}
	if options.LeaseFor <= 0 {
		options.LeaseFor = DefaultLeaseFor
	}
	if options.MaxFailures < 1 {
		options.MaxFailures = DefaultMaxFailures
	}
	if options.PollInterval <= 0 {
		options.PollInterval = DefaultPollInterval
	}
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = DefaultShutdownTimeout
	}
	return &Worker{
		queue:           options.Queue,
		inputs:          options.Inputs,
		judge:           options.Judge,
		clock:           options.Clock,
		logger:          options.Logger.With("workerId", options.WorkerID),
		workerID:        options.WorkerID,
		batch:           options.Batch,
		leaseFor:        options.LeaseFor,
		maxFailures:     options.MaxFailures,
		pollInterval:    options.PollInterval,
		shutdownTimeout: options.ShutdownTimeout,
	}, nil
}

// Run leases and judges jobs until the context is cancelled. On shutdown it
// stops leasing; an in-flight job is finished or its lease is released back
// to the durable pool without counting an infrastructure failure.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("worker started", "batch", w.batch, "lease", w.leaseFor.String())
	for {
		if ctx.Err() != nil {
			w.logger.Info("worker stopped leasing")
			return nil
		}
		jobs, err := w.queue.LeaseJobs(ctx, w.workerID, w.batch, w.clock.Now(), w.leaseFor)
		if err != nil {
			if ctx.Err() != nil {
				w.logger.Info("worker stopped leasing")
				return nil
			}
			w.logger.Error("lease jobs failed", "error", err)
			if !w.wait(ctx) {
				w.logger.Info("worker stopped leasing")
				return nil
			}
			continue
		}
		if len(jobs) == 0 {
			if !w.wait(ctx) {
				w.logger.Info("worker stopped leasing")
				return nil
			}
			continue
		}
		for _, job := range jobs {
			if !w.process(ctx, job) {
				w.logger.Info("worker finished shutdown after the current job")
				return nil
			}
		}
	}
}

// process handles one leased job. It reports false only when shutdown
// interrupted the job and the loop must exit.
func (w *Worker) process(ctx context.Context, job postgres.LeasedJob) bool {
	log := w.logger.With("jobId", job.JobID, "submissionId", job.SubmissionID,
		"labId", job.LabID, "attempt", job.Attempt)

	if ctx.Err() != nil {
		w.release(job, "shutdown-before-start")
		return false
	}
	if job.Kind != domain.KindJudge {
		// This worker process only judges. Releasing rather than failing
		// lets the appropriate worker pick the job up after the lease.
		log.Warn("unsupported job kind released", "kind", job.Kind)
		w.release(job, "unsupported-kind")
		return true
	}

	input, err := w.inputs.GetJudgeInput(ctx, job.SubmissionID)
	if err != nil {
		if ctx.Err() != nil {
			w.release(job, "shutdown-while-reading-input")
			return false
		}
		log.Error("read judge input failed", "error", err)
		w.fail(job, "read-input", err)
		return true
	}
	if err := validateInput(job.Job, input); err != nil {
		log.Error("judge input does not match the leased job", "error", err)
		w.fail(job, "validate-input", err)
		return true
	}

	log.Info("judging submission", "tests", len(input.Tests),
		"cpuSeconds", input.CPUSeconds, "memoryMiB", input.MemoryMiB)
	result, err := w.judge.Judge(ctx, sandbox.JudgeRequest{
		Source:     []byte(input.Source),
		CPUSeconds: input.CPUSeconds,
		Tests:      sandboxTests(input.Tests),
	})
	if err != nil {
		if ctx.Err() != nil {
			w.release(job, "shutdown-while-judging")
			return false
		}
		log.Error("sandbox judging failed", "error", err)
		w.fail(job, "judge", err)
		return true
	}

	outcome, err := evaluate(result, input.Tests)
	if err != nil {
		log.Error("judge result is incomplete", "error", err)
		w.fail(job, "evaluate", err)
		return true
	}
	// A signal may arrive after judging returned. Commit with a fresh bounded
	// context so a completed result is stored rather than thrown away; the
	// fence still rejects a lease that another worker already replaced.
	commitCtx, cancelCommit := context.WithTimeout(context.WithoutCancel(ctx), w.shutdownTimeout)
	err = w.queue.CommitJudgeResult(commitCtx, job.JobID, job.Attempt, outcome, w.clock.Now())
	cancelCommit()
	if err != nil {
		if errors.Is(err, postgres.ErrStaleFence) {
			// A late attempt after lease expiry must never replace or
			// duplicate the result another attempt committed.
			log.Warn("late judge attempt refused by the job fence")
			return true
		}
		if ctx.Err() != nil {
			w.release(job, "shutdown-while-committing")
			return false
		}
		// The result could not be stored. The lease will expire and a later
		// attempt will re-judge; uniqueness keeps one stored verdict set.
		log.Error("commit judge result failed", "error", err)
		return true
	}
	log.Info("judge result stored", "verdict", outcome.Verdict,
		"tests", len(outcome.Tests), "durationMs", result.Duration.Milliseconds())
	return true
}

// fail records one bounded infrastructure failure. The store makes the job
// visibly failed after the retry ceiling; it never becomes a student
// verdict.
func (w *Worker) fail(job postgres.LeasedJob, op string, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), w.shutdownTimeout)
	defer cancel()
	err := w.queue.FailJob(ctx, job.JobID, job.Attempt, w.clock.Now(), w.maxFailures)
	log := w.logger.With("jobId", job.JobID, "submissionId", job.SubmissionID,
		"attempt", job.Attempt, "op", op, "cause", cause.Error())
	switch {
	case err == nil:
		log.Warn("infrastructure failure recorded; job will retry")
	case errors.Is(err, postgres.ErrJobExhausted):
		log.Error("job exhausted its infrastructure retries and is visibly failed")
	case errors.Is(err, postgres.ErrStaleFence):
		log.Warn("failure not recorded because the lease was superseded")
	default:
		log.Error("record infrastructure failure failed", "error", err)
	}
}

// release returns a lease to the pool on graceful shutdown. The attempt
// fence is not reset, so a later result from this attempt remains stale.
func (w *Worker) release(job postgres.LeasedJob, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), w.shutdownTimeout)
	defer cancel()
	err := w.queue.ReleaseJob(ctx, job.JobID, job.Attempt, w.clock.Now())
	log := w.logger.With("jobId", job.JobID, "submissionId", job.SubmissionID,
		"attempt", job.Attempt, "reason", reason)
	switch {
	case err == nil:
		log.Info("leased job released for recovery")
	case errors.Is(err, postgres.ErrStaleFence):
		log.Warn("lease already superseded; nothing to release")
	default:
		log.Error("release leased job failed; it will recover when the lease expires", "error", err)
	}
}

func (w *Worker) wait(ctx context.Context) bool {
	timer := time.NewTimer(w.pollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func sandboxTests(tests []postgres.JudgeTest) []sandbox.TestCase {
	cases := make([]sandbox.TestCase, 0, len(tests))
	for _, test := range tests {
		cases = append(cases, sandbox.TestCase{ID: test.ID, Stdin: []byte(test.Input)})
	}
	return cases
}

// validateInput refuses to execute a submission whose stored source or lab
// revision tuple does not match the leased job envelope. A mismatch is
// corruption, never a verdict.
func validateInput(job domain.Job, input postgres.JudgeInput) error {
	if input.SubmissionID != job.SubmissionID {
		return fmt.Errorf("submission %s != job submission %s", input.SubmissionID, job.SubmissionID)
	}
	if input.LabID != job.LabID {
		return fmt.Errorf("lab %s != job lab %s", input.LabID, job.LabID)
	}
	if input.Generation != job.Generation {
		return fmt.Errorf("generation %d != job generation %d", input.Generation, job.Generation)
	}
	if input.SourceSHA256 != job.SourceSHA256 {
		return errors.New("stored source hash does not match the job envelope")
	}
	return nil
}
