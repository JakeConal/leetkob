package sandbox

import (
	"fmt"
	"os"
	"time"
)

// Defaults for the execution boundary. They encode the PA#3 rubric
// minimums: a 64 KiB source cap, a 1 MiB collection cap per stream, a
// fixed 10-second compile wall, a 256 MiB execution memory ceiling with
// no swap, 64 PIDs/threads, one CPU, and a per-test wall of three times
// the lab's CPU-second limit.
const (
	// DefaultImage is the locally built fixed image. Deployment should
	// reference the same image by digest; README.md records the base
	// digest and the resolved build.
	DefaultImage = "leetkob-sandbox:pa3"

	// DefaultSourceBytes is the maximum accepted source size.
	DefaultSourceBytes = 65536
	// DefaultOutputBytes caps collected stdout and stderr independently.
	DefaultOutputBytes = 1 << 20
	// DefaultCompileWall is the fixed host-enforced compilation deadline.
	DefaultCompileWall = 10 * time.Second
	// DefaultRunMemoryBytes is the execution memory ceiling; the same
	// value is passed as --memory-swap, so no swap is available.
	DefaultRunMemoryBytes int64 = 256 << 20
	// DefaultCompileMemoryBytes is a separate, larger compile ceiling so
	// a legitimate compiler invocation is not confused with a memory
	// attack. Compilation still runs inside the same network/user/mount
	// boundary.
	DefaultCompileMemoryBytes int64 = 512 << 20
	// DefaultPidsLimit bounds processes and threads in both phases.
	DefaultPidsLimit = 64
	// DefaultCPUs is the execution CPU quota.
	DefaultCPUs = 1.0
	// DefaultCompileCPUs is the compile CPU quota.
	DefaultCompileCPUs = 1.0

	// wallPerCPUSecond is the spec's per-test wall multiplier.
	wallPerCPUSecond = 3 * time.Second
)

// Config pins the image and every bound the controller enforces. There is
// deliberately no "allow network", "run as root" or "mount host path"
// option: weakening a control requires changing code and the attack tests
// that prove it.
type Config struct {
	// Image is the fixed execution image reference.
	Image string
	// DockerBin is the Docker CLI executable (default "docker").
	DockerBin string
	// WorkRoot is the parent directory for disposable work directories
	// (default os.TempDir()).
	WorkRoot string
	// SourceBytes is the maximum accepted source size.
	SourceBytes int
	// OutputBytes caps each collected stream (stdout and stderr).
	OutputBytes int
	// CompileWall is the host-enforced compilation deadline.
	CompileWall time.Duration
	// RunMemoryBytes is the execution memory ceiling and equals the swap
	// ceiling passed to Docker.
	RunMemoryBytes int64
	// CompileMemoryBytes is the compile-phase memory ceiling.
	CompileMemoryBytes int64
	// PidsLimit bounds processes and threads.
	PidsLimit int
	// CPUs is the execution CPU quota.
	CPUs float64
	// CompileCPUs is the compile CPU quota.
	CompileCPUs float64
}

// DefaultConfig returns the fixed PA#3 defaults.
func DefaultConfig() Config {
	return Config{
		Image:              DefaultImage,
		DockerBin:          "docker",
		WorkRoot:           "",
		SourceBytes:        DefaultSourceBytes,
		OutputBytes:        DefaultOutputBytes,
		CompileWall:        DefaultCompileWall,
		RunMemoryBytes:     DefaultRunMemoryBytes,
		CompileMemoryBytes: DefaultCompileMemoryBytes,
		PidsLimit:          DefaultPidsLimit,
		CPUs:               DefaultCPUs,
		CompileCPUs:        DefaultCompileCPUs,
	}
}

// normalize fills defaults and rejects unsafe or nonsensical values. It
// never substitutes a missing Image or a non-positive limit.
func (c Config) normalize() (Config, error) {
	if c.Image == "" {
		return Config{}, fmt.Errorf("%w: image is required", ErrInvalidConfig)
	}
	if c.DockerBin == "" {
		c.DockerBin = "docker"
	}
	if c.WorkRoot == "" {
		c.WorkRoot = os.TempDir()
	}
	if c.SourceBytes <= 0 {
		return Config{}, fmt.Errorf("%w: source cap must be positive", ErrInvalidConfig)
	}
	if c.OutputBytes <= 0 {
		return Config{}, fmt.Errorf("%w: output cap must be positive", ErrInvalidConfig)
	}
	if c.CompileWall <= 0 {
		return Config{}, fmt.Errorf("%w: compile wall must be positive", ErrInvalidConfig)
	}
	if c.RunMemoryBytes <= 0 {
		return Config{}, fmt.Errorf("%w: run memory must be positive", ErrInvalidConfig)
	}
	if c.CompileMemoryBytes <= 0 {
		c.CompileMemoryBytes = c.RunMemoryBytes
		if c.CompileMemoryBytes < DefaultCompileMemoryBytes {
			c.CompileMemoryBytes = DefaultCompileMemoryBytes
		}
	}
	if c.PidsLimit <= 0 {
		return Config{}, fmt.Errorf("%w: pid limit must be positive", ErrInvalidConfig)
	}
	if c.CPUs <= 0 {
		return Config{}, fmt.Errorf("%w: cpu quota must be positive", ErrInvalidConfig)
	}
	if c.CompileCPUs <= 0 {
		c.CompileCPUs = c.CPUs
	}
	if c.CompileWall >= 10*time.Minute {
		return Config{}, fmt.Errorf("%w: compile wall out of range", ErrInvalidConfig)
	}
	return c, nil
}
