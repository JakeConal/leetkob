package sandbox

import (
	"fmt"
	"time"
)

const (
	// ImageRef is the local tag the controller builds and runs.
	ImageRef = "leetkob-cpp17:pa3"
	// SandboxUser is the unprivileged uid:gid baked into the image.
	SandboxUser = "10001:10001"
	// PidCeiling is the process and thread cap, including the main thread.
	PidCeiling = 16
	// DefaultOutputBytes is the 1 MiB collection cap from the PA#3 contract.
	DefaultOutputBytes = 1 << 20
	// MaxSourceBytes is the fixed 65,536-byte source ceiling.
	MaxSourceBytes = 65536
	// CompileWall is the separate compilation deadline.
	CompileWall = 10 * time.Second
	// CompileCPUSeconds is the compilation CPU-time ceiling.
	CompileCPUSeconds = 10
	// CompileMemoryBytes is the compilation memory and swap ceiling.
	CompileMemoryBytes = 512 << 20

	minCPUSeconds = 1
	maxCPUSeconds = 5
	minMemory     = 64 << 20
	maxMemory     = 512 << 20
	maxWall       = 30 * time.Second
	maxExtract    = 8 << 20
)

// RunPolicy is the numeric limit set for one execution. Isolation flags are
// not fields: Compile and Execute always apply them. Judging callers use
// JudgeRunPolicy so the wall deadline stays three times the CPU allowance.
type RunPolicy struct {
	CPUSeconds  int
	MemoryBytes int64
	Wall        time.Duration
	OutputLimit int
}

// JudgeRunPolicy builds the execution policy for a lab limit. cpuSeconds is
// 1–5 and memoryMiB is 64–512. The wall deadline is three times the CPU
// allowance and output collection is capped at 1 MiB.
func JudgeRunPolicy(cpuSeconds, memoryMiB int) (RunPolicy, error) {
	policy := RunPolicy{
		CPUSeconds:  cpuSeconds,
		MemoryBytes: int64(memoryMiB) << 20,
		Wall:        time.Duration(cpuSeconds) * 3 * time.Second,
		OutputLimit: DefaultOutputBytes,
	}
	if err := policy.validate(); err != nil {
		return RunPolicy{}, err
	}
	return policy, nil
}

func (p RunPolicy) validate() error {
	if p.CPUSeconds < minCPUSeconds || p.CPUSeconds > maxCPUSeconds {
		return fmt.Errorf("%w: cpu seconds %d", ErrPolicy, p.CPUSeconds)
	}
	if p.MemoryBytes < minMemory || p.MemoryBytes > maxMemory || p.MemoryBytes%(1<<20) != 0 {
		return fmt.Errorf("%w: memory %d", ErrPolicy, p.MemoryBytes)
	}
	if p.Wall <= 0 || p.Wall > maxWall {
		return fmt.Errorf("%w: wall %s", ErrPolicy, p.Wall)
	}
	if p.OutputLimit < 1 || p.OutputLimit > DefaultOutputBytes {
		return fmt.Errorf("%w: output limit %d", ErrPolicy, p.OutputLimit)
	}
	return nil
}

func compileProfile() profile {
	return profile{
		Production:  true,
		Network:     "none",
		User:        SandboxUser,
		CapDropAll:  true,
		NoNewPrivs:  true,
		Pids:        PidCeiling,
		MemoryBytes: CompileMemoryBytes,
		MemorySwap:  CompileMemoryBytes,
		CPUSeconds:  CompileCPUSeconds,
	}
}

func runProfile(policy RunPolicy) (profile, error) {
	if err := policy.validate(); err != nil {
		return profile{}, err
	}
	return profile{
		Production:  true,
		Network:     "none",
		User:        SandboxUser,
		CapDropAll:  true,
		NoNewPrivs:  true,
		Pids:        PidCeiling,
		MemoryBytes: policy.MemoryBytes,
		MemorySwap:  policy.MemoryBytes,
		CPUSeconds:  policy.CPUSeconds,
	}, nil
}
