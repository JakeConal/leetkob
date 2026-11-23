package checker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JakeConal/leetkob/backend/internal/analyzer"
	"github.com/JakeConal/leetkob/backend/internal/budget"
	"github.com/JakeConal/leetkob/backend/internal/model"
)

// guardAnalyzer records hard-rule requests. A nil report keeps every name
// unsure, which is the safe tooling failure.
type guardAnalyzer struct {
	calls    int
	requests []analyzer.Request
	decide   func(analyzer.Request) (analyzer.Report, error)
}

func (g *guardAnalyzer) Analyze(_ context.Context, request analyzer.Request) (analyzer.Report, error) {
	g.calls++
	g.requests = append(g.requests, request)
	if g.decide == nil {
		return analyzer.UnsureReport(request.BannedNames, "test guard"), nil
	}
	return g.decide(request)
}

// guardSemantic fails the test if a dispatch happens on a path that must not
// dispatch. It records requests so a test can prove batch contents.
type guardSemantic struct {
	t        *testing.T
	calls    int
	requests []model.Request
	result   model.Result
	err      error
	forbid   bool
}

func (g *guardSemantic) Check(_ context.Context, request model.Request) (model.Result, error) {
	g.calls++
	g.requests = append(g.requests, request)
	if g.forbid {
		g.t.Fatalf("semantic transport was called on a no-dispatch path: %+v", request)
	}
	if g.err != nil {
		return model.Result{}, g.err
	}
	return g.result, nil
}

func violationReport(request analyzer.Request) (analyzer.Report, error) {
	lines := strings.Split(strings.TrimSuffix(request.Source, "\n"), "\n")
	results := make([]analyzer.Result, 0, len(request.BannedNames))
	for _, name := range request.BannedNames {
		switch name {
		case "std::sort":
			results = append(results, analyzer.Result{
				Name:     name,
				Verdict:  analyzer.VerdictViolation,
				Evidence: []analyzer.Evidence{{Line: 2, Text: lines[1]}},
			})
		default:
			results = append(results, analyzer.Result{Name: name, Verdict: analyzer.VerdictPass})
		}
	}
	return analyzer.Report{Version: analyzer.ProtocolVersion, Results: results}, nil
}

func passReport(request analyzer.Request) (analyzer.Report, error) {
	results := make([]analyzer.Result, 0, len(request.BannedNames))
	for _, name := range request.BannedNames {
		results = append(results, analyzer.Result{Name: name, Verdict: analyzer.VerdictPass})
	}
	return analyzer.Report{Version: analyzer.ProtocolVersion, Results: results}, nil
}

// clearModelEnv removes every provider and analyzer selection from the test
// process so a Run that reads the environment can never contact a provider or
// host tool.
func clearModelEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		model.EnvBaseURL,
		model.EnvModel,
		model.EnvAPIKey,
		model.EnvTimeoutSeconds,
		model.EnvMaxOutputTokens,
		EnvAnalyzerPath,
		EnvInputPrice,
		EnvOutputPrice,
		EnvLabID,
	} {
		t.Setenv(name, "")
	}
}

func newDataset(t *testing.T, constraints, source string) string {
	t.Helper()
	root := t.TempDir()
	problem := filepath.Join(root, "problems", "sample")
	mustMkdir(t, filepath.Join(problem, "solutions"))
	mustWrite(t, filepath.Join(problem, "statement.md"), "Sort the input.\n")
	mustWrite(t, filepath.Join(problem, "constraints.json"), constraints)
	mustWrite(t, filepath.Join(problem, "solutions", "s01.cpp"), source)
	return root
}

func readPredictions(t *testing.T, path string) []Prediction {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read predictions: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	predictions := make([]Prediction, 0, len(lines))
	for _, line := range lines {
		var prediction Prediction
		if err := json.Unmarshal([]byte(line), &prediction); err != nil {
			t.Fatalf("decode prediction %q: %v", line, err)
		}
		predictions = append(predictions, prediction)
	}
	return predictions
}

