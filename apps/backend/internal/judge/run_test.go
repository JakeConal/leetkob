package judge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/domain"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

type fakeRuntime struct {
	compile    sandbox.CompileResult
	compileErr error
	exec       func(stdin []byte) (sandbox.Observation, error)
	execCalls  int
}

func (f *fakeRuntime) Compile(context.Context, []byte) (sandbox.CompileResult, error) {
	return f.compile, f.compileErr
}

func (f *fakeRuntime) ExecuteInput(_ context.Context, _ []byte, _ sandbox.RunPolicy, _ []string, stdin []byte) (sandbox.Observation, error) {
	f.execCalls++
	return f.exec(stdin)
}

type memLoader struct {
	work Work
}

func (m memLoader) LoadJudgeWork(context.Context, domain.Job) (Work, error) {
	return m.work, nil
}

func TestSandboxJudgeUsesFakeObservations(t *testing.T) {
	job := domain.Job{SourceSHA256: "abc", SubmissionID: "s", LabID: "l"}
	work := Work{
		Source:       []byte("int main(){return 0;}"),
		SourceSHA256: "abc",
		CPUSeconds:   1,
		MemoryMiB:    64,
		Tests: []Case{
			{ID: "hidden", Input: "secret-input", Expected: "1 2"},
			{ID: "second", Input: "more", Expected: "9"},
		},
	}
	rt := &fakeRuntime{
		compile: sandbox.CompileResult{OK: true, Binary: []byte("bin"), Observation: sandbox.Observation{ExitCode: 0}},
		exec: func(stdin []byte) (sandbox.Observation, error) {
			if string(stdin) == "secret-input" {
				return sandbox.Observation{Stdout: []byte("1\n2\n")}, nil
			}
			return sandbox.Observation{ExitCode: 137}, nil
		},
	}
	got, err := (&SandboxJudge{Runtime: rt, Loader: memLoader{work}}).Execute(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != VerdictTLE {
		t.Fatalf("aggregate = %s, want TLE", got.Verdict)
	}
	if len(got.Tests) != 2 || got.Tests[0].Verdict != VerdictAC || got.Tests[1].Verdict != VerdictTLE {
		t.Fatalf("tests = %+v", got.Tests)
	}
	if rt.execCalls != 2 {
		t.Fatalf("execute calls = %d", rt.execCalls)
	}
}

func TestSandboxJudgeDoesNotRunWhenCompileFails(t *testing.T) {
	rt := &fakeRuntime{
		compile: sandbox.CompileResult{Observation: sandbox.Observation{ExitCode: 1}},
		exec: func([]byte) (sandbox.Observation, error) {
			t.Fatal("executed after compile failure")
			return sandbox.Observation{}, nil
		},
	}
	work := Work{
		SourceSHA256: "abc",
		CPUSeconds:   1,
		MemoryMiB:    64,
		Tests:        []Case{{ID: "t", Input: "in", Expected: "out"}},
	}
	got, err := (&SandboxJudge{Runtime: rt, Loader: memLoader{work}}).Execute(context.Background(), domain.Job{SourceSHA256: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != VerdictCE || got.Tests[0].Verdict != VerdictCE {
		t.Fatalf("result = %+v", got)
	}
}

func TestSandboxJudgeKeepsInfrastructureOffTheVerdict(t *testing.T) {
	rt := &fakeRuntime{compileErr: errors.New("docker daemon: dial unix")}
	_, err := (&SandboxJudge{Runtime: rt, Loader: memLoader{Work{
		SourceSHA256: "abc",
		CPUSeconds:   1,
		MemoryMiB:    64,
		Tests:        []Case{{ID: "t", Expected: "1"}},
	}}}).Execute(context.Background(), domain.Job{SourceSHA256: "abc"})
	if !errors.Is(err, ErrCompileInfra) {
		t.Fatalf("err = %v", err)
	}
	rt.compileErr = nil
	rt.compile = sandbox.CompileResult{OK: true, Binary: []byte("bin")}
	rt.exec = func([]byte) (sandbox.Observation, error) {
		return sandbox.Observation{}, errors.New("docker start: broken")
	}
	_, err = (&SandboxJudge{Runtime: rt, Loader: memLoader{Work{
		SourceSHA256: "abc",
		CPUSeconds:   1,
		MemoryMiB:    64,
		Tests:        []Case{{ID: "t", Expected: "1"}},
	}}}).Execute(context.Background(), domain.Job{SourceSHA256: "abc"})
	if !errors.Is(err, ErrExecuteInfra) {
		t.Fatalf("err = %v", err)
	}
}

func TestSandboxJudgeMeasuresNonNegativeTime(t *testing.T) {
	clock := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	rt := &fakeRuntime{
		compile: sandbox.CompileResult{OK: true, Binary: []byte("bin")},
		exec: func([]byte) (sandbox.Observation, error) {
			return sandbox.Observation{Stdout: []byte("ok")}, nil
		},
	}
	got, err := (&SandboxJudge{
		Runtime: rt,
		Loader: memLoader{Work{
			SourceSHA256: "abc",
			CPUSeconds:   1,
			MemoryMiB:    64,
			Tests:        []Case{{ID: "t", Expected: "ok"}},
		}},
		Now: func() time.Time {
			clock = clock.Add(5 * time.Millisecond)
			return clock
		},
	}).Execute(context.Background(), domain.Job{SourceSHA256: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != VerdictAC || got.Tests[0].TimeMS < 0 {
		t.Fatalf("result = %+v", got)
	}
}
