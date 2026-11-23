package sandbox

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// compileCommand is the fixed C++17 invocation. -O2 and -pthread match the
// judging profile; there is no shell and no host path.
var compileCommand = []string{
	"g++", "-std=c++17", "-O2", "-pthread", "-o", "/work/prog", "/work/main.cpp",
}

var containerNamePattern = regexp.MustCompile(`^leetkob-sbx-[0-9a-f]{16}$`)

type bind struct {
	Source string
	Target string
}

// profile is the Docker create configuration. Production profiles are built
// only by compileProfile and runProfile. Tests call relaxForNegativeControl
// to drop exactly one control and show the attack oracle can see it.
type profile struct {
	Production  bool
	Network     string
	User        string
	CapDropAll  bool
	NoNewPrivs  bool
	Pids        int
	MemoryBytes int64
	MemorySwap  int64
	CPUSeconds  int
	Privileged  bool
	Binds       []bind
	Env         []string
}

func (p profile) relaxForNegativeControl(reason string) (profile, error) {
	if strings.TrimSpace(reason) == "" {
		return profile{}, fmt.Errorf("%w: negative control requires a reason", ErrInsecureInvocation)
	}
	p.Production = false
	return p, nil
}

func createArgs(name, image string, p profile, command []string) ([]string, error) {
	if !containerNamePattern.MatchString(name) {
		return nil, fmt.Errorf("%w: container name", ErrInsecureInvocation)
	}
	if image == "" || len(command) == 0 {
		return nil, fmt.Errorf("%w: image and command are required", ErrInsecureInvocation)
	}
	if err := validateCommand(command); err != nil {
		return nil, err
	}
	args := []string{
		"create",
		"--name", name,
		"--network", p.Network,
		"--user", p.User,
		"--workdir", "/work",
		"--log-driver", "json-file",
		"--log-opt", "max-size=1m",
		"--log-opt", "max-file=1",
	}
	if p.CapDropAll {
		args = append(args, "--cap-drop", "ALL")
	}
	if p.NoNewPrivs {
		args = append(args, "--security-opt", "no-new-privileges:true")
	}
	if p.Pids > 0 {
		args = append(args, "--pids-limit", strconv.Itoa(p.Pids))
	}
	if p.MemoryBytes > 0 {
		swap := p.MemorySwap
		if swap == 0 {
			swap = p.MemoryBytes
		}
		args = append(args, "--memory", strconv.FormatInt(p.MemoryBytes, 10))
		args = append(args, "--memory-swap", strconv.FormatInt(swap, 10))
	}
	if p.CPUSeconds > 0 {
		args = append(args, "--ulimit", fmt.Sprintf("cpu=%d:%d", p.CPUSeconds, p.CPUSeconds))
	}
	if p.Privileged {
		args = append(args, "--privileged")
	}
	for _, mount := range p.Binds {
		if mount.Source == "" || mount.Target == "" {
			return nil, fmt.Errorf("%w: empty bind", ErrInsecureInvocation)
		}
		args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s,readonly", mount.Source, mount.Target))
	}
	for _, env := range p.Env {
		args = append(args, "-e", env)
	}
	if err := rejectLiveSecrets(args); err != nil {
		return nil, err
	}
	if p.Production {
		if err := assertProductionArgs(args); err != nil {
			return nil, err
		}
	}
	args = append(args, image)
	args = append(args, command...)
	return args, nil
}

func validateCommand(command []string) error {
	if len(command) > 8 {
		return fmt.Errorf("%w: command has %d arguments", ErrInsecureInvocation, len(command))
	}
	for _, arg := range command {
		if arg == "" || len(arg) > 256 || strings.ContainsAny(arg, "\x00\n\r") {
			return fmt.Errorf("%w: command argument", ErrInsecureInvocation)
		}
	}
	return nil
}

func rejectLiveSecrets(args []string) error {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if strings.Contains(arg, "MODEL_API_KEY") ||
			strings.Contains(lower, "docker.sock") ||
			arg == "--env-file" ||
			strings.HasPrefix(arg, "--env-file=") ||
			arg == "--privileged" {
			return fmt.Errorf("%w: %q", ErrInsecureInvocation, arg)
		}
	}
	return nil
}

func assertProductionArgs(args []string) error {
	required := [][2]string{
		{"--network", "none"},
		{"--user", SandboxUser},
		{"--cap-drop", "ALL"},
		{"--security-opt", "no-new-privileges:true"},
		{"--pids-limit", strconv.Itoa(PidCeiling)},
	}
	for _, pair := range required {
		if !hasPair(args, pair[0], pair[1]) {
			return fmt.Errorf("%w: missing %s %s", ErrInsecureInvocation, pair[0], pair[1])
		}
	}
	memory := flagValue(args, "--memory")
	swap := flagValue(args, "--memory-swap")
	if memory == "" || memory != swap {
		return fmt.Errorf("%w: memory and swap must be equal and set", ErrInsecureInvocation)
	}
	ulimit := flagValue(args, "--ulimit")
	if !strings.HasPrefix(ulimit, "cpu=") || !strings.Contains(ulimit, ":") {
		return fmt.Errorf("%w: cpu ulimit", ErrInsecureInvocation)
	}
	forbidden := []string{"-e", "--env", "-v", "--volume", "--mount", "--privileged", "--env-file"}
	for _, flag := range forbidden {
		if hasToken(args, flag) || hasPrefixToken(args, flag+"=") {
			return fmt.Errorf("%w: forbidden %s", ErrInsecureInvocation, flag)
		}
	}
	return nil
}

func hasPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func hasToken(args []string, token string) bool {
	for _, arg := range args {
		if arg == token {
			return true
		}
	}
	return false
}

func hasPrefixToken(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func flagValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