func byPair(predictions []Prediction) map[string]Prediction {
	index := make(map[string]Prediction, len(predictions))
	for _, prediction := range predictions {
		index[prediction.Solution+"\x00"+prediction.Constraint] = prediction
	}
	return index
}

const twoBanConstraints = `[
  {"id":"B1","text":"Do not call ` + "`std::sort`" + `."},
  {"id":"B2","text":"Do not use ` + "`std::vector`" + `."},
  {"id":"S1","text":"Keep the implementation simple."}
]`

const trapSource = "#include <algorithm>\nint main() { std::sort(a, b); }\n"

func TestModelOffNeverDispatchesAndDecidesBanList(t *testing.T) {
	t.Setenv("MODEL_OFF", "1")
	root := newDataset(t, twoBanConstraints, trapSource)
	hard := &guardAnalyzer{decide: violationReport}
	semantic := &guardSemantic{t: t, forbid: true}
	store := newMemoryBudgetStore()
	spend := budget.NewService(store, budget.SystemClock{}, budget.PriceSchedule{InputCentsPer1K: 100, OutputCentsPer1K: 100})

	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	summary, err := RunContext(context.Background(), root, output, Options{
		ModelOff:        true,
		Analyzer:        hard,
		Semantic:        semantic,
		Budget:          spend,
		MaxOutputTokens: 64,
	})
	if err != nil {
		t.Fatalf("RunContext: %v", err)
	}
	if semantic.calls != 0 {
		t.Fatalf("MODEL_OFF dispatched %d semantic calls", semantic.calls)
	}
	if summary.ModelCalls != 0 {
		t.Fatalf("summary.ModelCalls = %d, want 0", summary.ModelCalls)
	}
	if summary.Pass != 1 || summary.Violation != 1 || summary.Unsure != 1 {
		t.Fatalf("summary = %+v, want one pass, one violation, one unsure", summary)
	}
	if summary.UnsureCauses[CauseModelOff] != 1 {
		t.Fatalf("unsure causes = %v, want %s for the semantic pair", summary.UnsureCauses, CauseModelOff)
	}

	predictions := byPair(readPredictions(t, output))
	if got := predictions["sample/s01\x00B1"]; got.Verdict != VerdictViolation || len(got.Evidence) != 1 || got.Evidence[0].Line != 2 {
		t.Fatalf("B1 = %+v, want a violation with original line 2", got)
	}
	if got := predictions["sample/s01\x00B2"]; got.Verdict != VerdictPass {
		t.Fatalf("B2 = %+v, want pass", got)
	}
	if got := predictions["sample/s01\x00S1"]; got.Verdict != VerdictUnsure || len(got.Evidence) != 0 {
		t.Fatalf("S1 = %+v, want unsure with empty evidence", got)
	}
	if len(hard.requests) != 1 {
		t.Fatalf("analyzer calls = %d, want one bounded batch", len(hard.requests))
	}
	if names := hard.requests[0].BannedNames; len(names) != 2 || names[0] != "std::sort" || names[1] != "std::vector" {
		t.Fatalf("analyzer names = %v, want both ban-list names", names)
	}
}

func TestModelOffWithoutAnalyzerNeverGuessesViolation(t *testing.T) {
	root := "testdata/banlist-traps"
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	summary, err := RunContext(context.Background(), root, output, Options{ModelOff: true})
	if err != nil {
		t.Fatalf("RunContext: %v", err)
	}
	if summary.Violation != 0 {
		t.Fatalf("no analyzer configured but %d violations were invented", summary.Violation)
	}
	if summary.Pass != 0 {
		t.Fatalf("no analyzer configured but %d passes were invented", summary.Pass)
	}
	for _, prediction := range readPredictions(t, output) {
		if prediction.Verdict != VerdictUnsure {
			t.Fatalf("prediction %+v is not unsure without an analyzer", prediction)
		}
	}
}

