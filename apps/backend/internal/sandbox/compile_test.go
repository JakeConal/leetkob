package sandbox

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCompileFailureMapsToCE(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	result := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "compile_error.cpp"),
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "never-runs"}},
	})

	if result.Verdict != VerdictCompileError {
		t.Fatalf("verdict = %s, want CE", result.Verdict)
	}
	if result.Compile.Succeeded {
		t.Fatal("compile reported success for invalid source")
	}
	if len(result.Tests) != 0 {
		t.Fatalf("tests ran despite compile failure: %d", len(result.Tests))
	}
	if !strings.Contains(result.Compile.Stderr, "error") {
		t.Fatalf("compile diagnostics look empty: %q", result.Compile.Stderr)
	}
	assertContainerGone(t, result.Compile.Container)
}

func TestCompileTimeoutMapsToCE(t *testing.T) {
	requireDocker(t)
	cfg := testController(t).Config()
	// A 2-second compile wall is far shorter than the blocking preprocessing
	// fixture but long enough that Docker has created the container, so the
	// outcome is a compile deadline rather than an infrastructure failure.
	cfg.CompileWall = 2 * time.Second
	controller, err := New(cfg)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}

	result := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "compile_hang.cpp"),
		CPUSeconds: 1,
	})

	if result.Verdict != VerdictCompileError {
		t.Fatalf("verdict = %s, want CE", result.Verdict)
	}
	if !result.Compile.TimedOut {
		t.Fatalf("compile was not reported as timed out: %+v", result.Compile)
	}
	if result.Compile.Duration > 20*time.Second {
		t.Fatalf("compile ran for %s; wall was not enforced", result.Compile.Duration)
	}
	if len(result.Compile.Stdout) > controller.Config().OutputBytes ||
		len(result.Compile.Stderr) > controller.Config().OutputBytes {
		t.Fatalf("compile output exceeded the cap: stdout=%d stderr=%d",
			len(result.Compile.Stdout), len(result.Compile.Stderr))
	}
	assertContainerGone(t, result.Compile.Container)
}

func TestSourceCapRejectsBeforeAnyContainer(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	oversized := bytes.Repeat([]byte(" "), controller.Config().SourceBytes+1)

	_, err := controller.Judge(context.Background(), JudgeRequest{Source: oversized})
	if !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("error = %v, want ErrSourceTooLarge", err)
	}
	if IsInfrastructure(err) {
		t.Fatalf("oversized source must not be an infrastructure error: %v", err)
	}
}

func TestInfrastructureFailureIsDistinctFromVerdict(t *testing.T) {
	requireDocker(t)
	cfg := DefaultConfig()
	cfg.Image = sandboxImage
	cfg.DockerBin = "/bin/false"

	controller, err := New(cfg)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	result, err := controller.Judge(context.Background(), JudgeRequest{
		Source: attackSource(t, "compliant.cpp"),
	})
	if err == nil {
		t.Fatalf("expected an infrastructure error, got result %+v", result)
	}
	if !IsInfrastructure(err) {
		t.Fatalf("error %v is not an InfrastructureError", err)
	}
	if result.Verdict != "" {
		t.Fatalf("infrastructure failure produced a verdict %q", result.Verdict)
	}
}

func TestAggregateMapping(t *testing.T) {
	ok := CompileResult{Succeeded: true}
	tests := []struct {
		name    string
		compile CompileResult
		results []TestResult
		want    Verdict
	}{
		{name: "compile-failed", compile: CompileResult{Succeeded: false}, want: VerdictCompileError},
		{name: "all-ran", compile: ok, results: []TestResult{{Verdict: VerdictRan}}, want: VerdictRan},
		{name: "compile-only", compile: ok, want: VerdictRan},
		{name: "re", compile: ok, results: []TestResult{{Verdict: VerdictRan}, {Verdict: VerdictRuntimeError}}, want: VerdictRuntimeError},
		{name: "mle", compile: ok, results: []TestResult{{Verdict: VerdictMemoryLimitExceeded}}, want: VerdictMemoryLimitExceeded},
		{name: "tle", compile: ok, results: []TestResult{{Verdict: VerdictRuntimeError}, {Verdict: VerdictTimeLimitExceeded}}, want: VerdictTimeLimitExceeded},
		{name: "tle-beats-mle", compile: ok, results: []TestResult{{Verdict: VerdictMemoryLimitExceeded}, {Verdict: VerdictTimeLimitExceeded}}, want: VerdictTimeLimitExceeded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := aggregate(tc.compile, tc.results); got != tc.want {
				t.Fatalf("aggregate = %s, want %s", got, tc.want)
			}
		})
	}
}
