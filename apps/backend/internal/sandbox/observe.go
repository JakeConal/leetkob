package sandbox

import "errors"

var (
	// ErrPolicy means a numeric limit is outside the fixed PA#3 range.
	ErrPolicy = errors.New("sandbox: policy rejected")
	// ErrInsecureInvocation means a Docker argv would drop an isolation control
	// or would pass a credential, host mount, or the Docker socket.
	ErrInsecureInvocation = errors.New("sandbox: insecure invocation")
)

// Observation is one finished container. Verdict mapping belongs to task 3.5
// and is intentionally not applied here:
//
//   - Compile failure, or a compilation WallTimedOut, becomes CE.
//   - WallTimedOut, or a CPU-time death (non-zero signal exit with
//     OOMKilled, WallTimedOut, and OutputLimited all false), becomes TLE.
//   - OOMKilled becomes MLE. Docker records State.OOMKilled for the cgroup
//     killer, which distinguishes it from the wall-clock SIGKILL. A backend
//     that cannot report that bit must document the memory result as RE
//     before it is used. MemorySwap is 0 when cgroup v2 disables swap; that
//     is a zero swap ceiling, not an unlimited one. Unlimited swap is -1.
//   - OutputLimited, or any other non-zero exit, becomes RE.
//   - An error returned by Compile or Execute is an infrastructure failure.
//     It is not a student verdict.
type Observation struct {
	Name              string
	ExitCode          int
	Stdout            []byte
	Stderr            []byte
	OutputLimited     bool
	WallTimedOut      bool
	OOMKilled         bool
	ContainerRemoved  bool
	Invocation        []string
	MemoryBytes       int64
	MemorySwapBytes   int64
	PidsLimit         int64
	CPUSeconds        int64
	NetworkMode       string
	User              string
	Privileged        bool
	Binds             []string
	MountSources      []string
	MountDestinations []string
	CapDrop           []string
	SecurityOpt       []string
	Env               []string
}

// CompileResult is an isolated g++ invocation. Binary is populated only when
// the compiler exits 0 inside the sandbox. The bytes are untrusted and must
// be executed only by Execute.
type CompileResult struct {
	Observation
	Binary []byte
	OK     bool
}

// ImageProvenance is what Build observed from the local Docker daemon.
// RepoDigests is empty for a locally tagged image; the base pin lives in
// the Dockerfile. Package and compiler strings are command output, not a
// release attestation.
type ImageProvenance struct {
	Ref        string
	ImageID    string
	Compiler   string
	Packages   string
	Invocation []string
}
