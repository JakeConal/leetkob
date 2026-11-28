package checker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
)

// resolveDocker returns an absolute docker path by scanning PATH directly,
// so no subprocess is spawned through a PATH lookup.
func resolveDocker(t *testing.T) string {
	t.Helper()
	for _, directory := range strings.Split(os.Getenv("PATH"), ":") {
		if directory == "" {
			continue
		}
		candidate := filepath.Join(directory, "docker")
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	t.Skip("docker is unavailable")
	return ""
}

// TestImageMatchesTeamCaseLabels builds the real checker image and runs it
// with no network on the labelled team cases, comparing every verdict with
// the independently authored labels. It skips only when Docker is absent.
func TestImageMatchesTeamCaseLabels(t *testing.T) {
	docker := resolveDocker(t)
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	cases := filepath.Join(root, "evaluation", "team-cases")
	labelsPath := filepath.Join(cases, "labels", "labels.jsonl")
	if _, err := os.Stat(labelsPath); err != nil {
		t.Skipf("team case labels are unavailable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	image := "leetkob-checker:test"
	build := exec.CommandContext(ctx, docker, "build", "-q",
		"-f", filepath.Join(root, "apps", "backend", "checker", "Dockerfile"),
		"-t", image, filepath.Join(root, "apps", "backend"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build checker image: %v\n%s", err, output)
	}

	outputDir := t.TempDir()
	predictionsPath := filepath.Join(outputDir, "predictions.jsonl")
	inputs := filepath.Join(t.TempDir(), "inputs")
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for the dataset boundary preparer")
	}
	stage := exec.CommandContext(ctx, node, filepath.Join(root, "scripts", "evaluation", "sources.mjs"), "stage", cases, inputs)
	if output, err := stage.CombinedOutput(); err != nil {
		t.Fatalf("stage prediction-only inputs: %v\n%s", err, output)
	}
	run := exec.CommandContext(ctx, docker, "run", "--rm",
		"--network", "none", "--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "64", "--memory", "768m", "--memory-swap", "768m",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-v", inputs+":/data:ro", "-v", outputDir+":/out",
		"-e", "MODEL_OFF=1", image,
		"--data", "/data", "--predictions", "/out/predictions.jsonl")
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("run checker image: %v\n%s", err, output)
	}

	raw, err := os.ReadFile(predictionsPath)
	if err != nil {
		t.Fatalf("read predictions: %v", err)
	}
	predictions := map[string]schema.Prediction{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var prediction schema.Prediction
		if err := json.Unmarshal([]byte(line), &prediction); err != nil {
			t.Fatalf("parse prediction: %v", err)
		}
		if err := schema.ValidatePrediction(prediction); err != nil {
			t.Fatalf("invalid prediction: %v", err)
		}
		predictions[prediction.Solution+"::"+prediction.Constraint] = prediction
	}
	if len(predictions) == 0 {
		t.Fatal("checker produced no predictions")
	}

	labelsRaw, err := os.ReadFile(labelsPath)
	if err != nil {
		t.Fatalf("read labels: %v", err)
	}
	checked := 0
	for _, line := range strings.Split(strings.TrimSpace(string(labelsRaw)), "\n") {
		if line == "" {
			continue
		}
		var label struct {
			Solution   string `json:"solution"`
			Constraint string `json:"constraint"`
			Verdict    string `json:"verdict"`
		}
		if err := json.Unmarshal([]byte(line), &label); err != nil {
			t.Fatalf("parse label: %v", err)
		}
		prediction, ok := predictions[label.Solution+"::"+label.Constraint]
		if !ok {
			t.Fatalf("missing prediction for %s/%s", label.Solution, label.Constraint)
		}
		if string(prediction.Verdict) != label.Verdict {
			t.Fatalf("%s/%s verdict = %s, label = %s",
				label.Solution, label.Constraint, prediction.Verdict, label.Verdict)
		}
		checked++
	}
	if checked < 5 {
		t.Fatalf("checked only %d labelled pairs", checked)
	}
}
