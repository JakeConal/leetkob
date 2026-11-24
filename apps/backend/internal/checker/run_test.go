package checker_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JakeConal/leetkob/backend/internal/checker"
	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
)

// fakeEngine returns a fixed analyzer-result.v1 document per source marker.
// It never runs a process.
type fakeEngine struct {
	referenceLine int
}

func (f fakeEngine) Analyze(_ context.Context, source string) ([]byte, error) {
	if !strings.Contains(source, "std::sort") {
		return []byte(`{"version":1,"status":"ok","references":[]}`), nil
	}
	reference := map[string]any{
		"kind": "call", "name": "sort", "qualifiedName": "std::sort",
		"header": "algorithm", "resolved": true, "userDeclared": false,
		"line": f.referenceLine, "column": 3,
	}
	document, err := json.Marshal(map[string]any{
		"version": 1, "status": "ok", "references": []any{reference},
	})
	return document, err
}

func writeDataset(t *testing.T, sortLine int) string {
	t.Helper()
	root := t.TempDir()
	problem := filepath.Join(root, "problems", "p1")
	if err := os.MkdirAll(filepath.Join(problem, "solutions"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	statement := "Implement merge sort.\n"
	if err := os.WriteFile(filepath.Join(problem, "statement.md"), []byte(statement), 0o644); err != nil {
		t.Fatalf("statement: %v", err)
	}
	constraints := `[
	  {"id":"C1","text":"Do not call ` + "`std::sort`" + `.","deduction":3},
	  {"id":"C2","text":"Implement merge sort.","deduction":4}
	]`
	if err := os.WriteFile(filepath.Join(problem, "constraints.json"), []byte(constraints), 0o644); err != nil {
		t.Fatalf("constraints: %v", err)
	}
	body := make([]string, 0, sortLine)
	for i := 2; i < sortLine-1; i++ {
		body = append(body, "// filler")
	}
	body = append(body, "  std::sort(values.begin(), values.end());")
	sortedSource := "#include <algorithm>\n#include <vector>\n" + strings.Join(body, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(problem, "solutions", "a.cpp"), []byte(sortedSource), 0o644); err != nil {
		t.Fatalf("solution a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(problem, "solutions", "b.cpp"), []byte("int main() { return 0; }\n"), 0o644); err != nil {
		t.Fatalf("solution b: %v", err)
	}
	// Labels, scorer and tests must never be read; invalid content proves it.
	if err := os.WriteFile(filepath.Join(root, "labels.json"), []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("labels: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "eval.mjs"), []byte("this is not javascript ("), 0o644); err != nil {
		t.Fatalf("eval: %v", err)
	}
	return root
}

func readPredictions(t *testing.T, path string) []schema.Prediction {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read predictions: %v", err)
	}
	var predictions []schema.Prediction
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var prediction schema.Prediction
		if err := json.Unmarshal([]byte(line), &prediction); err != nil {
			t.Fatalf("parse prediction: %v", err)
		}
		if err := schema.ValidatePrediction(prediction); err != nil {
			t.Fatalf("invalid prediction %q: %v", line, err)
		}
		predictions = append(predictions, prediction)
	}
	return predictions
}

func TestRunDecidesBanListsAndWritesValidJSONL(t *testing.T) {
	root := writeDataset(t, 4)
	output := filepath.Join(t.TempDir(), "predictions.jsonl")

	summary, err := checker.Run(context.Background(), checker.RunOptions{
		Root: root, Predictions: output, Engine: fakeEngine{referenceLine: 4},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if summary.Problems != 1 || summary.Solutions != 2 || summary.Constraints != 2 || summary.Predictions != 4 {
		t.Fatalf("summary = %+v", summary)
	}

	predictions := readPredictions(t, output)
	byKey := map[string]schema.Prediction{}
	for _, prediction := range predictions {
		byKey[prediction.Solution+"::"+prediction.Constraint] = prediction
	}

	violation := byKey["p1/a::C1"]
	if violation.Verdict != schema.VerdictViolation || len(violation.Evidence) != 1 ||
		violation.Evidence[0].Line != 4 || !strings.Contains(violation.Evidence[0].Text, "std::sort") {
		t.Fatalf("violation = %+v", violation)
	}
	if pass := byKey["p1/b::C1"]; pass.Verdict != schema.VerdictPass || len(pass.Evidence) != 0 {
		t.Fatalf("pass = %+v", pass)
	}
	for _, key := range []string{"p1/a::C2", "p1/b::C2"} {
		if semantic := byKey[key]; semantic.Verdict != schema.VerdictUnsure {
			t.Fatalf("%s semantic verdict = %s, want unsure", key, semantic.Verdict)
		}
	}
}

func TestRunWithoutAnalyzerNeverGuesses(t *testing.T) {
	root := writeDataset(t, 4)
	output := filepath.Join(t.TempDir(), "predictions.jsonl")

	if _, err := checker.Run(context.Background(), checker.RunOptions{Root: root, Predictions: output, Engine: nil}); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, prediction := range readPredictions(t, output) {
		if prediction.Verdict != schema.VerdictUnsure {
			t.Fatalf("%s/%s = %s with no analyzer, want unsure", prediction.Solution, prediction.Constraint, prediction.Verdict)
		}
	}
}

func TestRunRejectsOversizedSource(t *testing.T) {
	root := writeDataset(t, 4)
	huge := filepath.Join(root, "problems", "p1", "solutions", "huge.cpp")
	if err := os.WriteFile(huge, []byte(strings.Repeat("a", 65537)), 0o644); err != nil {
		t.Fatalf("huge solution: %v", err)
	}
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	if _, err := checker.Run(context.Background(), checker.RunOptions{
		Root: root, Predictions: output, Engine: fakeEngine{referenceLine: 4},
	}); err == nil {
		t.Fatal("expected an oversized-source error")
	}
}

func TestRunRequiresOptions(t *testing.T) {
	if _, err := checker.Run(context.Background(), checker.RunOptions{}); err == nil {
		t.Fatal("expected an error for missing options")
	}
}