func TestBanListTrapFixtureWithReviewedHardRuleReportHasZeroFalseViolations(t *testing.T) {
	// The image test runs the real libclang analyzer. This unit test uses the
	// same reviewed expected file as a scripted analyzer, proving the engine
	// maps decisions without adding or dropping verdicts.
	expected := readPredictions(t, filepath.Join("testdata", "banlist-traps", "expected.jsonl"))
	bySource := make(map[string]map[string]Prediction)
	for _, prediction := range expected {
		parts := strings.SplitN(prediction.Solution, "/", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected solution id %q", prediction.Solution)
		}
		if bySource[parts[1]] == nil {
			bySource[parts[1]] = make(map[string]Prediction)
		}
		bySource[parts[1]][prediction.Constraint] = prediction
	}
	hard := &guardAnalyzer{decide: func(request analyzer.Request) (analyzer.Report, error) {
		// A real analyzer would return per-source results, but the engine
		// calls it with source only. Derive the expected result from the
		// identical tooling fixture by matching the source text.
		sourceFile := matchFixtureSource(t, request.Source)
		results := make([]analyzer.Result, 0, len(request.BannedNames))
		for _, name := range request.BannedNames {
			constraint := "B1"
			if name == "malloc" {
				constraint = "B2"
			}
			want := bySource[sourceFile][constraint]
			result := analyzer.Result{Name: name, Verdict: analyzer.Verdict(want.Verdict)}
			for _, evidence := range want.Evidence {
				result.Evidence = append(result.Evidence, analyzer.Evidence{Line: evidence.Line, Text: evidence.Text})
			}
			results = append(results, result)
		}
		return analyzer.Report{Version: analyzer.ProtocolVersion, Results: results}, nil
	}}
	t.Setenv(EnvAnalyzerPath, "")
	semantic := &guardSemantic{t: t, forbid: true}
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	summary, err := RunContext(context.Background(), "testdata/banlist-traps", output, Options{
		ModelOff: true,
		Analyzer: hard,
		Semantic: semantic,
	})
	if err != nil {
		t.Fatalf("RunContext: %v", err)
	}
	if semantic.calls != 0 {
		t.Fatalf("MODEL_OFF dispatched %d semantic calls", semantic.calls)
	}
	got := readPredictions(t, output)
	index := byPair(got)
	for _, want := range expected {
		key := want.Solution + "\x00" + want.Constraint
		actual, ok := index[key]
		if !ok {
			t.Fatalf("missing prediction for %s", key)
		}
		if actual.Verdict != want.Verdict {
			t.Fatalf("%s verdict = %q, want %q", key, actual.Verdict, want.Verdict)
		}
		if want.Verdict == VerdictViolation {
			if len(actual.Evidence) != len(want.Evidence) {
				t.Fatalf("%s evidence = %v, want %v", key, actual.Evidence, want.Evidence)
			}
			for i, evidence := range want.Evidence {
				if actual.Evidence[i] != evidence {
					t.Fatalf("%s evidence[%d] = %+v, want %+v", key, i, actual.Evidence[i], evidence)
				}
			}
		}
	}
	if summary.Violation != 5 {
		t.Fatalf("violations = %d, want the 5 real uses", summary.Violation)
	}
	if summary.Unsure != 15 {
		t.Fatalf("unsure = %d, want 11 semantic plus 4 unparseable hard-rule pairs", summary.Unsure)
	}
}

