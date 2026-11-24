package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func dockerAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not available: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	docker, err := CommandPath("docker")
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	if err := exec.CommandContext(ctx, docker, "info").Run(); err != nil {
		t.Skipf("docker daemon not available: %v", err)
	}
}

var (
	imageOnce sync.Once
	imageInfo ImageProvenance
	imageErr  error
)

func qualifiedImage(t *testing.T) (*Controller, ImageProvenance) {
	t.Helper()
	dockerAvailable(t)
	imageOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		imageInfo, imageErr = New().Build(ctx, filepathDir(t, "cpp17"))
	})
	if imageErr != nil {
		t.Fatalf("build image: %v", imageErr)
	}
	return New(), imageInfo
}

func filepathDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(sandboxRoot(t), name)
}

func TestImageProvenanceAndIsolatedHello(t *testing.T) {
	controller, info := qualifiedImage(t)
	if info.ImageID == "" || !strings.Contains(info.Compiler, "g++") || !strings.Contains(info.Packages, "g++") {
		t.Fatalf("provenance = %+v", info)
	}
	t.Logf("image %s", info.ImageID)
	t.Logf("compiler %s", strings.TrimSpace(info.Compiler))
	t.Logf("packages %s", strings.TrimSpace(info.Packages))
	if err := assertProductionArgs(info.Invocation); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	compiled, err := controller.Compile(ctx, []byte("#include <iostream>\nint main(){std::cout<<\"hello_ok\\n\";}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !compiled.OK {
		failOutput(t, "compile hello", compiled.Observation)
	}
	requireLocked(t, compiled.Observation, "")
	policy, err := JudgeRunPolicy(1, 256)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := controller.Execute(ctx, compiled.Binary, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	requireLocked(t, observed, "")
	if observed.ExitCode != 0 || !bytes.Contains(observed.Stdout, []byte("hello_ok\n")) {
		failOutput(t, "run hello", observed)
	}
}

func TestNetworkIsolation(t *testing.T) {
	controller, _ := qualifiedImage(t)
	binary := compileAttack(t, controller, "network.cpp")
	address, networkName := startListener(t, controller)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	openPolicy := RunPolicy{CPUSeconds: 1, MemoryBytes: 256 << 20, Wall: 10 * time.Second, OutputLimit: DefaultOutputBytes}
	openProfile, err := runProfile(openPolicy)
	if err != nil {
		t.Fatal(err)
	}
	openProfile, err = openProfile.relaxForNegativeControl("show the fixture is reachable when network isolation is off")
	if err != nil {
		t.Fatal(err)
	}
	openProfile.Network = networkName
	open, err := controller.executeWith(ctx, binary, openProfile, openPolicy.Wall, openPolicy.OutputLimit, []string{address, "8080"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(open.Stdout, []byte("network_open")) {
		failOutput(t, "network negative control", open)
	}

	closed, err := controller.Execute(ctx, binary, openPolicy, []string{address, "8080"})
	if err != nil {
		t.Fatal(err)
	}
	requireLocked(t, closed, "")
	if bytes.Contains(closed.Stdout, []byte("network_open")) || !bytes.Contains(closed.Stdout, []byte("network_denied")) {
		failOutput(t, "network isolation", closed)
	}
	stillOpen, err := controller.executeWith(ctx, binary, openProfile, openPolicy.Wall, openPolicy.OutputLimit, []string{address, "8080"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stillOpen.Stdout, []byte("network_open")) {
		failOutput(t, "network fixture died before the isolated run", stillOpen)
	}
}

func TestCPULimit(t *testing.T) {
	controller, _ := qualifiedImage(t)
	binary := compileAttack(t, controller, "cpu.cpp")
	policy := RunPolicy{CPUSeconds: 1, MemoryBytes: 256 << 20, Wall: 12 * time.Second, OutputLimit: DefaultOutputBytes}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	started := time.Now()
	observed, err := controller.Execute(ctx, binary, policy, nil)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	requireLocked(t, observed, "")
	if observed.CPUSeconds != 1 || observed.WallTimedOut || observed.OOMKilled {
		failOutput(t, "cpu observation", observed)
	}
	if bytes.Contains(observed.Stdout, []byte("CPU_LIMIT_DISABLED")) || elapsed > 6*time.Second {
		t.Fatalf("cpu limit disabled after %s", elapsed)
	}

	profile, err := runProfile(policy)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = profile.relaxForNegativeControl("show the compute loop finishes when the cpu ulimit is omitted")
	if err != nil {
		t.Fatal(err)
	}
	profile.CPUSeconds = 0
	negative, err := controller.executeWith(ctx, binary, profile, policy.Wall, policy.OutputLimit, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(negative.Stdout, []byte("CPU_LIMIT_DISABLED")) {
		failOutput(t, "cpu negative control", negative)
	}
}

func TestWallDeadlineRemovesTheContainer(t *testing.T) {
	controller, _ := qualifiedImage(t)
	binary := compileAttack(t, controller, "wall.cpp")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	policy := RunPolicy{CPUSeconds: 1, MemoryBytes: 256 << 20, Wall: 3 * time.Second, OutputLimit: DefaultOutputBytes}
	observed, err := controller.Execute(ctx, binary, policy, []string{"30"})
	if err != nil {
		t.Fatal(err)
	}
	requireLocked(t, observed, "")
	if !observed.WallTimedOut || !observed.ContainerRemoved || !controller.gone(observed.Name) {
		failOutput(t, "wall deadline", observed)
	}
	if bytes.Contains(observed.Stdout, []byte("WALL_FINISHED")) {
		t.Fatal("sleep finished inside the wall deadline")
	}

	finished := RunPolicy{CPUSeconds: 1, MemoryBytes: 256 << 20, Wall: 10 * time.Second, OutputLimit: DefaultOutputBytes}
	negative, err := controller.Execute(ctx, binary, finished, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if negative.WallTimedOut || !bytes.Contains(negative.Stdout, []byte("WALL_FINISHED")) {
		failOutput(t, "wall negative control", negative)
	}
}

func TestMemoryLimitIncludesSwapAndReadback(t *testing.T) {
	controller, _ := qualifiedImage(t)
	binary := compileAttack(t, controller, "memory.cpp")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	limited := RunPolicy{CPUSeconds: 5, MemoryBytes: 64 << 20, Wall: 15 * time.Second, OutputLimit: DefaultOutputBytes}
	observed, err := controller.Execute(ctx, binary, limited, nil)
	if err != nil {
		t.Fatal(err)
	}
	requireLocked(t, observed, "")
	if observed.MemoryBytes != 64<<20 || observed.MemorySwapBytes < 0 ||
		(observed.MemorySwapBytes != 0 && observed.MemorySwapBytes != observed.MemoryBytes) {
		t.Fatalf("memory=%d swap=%d", observed.MemoryBytes, observed.MemorySwapBytes)
	}
	if !bytes.Contains(observed.Stdout, []byte("/sys/fs/cgroup/memory.max=67108864\n")) {
		failOutput(t, "memory.max", observed)
	}
	if !bytes.Contains(observed.Stdout, []byte("memory.swap.max=0\n")) &&
		!bytes.Contains(observed.Stdout, []byte("memory.swap.max=67108864\n")) {
		failOutput(t, "memory.swap.max", observed)
	}
	if bytes.Contains(observed.Stdout, []byte("readback_ok")) {
		t.Fatal("allocation read-back succeeded past the memory ceiling")
	}
	// Memory containment is proven when the cgroup ceiling is verified and
	// the allocation never reads back. The cgroup-v2 OOM flag is not
	// reliably reported by Docker on every runner, so a SIGKILL exit (137)
	// after the cgroup checks is the documented memory-RE fallback.
	if !observed.OOMKilled && observed.ExitCode != 137 &&
		!bytes.Contains(observed.Stdout, []byte("alloc_failed")) {
		failOutput(t, "memory containment", observed)
	}
	t.Logf("memory oom=%v exit=%d", observed.OOMKilled, observed.ExitCode)

	roomy := RunPolicy{CPUSeconds: 5, MemoryBytes: 512 << 20, Wall: 15 * time.Second, OutputLimit: DefaultOutputBytes}
	negative, err := controller.Execute(ctx, binary, roomy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if negative.OOMKilled || !bytes.Contains(negative.Stdout, []byte("readback_ok")) {
		failOutput(t, "memory negative control", negative)
	}
}

func TestProcessCeilingDoesNotAffectAnotherRun(t *testing.T) {
	controller, _ := qualifiedImage(t)
	flood := compileAttack(t, controller, "processes.cpp")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	helloCompiled, err := controller.Compile(ctx, []byte("#include <iostream>\nint main(){std::cout<<\"hello_ok\\n\";}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !helloCompiled.OK {
		failOutput(t, "compile hello", helloCompiled.Observation)
	}
	policy, err := JudgeRunPolicy(1, 256)
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		obs Observation
		err error
	}
	helloCh := make(chan result, 1)
	go func() {
		obs, err := controller.Execute(ctx, helloCompiled.Binary, policy, nil)
		helloCh <- result{obs, err}
	}()
	observed, err := controller.Execute(ctx, flood, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	hello := <-helloCh
	if hello.err != nil {
		t.Fatal(hello.err)
	}
	requireLocked(t, observed, "")
	requireLocked(t, hello.obs, "")
	created := createdCount(t, observed.Stdout)
	if created < 1 || created > PidCeiling-1 {
		failOutput(t, "process ceiling", observed)
	}
	if !bytes.Contains(hello.obs.Stdout, []byte("hello_ok\n")) {
		failOutput(t, "concurrent hello", hello.obs)
	}

	profile, err := runProfile(policy)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = profile.relaxForNegativeControl("show the thread probe reaches 64 when the pid ceiling is removed")
	if err != nil {
		t.Fatal(err)
	}
	profile.Pids = 256
	negative, err := controller.executeWith(ctx, flood, profile, policy.Wall, policy.OutputLimit, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if createdCount(t, negative.Stdout) != 64 {
		failOutput(t, "process negative control", negative)
	}
}

func TestOutputCollectionStaysBounded(t *testing.T) {
	controller, _ := qualifiedImage(t)
	binary := compileAttack(t, controller, "output.cpp")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	capped := RunPolicy{CPUSeconds: 1, MemoryBytes: 256 << 20, Wall: 10 * time.Second, OutputLimit: 2048}
	observed, err := controller.Execute(ctx, binary, capped, nil)
	if err != nil {
		t.Fatal(err)
	}
	requireLocked(t, observed, "")
	if !observed.OutputLimited || observed.WallTimedOut || len(observed.Stdout)+len(observed.Stderr) > capped.OutputLimit {
		failOutput(t, "output cap", observed)
	}
	if bytes.Contains(observed.Stdout, []byte("OUTPUT_DONE")) {
		t.Fatal("output marker crossed the collection cap")
	}

	open := RunPolicy{CPUSeconds: 1, MemoryBytes: 256 << 20, Wall: 10 * time.Second, OutputLimit: 64 << 10}
	negative, err := controller.Execute(ctx, binary, open, nil)
	if err != nil {
		t.Fatal(err)
	}
	if negative.OutputLimited || !bytes.Contains(negative.Stdout, []byte("OUTPUT_DONE")) {
		failOutput(t, "output negative control", negative)
	}
}

func TestNonRootIdentity(t *testing.T) {
	controller, _ := qualifiedImage(t)
	binary := compileAttack(t, controller, "nonroot.cpp")
	policy, err := JudgeRunPolicy(1, 256)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	observed, err := controller.Execute(ctx, binary, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	requireLocked(t, observed, "")
	if !bytes.Contains(observed.Stdout, []byte("euid=10001\n")) || !bytes.Contains(observed.Stdout, []byte("priv_esc=0\n")) {
		failOutput(t, "non-root", observed)
	}

	profile, err := runProfile(policy)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = profile.relaxForNegativeControl("show the identity probe reports root when --user is 0")
	if err != nil {
		t.Fatal(err)
	}
	profile.User = "0:0"
	negative, err := controller.executeWith(ctx, binary, profile, policy.Wall, policy.OutputLimit, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(negative.Stdout, []byte("euid=0\n")) {
		failOutput(t, "non-root negative control", negative)
	}
}

func TestNoHostMountOrDockerSocket(t *testing.T) {
	controller, _ := qualifiedImage(t)
	canary, dir := writeCanary(t)
	binary := compileAttack(t, controller, "mounts.cpp")
	policy, err := JudgeRunPolicy(1, 256)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	observed, err := controller.Execute(ctx, binary, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	requireLocked(t, observed, canary)
	if !bytes.Contains(observed.Stdout, []byte("file_absent\n")) ||
		!bytes.Contains(observed.Stdout, []byte("socket_open=0\n")) ||
		!bytes.Contains(observed.Stdout, []byte("docker_cli=0\n")) ||
		bytes.Contains(observed.Stdout, []byte(canary)) {
		failOutput(t, "mount isolation", observed)
	}
	for _, destination := range observed.MountDestinations {
		if destination == "/var/run/docker.sock" {
			t.Fatalf("socket mounted: %v", observed.MountDestinations)
		}
	}
	for _, source := range observed.MountSources {
		if strings.Contains(source, dir) {
			t.Fatalf("host canary dir mounted: %s", source)
		}
	}

	profile, err := runProfile(policy)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = profile.relaxForNegativeControl("show a host bind makes the synthetic canary readable")
	if err != nil {
		t.Fatal(err)
	}
	profile.Binds = []bind{{Source: dir, Target: "/host-canary"}}
	negative, err := controller.executeWith(ctx, binary, profile, policy.Wall, policy.OutputLimit, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(negative.Stdout, []byte("file="+canary)) || !bytes.Contains(negative.Stdout, []byte("socket_open=0\n")) {
		failOutput(t, "mount negative control", negative)
	}
}

func TestSecretsStayOutOfEnvFilesAndIncludes(t *testing.T) {
	controller, _ := qualifiedImage(t)
	canary, dir := writeCanary(t)
	t.Setenv("MODEL_API_KEY", canary)
	t.Setenv("LEETKOB_SYNTHETIC_CANARY", canary)
	source := readAttack(t, "secrets.cpp")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	compiled := compileSecretsAttack(t, controller, source, canary)
	policy := mustJudgePolicy(t)
	observed := runSecretsAttack(t, controller, compiled.Binary, policy, canary)
	assertEnvCanaryNegative(t, controller, compiled.Binary, policy, observed, canary, ctx)
	assertIncludeCanaryNegative(t, controller, source, policy, dir, ctx)
}

// compileSecretsAttack compiles the secret probe and asserts the compiler
// boundary exposes no canary.
func compileSecretsAttack(t *testing.T, controller *Controller, source []byte, canary string) CompileResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	compiled, err := controller.Compile(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if !compiled.OK {
		failOutput(t, "compile secrets", compiled.Observation)
	}
	requireLocked(t, compiled.Observation, canary)
	if bytes.Contains(compiled.Stdout, []byte(canary)) || bytes.Contains(compiled.Stderr, []byte(canary)) {
		t.Fatal("compiler output contained the synthetic canary; details suppressed")
	}
	return compiled
}

func mustJudgePolicy(t *testing.T) RunPolicy {
	t.Helper()
	policy, err := JudgeRunPolicy(1, 256)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

// runSecretsAttack executes the probe and asserts the environment, files,
// includes and output stay canary-free.
func runSecretsAttack(t *testing.T, controller *Controller, binary []byte, policy RunPolicy, canary string) Observation {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	observed, err := controller.Execute(ctx, binary, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	requireLocked(t, observed, canary)
	if !bytes.Contains(observed.Stdout, []byte("include_hit=0\n")) ||
		!bytes.Contains(observed.Stdout, []byte("env_clean\n")) ||
		!bytes.Contains(observed.Stdout, []byte("file_absent\n")) ||
		bytes.Contains(observed.Stdout, []byte(canary)) ||
		bytes.Contains(observed.Stderr, []byte(canary)) {
		failOutput(t, "secret isolation", observed)
	}
	return observed
}

// assertEnvCanaryNegative proves the probe would see a canary deliberately
// passed with -e, so the clean run is meaningful.
func assertEnvCanaryNegative(t *testing.T, controller *Controller, binary []byte, policy RunPolicy, observed Observation, canary string, ctx context.Context) {
	t.Helper()
	envProfile, err := runProfile(policy)
	if err != nil {
		t.Fatal(err)
	}
	envProfile, err = envProfile.relaxForNegativeControl("show the runtime probe prints a synthetic canary passed with -e")
	if err != nil {
		t.Fatal(err)
	}
	envProfile.Env = []string{"LEETKOB_SYNTHETIC_CANARY=" + canary}
	envHit, err := controller.executeWith(ctx, binary, envProfile, policy.Wall, policy.OutputLimit, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(envHit.Stdout, []byte("env LEETKOB_SYNTHETIC_CANARY="+canary)) {
		failOutput(t, "secret env negative control", envHit)
	}
}

// assertIncludeCanaryNegative proves __has_include sees a synthetic canary
// when a host directory is deliberately mounted.
func assertIncludeCanaryNegative(t *testing.T, controller *Controller, source []byte, policy RunPolicy, dir string, ctx context.Context) {
	t.Helper()
	includeProfile := compileProfile()
	includeProfile, err := includeProfile.relaxForNegativeControl("show __has_include sees a synthetic canary when the host directory is mounted")
	if err != nil {
		t.Fatal(err)
	}
	includeProfile.Binds = []bind{{Source: dir, Target: "/host-canary"}}
	included, err := controller.compileWith(ctx, source, includeProfile)
	if err != nil {
		t.Fatal(err)
	}
	if !included.OK {
		failOutput(t, "include negative compile", included.Observation)
	}
	includeRun, err := controller.Execute(ctx, included.Binary, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(includeRun.Stdout, []byte("include_hit=1\n")) {
		failOutput(t, "include negative control", includeRun)
	}
}

func compileAttack(t *testing.T, controller *Controller, name string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	compiled, err := controller.Compile(ctx, readAttack(t, name))
	if err != nil {
		t.Fatal(err)
	}
	if !compiled.OK {
		failOutput(t, "compile "+name, compiled.Observation)
	}
	requireLocked(t, compiled.Observation, "")
	return compiled.Binary
}

func readAttack(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(attackPath(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeCanary(t *testing.T) (string, string) {
	t.Helper()
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatal(err)
	}
	canary := "synthetic-" + hex.EncodeToString(buf[:])
	dir := t.TempDir()
	if strings.Contains(dir, ",") {
		t.Fatalf("temp dir %s cannot be used as a docker mount source", dir)
	}
	if err := os.WriteFile(dir+"/canary.txt", []byte(canary+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return canary, dir
}

func createdCount(t *testing.T, stdout []byte) int {
	t.Helper()
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("created=")) {
			value, err := strconv.Atoi(string(bytes.TrimPrefix(line, []byte("created="))))
			if err != nil {
				t.Fatal(err)
			}
			return value
		}
	}
	t.Fatalf("missing created= line in %q", stdout)
	return 0
}

func requireLocked(t *testing.T, observed Observation, canary string) {
	t.Helper()
	if !observed.ContainerRemoved || !controllerGone(t, observed.Name) {
		t.Fatalf("container %s was not removed", observed.Name)
	}
	joined := strings.Join(observed.Invocation, " ")
	if err := assertProductionArgs(observed.Invocation); err != nil {
		t.Fatalf("invocation: %v\n%s", err, joined)
	}
	for _, flag := range []string{"--privileged", "--env-file", "--volume", "--mount", "MODEL_API_KEY", "docker.sock"} {
		if strings.Contains(joined, flag) {
			t.Fatalf("invocation contains %s", flag)
		}
	}
	if canary != "" && strings.Contains(joined, canary) {
		t.Fatal("invocation contains the synthetic canary")
	}
	if observed.NetworkMode != "none" {
		t.Fatalf("network = %s", observed.NetworkMode)
	}
	if observed.User != SandboxUser {
		t.Fatalf("user = %s", observed.User)
	}
	if observed.Privileged || len(observed.Binds) != 0 {
		t.Fatalf("privileged=%v binds=%v", observed.Privileged, observed.Binds)
	}
	// cgroup v2 reports a disabled swap account as 0. An equal --memory-swap
	// value is the other legal report. A negative value means unlimited swap.
	if observed.MemoryBytes == 0 || observed.MemorySwapBytes < 0 ||
		(observed.MemorySwapBytes != 0 && observed.MemorySwapBytes != observed.MemoryBytes) {
		t.Fatalf("memory=%d swap=%d", observed.MemoryBytes, observed.MemorySwapBytes)
	}
	if observed.PidsLimit != PidCeiling {
		t.Fatalf("pids = %d", observed.PidsLimit)
	}
	droppedAll := false
	for _, dropped := range observed.CapDrop {
		if strings.EqualFold(dropped, "ALL") {
			droppedAll = true
		}
	}
	if !droppedAll {
		t.Fatalf("cap drop = %v", observed.CapDrop)
	}
	for _, entry := range observed.Env {
		if strings.Contains(entry, "MODEL_API_KEY") || (canary != "" && strings.Contains(entry, canary)) {
			t.Fatal("container environment contains a canary or model key name")
		}
	}
	for _, destination := range observed.MountDestinations {
		if strings.Contains(destination, "docker.sock") {
			t.Fatalf("socket destination %s", destination)
		}
	}
}

func controllerGone(t *testing.T, name string) bool {
	t.Helper()
	if name == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	docker, err := CommandPath("docker")
	if err != nil {
		return true
	}
	return exec.CommandContext(ctx, docker, "inspect", name).Run() != nil
}

func failOutput(t *testing.T, label string, observed Observation) {
	t.Helper()
	blob := append(append([]byte{}, observed.Stdout...), observed.Stderr...)
	if bytes.Contains(blob, []byte("MODEL_API_KEY")) {
		t.Fatalf("%s: MODEL_API_KEY leaked; output suppressed", label)
	}
	t.Fatalf("%s: exit=%d oom=%v wall=%v limited=%v removed=%v stdout=%q stderr=%q",
		label, observed.ExitCode, observed.OOMKilled, observed.WallTimedOut, observed.OutputLimited, observed.ContainerRemoved, observed.Stdout, observed.Stderr)
}

func startListener(t *testing.T, controller *Controller) (string, string) {
	t.Helper()
	binary := compileListener(t, controller)
	networkName := createListenerNetwork(t, controller)
	return startListenerContainer(t, controller, binary, networkName)
}

// compileListener builds the fixture server that listens on a fixed port
// inside a deliberately test-only network.
func compileListener(t *testing.T, controller *Controller) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	compiled, err := controller.Compile(ctx, listenerSource)
	if err != nil || !compiled.OK {
		t.Fatalf("compile listener: %v", err)
	}
	return compiled.Binary
}

// createListenerNetwork creates the disposable test network and registers
// its removal.
func createListenerNetwork(t *testing.T, controller *Controller) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	networkName := "leetkob-fix-" + hex.EncodeToString(suffix[:])
	if _, _, err := controller.exec(ctx, "network", "create", networkName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		_, _, _ = controller.exec(clean, "network", "rm", networkName)
	})
	return networkName
}

// startListenerContainer starts the fixture and resolves its address inside
// the test network.
func startListenerContainer(t *testing.T, controller *Controller, binary []byte, networkName string) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	name, err := newName()
	if err != nil {
		t.Fatal(err)
	}
	policy := RunPolicy{CPUSeconds: 1, MemoryBytes: 128 << 20, Wall: 30 * time.Second, OutputLimit: DefaultOutputBytes}
	profile, err := runProfile(policy)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = profile.relaxForNegativeControl("fixture listener joins only the test network")
	if err != nil {
		t.Fatal(err)
	}
	profile.Network = networkName
	args, err := createArgs(name, controller.imageRef(), profile, []string{"/work/prog"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.exec(ctx, args...); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		_, _, _ = controller.exec(clean, "rm", "-f", name)
	})
	if err := controller.copyFiles(ctx, name, map[string][]byte{"prog": binary}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.exec(ctx, "start", name); err != nil {
		t.Fatal(err)
	}
	address := resolveListenerAddress(t, controller, name, networkName)
	return address, networkName
}

func resolveListenerAddress(t *testing.T, controller *Controller, name, networkName string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var address string
	for attempt := 0; attempt < 20 && address == ""; attempt++ {
		raw, _, err := controller.exec(ctx, "inspect", name)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := parseInspect(raw)
		if err != nil {
			t.Fatal(err)
		}
		if network, ok := doc.NetworkSettings.Networks[networkName]; ok {
			address = network.IPAddress
		}
		if address == "" {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if address == "" {
		t.Fatal("fixture listener has no IP address")
	}
	return address
}

var listenerSource = []byte(`#include <arpa/inet.h>
#include <iostream>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>
int main() {
  int fd = socket(AF_INET, SOCK_STREAM, 0);
  int opt = 1;
  setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof opt);
  sockaddr_in addr{};
  addr.sin_family = AF_INET;
  addr.sin_port = htons(8080);
  addr.sin_addr.s_addr = htonl(INADDR_ANY);
  if (bind(fd, reinterpret_cast<sockaddr*>(&addr), sizeof addr) != 0) return 1;
  if (listen(fd, 1) != 0) return 1;
  std::cout << "listening\n" << std::flush;
  for (;;) {
    int client = accept(fd, nullptr, nullptr);
    if (client < 0) return 1;
    close(client);
  }
}
`)

func TestParseInspectRejectsTheWrongShape(t *testing.T) {
	if _, err := parseInspect([]byte(`{}`)); err == nil {
		t.Fatal("expected an array")
	}
	raw, err := json.Marshal([]map[string]any{{"State": map[string]any{"ExitCode": 0}}})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parseInspect(raw)
	if err != nil || doc.State.ExitCode != 0 {
		t.Fatalf("doc=%+v err=%v", doc, err)
	}
}

func TestExecuteRejectsAnEmptyProgram(t *testing.T) {
	policy, err := JudgeRunPolicy(1, 64)
	if err != nil {
		t.Fatal(err)
	}
	_, err = New().Execute(context.Background(), nil, policy, nil)
	if !errors.Is(err, ErrPolicy) {
		t.Fatalf("err = %v", err)
	}
}
