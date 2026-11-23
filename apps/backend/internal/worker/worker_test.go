package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/domain"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type commitCall struct {
	jobID   string
	attempt int
	outcome postgres.JudgeOutcome
}

type failCall struct {
	jobID       string
	attempt     int
	maxFailures int
}

type releaseCall struct {
	jobID   string
	attempt int
}

type fakeQueue struct {
	mu        sync.Mutex
	pending   [][]postgres.LeasedJob
	leaseErr  error
	commits   []commitCall
	commitErr error
	fails     []failCall
	failErr   error
	releases  []releaseCall
}

func (f *fakeQueue) LeaseJobs(_ context.Context, _ string, _ int, _ time.Time, _ time.Duration) ([]postgres.LeasedJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.leaseErr != nil {
		return nil, f.leaseErr
	}
	if len(f.pending) == 0 {
		return nil, nil
	}
	next := f.pending[0]
	f.pending = f.pending[1:]
	return next, nil
}

func (f *fakeQueue) CommitJudgeResult(_ context.Context, jobID string, attempt int, outcome postgres.JudgeOutcome, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commits = append(f.commits, commitCall{jobID: jobID, attempt: attempt, outcome: outcome})
	return f.commitErr
}

func (f *fakeQueue) FailJob(_ context.Context, jobID string, attempt int, _ time.Time, maxFailures int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fails = append(f.fails, failCall{jobID: jobID, attempt: attempt, maxFailures: maxFailures})
	return f.failErr
}

func (f *fakeQueue) ReleaseJob(_ context.Context, jobID string, attempt int, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releases = append(f.releases, releaseCall{jobID: jobID, attempt: attempt})
	return nil
}

func (f *fakeQueue) snapshot() (commits []commitCall, fails []failCall, releases []releaseCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]commitCall(nil), f.commits...),
		append([]failCall(nil), f.fails...),
		append([]releaseCall(nil), f.releases...)
}

type fakeInputs struct {
	input postgres.JudgeInput
	err   error
}

func (f *fakeInputs) GetJudgeInput(_ context.Context, _ string) (postgres.JudgeInput, error) {
	return f.input, f.err
}

type fakeJudge struct {
	result  sandbox.JudgeResult
	err     error
	started chan struct{}
	block   bool
	// commitAfterCancel returns a successful result after cancellation,
	// simulating a signal that arrived just as judging completed.
	commitAfterCancel bool
}

func (f *fakeJudge) Judge(ctx context.Context, _ sandbox.JudgeRequest) (sandbox.JudgeResult, error) {
	if f.started != nil {
		close(f.started)
	}
	if f.block {
		<-ctx.Done()
		if f.commitAfterCancel {
			return f.result, nil
		}
		return sandbox.JudgeResult{}, ctx.Err()
	}
	return f.result, f.err
}

func testJob() postgres.LeasedJob {
	return postgres.LeasedJob{
		Job: domain.Job{
			Version: 1, JobID: "job-1", Attempt: 1, Kind: domain.KindJudge,
			SubmissionID: "submission-1", LabID: "lab-1", Generation: 1,
			TestRevision: 1, ConstraintRevision: 1, SourceSHA256: "abc123",
		},
	}
}

func testInput() postgres.JudgeInput {
	return postgres.JudgeInput{
		SubmissionID: "submission-1", LabID: "lab-1", Source: "int main(){}",
		SourceSHA256: "abc123", Generation: 1, CPUSeconds: 1, MemoryMiB: 256,
		Tests: []postgres.JudgeTest{
			{ID: "01", Input: "1 2\n", ExpectedOutput: "3\n"},
			{ID: "02", Input: "4 5\n", ExpectedOutput: "9\n"},
		},
	}
}

func goodResult() sandbox.JudgeResult {
	return sandbox.JudgeResult{
		Verdict: sandbox.VerdictRan,
		Compile: sandbox.CompileResult{Succeeded: true},
		Tests: []sandbox.TestResult{
			{ID: "01", Verdict: sandbox.VerdictRan, Tokens: []string{"3"}, Duration: 4 * time.Millisecond},
			{ID: "02", Verdict: sandbox.VerdictRan, Tokens: []string{"9"}, Duration: 6 * time.Millisecond},
		},
	}
}

