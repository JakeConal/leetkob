package checker

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const unfamiliarFixture = "testdata/unfamiliar"

func TestDiscoverUnfamiliarLayout(t *testing.T) {
	problems, err := Discover(unfamiliarFixture)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	wantProblems := []string{"bespoke-harbor", "hollow-reef", "odd-lab", "quiet-orbit"}
	var gotProblems []string
	for _, problem := range problems {
		gotProblems = append(gotProblems, problem.ID)
	}
	if !slices.Equal(gotProblems, wantProblems) {
		t.Fatalf("problem IDs = %v, want %v", gotProblems, wantProblems)
	}

	byID := make(map[string]Problem, len(problems))
	for _, problem := range problems {
		byID[problem.ID] = problem
	}

	hollow := byID["hollow-reef"]
	wantConstraintIDs := []string{"C1", "C2", "C3"}
	var constraintIDs []string
	for _, constraint := range hollow.Constraints {
		constraintIDs = append(constraintIDs, constraint.ID)
	}
	if !slices.Equal(constraintIDs, wantConstraintIDs) {
		t.Fatalf("hollow-reef constraints = %v, want %v", constraintIDs, wantConstraintIDs)
	}
	if got := hollow.Constraints[0].BannedNames; !slices.Equal(got, []string{"std::sort"}) {
		t.Fatalf("C1 banned names = %v, want [std::sort]", got)
	}
	if got := hollow.Constraints[2].BannedNames; !slices.Equal(got, []string{"malloc", "std::vector"}) {
		t.Fatalf("C3 banned names = %v, want [malloc std::vector]", got)
	}

	wantSolutions := []string{"hollow-reef/s01", "hollow-reef/s02", "hollow-reef/s03"}
	var solutionIDs []string
	for _, solution := range hollow.Solutions {
		solutionIDs = append(solutionIDs, solution.ID)
	}
	if !slices.Equal(solutionIDs, wantSolutions) {
		t.Fatalf("hollow-reef solutions = %v, want %v", solutionIDs, wantSolutions)
	}

	quiet := byID["quiet-orbit"]
	quietConstraints := []string{}
	for _, constraint := range quiet.Constraints {
		quietConstraints = append(quietConstraints, constraint.ID+":"+constraint.Text)
	}
	wantQuiet := []string{
		"K1:Do not use `std::map` or `std::set`.",
		"K2:Keep every operation worst-case O(1).",
		"C3:Do not call `exit`.",
	}
	if !slices.Equal(quietConstraints, wantQuiet) {
		t.Fatalf("quiet-orbit constraints = %q, want %q", quietConstraints, wantQuiet)
	}
	if got := quiet.Constraints[2].BannedNames; !slices.Equal(got, []string{"exit"}) {
		t.Fatalf("generated C3 banned names = %v, want [exit]", got)
	}

	odd := byID["odd-lab"]
	if len(odd.Constraints) != 2 || odd.Constraints[0].ID != "L1" {
		t.Fatalf("odd-lab constraints = %+v, want tab-separated L1 and L2", odd.Constraints)
	}
	if !strings.HasSuffix(odd.StatementPath, "statement-final.md") {
		t.Fatalf("odd-lab statement = %q, want statement-final.md", odd.StatementPath)
	}

	bespoke := byID["bespoke-harbor"]
	var bespokeSolutions []string
	for _, solution := range bespoke.Solutions {
		bespokeSolutions = append(bespokeSolutions, solution.ID)
	}
	if !slices.Equal(bespokeSolutions, []string{"bespoke-harbor/x1", "bespoke-harbor/x2"}) {
		t.Fatalf("bespoke-harbor solutions = %v", bespokeSolutions)
	}
	if len(bespoke.Constraints) != 2 {
		t.Fatalf("bespoke-harbor constraints = %v, want 2 from JSON object wrapper", bespoke.Constraints)
	}
}

// recordingOps records every filesystem access so a test can prove that
// metadata files are never opened, not merely that parsing them was skipped.
type recordedAccess struct {
	op   string
	path string
}

type recordingOps struct {
	inner    fileOps
	accesses []recordedAccess
}

func newRecordingOps() *recordingOps {
	return &recordingOps{inner: defaultFileOps}
}

func (r *recordingOps) readDir(path string) ([]os.DirEntry, error) {
	r.accesses = append(r.accesses, recordedAccess{op: "readDir", path: path})
	return r.inner.readDir(path)
}

func (r *recordingOps) readFile(path string) ([]byte, error) {
	r.accesses = append(r.accesses, recordedAccess{op: "readFile", path: path})
	return r.inner.readFile(path)
}

func (r *recordingOps) stat(path string) (os.FileInfo, error) {
	r.accesses = append(r.accesses, recordedAccess{op: "stat", path: path})
	return r.inner.stat(path)
}

func (r *recordingOps) ops() fileOps {
	return fileOps{readDir: r.readDir, readFile: r.readFile, stat: r.stat}
}

