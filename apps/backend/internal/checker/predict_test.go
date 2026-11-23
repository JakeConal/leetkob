package checker

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type predictionSchema struct {
	Type                 string `json:"type"`
	AdditionalProperties bool   `json:"additionalProperties"`
	Required             []string
	Properties           map[string]struct {
		Type      string   `json:"type"`
		MinLength int      `json:"minLength"`
		MaxLength int      `json:"maxLength"`
		Enum      []string `json:"enum"`
		Items     struct {
			Type                 string `json:"type"`
			AdditionalProperties bool   `json:"additionalProperties"`
			Required             []string
			Properties           map[string]struct {
				Type      string `json:"type"`
				Minimum   int    `json:"minimum"`
				MinLength int    `json:"minLength"`
				MaxLength int    `json:"maxLength"`
			} `json:"properties"`
		} `json:"items"`
	} `json:"properties"`
}

func loadPredictionSchema(t *testing.T) predictionSchema {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		candidate := filepath.Join(dir, "docs", "contract", "schemas", "checker-prediction.v1.json")
		if _, err := os.Stat(candidate); err == nil {
			data, err := os.ReadFile(candidate)
			if err != nil {
				t.Fatalf("read schema: %v", err)
			}
			var schema predictionSchema
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatalf("parse schema: %v", err)
			}
			return schema
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate docs/contract/schemas/checker-prediction.v1.json")
		}
		dir = parent
	}
}

