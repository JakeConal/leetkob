package analyzer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultImage is the locally built analyzer image used when the caller
// does not supply one. The image is built from apps/backend/analyzer; see
// that directory's README for the pinned digest and toolchain.
const DefaultImage = "leetkob-analyzer:pa3"

// Docker runs the analyzer image as a disposable, networkless,
// non-root container. The Go process only speaks bounded JSON over
// stdin/stdout; it never links libclang and never mounts a host path.
type Docker struct {
	Image   string
	Timeout time.Duration

	command string
	run     runnerFunc
}

// runnerFunc is the container boundary, injectable for tests. A real
// implementation writes stdin and returns bounded stdout.
type runnerFunc func(ctx context.Context, stdin []byte) ([]byte, error)

// NewDocker returns a Docker analyzer for image. An empty image selects
// DefaultImage.
func NewDocker(image string) *Docker {
	if image == "" {
		image = DefaultImage
	}
	return &Docker{Image: image, Timeout: DefaultTimeout, command: "docker"}
}

// New selects the real analyzer only when Docker answers. Otherwise it
// returns Fallback, so a missing Docker daemon always becomes unsure
// rather than a violation.
func New(ctx context.Context, image string) Analyzer {
	if err := CheckDocker(ctx); err != nil {
		return Fallback{Reason: err.Error()}
	}
	return NewDocker(image)
}

// CheckDocker verifies that the Docker CLI exists and its daemon answers.
func CheckDocker(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("%w: %v", ErrDockerUnavailable, err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(pingCtx, "docker", "version", "--format", "{{.Server.Version}}").Output()
	if err != nil {
		return fmt.Errorf("%w: daemon did not answer: %v", ErrDockerUnavailable, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("%w: daemon returned no version", ErrDockerUnavailable)
	}
	return nil
}

// Analyze implements Analyzer. It validates the request and reply itself;
// a malformed reply is an error, never a partial verdict set.
func (d *Docker) Analyze(ctx context.Context, req Request) (Report, error) {
	names, err := validateRequest(req)
	if err != nil {
		return Report{}, err
	}
	input, err := encodeRequest(req, names)
	if err != nil {
		return Report{}, err
	}
	run := d.run
	if run == nil {
		run = d.runContainer
	}
	runCtx, cancel := d.withTimeout(ctx)
	defer cancel()
	raw, err := run(runCtx, input)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return Report{}, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return Report{}, err
	}
	return decodeReport(raw, req.Source, names)
}

func (d *Docker) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	deadline := time.Now().Add(timeout)
	if current, ok := ctx.Deadline(); ok && current.Before(deadline) {
		deadline = current
	}
	return context.WithDeadline(ctx, deadline)
}

func (d *Docker) runContainer(ctx context.Context, stdin []byte) ([]byte, error) {
	command := d.command
	if command == "" {
		command = "docker"
	}
	image := d.Image
	if image == "" {
		image = DefaultImage
	}
	name := containerName()
	args := []string{
		"run", "--rm", "--interactive",
		"--name", name,
		"--network", "none",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=16m",
		"--user", "65534:65534",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "64",
		"--memory", "512m",
		"--memory-swap", "512m",
		"--cpus", "1",
		"--stop-timeout", "1",
		image,
	}
	cmd := exec.CommandContext(ctx, command, args...)
	stdout := &boundedBuffer{limit: MaxOutputBytes}
	stderr := &cappedBuffer{limit: 8 << 10}
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		cleanupContainer(command, name)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, ctx.Err())
		}
		return nil, fmt.Errorf("%w: %v", ErrContainerFailed, ctx.Err())
	}
	if stdout.overflow {
		return nil, fmt.Errorf("%w: maximum is %d bytes", ErrOutputTooLarge, MaxOutputBytes)
	}
	if err != nil {
		// Do not include stderr in errors: libclang diagnostics can quote
		// student source, and tooling logs must not leak it. The caller
		// records a generic unsure cause instead.
		cleanupContainer(command, name)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%w: exit code %d", ErrContainerFailed, exitErr.ExitCode())
		}
		return nil, fmt.Errorf("%w: %v", ErrContainerFailed, err)
	}
	return stdout.Bytes(), nil
}

func cleanupContainer(command, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, command, "rm", "--force", name).Run()
}

func containerName() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("leetkob-analyzer-%d", time.Now().UnixNano())
	}
	return "leetkob-analyzer-" + hex.EncodeToString(raw[:])
}

// boundedBuffer fails the copy when the analyzer writes more than limit,
// so an unbounded reply cannot grow Go memory.
type boundedBuffer struct {
	limit    int
	buf      bytes.Buffer
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > b.limit {
		b.overflow = true
		return 0, ErrOutputTooLarge
	}
	return b.buf.Write(p)
}

func (b *boundedBuffer) Bytes() []byte { return b.buf.Bytes() }

// cappedBuffer keeps a small diagnostic prefix and discards the rest
// without failing the command; stderr is never included in an error.
type cappedBuffer struct {
	limit    int
	buf      bytes.Buffer
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.buf.Write(p)
		} else {
			_, _ = b.buf.Write(p[:remaining])
			b.overflow = true
		}
	} else {
		b.overflow = true
	}
	return len(p), nil
}
