package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Fixed paths and identities inside the execution image.
const (
	// sandboxUser is the unprivileged container identity. The image also
	// sets USER 65534:65534; passing it here keeps the boundary explicit
	// and inspectable in the argument list.
	sandboxUser = "65534:65534"
	// harnessPath is the small fixed run harness baked into the image.
	harnessPath = "/usr/local/bin/run-harness"
	// workDirInContainer is the only bind-mounted path.
	workDirInContainer = "/work"

	runTmpfsSize     = "16m"
	compileTmpfsSize = "256m"

	// cleanupTimeout bounds docker kill/rm so a wedged daemon cannot hang
	// a worker forever. Kill/remove use a fresh context because the run
	// context may already be expired.
	cleanupTimeout = 10 * time.Second
	// inspectTimeout bounds the post-run state read.
	inspectTimeout = 5 * time.Second
)

type phase string

const (
	phaseCompile phase = "compile"
	phaseRun     phase = "run"
)

// runSpec describes exactly one disposable container. It contains no
// opt-out switches: every field is a bound, never a relaxation.
type runSpec struct {
	phase        phase
	name         string
	workDir      string
	readOnlyWork bool
	command      []string
	stdin        []byte
	timeout      time.Duration
	memoryBytes  int64
	cpus         float64
}

// containerState is the subset of `docker inspect .State` the controller
// needs. OOMKilled is what makes MLE observable instead of a plain RE.
type containerState struct {
	Status    string `json:"Status"`
	Running   bool   `json:"Running"`
	ExitCode  int    `json:"ExitCode"`
	OOMKilled bool   `json:"OOMKilled"`
	Error     string `json:"Error"`
}

type runOutcome struct {
	containerName string
	state         containerState
	timedOut      bool
	stdout        *limitedBuffer
	stderr        *limitedBuffer
	duration      time.Duration
}

// containerArgs returns the complete Docker argument vector. It is a pure
// function of the spec and configuration so tests can assert every
// required control without starting a container.
func (c *Controller) containerArgs(spec runSpec) []string {
	tmpfs := runTmpfsSize
	if spec.phase == phaseCompile {
		tmpfs = compileTmpfsSize
	}
	mount := "type=bind,src=" + spec.workDir + ",dst=" + workDirInContainer
	if spec.readOnlyWork {
		mount += ",readonly"
	}

	args := []string{
		"run",
		"--name", spec.name,
		// No network namespace route, loopback not needed either.
		"--network", "none",
		// Unprivileged identity, repeated from the image for inspectability.
		"--user", sandboxUser,
		// Immutable root filesystem; only /work and a private /tmp are writable.
		"--read-only",
		"--tmpfs", "/tmp:rw,nosuid,nodev,mode=1777,size=" + tmpfs,
		// No Linux capabilities and no privilege escalation via setuid binaries.
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges:true",
		// Memory ceiling AND an equal swap ceiling: equal means no swap.
		"--memory", strconv.FormatInt(spec.memoryBytes, 10),
		"--memory-swap", strconv.FormatInt(spec.memoryBytes, 10),
		// Process/thread ceiling.
		"--pids-limit", strconv.Itoa(c.cfg.PidsLimit),
		// CPU quota; the wall deadline below is the actual kill.
		"--cpus", formatFloat(spec.cpus),
		// Never write a core dump into the disposable mount.
		"--ulimit", "core=0:0",
		// The only host path visible: a disposable copy of the work dir.
		"--mount", mount,
		"--workdir", workDirInContainer,
		"--env", "HOME=/tmp",
		"--env", "TMPDIR=/tmp",
	}
	if spec.phase == phaseRun {
		// Keep stdin attached for the test fixture; EOF otherwise.
		args = append(args, "--interactive")
	}
	args = append(args, c.cfg.Image)
	args = append(args, spec.command...)
	return args
}