// matchFixtureSource maps fixture source bytes back to the solution file base
// name without reading labels or expected verdicts.
func matchFixtureSource(t *testing.T, source string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("testdata", "banlist-traps", "problems", "trap-garden", "solutions"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("testdata", "banlist-traps", "problems", "trap-garden", "solutions", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) == source {
			return strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
	}
	t.Fatalf("no fixture source matches %q", source)
	return ""
}

func TestSemanticOnlyGoesToModelAfterPricedReservation(t *testing.T) {
	root := newDataset(t, twoBanConstraints, trapSource)
	hard := &guardAnalyzer{decide: passReport}
	semantic := &guardSemantic{t: t, result: model.Result{
		Constraints:  []model.ConstraintResult{{ConstraintID: "S1", Verdict: model.VerdictPass}},
		Attempts:     1,
		InputTokens:  100,
		OutputTokens: 10,
	}}
	store := newMemoryBudgetStore()
	spend := budget.NewService(store, budget.SystemClock{}, budget.PriceSchedule{InputCentsPer1K: 1, OutputCentsPer1K: 1})

	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	summary, err := RunContext(context.Background(), root, output, Options{
		Analyzer:        hard,
		Semantic:        semantic,
		Budget:          spend,
		MaxOutputTokens: 64,
	})
	if err != nil {
		t.Fatalf("RunContext: %v", err)
	}
	if semantic.calls != 1 {
		t.Fatalf("semantic calls = %d, want 1", semantic.calls)
	}
	if summary.ModelCalls != 1 {
		t.Fatalf("summary.ModelCalls = %d, want 1", summary.ModelCalls)
	}
	if len(semantic.requests) != 1 || len(semantic.requests[0].Constraints) != 1 || semantic.requests[0].Constraints[0].ID != "S1" {
		t.Fatalf("semantic request = %+v, want only the semantic constraint", semantic.requests)
	}
	for _, constraint := range semantic.requests[0].Constraints {
		if IsBanListConstraint(constraint.Text) {
			t.Fatalf("ban-list constraint %q reached the model", constraint.ID)
		}
	}
	committed, err := store.CommittedCents(context.Background(), "checker")
	if err != nil {
		t.Fatal(err)
	}
	if committed != 2 {
		t.Fatalf("committed cents = %d, want 2 (ceil 100/1000 + ceil 10/1000)", committed)
	}
}

func TestSemanticRefusesDispatchWithoutBudgetPriceOrPort(t *testing.T) {
	tests := []struct {
		name    string
		options func(*guardSemantic) Options
		cause   string
	}{
		{
			name: "no budget service",
			options: func(semantic *guardSemantic) Options {
				return Options{Analyzer: &guardAnalyzer{decide: passReport}, Semantic: semantic}
			},
			cause: CauseBudgetUnavailable,
		},
		{
			name: "unknown price",
			options: func(semantic *guardSemantic) Options {
				return Options{
					Analyzer: &guardAnalyzer{decide: passReport},
					Semantic: semantic,
					Budget:   budget.NewService(newMemoryBudgetStore(), budget.SystemClock{}, budget.PriceSchedule{}),
				}
			},
			cause: CauseBudgetUnknownPrice,
		},
		{
			name: "cap exceeded",
			options: func(semantic *guardSemantic) Options {
				spend := budget.NewService(newMemoryBudgetStore(), budget.SystemClock{}, budget.PriceSchedule{InputCentsPer1K: 10000, OutputCentsPer1K: 10000})
				spend.CapCents = 1
				return Options{Analyzer: &guardAnalyzer{decide: passReport}, Semantic: semantic, Budget: spend}
			},
			cause: CauseBudgetCapExceeded,
		},
		{
			name: "no semantic port",
			options: func(*guardSemantic) Options {
				return Options{Analyzer: &guardAnalyzer{decide: passReport}}
			},
			cause: CauseModelNotConfigured,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newDataset(t, twoBanConstraints, trapSource)
			semantic := &guardSemantic{t: t, forbid: true}
			output := filepath.Join(t.TempDir(), "predictions.jsonl")
			summary, err := RunContext(context.Background(), root, output, test.options(semantic))
			if err != nil {
				t.Fatalf("RunContext: %v", err)
			}
			if semantic.calls != 0 {
				t.Fatalf("semantic calls = %d, want 0", semantic.calls)
			}
			if summary.UnsureCauses[test.cause] != 1 {
				t.Fatalf("unsure causes = %v, want one %s", summary.UnsureCauses, test.cause)
			}
			predictions := byPair(readPredictions(t, output))
			if got := predictions["sample/s01\x00S1"]; got.Verdict != VerdictUnsure {
				t.Fatalf("semantic pair = %+v, want unsure", got)
			}
		})
	}
}

