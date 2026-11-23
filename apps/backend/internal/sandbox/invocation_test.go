package sandbox

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func exampleRunArgs(t *testing.T) []string {
	t.Helper()
	policy, err := JudgeRunPolicy(1, 256)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := runProfile(policy)
	if err != nil {
		t.Fatal(err)
	}
	args, err := createArgs("leetkob-sbx-0123456789abcdef", ImageRef, profile, []string{"/work/prog"})
	if err != nil {
		t.Fatal(err)
	}
	return args
}

func TestDocumentedRunInvocation(t *testing.T) {
	args := exampleRunArgs(t)
	joined := strings.Join(args, " ")
	readme, err := os.ReadFile(sandboxDocPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), joined) {
		t.Fatalf("README is missing the controller invocation:\n%s", joined)
	}
	if err := assertProductionArgs(args); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentedCompileInvocation(t *testing.T) {
	args, err := createArgs("leetkob-sbx-0123456789abcdef", ImageRef, compileProfile(), compileCommand)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	readme, err := os.ReadFile(sandboxDocPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), joined) {
		t.Fatalf("README is missing the compile invocation:\n%s", joined)
	}
}

func TestCreateArgsDoNotCopyProcessEnvironment(t *testing.T) {
	t.Setenv("MODEL_API_KEY", "synthetic-model-key-canary")
	t.Setenv("LEETKOB_SYNTHETIC_CANARY", "synthetic-canary-value")
	joined := strings.Join(exampleRunArgs(t), "\n")
	if strings.Contains(joined, "synthetic-model-key-canary") ||
		strings.Contains(joined, "synthetic-canary-value") ||
		strings.Contains(joined, "MODEL_API_KEY") ||
		strings.Contains(joined, "LEETKOB_SYNTHETIC_CANARY") {
		t.Fatalf("invocation copied the process environment:\n%s", joined)
	}
}

func TestProductionArgsRejectADisabledControl(t *testing.T) {
	policy, err := JudgeRunPolicy(1, 256)
	if err != nil {
		t.Fatal(err)
	}
	base, err := runProfile(policy)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		edit func(*profile)
	}{
		{"network", func(p *profile) { p.Network = "bridge" }},
		{"user", func(p *profile) { p.User = "0:0" }},
		{"caps", func(p *profile) { p.CapDropAll = false }},
		{"privileges", func(p *profile) { p.NoNewPrivs = false }},
		{"pids", func(p *profile) { p.Pids = 0 }},
		{"memory", func(p *profile) { p.MemoryBytes = 0 }},
		{"swap", func(p *profile) { p.MemorySwap = p.MemoryBytes * 2 }},
		{"cpu", func(p *profile) { p.CPUSeconds = 0 }},
		{"bind", func(p *profile) { p.Binds = []bind{{Source: "/tmp/canary", Target: "/host-canary"}} }},
		{"env", func(p *profile) { p.Env = []string{"LEETKOB_SYNTHETIC_CANARY=synthetic"} }},
		{"privileged", func(p *profile) { p.Privileged = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := base
			tc.edit(&profile)
			_, err := createArgs("leetkob-sbx-0123456789abcdef", ImageRef, profile, []string{"/work/prog"})
			if !errors.Is(err, ErrInsecureInvocation) {
				t.Fatalf("err = %v, want ErrInsecureInvocation", err)
			}
		})
	}
}

func TestLiveCredentialCannotBePassedToANegativeControl(t *testing.T) {
	policy, err := JudgeRunPolicy(1, 64)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := runProfile(policy)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = profile.relaxForNegativeControl("secret oracle must still refuse a live key name")
	if err != nil {
		t.Fatal(err)
	}
	profile.Env = []string{"MODEL_API_KEY=synthetic-model-key-canary"}
	if _, err := createArgs("leetkob-sbx-0123456789abcdef", ImageRef, profile, []string{"/work/prog"}); !errors.Is(err, ErrInsecureInvocation) {
		t.Fatalf("err = %v", err)
	}
	profile.Env = []string{"LEETKOB_SYNTHETIC_CANARY=synthetic-canary-value"}
	args, err := createArgs("leetkob-sbx-0123456789abcdef", ImageRef, profile, []string{"/work/prog"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasPair(args, "-e", "LEETKOB_SYNTHETIC_CANARY=synthetic-canary-value") {
		t.Fatalf("negative control lost the synthetic canary: %s", strings.Join(args, " "))
	}
}

func TestDockerfilePinsTheBaseImageAndUnprivilegedUser(t *testing.T) {
	body, err := os.ReadFile(dockerfilePath(t))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"debian:bookworm-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc",
		"USER 10001:10001",
		"g++",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Dockerfile missing %s", want)
		}
	}
	lower := strings.ToLower(text)
	for _, forbidden := range []string{"model_api_key", "docker.sock", "--privileged", "env-file"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("Dockerfile contains %s", forbidden)
		}
	}
}

func TestAttackFixturesExistForEachControl(t *testing.T) {
	for _, name := range []string{
		"network.cpp",
		"cpu.cpp",
		"wall.cpp",
		"memory.cpp",
		"processes.cpp",
		"output.cpp",
		"nonroot.cpp",
		"mounts.cpp",
		"secrets.cpp",
	} {
		body, err := os.ReadFile(attackPath(t, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(strings.TrimSpace(string(body))) == 0 {
			t.Fatalf("%s is empty", name)
		}
		if strings.Contains(string(body), "MODEL_API_KEY=") {
			t.Fatalf("%s contains a key assignment", name)
		}
	}
}