// run starts one container, enforces the wall deadline, reads its state,
// and force-removes it before returning. A missing container after the
// command finishes is an infrastructure failure, never a verdict.
func (c *Controller) run(ctx context.Context, spec runSpec) (runOutcome, error) {
	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, spec.timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, c.cfg.DockerBin, c.containerArgs(spec)...)
	cmd.Env = dockerEnv()
	if len(spec.stdin) > 0 {
		cmd.Stdin = bytes.NewReader(spec.stdin)
	}
	stdout := newLimitedBuffer(c.cfg.OutputBytes)
	stderr := newLimitedBuffer(c.cfg.OutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	duration := time.Since(start)
	// A wall deadline belongs to this run. A cancelled or expired caller
	// context is an infrastructure interruption, never a TLE.
	parentErr := ctx.Err()
	timedOut := parentErr == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded)

	state, inspectErr := c.inspect(spec.name)
	// Always clean up, even when inspection failed: an orphaned container
	// would otherwise keep consuming resources.
	c.forceRemove(spec.name)

	if parentErr != nil {
		return runOutcome{}, &InfrastructureError{Op: "docker run", Err: parentErr}
	}
	if inspectErr != nil {
		return runOutcome{}, &InfrastructureError{
			Op:  "docker inspect",
			Err: fmt.Errorf("%w (run error: %v, stderr: %s)", inspectErr, runErr, tail(stderr.String(), 512)),
		}
	}
	if state.Error != "" && state.Status != "exited" && state.Status != "running" {
		return runOutcome{}, &InfrastructureError{
			Op:  "docker run",
			Err: fmt.Errorf("container reported state error %q (status %q)", state.Error, state.Status),
		}
	}
	return runOutcome{
		containerName: spec.name,
		state:         state,
		timedOut:      timedOut,
		stdout:        stdout,
		stderr:        stderr,
		duration:      duration,
	}, nil
}

// inspect reads the container state with a fresh context, because the run
// context may already have expired.
func (c *Controller) inspect(name string) (containerState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), inspectTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.cfg.DockerBin,
		"inspect", "--format", "{{json .State}}", name,
	)
	cmd.Env = dockerEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return containerState{}, fmt.Errorf("%w: %s", err, tail(string(out), 512))
	}
	var state containerState
	if err := json.Unmarshal(bytes.TrimSpace(out), &state); err != nil {
		return containerState{}, fmt.Errorf("decode container state: %w", err)
	}
	return state, nil
}

// forceRemove kills and removes the container with bounded contexts.
// Errors are intentionally ignored: the caller has no safe recovery and
// a leaked stopped container is less harmful than blocking the worker.
// The attack tests independently assert the container is gone.
func (c *Controller) forceRemove(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	kill := exec.CommandContext(ctx, c.cfg.DockerBin, "kill", name)
	kill.Env = dockerEnv()
	_ = kill.Run()

	ctx2, cancel2 := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel2()
	remove := exec.CommandContext(ctx2, c.cfg.DockerBin, "rm", "--force", name)
	remove.Env = dockerEnv()
	_ = remove.Run()
}

// dockerEnv passes only what the Docker CLI needs. The container receives
// no host environment at all (Docker forwards only explicit --env/--env-file
// values), and this also keeps unrelated host secrets out of the trusted
// CLI process.
func dockerEnv() []string {
	passthrough := []string{
		"PATH",
		"HOME",
		"DOCKER_HOST",
		"DOCKER_CONFIG",
		"DOCKER_CONTEXT",
		"DOCKER_TLS_VERIFY",
		"DOCKER_CERT_PATH",
		"XDG_RUNTIME_DIR",
	}
	env := make([]string, 0, len(passthrough))
	for _, key := range passthrough {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	if os.Getenv("PATH") == "" {
		env = append(env, "PATH=/usr/local/bin:/usr/bin:/bin")
	}
	return env
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// tail returns at most the last n bytes, keeping diagnostics bounded.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// compileCommand is the fixed C++17 invocation. -O2 is part of the image
// contract; no caller-supplied flags reach the compiler.
func compileCommand() []string {
	return []string{
		"g++",
		"-O2",
		"-std=c++17",
		"-pipe",
		"-fno-diagnostics-color",
		"-o", workDirInContainer + "/program",
		workDirInContainer + "/main.cpp",
	}
}

// tokenFields splits bounded stdout for task 3.5 token comparison. The
// token count is capped so a 1 MiB flood cannot inflate worker memory.
const maxTokenCount = 65536

func tokenFields(stdout string) []string {
	fields := strings.Fields(stdout)
	if len(fields) > maxTokenCount {
		return fields[:maxTokenCount]
	}
	return fields
}