func TestUnsafeSanitizationProducesUnsureWithoutDispatch(t *testing.T) {
	unsafe := "int main() {\n  /* never closed\n  return 0;\n}\n"
	root := newDataset(t, twoBanConstraints, unsafe)
	semantic := &guardSemantic{t: t, forbid: true}
	store := newMemoryBudgetStore()
	spend := budget.NewService(store, budget.SystemClock{}, budget.PriceSchedule{InputCentsPer1K: 100, OutputCentsPer1K: 100})
	output := filepath.Join(t.TempDir(), "predictions.jsonl")

	summary, err := RunContext(context.Background(), root, output, Options{
		Analyzer: &guardAnalyzer{},
		Semantic: semantic,
		Budget:   spend,
	})
	if err != nil {
		t.Fatalf("RunContext: %v", err)
	}
	if semantic.calls != 0 {
		t.Fatal("unsafe sanitization dispatched a semantic request")
	}
	if summary.ModelCalls != 0 {
		t.Fatalf("summary.ModelCalls = %d, want 0", summary.ModelCalls)
	}
	if summary.UnsureCauses[CauseSanitizeUnsafe] != 1 {
		t.Fatalf("unsure causes = %v, want %s", summary.UnsureCauses, CauseSanitizeUnsafe)
	}
	if committed, _ := store.CommittedCents(context.Background(), "checker"); committed != 0 {
		t.Fatalf("committed cents = %d, want 0", committed)
	}
}

func TestOptionsFromEnvSelectsModelOffBeforeProviderConfiguration(t *testing.T) {
	t.Setenv("MODEL_OFF", "1")
	t.Setenv("MODEL_BASE_URL", "http://127.0.0.1:1/v1")
	t.Setenv("MODEL_NAME", "test-model")
	t.Setenv(model.EnvAPIKey, "must-not-be-used")
	t.Setenv(EnvAnalyzerPath, "/usr/local/bin/leetkob-analyzer")

	options := OptionsFromEnv()
	if !options.ModelOff {
		t.Fatal("MODEL_OFF=1 was not selected")
	}
	if options.Semantic != nil || options.Budget != nil {
		t.Fatal("MODEL_OFF=1 created a semantic port or budget service")
	}
	if options.Analyzer == nil {
		t.Fatal("MODEL_OFF=1 must still configure the bundled hard-rule analyzer")
	}
}

func TestModelOffReturnTolerance(t *testing.T) {
	// Errors from an already-selected offline port must not become a pass.
	semantic := &guardSemantic{t: t, err: model.ErrModelOff}
	root := newDataset(t, twoBanConstraints, trapSource)
	spend := budget.NewService(newMemoryBudgetStore(), budget.SystemClock{}, budget.PriceSchedule{InputCentsPer1K: 1, OutputCentsPer1K: 1})
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	summary, err := RunContext(context.Background(), root, output, Options{
		Analyzer: &guardAnalyzer{decide: passReport},
		Semantic: semantic,
		Budget:   spend,
	})
	if err != nil {
		t.Fatalf("RunContext: %v", err)
	}
	if semantic.calls != 1 {
		t.Fatalf("semantic calls = %d, want 1", semantic.calls)
	}
	if summary.UnsureCauses[CauseModelOffAfterSelect] != 1 {
		t.Fatalf("unsure causes = %v, want %s", summary.UnsureCauses, CauseModelOffAfterSelect)
	}
}

func TestRunContextHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := newDataset(t, twoBanConstraints, trapSource)
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	_, err := RunContext(ctx, root, output, Options{ModelOff: true})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