func waitFor[T any](t *testing.T, read func() []T) []T {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if values := read(); len(values) > 0 {
			return values
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the worker")
	return nil
}

func startWorker(t *testing.T, queue Queue, inputs Inputs, judge Judge) (context.CancelFunc, <-chan error) {
	t.Helper()
	instance, err := New(Options{
		Queue: queue, Inputs: inputs, Judge: judge, Logger: quietLogger(),
		WorkerID: "test-worker", PollInterval: time.Millisecond, LeaseFor: time.Minute,
		MaxFailures: 2, ShutdownTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- instance.Run(ctx) }()
	return cancel, done
}

func TestEvaluateComparesTokensAndMapsVerdicts(t *testing.T) {
	compileFailure, err := evaluate(sandbox.JudgeResult{
		Verdict: sandbox.VerdictCompileError,
		Compile: sandbox.CompileResult{Succeeded: false},
	}, testInput().Tests)
	if err != nil {
		t.Fatalf("compile failure: %v", err)
	}
	if compileFailure.Verdict != "CE" || len(compileFailure.Tests) != 0 {
		t.Fatalf("compile outcome = %+v, want CE with no per-test rows", compileFailure)
	}

	outcome, err := evaluate(sandbox.JudgeResult{
		Verdict: sandbox.VerdictRan,
		Tests: []sandbox.TestResult{
			{ID: "01", Verdict: sandbox.VerdictRan, Tokens: []string{"1", "2", "3"}, Duration: 3 * time.Millisecond},
			{ID: "02", Verdict: sandbox.VerdictRan, Tokens: []string{"1", "2", "4"}, Duration: 5 * time.Millisecond},
		},
	}, []postgres.JudgeTest{
		{ID: "01", ExpectedOutput: "1  2\n3\n"},
		{ID: "02", ExpectedOutput: "1 2 3\n"},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if outcome.Verdict != "WA" || len(outcome.Tests) != 2 {
		t.Fatalf("outcome = %+v, want WA with two rows", outcome)
	}
	if outcome.Tests[0].Verdict != "AC" || outcome.Tests[0].TimeMs != 3 ||
		outcome.Tests[1].Verdict != "WA" || outcome.Tests[1].TimeMs != 5 {
		t.Fatalf("per-test rows = %+v", outcome.Tests)
	}

	cases := []struct {
		verdict sandbox.Verdict
		want    string
	}{
		{sandbox.VerdictTimeLimitExceeded, "TLE"},
		{sandbox.VerdictMemoryLimitExceeded, "MLE"},
		{sandbox.VerdictRuntimeError, "RE"},
	}
	for _, testCase := range cases {
		got, err := evaluate(sandbox.JudgeResult{
			Verdict: testCase.verdict,
			Tests:   []sandbox.TestResult{{ID: "01", Verdict: testCase.verdict, Duration: time.Millisecond}},
		}, testInput().Tests[:1])
		if err != nil {
			t.Fatalf("%s evaluate: %v", testCase.verdict, err)
		}
		if got.Verdict != testCase.want || got.Tests[0].Verdict != testCase.want {
			t.Fatalf("%s outcome = %+v", testCase.verdict, got)
		}
	}

	// Aggregate priority: TLE dominates an earlier WA.
	priority, err := evaluate(sandbox.JudgeResult{
		Verdict: sandbox.VerdictTimeLimitExceeded,
		Tests: []sandbox.TestResult{
			{ID: "01", Verdict: sandbox.VerdictRan, Tokens: []string{"wrong"}},
			{ID: "02", Verdict: sandbox.VerdictTimeLimitExceeded},
		},
	}, testInput().Tests)
	if err != nil {
		t.Fatalf("priority: %v", err)
	}
	if priority.Verdict != "TLE" {
		t.Fatalf("aggregate = %s, want TLE", priority.Verdict)
	}
}

func TestEvaluateRejectsIncompleteResults(t *testing.T) {
	_, err := evaluate(sandbox.JudgeResult{
		Verdict: sandbox.VerdictRan,
		Tests:   []sandbox.TestResult{{ID: "01", Verdict: sandbox.VerdictRan}},
	}, testInput().Tests)
	if err == nil {
		t.Fatal("expected a missing-test result to be refused")
	}
}

func TestValidateInputRejectsRevisionMismatch(t *testing.T) {
	if err := validateInput(testJob().Job, testInput()); err != nil {
		t.Fatalf("matching input: %v", err)
	}
	for name, mutate := range map[string]func(*postgres.JudgeInput){
		"lab":        func(input *postgres.JudgeInput) { input.LabID = "lab-2" },
		"generation": func(input *postgres.JudgeInput) { input.Generation = 2 },
		"hash":       func(input *postgres.JudgeInput) { input.SourceSHA256 = "different" },
	} {
		input := testInput()
		mutate(&input)
		if err := validateInput(testJob().Job, input); err == nil {
			t.Fatalf("%s mismatch was accepted", name)
		}
	}
}

func TestWorkerCommitsOneFencedResult(t *testing.T) {
	queue := &fakeQueue{pending: [][]postgres.LeasedJob{{testJob()}}}
	judge := &fakeJudge{result: goodResult()}
	cancel, done := startWorker(t, queue, &fakeInputs{input: testInput()}, judge)

	commits := waitFor(t, func() []commitCall {
		got, _, _ := queue.snapshot()
		return got
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(commits) != 1 || commits[0].jobID != "job-1" || commits[0].attempt != 1 {
		t.Fatalf("commits = %+v, want one at the leased fence", commits)
	}
	if commits[0].outcome.Verdict != "AC" || len(commits[0].outcome.Tests) != 2 {
		t.Fatalf("outcome = %+v", commits[0].outcome)
	}
}

func TestWorkerRecordsBoundedInfrastructureFailureWithoutVerdict(t *testing.T) {
	queue := &fakeQueue{pending: [][]postgres.LeasedJob{{testJob()}}}
	judge := &fakeJudge{err: &sandbox.InfrastructureError{Op: "docker run", Err: errors.New("daemon down")}}
	cancel, done := startWorker(t, queue, &fakeInputs{input: testInput()}, judge)

	fails := waitFor(t, func() []failCall {
		_, got, _ := queue.snapshot()
		return got
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
	commits, _, _ := queue.snapshot()
	if len(commits) != 0 {
		t.Fatalf("infrastructure failure stored a verdict: %+v", commits)
	}
	if len(fails) != 1 || fails[0].attempt != 1 || fails[0].maxFailures != 2 {
		t.Fatalf("fail calls = %+v", fails)
	}
}

func TestWorkerShutdownReleasesTheCurrentLease(t *testing.T) {
	queue := &fakeQueue{pending: [][]postgres.LeasedJob{{testJob()}}}
	judge := &fakeJudge{started: make(chan struct{}), block: true}
	cancel, done := startWorker(t, queue, &fakeInputs{input: testInput()}, judge)

	select {
	case <-judge.started:
	case <-time.After(2 * time.Second):
		t.Fatal("judge did not start")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
	commits, fails, releases := queue.snapshot()
	if len(commits) != 0 || len(fails) != 0 {
		t.Fatalf("shutdown stored effects: commits=%+v fails=%+v", commits, fails)
	}
	if len(releases) != 1 || releases[0].attempt != 1 {
		t.Fatalf("releases = %+v, want the current lease", releases)
	}
}

func TestWorkerCommitsACompletedResultDuringShutdown(t *testing.T) {
	queue := &fakeQueue{pending: [][]postgres.LeasedJob{{testJob()}}}
	judge := &fakeJudge{started: make(chan struct{}), block: true, commitAfterCancel: true, result: goodResult()}
	cancel, done := startWorker(t, queue, &fakeInputs{input: testInput()}, judge)

	select {
	case <-judge.started:
	case <-time.After(2 * time.Second):
		t.Fatal("judge did not start")
	}
	cancel() // the result becomes available as shutdown begins
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
	commits, fails, releases := queue.snapshot()
	if len(commits) != 1 || commits[0].attempt != 1 || commits[0].outcome.Verdict != "AC" {
		t.Fatalf("completed result was not stored on shutdown: %+v", commits)
	}
	if len(fails) != 0 || len(releases) != 0 {
		t.Fatalf("completed result should not fail or release: fails=%+v releases=%+v", fails, releases)
	}
}

func TestWorkerRefusesMismatchedInputAsAnOperationalFailure(t *testing.T) {
	queue := &fakeQueue{pending: [][]postgres.LeasedJob{{testJob()}}}
	input := testInput()
	input.SourceSHA256 = "tampered"
	judge := &fakeJudge{result: goodResult()}
	cancel, done := startWorker(t, queue, &fakeInputs{input: input}, judge)

	fails := waitFor(t, func() []failCall {
		_, got, _ := queue.snapshot()
		return got
	})
	cancel()
	<-done
	commits, _, _ := queue.snapshot()
	if len(commits) != 0 || len(fails) != 1 {
		t.Fatalf("mismatched input commits=%+v fails=%+v", commits, fails)
	}
}