func TestRunNeverOpensLabelsScorerOrTests(t *testing.T) {
	recorder := newRecordingOps()
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	if _, err := runWithOps(unfamiliarFixture, output, recorder.ops()); err != nil {
		t.Fatalf("runWithOps: %v", err)
	}

	forbiddenNames := map[string]bool{
		"labels.json":     true,
		"labels.jsonl":    true,
		"eval.mjs":        true,
		"report.json":     true,
		"provenance.json": true,
		"README.md":       true,
		"notes.md":        true,
		"01.in":           true,
		"01.out":          true,
	}
	readFiles := 0
	for _, access := range recorder.accesses {
		relative, err := filepath.Rel(unfamiliarFixture, access.path)
		if err != nil {
			t.Fatalf("relative path for %s: %v", access.path, err)
		}
		slashed := filepath.ToSlash(relative)
		if strings.HasPrefix(slashed, "tests/") || strings.HasPrefix(slashed, "team-review/") {
			t.Fatalf("%s opened forbidden directory: %s", access.op, slashed)
		}
		if forbiddenNames[filepath.Base(access.path)] {
			t.Fatalf("%s opened forbidden metadata file: %s", access.op, slashed)
		}
		if access.op == "readFile" {
			readFiles++
		}
	}
	if readFiles == 0 {
		t.Fatal("no files were read; recorder did not observe discovery")
	}
	if !strings.Contains(output, "predictions") {
		t.Fatalf("unexpected output path %q", output)
	}
}

func TestDiscoverEmptyDataset(t *testing.T) {
	empty := t.TempDir()
	if _, err := Discover(empty); !errors.Is(err, ErrDatasetEmpty) {
		t.Fatalf("err = %v, want ErrDatasetEmpty", err)
	}
}

func TestDiscoverRejectsMissingStatementOrConstraints(t *testing.T) {
	root := t.TempDir()
	problem := filepath.Join(root, "problems", "no-constraints")
	mustMkdir(t, filepath.Join(problem, "solutions"))
	mustWrite(t, filepath.Join(problem, "statement.md"), "statement\n")
	mustWrite(t, filepath.Join(problem, "solutions", "s01.cpp"), "int main() { return 0; }\n")
	if _, err := Discover(root); !errors.Is(err, ErrProblemMissingConstraints) {
		t.Fatalf("err = %v, want ErrProblemMissingConstraints", err)
	}
	if err := os.RemoveAll(problem); err != nil {
		t.Fatal(err)
	}

	problem = filepath.Join(root, "problems", "no-statement")
	mustMkdir(t, filepath.Join(problem, "solutions"))
	mustWrite(t, filepath.Join(problem, "constraints.json"), `[{"id":"C1","text":"Do nothing."}]`)
	mustWrite(t, filepath.Join(problem, "solutions", "s01.cpp"), "int main() { return 0; }\n")
	_, err := Discover(root)
	if !errors.Is(err, ErrProblemMissingStatement) {
		t.Fatalf("err = %v, want ErrProblemMissingStatement", err)
	}
}

func TestDiscoverRejectsProblemWithoutSolutions(t *testing.T) {
	root := t.TempDir()
	problem := filepath.Join(root, "solutionsless")
	mustMkdir(t, problem)
	mustWrite(t, filepath.Join(problem, "statement.md"), "statement\n")
	mustWrite(t, filepath.Join(problem, "constraints.json"), `[{"id":"C1","text":"Do nothing."}]`)
	if _, err := Discover(root); !errors.Is(err, ErrProblemHasNoSolutions) {
		t.Fatalf("err = %v, want ErrProblemHasNoSolutions", err)
	}
}

func TestDiscoverRejectsDuplicateProblemIDsAcrossContainers(t *testing.T) {
	root := t.TempDir()
	for _, container := range []string{"problems", "labs"} {
		problem := filepath.Join(root, container, "same-id")
		mustMkdir(t, filepath.Join(problem, "solutions"))
		mustWrite(t, filepath.Join(problem, "statement.md"), "statement\n")
		mustWrite(t, filepath.Join(problem, "constraints.json"), `[{"id":"C1","text":"Do nothing."}]`)
		mustWrite(t, filepath.Join(problem, "solutions", "s01.cpp"), "int main() { return 0; }\n")
	}
	if _, err := Discover(root); !errors.Is(err, ErrDuplicateProblem) {
		t.Fatalf("err = %v, want ErrDuplicateProblem", err)
	}
}

func TestDiscoverRejectsDuplicateSolutionAndOversizedSource(t *testing.T) {
	root := t.TempDir()
	problem := filepath.Join(root, "dup")
	mustMkdir(t, filepath.Join(problem, "solutions"))
	mustWrite(t, filepath.Join(problem, "statement.md"), "statement\n")
	mustWrite(t, filepath.Join(problem, "constraints.json"), `[{"id":"C1","text":"Do nothing."}]`)
	mustWrite(t, filepath.Join(problem, "solutions", "s01.cpp"), "int main() { return 0; }\n")
	mustWrite(t, filepath.Join(root, "solutions", "dup", "s01.cc"), "int main() { return 0; }\n")
	if _, err := Discover(root); !errors.Is(err, ErrDuplicateSolution) {
		t.Fatalf("err = %v, want ErrDuplicateSolution", err)
	}

	if err := os.RemoveAll(filepath.Join(root, "solutions")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(problem, "solutions", "big.cpp"), strings.Repeat("x", maxSourceBytes+1))
	if _, err := Discover(root); err != nil {
		t.Fatalf("Discover must not read source bodies: %v", err)
	}
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	if _, err := Run(root, output); !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("Run err = %v, want ErrSourceTooLarge", err)
	}
}

