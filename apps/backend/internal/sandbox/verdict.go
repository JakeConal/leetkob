package sandbox

import "time"

// Verdict is the outcome the sandbox can establish on its own. AC/WA are
// deliberately not produced here: the worker compares the returned
// stdout tokens against the test's expected tokens (task 3.5). An
// execution that stays within every bound is VerdictRan.
type Verdict string

const (
	// VerdictRan means compilation succeeded and execution completed
	// within every bound. Token comparison decides AC or WA later.
	VerdictRan Verdict = "RAN"
	// VerdictCompileError means compilation failed or exceeded the fixed
	// compile wall.
	VerdictCompileError Verdict = "CE"
	// VerdictTimeLimitExceeded means the CPU budget or the host-enforced
	// per-test wall deadline ended the run.
	VerdictTimeLimitExceeded Verdict = "TLE"
	// VerdictMemoryLimitExceeded means Docker observed that the container
	// was OOM-killed at the configured memory/swap ceiling.
	VerdictMemoryLimitExceeded Verdict = "MLE"
	// VerdictRuntimeError means a non-zero exit, a crash, or output that
	// exceeded the configured collection cap.
	VerdictRuntimeError Verdict = "RE"
)

// String implements fmt.Stringer.
func (v Verdict) String() string { return string(v) }

// JudgeRequest is one bounded submission. Tests are trusted task inputs
// supplied by the lab author; only Source is untrusted.
type JudgeRequest struct {
	// Source is the C++17 translation unit. It must not exceed the
	// configured source cap.
	Source []byte
	// CPUSeconds is the lab's per-test CPU constraint. The host-enforced
	// wall deadline is three times this value. Zero means one second.
	CPUSeconds int
	// Tests are executed in order against the single compiled artifact.
	// An empty slice compiles only.
	Tests []TestCase
}

// TestCase is one stdin fixture. Expected output belongs to task 3.5.
type TestCase struct {
	// ID is echoed into TestResult for the caller's bookkeeping.
	ID string
	// Stdin is fed to the submission on stdin.
	Stdin []byte
}

// JudgeResult is the complete, bounded result of one submission.
type JudgeResult struct {
	// Verdict is the aggregate: CE when compilation failed, otherwise the
	// highest-priority test verdict (TLE, then MLE, then RE, then RAN).
	Verdict Verdict
	// Compile records the fixed compile phase.
	Compile CompileResult
	// Tests holds one entry per requested test case, in order.
	Tests []TestResult
	// Duration is the total controller-observed time.
	Duration time.Duration
}

// CompileResult records the compile phase. Container is the disposable
// container that performed it; it has already been force-removed when
// Judge returns.
type CompileResult struct {
	Succeeded       bool
	TimedOut        bool
	Stdout          string
	Stderr          string
	OutputTruncated bool
	Duration        time.Duration
	Container       string
}

// TestResult records one execution. Stdout and Stderr are capped at
// Config.OutputBytes each; Tokens is a whitespace split of the collected
// stdout for later token comparison. WallDeadline is the host-enforced
// deadline applied to this test.
type TestResult struct {
	ID              string
	Verdict         Verdict
	Stdout          string
	Stderr          string
	OutputTruncated bool
	Tokens          []string
	ExitCode        int
	Duration        time.Duration
	WallDeadline    time.Duration
	// Container is the disposable container name that ran this test. It is
	// force-removed before Judge returns, so an independent `docker
	// inspect` must fail; tests use this to prove the wall-case container
	// is gone rather than merely its client process.
	Container string
}

// aggregate maps per-test verdicts to one result. Priority follows the
// rubric mapping: TLE, then MLE, then RE, then RAN.
func aggregate(compile CompileResult, tests []TestResult) Verdict {
	if !compile.Succeeded {
		return VerdictCompileError
	}
	verdicts := map[Verdict]bool{}
	for _, test := range tests {
		verdicts[test.Verdict] = true
	}
	switch {
	case verdicts[VerdictTimeLimitExceeded]:
		return VerdictTimeLimitExceeded
	case verdicts[VerdictMemoryLimitExceeded]:
		return VerdictMemoryLimitExceeded
	case verdicts[VerdictRuntimeError]:
		return VerdictRuntimeError
	default:
		return VerdictRan
	}
}
