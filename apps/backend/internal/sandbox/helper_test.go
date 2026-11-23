package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	// dockerSkipReason is non-empty only when Docker or its daemon is not
	// available. Tests then skip with this message.
	dockerSkipReason string
	// imageBuildErr reports a failure to provide the image while Docker is
	// available; that is a real failure, not a skip.
	imageBuildErr error
	// sandboxImage is the image under test, overridable for a locally
	// pinned digest run.
	sandboxImage = DefaultImage
)

const (
	attacksRelativeDir = "../../sandbox/attacks"
	sandboxRelativeDir = "../../sandbox"
)

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("docker"); err != nil {
		dockerSkipReason = "docker CLI unavailable: " + err.Error()
		os.Exit(m.Run())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	info, infoErr := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput()
	cancel()
	if infoErr != nil {
		dockerSkipReason = fmt.Sprintf("docker daemon unavailable: %v (%s)", infoErr, strings.TrimSpace(string(info)))
		os.Exit(m.Run())
	}

	if override := os.Getenv("TEST_SANDBOX_IMAGE"); override != "" {
		sandboxImage = override
		if exec.Command("docker", "image", "inspect", sandboxImage).Run() != nil {
			imageBuildErr = fmt.Errorf("TEST_SANDBOX_IMAGE %q is not present locally", sandboxImage)
		}
	} else {
		ensureImage()
	}
	os.Exit(m.Run())
}

// ensureImage builds the fixed image if it is not already present.
// Building may use the network for base image/apt packages; the tests
// themselves never make external network calls.
func ensureImage() {
	if exec.Command("docker", "image", "inspect", sandboxImage).Run() == nil {
		return
	}

	dir, err := filepath.Abs(sandboxRelativeDir)
	if err != nil {
		imageBuildErr = err
		return
	}
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer buildCancel()
	build := exec.CommandContext(buildCtx, "docker", "build", "--tag", sandboxImage, dir)
	build.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	if out, err := build.CombinedOutput(); err != nil {
		imageBuildErr = fmt.Errorf("%w: %s", err, tail(string(out), 4096))
	}
}

// requireDocker skips only when Docker itself is unavailable. A Docker
// installation that cannot provide the fixed image is a test failure.
func requireDocker(t *testing.T) {
	t.Helper()
	if dockerSkipReason != "" {
		t.Skip(dockerSkipReason)
	}
	if imageBuildErr != nil {
		t.Fatalf("fixed sandbox image unavailable: %v", imageBuildErr)
	}
}

// testController returns a controller with test-sized limits: 64 MiB of
// memory (including swap), 64 PIDs and one CPU, so attacks are contained
// quickly. The source/output caps and 10-second compile wall stay at their
// production defaults.
func testController(t *testing.T) *Controller {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Image = sandboxImage
	cfg.RunMemoryBytes = 64 << 20
	cfg.CompileMemoryBytes = 512 << 20
	cfg.PidsLimit = 64
	cfg.CPUs = 1
	controller, err := New(cfg)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	return controller
}

func attackSource(t *testing.T, name string) []byte {
	t.Helper()
	path, err := filepath.Abs(filepath.Join(attacksRelativeDir, name))
	if err != nil {
		t.Fatalf("resolve fixture %s: %v", name, err)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return source
}

// judge runs the controller with an overall guard so a wedged daemon fails
// the test instead of hanging the suite.
func judge(t *testing.T, controller *Controller, req JudgeRequest) JudgeResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := controller.Judge(ctx, req)
	if err != nil {
		if IsInfrastructure(err) {
			t.Fatalf("sandbox infrastructure error: %v", err)
		}
		t.Fatalf("judge failed: %v", err)
	}
	return result
}

func singleTestResult(t *testing.T, result JudgeResult) TestResult {
	t.Helper()
	if len(result.Tests) != 1 {
		t.Fatalf("expected 1 test result, got %d", len(result.Tests))
	}
	return result.Tests[0]
}

// assertContainerGone independently proves the disposable container was
// removed; a wall-case test must show the container is gone, not merely
// that its client process exited.
func assertContainerGone(t *testing.T, containerID string) {
	t.Helper()
	if containerID == "" {
		t.Fatal("result did not record a container ID")
	}
	out, err := exec.Command("docker", "inspect", "--format", "{{.Id}}", containerID).CombinedOutput()
	if err == nil {
		t.Fatalf("container %s still exists after the run: %s", containerID, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(strings.ToLower(string(out)), "no such") {
		t.Logf("docker inspect %s after cleanup: %s", containerID, strings.TrimSpace(string(out)))
	}
}

func assertContains(t *testing.T, output string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\n--- output ---\n%s", want, output)
		}
	}
}

func assertNotContains(t *testing.T, output string, unwanted ...string) {
	t.Helper()
	for _, deny := range unwanted {
		if strings.Contains(output, deny) {
			t.Fatalf("output unexpectedly contains %q\n--- output ---\n%s", deny, output)
		}
	}
}

// prelude prepends compile-time definitions to an attack fixture.
func prelude(source []byte, defines ...string) []byte {
	var builder strings.Builder
	for _, define := range defines {
		builder.WriteString(define)
		builder.WriteString("\n")
	}
	builder.Write(source)
	return []byte(builder.String())
}
