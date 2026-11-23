package sandbox

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestContainerArgsEnforceEveryControl(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Image = "sandbox-image"
	cfg.RunMemoryBytes = 128 << 20
	cfg.CompileMemoryBytes = 512 << 20
	cfg.PidsLimit = 32
	cfg.CPUs = 0.5
	controller, err := New(cfg)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}

	runArgs := controller.containerArgs(runSpec{
		phase:        phaseRun,
		name:         "leetkob-sandbox-test",
		workDir:      "/tmp/leetkob-work",
		readOnlyWork: true,
		command:      []string{harnessPath},
		memoryBytes:  cfg.RunMemoryBytes,
		cpus:         cfg.CPUs,
	})

	if runArgs[0] != "run" {
		t.Fatalf("first argument = %q, want run", runArgs[0])
	}
	checks := map[string]string{
		"--network":      "none",
		"--user":         sandboxUser,
		"--cap-drop":     "ALL",
		"--security-opt": "no-new-privileges:true",
		"--pids-limit":   strconv.Itoa(cfg.PidsLimit),
		"--cpus":         "0.5",
		"--workdir":      "/work",
		"--ulimit":       "core=0:0",
	}
	for flag, want := range checks {
		if got := flagValue(t, runArgs, flag); got != want {
			t.Fatalf("%s = %q, want %q", flag, got, want)
		}
	}
	if !hasFlag(runArgs, "--read-only") {
		t.Fatal("run arguments lack --read-only")
	}
	if got := flagValue(t, runArgs, "--memory"); got != strconv.FormatInt(cfg.RunMemoryBytes, 10) {
		t.Fatalf("--memory = %q", got)
	}
	if got, want := flagValue(t, runArgs, "--memory-swap"), flagValue(t, runArgs, "--memory"); got != want {
		t.Fatalf("--memory-swap = %q, want equal to --memory %q (no swap)", got, want)
	}
	if !hasFlag(runArgs, "--interactive") {
		t.Fatal("run arguments lack --interactive for stdin fixtures")
	}

	mounts := flagValues(runArgs, "--mount")
	if len(mounts) != 1 {
		t.Fatalf("expected exactly one bind mount, got %#v", mounts)
	}
	wantMount := "type=bind,src=/tmp/leetkob-work,dst=/work,readonly"
	if mounts[0] != wantMount {
		t.Fatalf("mount = %q, want %q", mounts[0], wantMount)
	}

	envs := flagValues(runArgs, "--env")
	if len(envs) != 2 || envs[0] != "HOME=/tmp" || envs[1] != "TMPDIR=/tmp" {
		t.Fatalf("unexpected environment: %#v", envs)
	}

	argv := strings.Join(runArgs, " ")
	for _, forbidden := range []string{
		"docker.sock",
		"--privileged",
		"--pid=host",
		"--ipc=host",
		"--network=host",
		"--volume",
		"--device",
		"--env-file",
		"/var/run",
	} {
		if strings.Contains(argv, forbidden) {
			t.Fatalf("run arguments contain forbidden %q: %s", forbidden, argv)
		}
	}

	imageIndex := indexOf(runArgs, cfg.Image)
	if imageIndex < 0 || imageIndex == len(runArgs)-1 {
		t.Fatalf("image/command boundary is missing: %v", runArgs)
	}
	if runArgs[imageIndex+1] != harnessPath {
		t.Fatalf("run command = %q, want harness %q", runArgs[imageIndex+1], harnessPath)
	}

	compileArgs := controller.containerArgs(runSpec{
		phase:        phaseCompile,
		name:         "leetkob-sandbox-test",
		workDir:      "/tmp/leetkob-work",
		readOnlyWork: false,
		command:      compileCommand(),
		memoryBytes:  cfg.CompileMemoryBytes,
		cpus:         cfg.CompileCPUs,
	})
	if hasFlag(compileArgs, "--interactive") {
		t.Fatal("compile phase should not attach stdin")
	}
	if got := flagValue(t, compileArgs, "--memory"); got != strconv.FormatInt(cfg.CompileMemoryBytes, 10) {
		t.Fatalf("compile --memory = %q", got)
	}
	if got, want := flagValue(t, compileArgs, "--memory-swap"), flagValue(t, compileArgs, "--memory"); got != want {
		t.Fatalf("compile --memory-swap = %q, want %q", got, want)
	}
	compileMount := flagValue(t, compileArgs, "--mount")
	if !strings.HasSuffix(compileMount, ",dst=/work") {
		t.Fatalf("compile mount must be writable: %q", compileMount)
	}
	compileImage := indexOf(compileArgs, cfg.Image)
	if compileImage < 0 || compileArgs[compileImage+1] != "g++" {
		t.Fatalf("compile command does not start with g++: %v", compileArgs)
	}
	compileCommandText := strings.Join(compileArgs[compileImage+1:], " ")
	for _, want := range []string{"-O2", "-std=c++17"} {
		if !strings.Contains(compileCommandText, want) {
			t.Fatalf("compile command lacks %s: %s", want, compileCommandText)
		}
	}
}

func TestNewRejectsIncompleteOrWeakenedConfig(t *testing.T) {
	base := DefaultConfig()
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "missing-image", mutate: func(c *Config) { c.Image = "" }},
		{name: "no-source-cap", mutate: func(c *Config) { c.SourceBytes = 0 }},
		{name: "no-output-cap", mutate: func(c *Config) { c.OutputBytes = 0 }},
		{name: "no-compile-wall", mutate: func(c *Config) { c.CompileWall = 0 }},
		{name: "unbounded-compile-wall", mutate: func(c *Config) { c.CompileWall = time.Hour }},
		{name: "no-run-memory", mutate: func(c *Config) { c.RunMemoryBytes = 0 }},
		{name: "no-pid-limit", mutate: func(c *Config) { c.PidsLimit = 0 }},
		{name: "no-cpu-quota", mutate: func(c *Config) { c.CPUs = 0 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.mutate(&cfg)
			if _, err := New(cfg); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestDefaultConfigNormalizes(t *testing.T) {
	controller, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("default config rejected: %v", err)
	}
	if controller.Config().Image != DefaultImage {
		t.Fatalf("image = %q", controller.Config().Image)
	}
	if controller.Config().OutputBytes != 1<<20 {
		t.Fatalf("output cap = %d", controller.Config().OutputBytes)
	}
}

func flagValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	values := flagValues(args, flag)
	if len(values) == 0 {
		t.Fatalf("flag %s is missing from %v", flag, args)
	}
	return values[0]
}

func flagValues(args []string, flag string) []string {
	var values []string
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			values = append(values, args[i+1])
		}
	}
	return values
}

func hasFlag(args []string, flag string) bool { return indexOf(args, flag) >= 0 }

func indexOf(args []string, want string) int {
	for i, arg := range args {
		if arg == want {
			return i
		}
	}
	return -1
}