func TestParseConstraintLines(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantIDs    []string
		wantTexts  []string
		bannedName []string
	}{
		{
			name:       "explicit colon",
			input:      "C1: Do not call `std::sort`.\n",
			wantIDs:    []string{"C1"},
			wantTexts:  []string{"Do not call `std::sort`."},
			bannedName: []string{"std::sort"},
		},
		{
			name:      "digit identifier without separator",
			input:     "K2 Keep every operation worst-case O(1).\n",
			wantIDs:   []string{"K2"},
			wantTexts: []string{"Keep every operation worst-case O(1)."},
		},
		{
			name:       "tab separator",
			input:      "L1\tDo not use `malloc`.\n",
			wantIDs:    []string{"L1"},
			wantTexts:  []string{"Do not use `malloc`."},
			bannedName: []string{"malloc"},
		},
		{
			name:       "generated identifier keeps leading word",
			input:      "Do not call `exit`.\n",
			wantIDs:    []string{"C1"},
			wantTexts:  []string{"Do not call `exit`."},
			bannedName: []string{"exit"},
		},
		{
			name:      "comments and blank lines skipped",
			input:     "# comment\n\n// another\nQ7 Do something.\n",
			wantIDs:   []string{"Q7"},
			wantTexts: []string{"Do something."},
		},
		{
			name:      "prose colon is not an identifier",
			input:     "Note: keep the loop simple.\n",
			wantIDs:   []string{"C1"},
			wantTexts: []string{"Note: keep the loop simple."},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			constraints, err := parseConstraintLines([]byte(test.input))
			if err != nil {
				t.Fatalf("parseConstraintLines: %v", err)
			}
			var ids, texts []string
			for _, constraint := range constraints {
				ids = append(ids, constraint.ID)
				texts = append(texts, constraint.Text)
			}
			if !slices.Equal(ids, test.wantIDs) || !slices.Equal(texts, test.wantTexts) {
				t.Fatalf("got %v %v, want %v %v", ids, texts, test.wantIDs, test.wantTexts)
			}
			if test.bannedName != nil && !slices.Equal(constraints[0].BannedNames, test.bannedName) {
				t.Fatalf("banned names = %v, want %v", constraints[0].BannedNames, test.bannedName)
			}
		})
	}

	if _, err := parseConstraintLines([]byte("# only comments\n")); !errors.Is(err, ErrNoConstraints) {
		t.Fatalf("empty constraints err = %v, want ErrNoConstraints", err)
	}
	if _, err := parseConstraintLines([]byte("C1 Do a.\nC1 Do b.\n")); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate ID err = %v, want duplicate error", err)
	}
}

func TestParseConstraintsJSON(t *testing.T) {
	constraints, err := parseConstraintsJSON([]byte(
		"[{\"id\":\"C1\",\"text\":\"Do not use `goto`.\",\"deduction\":3}," +
			"{\"id\":\"C2\",\"text\":\"Keep it simple.\"}]"))
	if err != nil {
		t.Fatalf("parseConstraintsJSON: %v", err)
	}
	if len(constraints) != 2 || constraints[0].ID != "C1" || len(constraints[0].BannedNames) != 1 {
		t.Fatalf("constraints = %+v", constraints)
	}
	if _, err := parseConstraintsJSON([]byte(`{"constraints":[]}`)); !errors.Is(err, ErrNoConstraints) {
		t.Fatalf("empty object err = %v, want ErrNoConstraints", err)
	}
	if _, err := parseConstraintsJSON([]byte(`[{"id":"C1","text":""}]`)); err == nil {
		t.Fatal("empty constraint text must be rejected")
	}
	if _, err := parseConstraintsJSON([]byte(`[{"id":"C1","text":"a"},{"id":"C1","text":"b"}]`)); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate ID err = %v, want duplicate error", err)
	}
}

func TestBannedNames(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"Do not call `std::sort`.", []string{"std::sort"}},
		{"Do not use `malloc` or `std::vector`.", []string{"malloc", "std::vector"}},
		{"  Do not use `a`, `a`, `b`.", []string{"a", "b"}},
		{"Implement merge sort.", nil},
		{"Do not eat `x`.", nil},
		{"Do not use `unterminated.", nil},
		{"Do not call ``.", nil},
	}
	for _, test := range tests {
		if got := BannedNames(test.text); !slices.Equal(got, test.want) {
			t.Errorf("BannedNames(%q) = %v, want %v", test.text, got, test.want)
		}
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