// validatePredictionLine decodes one line with unknown fields rejected and
// checks every constraint the contract schema expresses, without adding a
// JSON-schema dependency.
func validatePredictionLine(t *testing.T, line []byte, schema predictionSchema) Prediction {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	var prediction Prediction
	if err := decoder.Decode(&prediction); err != nil {
		t.Fatalf("decode prediction %q: %v", line, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("prediction line has trailing data: %q", line)
	}

	if len(prediction.Solution) < 1 || len(prediction.Solution) > schema.Properties["solution"].MaxLength {
		t.Fatalf("solution length out of schema bounds: %q", prediction.Solution)
	}
	if len(prediction.Constraint) < 1 || len(prediction.Constraint) > schema.Properties["constraint"].MaxLength {
		t.Fatalf("constraint length out of schema bounds: %q", prediction.Constraint)
	}
	verdictEnum := schema.Properties["verdict"].Enum
	if !slices.Contains(verdictEnum, string(prediction.Verdict)) {
		t.Fatalf("verdict %q not in schema enum %v", prediction.Verdict, verdictEnum)
	}
	if prediction.Evidence == nil {
		t.Fatalf("evidence must encode as [] not null: %q", line)
	}
	evidenceSchema := schema.Properties["evidence"].Items
	for _, evidence := range prediction.Evidence {
		if evidence.Line < 1 {
			t.Fatalf("evidence line %d below schema minimum", evidence.Line)
		}
		if len(evidence.Text) < evidenceSchema.Properties["text"].MinLength ||
			len(evidence.Text) > evidenceSchema.Properties["text"].MaxLength {
			t.Fatalf("evidence text length out of schema bounds: %q", evidence.Text)
		}
	}
	return prediction
}

func TestRunWritesOneValidPredictionPerPair(t *testing.T) {
	t.Setenv("MODEL_OFF", "1")
	// Without a bundled analyzer the core must never guess a hard-rule
	// verdict; this keeps the discovery test independent of host tooling.
	t.Setenv(EnvAnalyzerPath, "")
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	summary, err := Run(unfamiliarFixture, output)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	problems, err := Discover(unfamiliarFixture)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	expectedPairs := make(map[string]bool)
	wantSolutions, wantConstraints := 0, 0
	for _, problem := range problems {
		wantSolutions += len(problem.Solutions)
		wantConstraints += len(problem.Constraints)
		for _, solution := range problem.Solutions {
			for _, constraint := range problem.Constraints {
				expectedPairs[solution.ID+"\x00"+constraint.ID] = true
			}
		}
	}
	if summary.Problems != len(problems) || summary.Solutions != wantSolutions ||
		summary.Constraints != wantConstraints || summary.Predictions != len(expectedPairs) ||
		summary.Unsure != len(expectedPairs) {
		t.Fatalf("summary = %+v, want %d problems, %d solutions, %d constraints, %d pairs",
			summary, len(problems), wantSolutions, wantConstraints, len(expectedPairs))
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read predictions: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatal("predictions must be newline-terminated JSONL")
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(lines) != len(expectedPairs) {
		t.Fatalf("prediction lines = %d, want %d", len(lines), len(expectedPairs))
	}
	if bytes.Contains(data, []byte(`"evidence":null`)) {
		t.Fatal("evidence must never be null")
	}

	schema := loadPredictionSchema(t)
	if schema.Type != "object" || schema.AdditionalProperties {
		t.Fatalf("unexpected schema shape: type=%q additionalProperties=%v", schema.Type, schema.AdditionalProperties)
	}
	if !slices.Equal(schema.Required, []string{"solution", "constraint", "verdict", "evidence"}) {
		t.Fatalf("schema required fields changed: %v", schema.Required)
	}
	if !slices.Equal(schema.Properties["evidence"].Items.Required, []string{"line", "text"}) {
		t.Fatalf("schema evidence fields changed: %v", schema.Properties["evidence"].Items.Required)
	}

	seenPairs := make(map[string]bool)
	for _, line := range lines {
		prediction := validatePredictionLine(t, line, schema)
		if prediction.Verdict != VerdictUnsure {
			t.Fatalf("task 7.1 must emit only unsure, got %q for %s/%s",
				prediction.Verdict, prediction.Solution, prediction.Constraint)
		}
		if len(prediction.Evidence) != 0 {
			t.Fatalf("unsure prediction must carry empty evidence: %+v", prediction)
		}
		key := prediction.Solution + "\x00" + prediction.Constraint
		if !expectedPairs[key] {
			t.Fatalf("unexpected solution/constraint pair %q", key)
		}
		if seenPairs[key] {
			t.Fatalf("duplicate prediction for pair %q", key)
		}
		seenPairs[key] = true
	}
	if len(seenPairs) != len(expectedPairs) {
		t.Fatalf("predicted %d pairs, want %d", len(seenPairs), len(expectedPairs))
	}
}

func TestPredictionOutputIgnoresLabelsScorerAndTests(t *testing.T) {
	clearModelEnv(t)
	baseData, err := os.ReadFile(filepath.Join(unfamiliarFixture, "labels.json"))
	if err != nil {
		t.Fatal(err)
	}
	if json.Valid(baseData) {
		t.Fatal("fixture labels.json must be invalid JSON so a parse attempt would fail")
	}
	scorer, err := os.ReadFile(filepath.Join(unfamiliarFixture, "eval.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(scorer, []byte("{{{")) {
		t.Fatal("fixture eval.mjs must be an invalid scorer sentinel")
	}

	withMetadata := filepath.Join(t.TempDir(), "with-metadata")
	copyTree(t, unfamiliarFixture, withMetadata)
	withOutput := filepath.Join(t.TempDir(), "with.jsonl")
	if _, err := Run(withMetadata, withOutput); err != nil {
		t.Fatalf("Run with metadata: %v", err)
	}
	withBytes, err := os.ReadFile(withOutput)
	if err != nil {
		t.Fatal(err)
	}

	withoutMetadata := filepath.Join(t.TempDir(), "without-metadata")
	copyTree(t, unfamiliarFixture, withoutMetadata)
	for _, metadata := range []string{"labels.json", "eval.mjs", "report.json", "team-review"} {
		if err := os.RemoveAll(filepath.Join(withoutMetadata, metadata)); err != nil {
			t.Fatal(err)
		}
	}
	withoutOutput := filepath.Join(t.TempDir(), "without.jsonl")
	if _, err := Run(withoutMetadata, withoutOutput); err != nil {
		t.Fatalf("Run without metadata: %v", err)
	}
	withoutBytes, err := os.ReadFile(withoutOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(withBytes, withoutBytes) {
		t.Fatal("labels/scorer/test presence changed predictions")
	}

	contradictory := filepath.Join(t.TempDir(), "contradictory-labels")
	copyTree(t, unfamiliarFixture, contradictory)
	contradiction := []byte(`{"solution":"hollow-reef/s01","constraint":"C1","verdict":"pass","evidence":[]}` + "\n")
	if err := os.WriteFile(filepath.Join(contradictory, "labels.json"), contradiction, 0o644); err != nil {
		t.Fatal(err)
	}
	contradictoryOutput := filepath.Join(t.TempDir(), "contradictory.jsonl")
	if _, err := Run(contradictory, contradictoryOutput); err != nil {
		t.Fatalf("Run with contradictory labels: %v", err)
	}
	contradictoryBytes, err := os.ReadFile(contradictoryOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(withBytes, contradictoryBytes) {
		t.Fatal("parseable labels influenced predictions")
	}
}

func TestRunIsDeterministicAndModelOffIndependent(t *testing.T) {
	clearModelEnv(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "first.jsonl")
	second := filepath.Join(dir, "second.jsonl")
	t.Setenv("MODEL_OFF", "")
	if _, err := Run(unfamiliarFixture, first); err != nil {
		t.Fatalf("first run: %v", err)
	}
	t.Setenv("MODEL_OFF", "1")
	if _, err := Run(unfamiliarFixture, second); err != nil {
		t.Fatalf("second run: %v", err)
	}
	firstBytes, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("two runs over the same dataset produced different JSONL")
	}
}

func TestRunFailurePreservesExistingPredictions(t *testing.T) {
	badDataset := t.TempDir()
	problem := filepath.Join(badDataset, "broken")
	mustMkdir(t, filepath.Join(problem, "solutions"))
	mustWrite(t, filepath.Join(problem, "statement.md"), "statement\n")
	mustWrite(t, filepath.Join(problem, "solutions", "s01.cpp"), "int main() { return 0; }\n")

	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	mustWrite(t, output, "previous-valid-run\n")
	if _, err := Run(badDataset, output); !errors.Is(err, ErrProblemMissingConstraints) {
		t.Fatalf("err = %v, want ErrProblemMissingConstraints", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "previous-valid-run\n" {
		t.Fatalf("failed run replaced existing predictions: %q", data)
	}
}

func TestCoreImportsNoNetworkPackage(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	// Task 7.2 allows exactly one process boundary: the analyzer executable
	// bundled in the checker image, invoked without a shell. The checker
	// package itself still imports no network package; semantic transport
	// stays behind the internal/model port, and MODEL_OFF never constructs
	// it.
	forbidden := []string{"net", "net/http", "syscall"}
	allowed := map[string]bool{"os/exec": true}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, spec := range file.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			if allowed[imported] {
				continue
			}
			if slices.Contains(forbidden, imported) || strings.HasPrefix(imported, "net/") {
				t.Fatalf("%s imports %q; the shared checker core must not open a network", name, imported)
			}
		}
	}
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
}
