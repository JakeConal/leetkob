package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Controller compiles and runs one C++17 submission at a time inside the
// fixed execution image. It is safe for concurrent use: every run gets its
// own disposable directory and container name, and no mutable state is
// shared between Judge calls.
type Controller struct {
	cfg Config
}

// New validates the configuration and returns a controller. Configuration
// that would weaken or omit a limit is rejected rather than defaulted.
func New(cfg Config) (*Controller, error) {
	normalized, err := cfg.normalize()
	if err != nil {
		return nil, err
	}
	return &Controller{cfg: normalized}, nil
}

// Config returns a copy of the validated configuration. It is intended for
// observability and tests; it cannot be used to weaken a running controller.
func (c *Controller) Config() Config { return c.cfg }

// Judge compiles the submission once and runs every test against that
// artifact. It returns a distinct InfrastructureError when the boundary
// itself could not be established; callers must not map that to a verdict.
func (c *Controller) Judge(ctx context.Context, req JudgeRequest) (JudgeResult, error) {
	start := time.Now()
	if len(req.Source) > c.cfg.SourceBytes {
		return JudgeResult{}, fmt.Errorf("%w: %d bytes exceeds %d", ErrSourceTooLarge, len(req.Source), c.cfg.SourceBytes)
	}

	cpuSeconds := req.CPUSeconds
	if cpuSeconds <= 0 {
		cpuSeconds = 1
	}
	wall := wallFor(cpuSeconds)

	rootDir, workDir, err := c.newWorkDir()
	if err != nil {
		return JudgeResult{}, &InfrastructureError{Op: "prepare work directory", Err: err}
	}
	defer func() { _ = os.RemoveAll(rootDir) }()

	sourcePath := filepath.Join(workDir, "main.cpp")
	if err := os.WriteFile(sourcePath, req.Source, 0o644); err != nil {
		return JudgeResult{}, &InfrastructureError{Op: "write source", Err: err}
	}

	compile, err := c.compile(ctx, workDir)
	if err != nil {
		return JudgeResult{}, err
	}
	result := JudgeResult{Compile: compile}
	if !compile.Succeeded {
		result.Verdict = VerdictCompileError
		result.Duration = time.Since(start)
		return result, nil
	}

	for _, test := range req.Tests {
		testResult, err := c.runTest(ctx, workDir, test, wall)
		if err != nil {
			return JudgeResult{}, err
		}
		result.Tests = append(result.Tests, testResult)
	}
	result.Verdict = aggregate(compile, result.Tests)
	result.Duration = time.Since(start)
	return result, nil
}

// compile runs the fixed g++ invocation in the same isolation as
// execution. Nothing is ever compiled on the trusted host.
func (c *Controller) compile(ctx context.Context, workDir string) (CompileResult, error) {
	name, err := newContainerName()
	if err != nil {
		return CompileResult{}, &InfrastructureError{Op: "generate container name", Err: err}
	}
	out, err := c.run(ctx, runSpec{
		phase:        phaseCompile,
		name:         name,
		workDir:      workDir,
		readOnlyWork: false,
		command:      compileCommand(),
		timeout:      c.cfg.CompileWall,
		memoryBytes:  c.cfg.CompileMemoryBytes,
		cpus:         c.cfg.CompileCPUs,
	})
	if err != nil {
		return CompileResult{}, err
	}
	return CompileResult{
		Succeeded:       !out.timedOut && out.state.ExitCode == 0,
		TimedOut:        out.timedOut,
		Stdout:          out.stdout.String(),
		Stderr:          out.stderr.String(),
		OutputTruncated: out.stdout.Truncated() || out.stderr.Truncated(),
		Duration:        out.duration,
		Container:       out.containerName,
	}, nil
}

// runTest executes the compiled artifact with the lab's wall deadline.
func (c *Controller) runTest(ctx context.Context, workDir string, test TestCase, wall time.Duration) (TestResult, error) {
	name, err := newContainerName()
	if err != nil {
		return TestResult{}, &InfrastructureError{Op: "generate container name", Err: err}
	}
	out, err := c.run(ctx, runSpec{
		phase:        phaseRun,
		name:         name,
		workDir:      workDir,
		readOnlyWork: true,
		command:      []string{harnessPath},
		stdin:        test.Stdin,
		timeout:      wall,
		memoryBytes:  c.cfg.RunMemoryBytes,
		cpus:         c.cfg.CPUs,
	})
	if err != nil {
		return TestResult{}, err
	}
	stdout := out.stdout.String()
	return TestResult{
		ID:              test.ID,
		Verdict:         verdictFor(out),
		Stdout:          stdout,
		Stderr:          out.stderr.String(),
		OutputTruncated: out.stdout.Truncated() || out.stderr.Truncated(),
		Tokens:          tokenFields(stdout),
		ExitCode:        out.state.ExitCode,
		Duration:        out.duration,
		WallDeadline:    wall,
		Container:       out.containerName,
	}, nil
}

// verdictFor maps one observed run to a verdict. MLE is only claimed when
// Docker itself reports the OOM kill; without that signal memory
// exhaustion is a documented RE (exit 137), never an invented MLE.
func verdictFor(out runOutcome) Verdict {
	switch {
	case out.state.OOMKilled:
		return VerdictMemoryLimitExceeded
	case out.timedOut:
		return VerdictTimeLimitExceeded
	case out.stdout.Truncated() || out.stderr.Truncated():
		return VerdictRuntimeError
	case out.state.ExitCode != 0:
		return VerdictRuntimeError
	default:
		return VerdictRan
	}
}

// newWorkDir creates a private controller directory and, inside it, the
// disposable directory the unprivileged container may write to during
// compilation. Only the inner directory is bind-mounted.
//
// The outer directory keeps MkdirTemp's 0700 mode so another host user
// cannot traverse into the container-writable directory and swap the
// compiled artifact between compile and run. The inner directory is 0777
// because uid/gid 65534 must be able to write the compiled program.
func (c *Controller) newWorkDir() (rootDir, workDir string, err error) {
	rootDir, err = os.MkdirTemp(c.cfg.WorkRoot, "leetkob-sandbox-")
	if err != nil {
		return "", "", err
	}
	workDir = filepath.Join(rootDir, "work")
	if err := os.Mkdir(workDir, 0o777); err != nil {
		_ = os.RemoveAll(rootDir)
		return "", "", err
	}
	// Mkdir applies the process umask, so set the mode explicitly.
	if err := os.Chmod(workDir, 0o777); err != nil {
		_ = os.RemoveAll(rootDir)
		return "", "", err
	}
	return rootDir, workDir, nil
}

func wallFor(cpuSeconds int) time.Duration {
	if cpuSeconds <= 0 {
		cpuSeconds = 1
	}
	return time.Duration(cpuSeconds) * wallPerCPUSecond
}

func newContainerName() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "leetkob-sandbox-" + hex.EncodeToString(raw[:]), nil
}
