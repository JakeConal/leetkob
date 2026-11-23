package judge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/domain"
)

type memQueue struct {
	jobs     []domain.Job
	leased   int
	commits  []Result
	failures []string
	stale    bool
}

func (q *memQueue) LeaseJudgeJobs(context.Context, string, int, time.Time, time.Time) ([]domain.Job, error) {
	q.leased++
	if len(q.jobs) == 0 {
		return nil, nil
	}
	job := q.jobs[0]
	q.jobs = q.jobs[1:]
	return []domain.Job{job}, nil
}

func (q *memQueue) CommitVerdict(_ context.Context, _ domain.Job, result Result) error {
	if q.stale {
		return ErrStaleAttempt
	}
	q.commits = append(q.commits, result)
	return nil
}

func (q *memQueue) FailJudge(_ context.Context, _ domain.Job, reason string) error {
	q.failures = append(q.failures, reason)
	return nil
}

type scriptExec struct {
	result Result
	err    error
	calls  int
}

func (s *scriptExec) Execute(context.Context, domain.Job) (Result, error) {
	s.calls++
	return s.result, s.err
}

func TestWorkerCallsJudgeAndCommits(t *testing.T) {
	exec := &scriptExec{result: Result{Verdict: VerdictAC, Tests: []TestResult{{ID: "t", Verdict: VerdictAC}}}}
	queue := &memQueue{jobs: []domain.Job{{JobID: "j", Attempt: 1, Kind: domain.KindJudge}}}
	worker := &Worker{Queue: queue, Exec: exec, ID: "w", LeaseFor: time.Second, Now: func() time.Time {
		return time.Date(2026, 11, 24, 1, 0, 0, 0, time.UTC)
	}}
	n, err := worker.Process(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("process = %d, %v", n, err)
	}
	if exec.calls != 1 || len(queue.commits) != 1 || queue.commits[0].Verdict != VerdictAC {
		t.Fatalf("calls=%d commits=%+v", exec.calls, queue.commits)
	}
	n, err = worker.Process(context.Background())
	if err != nil || n != 0 || exec.calls != 1 {
		t.Fatalf("second process = %d calls=%d err=%v", n, exec.calls, err)
	}
}

func TestWorkerDoesNotTurnInfrastructureIntoAVerdict(t *testing.T) {
	exec := &scriptExec{err: ErrExecuteInfra}
	queue := &memQueue{jobs: []domain.Job{{JobID: "j", Attempt: 1}}}
	worker := &Worker{Queue: queue, Exec: exec, ID: "w", LeaseFor: time.Second}
	if _, err := worker.Process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(queue.commits) != 0 || len(queue.failures) != 1 || queue.failures[0] != ErrExecuteInfra.Error() {
		t.Fatalf("commits=%d failures=%v", len(queue.commits), queue.failures)
	}
}

func TestWorkerDropsStaleAttempt(t *testing.T) {
	exec := &scriptExec{result: Result{Verdict: VerdictWA, Tests: []TestResult{{ID: "t", Verdict: VerdictWA}}}}
	queue := &memQueue{jobs: []domain.Job{{JobID: "j", Attempt: 1}}, stale: true}
	worker := &Worker{Queue: queue, Exec: exec, ID: "w", LeaseFor: time.Second}
	if _, err := worker.Process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(queue.commits) != 0 {
		t.Fatal("stale attempt committed")
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker := &Worker{
		Queue:    &memQueue{},
		Exec:     &scriptExec{},
		ID:       "w",
		LeaseFor: time.Second,
		Sleep:    time.Hour,
	}
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFinishDoesNotCommitAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	queue := &memQueue{}
	worker := &Worker{Queue: queue, Exec: &scriptExec{err: errors.New("should not be stored")}, ID: "w"}
	err := worker.finish(ctx, domain.Job{JobID: "j"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if len(queue.commits) != 0 || len(queue.failures) != 0 {
		t.Fatal("cancelled attempt was stored")
	}
}
