package checker_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/JakeConal/leetkob/backend/internal/checker"
	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

// fakeSemantic records the dispatched input and returns fixed findings. It
// proves the offline path never needs a provider.
type fakeSemantic struct {
	inputs   []semantic.Input
	findings []semantic.Finding
	err      error
}

func (f *fakeSemantic) Check(_ context.Context, in semantic.Input) (semantic.Result, error) {
	f.inputs = append(f.inputs, in)
	if f.err != nil {
		return semantic.Result{}, f.err
	}
	return semantic.Result{Findings: f.findings, ModelCalls: 1}, nil
}

func TestRunUsesSemanticEngineOnlyForSemanticPairs(t *testing.T) {
	root := writeDataset(t, 4)
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	engine := &fakeSemantic{findings: []semantic.Finding{
		{ConstraintID: "C2", Verdict: semantic.VerdictViolation, Evidence: []semantic.Evidence{{Line: 2, Text: "#include <vector>"}}},
	}}

	summary, err := checker.Run(context.Background(), checker.RunOptions{
		Root: root, Predictions: output, Engine: fakeEngine{referenceLine: 4}, Semantic: engine,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if summary.Predictions != 4 {
		t.Fatalf("predictions = %d", summary.Predictions)
	}
	// One semantic dispatch per solution; ban-list pairs never reach it.
	if len(engine.inputs) != 2 {
		t.Fatalf("semantic dispatches = %d, want 2 (one per solution)", len(engine.inputs))
	}
	for _, input := range engine.inputs {
		for _, constraint := range input.Constraints {
			if constraint.ID == "C1" {
				t.Fatal("ban-list constraint C1 must never be sent to the semantic engine")
			}
		}
		if input.Statement == "" {
			t.Fatal("semantic input must carry the statement")
		}
	}

	predictions := readPredictions(t, output)
	byKey := map[string]schema.Prediction{}
	for _, prediction := range predictions {
		byKey[prediction.Solution+"::"+prediction.Constraint] = prediction
	}
	if got := byKey["p1/a::C2"]; got.Verdict != schema.VerdictViolation || len(got.Evidence) != 1 || got.Evidence[0].Line != 2 {
		t.Fatalf("semantic violation = %+v", got)
	}
	if got := byKey["p1/b::C2"]; got.Verdict != schema.VerdictViolation {
		t.Fatalf("semantic pass mapping = %+v", got)
	}
	if got := byKey["p1/a::C1"]; got.Verdict != schema.VerdictViolation {
		t.Fatalf("hard-rule pair = %+v", got)
	}
}

func TestRunSemanticFailureBecomesUnsure(t *testing.T) {
	root := writeDataset(t, 4)
	output := filepath.Join(t.TempDir(), "predictions.jsonl")
	engine := &fakeSemantic{err: errors.New("provider down")}

	if _, err := checker.Run(context.Background(), checker.RunOptions{
		Root: root, Predictions: output, Engine: fakeEngine{referenceLine: 4}, Semantic: engine,
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, prediction := range readPredictions(t, output) {
		if prediction.Constraint == "C2" && prediction.Verdict != schema.VerdictUnsure {
			t.Fatalf("semantic pair after provider failure = %s, want unsure", prediction.Verdict)
		}
	}
}

func TestSemanticEngineDispatchesNothingUnderModelOff(t *testing.T) {
	t.Setenv("MODEL_OFF", "1")
	t.Setenv("MODEL_URL", "https://example.invalid/v1/chat/completions")
	t.Setenv("MODEL_NAME", "test-model")
	t.Setenv("MODEL_TOKEN", "not-a-real-token")
	engine, err := checker.NewSemanticEngineFromEnv()
	if err != nil || engine != nil {
		t.Fatalf("MODEL_OFF engine = %v, %v; want nil, nil", engine, err)
	}
}

func TestSemanticEngineRequiresConfiguration(t *testing.T) {
	t.Setenv("CHECKER_ISOLATED_ANALYZER", "1")
	t.Setenv("MODEL_OFF", "")
	t.Setenv("MODEL_URL", "")
	t.Setenv("MODEL_NAME", "")
	t.Setenv("MODEL_TOKEN", "")
	if _, err := checker.NewSemanticEngineFromEnv(); err == nil {
		t.Fatal("missing configuration must be refused")
	}
}

func TestLiveSemanticEngineRefusesCredentialBearingNativeAnalysis(t *testing.T) {
	t.Setenv("MODEL_OFF", "0")
	t.Setenv("MODEL_URL", "https://fixture.invalid/v1/chat")
	t.Setenv("MODEL_NAME", "synthetic-model")
	t.Setenv("MODEL_TOKEN", "SYNTHETIC-NOT-A-LIVE-TOKEN")
	t.Setenv("CHECKER_ISOLATED_ANALYZER", "")
	if _, err := checker.NewSemanticEngineFromEnv(); err == nil {
		t.Fatal("live mode accepted native parsing beside provider credentials")
	}
}

func TestLiveAnalyzerSelectsIsolationWithoutInvokingNativeTools(t *testing.T) {
	t.Setenv("MODEL_OFF", "0")
	t.Setenv("CHECKER_ISOLATED_ANALYZER", "1")
	t.Setenv("CHECKER_ANALYZER", "/must-not-execute-native-analyzer")
	t.Setenv("PATH", t.TempDir())
	engine, err := checker.NewNativeAnalyzer(context.Background())
	if err != nil || engine == nil || engine.Isolated == nil || engine.Binary != "" || len(engine.CompilerArgs) != 0 {
		t.Fatalf("live analysis did not stay isolated: engine=%+v err=%v", engine, err)
	}
}

func TestIsolatedAnalysisFailureNeverFallsBackToNativeParsing(t *testing.T) {
	calls := 0
	denied := errors.New("fixture isolation failure")
	engine := &checker.NativeAnalyzer{Binary: "/must-not-execute-native-analyzer", Isolated: func(context.Context, string) ([]byte, error) {
		calls++
		return nil, denied
	}}
	if _, err := engine.Analyze(context.Background(), "int main(){}"); !errors.Is(err, denied) || calls != 1 {
		t.Fatalf("isolation failure fell through: calls=%d err=%v", calls, err)
	}
}

func TestLiveSemanticEngineRequiresExplicitKnownPrices(t *testing.T) {
	t.Setenv("MODEL_OFF", "0")
	t.Setenv("CHECKER_ISOLATED_ANALYZER", "1")
	t.Setenv("MODEL_URL", "https://fixture.invalid/v1/chat")
	t.Setenv("MODEL_NAME", "synthetic-model")
	t.Setenv("MODEL_TOKEN", "SYNTHETIC-NOT-A-LIVE-TOKEN")
	t.Setenv("MODEL_INPUT_MICRO_USD_PER_TOKEN", "")
	t.Setenv("MODEL_OUTPUT_MICRO_USD_PER_TOKEN", "")
	if _, err := checker.NewSemanticEngineFromEnv(); err == nil {
		t.Fatal("missing price silently became a free schedule")
	}
	t.Setenv("MODEL_INPUT_MICRO_USD_PER_TOKEN", "0")
	t.Setenv("MODEL_OUTPUT_MICRO_USD_PER_TOKEN", "0")
	if _, err := checker.NewSemanticEngineFromEnv(); err != nil {
		t.Fatalf("explicit free-tier price was rejected: %v", err)
	}
}
