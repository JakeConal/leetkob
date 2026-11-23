package judge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/domain"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

// Infrastructure errors. They are operational and must not be stored as
// AC, WA, TLE, CE, RE, or MLE.
var (
	ErrCompileInfra = errors.New("compile infrastructure failure")
	ErrExecuteInfra = errors.New("execute infrastructure failure")
	ErrNoTests      = errors.New("judge: lab has no tests")
	ErrStaleAttempt = errors.New("judge: stale attempt")
)

// MaxAttempts is the fence bound. A job that fails infrastructure this
// many times stays failed and visible; it does not become a student verdict.
const MaxAttempts = 3

// TestResult is one private test's student-visible outcome. It carries no
// input, expected output, or program output.
type TestResult struct {
	ID      string
	Verdict string
	TimeMS  int
}

// Result is one stored verdict set for a submission and input revision.
type Result struct {
	Verdict string
	Tests   []TestResult
}

// Work is the trusted lab material the sandbox judge needs. Source is the
// submitted bytes. Tests stay inside this process.
type Work struct {
	Source       []byte
	SourceSHA256 string
	AuthorUserID string
	CPUSeconds   int
	MemoryMiB    int
	Tests        []Case
}

// Case is one lab test. Input is the program's stdin.
type Case struct {
	ID       string
	Input    string
	Expected string
}

// Loader reads the immutable source and tests for a leased job.
type Loader interface {
	LoadJudgeWork(ctx context.Context, job domain.Job) (Work, error)
}

// Runtime is the sandbox boundary. Tests substitute finished observations.
type Runtime interface {
	Compile(ctx context.Context, source []byte) (sandbox.CompileResult, error)
	ExecuteInput(ctx context.Context, program []byte, policy sandbox.RunPolicy, argv []string, stdin []byte) (sandbox.Observation, error)
}

// SandboxJudge compiles with the sandbox and compares output tokens.
type SandboxJudge struct {
	Runtime Runtime
	Loader  Loader
	Now     func() time.Time
}

// Execute judges one leased job. A non-nil error is infrastructure: the
// caller must not commit Result as a student verdict.
func (j *SandboxJudge) Execute(ctx context.Context, job domain.Job) (Result, error) {
	if j == nil || j.Runtime == nil || j.Loader == nil {
		return Result{}, errors.New("judge: runtime is not configured")
	}
	work, err := j.Loader.LoadJudgeWork(ctx, job)
	if err != nil {
		return Result{}, err
	}
	if work.SourceSHA256 != job.SourceSHA256 {
		return Result{}, errors.New("judge: source digest mismatch")
	}
	if len(work.Tests) == 0 {
		return Result{}, ErrNoTests
	}
	compiled, err := j.Runtime.Compile(ctx, work.Source)
	if err != nil {
		return Result{}, fmt.Errorf("%w", ErrCompileInfra)
	}
	if verdict, failed := CompileVerdict(compiled.Observation, compiled.OK); failed {
		tests := make([]TestResult, len(work.Tests))
		for i, tc := range work.Tests {
			tests[i] = TestResult{ID: tc.ID, Verdict: verdict, TimeMS: 0}
		}
		return Result{Verdict: verdict, Tests: tests}, nil
	}
	policy, err := sandbox.JudgeRunPolicy(work.CPUSeconds, work.MemoryMiB)
	if err != nil {
		return Result{}, err
	}
	now := time.Now
	if j.Now != nil {
		now = j.Now
	}
	tests := make([]TestResult, 0, len(work.Tests))
	for _, tc := range work.Tests {
		start := now()
		obs, err := j.Runtime.ExecuteInput(ctx, compiled.Binary, policy, nil, []byte(tc.Input))
		if err != nil {
			return Result{}, fmt.Errorf("%w", ErrExecuteInfra)
		}
		elapsed := now().Sub(start)
		ms := int(elapsed / time.Millisecond)
		if ms < 0 {
			ms = 0
		}
		tests = append(tests, TestResult{
			ID:      tc.ID,
			Verdict: ExecuteVerdict(obs, tc.Expected),
			TimeMS:  ms,
		})
	}
	return Result{Verdict: Aggregate(tests), Tests: tests}, nil
}
