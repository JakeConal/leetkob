package checker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	checkerImageEnv = "CHECKER_IMAGE"
	defaultImage    = "leetkob-checker:pa3"
)

func dockerDaemonAvailable() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return err
	}
	out, err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("docker daemon returned no version")
	}
	return nil
}

func requireCheckerImage(t *testing.T, image string) {
	t.Helper()
	if err := dockerDaemonAvailable(); err != nil {
		t.Skipf("docker unavailable, skipping checker image test: %v", err)
	}
	if out, err := exec.Command("docker", "image", "inspect", image).CombinedOutput(); err != nil {
		t.Fatalf("checker image %q is not built (%v):\n%s\nBuild it with:\n  docker build -f apps/backend/analyzer/Dockerfile -t leetkob-analyzer:pa3 apps/backend/analyzer\n  docker build -f apps/backend/checker/Dockerfile -t %s apps/backend", image, err, out, image)
	}
}

func runCheckerImage(t *testing.T, image, dataDir string) (string, string, error) {
	t.Helper()
	outputDir := t.TempDir()
	if err := os.Chmod(outputDir, 0o777); err != nil {
		t.Fatal(err)
	}
	absoluteData, err := filepath.Abs(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("leetkob-checker-test-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	args := []string{
		"run", "--rm",
		"--name", name,
		"--network", "none",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "64",
		"--memory", "512m",
		"--memory-swap", "512m",
		"--cpus", "1",
		"--user", "65534:65534",
		"-e", "MODEL_OFF=1",
		"-v", absoluteData + ":/data:ro",
		"-v", outputDir + ":/out",
		image,
		"--data", "/data",
		"--predictions", "/out/predictions.jsonl",
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		_ = exec.Command("docker", "rm", "--force", name).Run()
		return stdout.String(), stderr.String(), ctx.Err()
	}
	if err == nil {
		predictions, readErr := os.ReadFile(filepath.Join(outputDir, "predictions.jsonl"))
		if readErr != nil {
			t.Fatalf("checker image wrote no predictions: %v", readErr)
		}
		stdout.Write(predictions)
	}
	return stdout.String(), stderr.String(), err
}

// TestCheckerImageModelOffRunsWithoutNetworkOrDockerSocket is the PA3-EVAL-02
// marker-shaped check: the built image runs with --network none, memory/PID
// limits, no keys and no Docker socket, then writes valid predictions.
func TestCheckerImageModelOffRunsWithoutNetworkOrDockerSocket(t *testing.T) {
	image := os.Getenv(checkerImageEnv)
	if image == "" {
		image = defaultImage
	}
	requireCheckerImage(t, image)

	stdout, stderr, err := runCheckerImage(t, image, filepath.Join("testdata", "banlist-traps"))
	if err != nil {
		t.Fatalf("checker image run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, `"modelCalls":0`) {
		t.Fatalf("image run did not report zero model calls:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"modelOff":true`) {
		t.Fatalf("image run did not select MODEL_OFF:\n%s", stdout)
	}

	// runCheckerImage appends the prediction file to stdout after the log
	// lines. Split the JSONL record prefix back out.
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	var predictionLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, `{"solution":`) {
			predictionLines = append(predictionLines, line)
		}
	}
	if len(predictionLines) != len(strings.Split(strings.TrimSuffix(mustReadFile(t, filepath.Join("testdata", "banlist-traps", "expected.jsonl")), "\n"), "\n")) {
		t.Fatalf("prediction line count = %d, want the reviewed fixture count", len(predictionLines))
	}
	schema := loadPredictionSchema(t)
	actual := make(map[string]Prediction, len(predictionLines))
	for _, line := range predictionLines {
		prediction := validatePredictionLine(t, []byte(line), schema)
		actual[prediction.Solution+"\x00"+prediction.Constraint] = prediction
	}
	expected := byPair(readPredictions(t, filepath.Join("testdata", "banlist-traps", "expected.jsonl")))
	if len(actual) != len(expected) {
		t.Fatalf("predicted %d pairs, want %d", len(actual), len(expected))
	}
	violations := 0
	for key, want := range expected {
		got, ok := actual[key]
		if !ok {
			t.Fatalf("missing prediction for %s", key)
		}
		if got.Verdict != want.Verdict {
			t.Fatalf("%s verdict = %q, want %q", key, got.Verdict, want.Verdict)
		}
		if want.Verdict == VerdictViolation {
			violations++
			if len(got.Evidence) != len(want.Evidence) {
				t.Fatalf("%s evidence = %+v, want %+v", key, got.Evidence, want.Evidence)
			}
			for i, evidence := range want.Evidence {
				if got.Evidence[i] != evidence {
					t.Fatalf("%s evidence[%d] = %+v, want %+v", key, i, got.Evidence[i], evidence)
				}
			}
		}
		if strings.HasPrefix(key, "trap-garden/") && strings.HasSuffix(key, "\x00B3") && got.Verdict != VerdictUnsure {
			t.Fatalf("semantic pair %s = %q, want unsure under MODEL_OFF", key, got.Verdict)
		}
	}
	if violations != 5 {
		t.Fatalf("violations = %d, want exactly the 5 real uses and zero wrongful deductions", violations)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestCheckerImageBundlesAnalyzerWithoutSocketOrKeys inspects the built image
// for the bundled analyzer and the absence of Docker/key material.
func TestCheckerImageBundlesAnalyzerWithoutSocketOrKeys(t *testing.T) {
	image := os.Getenv(checkerImageEnv)
	if image == "" {
		image = defaultImage
	}
	requireCheckerImage(t, image)

	script := `test -x /usr/local/bin/leetkob-analyzer
test -z "$MODEL_API_KEY"
test ! -e /var/run/docker.sock
test ! -e /run/docker.sock
! command -v docker >/dev/null 2>&1
echo image-ok`
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "none", "--entrypoint", "/bin/sh", image, "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("image inspection failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "image-ok") {
		t.Fatalf("unexpected inspection output: %s", out)
	}
}

// TestOptionalUnmodifiedScorerAcceptsPredictions runs the real, unmodified
// scorer only when a verified local checkout is available. It never downloads
// or modifies the scorer and never invents a score.
func TestOptionalUnmodifiedScorerAcceptsPredictions(t *testing.T) {
	official := strings.TrimSpace(os.Getenv("LEETKOB_OFFICIAL_EVAL_DIR"))
	if official == "" {
		t.Skip("set LEETKOB_OFFICIAL_EVAL_DIR to a verified fithcmus/awad-leetkob-eval checkout to run the unmodified scorer; see evaluation/README.md")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node unavailable, skipping scorer compatibility check: %v", err)
	}
	if _, err := os.Stat(filepath.Join(official, "eval.mjs")); err != nil {
		t.Fatalf("LEETKOB_OFFICIAL_EVAL_DIR does not contain eval.mjs: %v", err)
	}
	predictions := filepath.Join(t.TempDir(), "predictions.jsonl")
	summary, err := RunContext(context.Background(), official, predictions, Options{ModelOff: true})
	if err != nil {
		t.Fatalf("generate predictions: %v", err)
	}
	if summary.Predictions == 0 {
		t.Fatal("no predictions were generated")
	}
	reportPath := filepath.Join(t.TempDir(), "summary.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, filepath.Join(official, "eval.mjs"),
		"--data", official,
		"--predictions", predictions,
		"--json", reportPath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		t.Logf("unmodified scorer exit 0 (valid run)")
		return
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run scorer: %v", err)
	}
	switch exitErr.ExitCode() {
	case 1:
		t.Logf("unmodified scorer exit 1 (valid PA#3 run with threshold failure)")
	case 2:
		t.Fatalf("unmodified scorer exit 2: invalid predictions or inputs\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	default:
		t.Fatalf("unexpected scorer exit %d\nstdout:\n%s\nstderr:\n%s", exitErr.ExitCode(), stdout.String(), stderr.String())
	}
}
