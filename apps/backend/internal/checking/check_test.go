package checking

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/checker/hardrule"
	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

const pipelineSource = "int main() { // cohort\n  std::sort(a, a + n);\n}\n"

func TestNonAcceptedSubmissionMakesNoModelCallAndStoresNothing(t *testing.T) {
	cases := []struct {
		name    string
		verdict string
		counted bool
	}{
		{name: "wrong answer", verdict: "WA", counted: true},
		{name: "not counted", verdict: "AC", counted: false},
		{name: "lowercase is not AC", verdict: "ac", counted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryStore{sub: baseSubmission()}
			store.sub.JudgeVerdict = tc.verdict
			store.sub.Counted = tc.counted
			model := &scriptedModel{responses: []semantic.ModelResponse{{Content: `{"results":[]}`, TokensKnown: true}}}
			analyzerCalls := 0
			outcome, err := service(store, model, func(context.Context, string) ([]byte, error) {
				analyzerCalls++
				return []byte(`{"version":1,"status":"ok","references":[]}`), nil
			}).Run(context.Background(), store.sub.ID)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Persisted || outcome.ModelCalls != 0 || len(outcome.Findings) != 0 || model.calls != 0 || analyzerCalls != 0 || store.saveCalls != 0 {
				t.Fatalf("outcome %+v model %d analyzer %d saves %d", outcome, model.calls, analyzerCalls, store.saveCalls)
			}
		})
	}
}

func TestHardRuleDecisionDoesNotCallTheModel(t *testing.T) {
	store := &memoryStore{sub: baseSubmission()}
	store.sub.Constraints = []Constraint{{
		ID: "ban-sort", Text: "Do not use `std::sort`.", DeductionHundredths: 300, Position: 1,
	}}
	model := &scriptedModel{responses: []semantic.ModelResponse{{Content: `{"score":9}`, TokensKnown: true}}}
	raw := analyzerDocument(t, hardrule.Ref{
		Kind: "call", Name: "sort", QualifiedName: "std::sort", Header: "algorithm", Resolved: true, Line: 2, Column: 3,
	})
	outcome, err := service(store, model, func(context.Context, string) ([]byte, error) {
		return raw, nil
	}).Run(context.Background(), store.sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 0 || outcome.ModelCalls != 0 || !outcome.Persisted || len(store.saved) != 1 {
		t.Fatalf("calls %d outcome %+v saved %d", model.calls, outcome, len(store.saved))
	}
	got := store.saved[0]
	if got.SourceKind != SourceHardRule || got.Verdict != VerdictViolation || got.DeductionHundredths != 300 || got.Revision != 1 || !got.RequiredReview {
		t.Fatalf("finding = %+v", got)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Line != 2 || got.Evidence[0].Text != "  std::sort(a, a + n);" {
		t.Fatalf("evidence = %+v, want the original line", got.Evidence)
	}
}

func TestHardRuleUnsureDoesNotCallTheModel(t *testing.T) {
	store := &memoryStore{sub: baseSubmission()}
	store.sub.Constraints = []Constraint{{
		ID: "ban-sort", Text: "Do not use `std::sort`.", DeductionHundredths: 300, Position: 1,
	}}
	model := &scriptedModel{}
	outcome, err := service(store, model, func(context.Context, string) ([]byte, error) {
		return []byte("not-json"), nil
	}).Run(context.Background(), store.sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 0 || !outcome.Persisted || len(store.saved) != 1 {
		t.Fatalf("calls %d saved %+v", model.calls, store.saved)
	}
	got := store.saved[0]
	if got.Verdict != VerdictUnsure || got.UnsureCause != "analyzer-result" || got.DeductionHundredths != 300 || len(got.Evidence) != 0 {
		t.Fatalf("finding = %+v", got)
	}
}

func TestFakeModelSemanticFindingKeepsConfiguredDeduction(t *testing.T) {
	store := &memoryStore{sub: baseSubmission()}
	model := &scriptedModel{responses: []semantic.ModelResponse{{
		Content:     `{"results":[{"constraintId":"prints","verdict":"violation","evidence":[{"line":1,"text":"int main() { "}]}]}`,
		TokensKnown: true, InputTokens: 10, OutputTokens: 5, RequestID: "fake-1",
	}}}
	raw := analyzerDocument(t, hardrule.Ref{
		Kind: "call", Name: "sort", QualifiedName: "std::sort", Header: "algorithm", Resolved: true, Line: 2, Column: 3,
	})
	outcome, err := service(store, model, func(context.Context, string) ([]byte, error) {
		return raw, nil
	}).Run(context.Background(), store.sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 1 || outcome.ModelCalls != 1 || len(store.saved) != 2 {
		t.Fatalf("calls %d saved %d", model.calls, len(store.saved))
	}
	var payload struct {
		Constraints []struct {
			ID string `json:"id"`
		} `json:"constraints"`
	}
	if err := json.Unmarshal([]byte(model.users[0]), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Constraints) != 1 || payload.Constraints[0].ID != "prints" || jsonContains(model.users[0], "Do not use") {
		t.Fatalf("model saw the hard-rule constraint: %s", model.users[0])
	}
	hard := store.saved[0]
	semanticFinding := store.saved[1]
	if hard.ConstraintID != "ban-sort" || hard.SourceKind != SourceHardRule || hard.Verdict != VerdictViolation {
		t.Fatalf("hard finding = %+v", hard)
	}
	if hard.Evidence[0].Text != "  std::sort(a, a + n);" || hard.DeductionHundredths != 300 {
		t.Fatalf("hard evidence/deduction = %+v", hard)
	}
	if semanticFinding.SourceKind != SourceSemantic || semanticFinding.Verdict != VerdictViolation || semanticFinding.DeductionHundredths != 150 {
		t.Fatalf("semantic finding = %+v", semanticFinding)
	}
	if len(semanticFinding.Evidence) != 1 || semanticFinding.Evidence[0].Text != "int main() { // cohort" {
		t.Fatalf("stored model text instead of the original line: %+v", semanticFinding.Evidence)
	}
	if !hard.RequiredReview || !semanticFinding.RequiredReview {
		t.Fatal("violations must be required")
	}
}

func TestInvalidModelScoreDoesNotSetTheDeduction(t *testing.T) {
	store := &memoryStore{sub: baseSubmission()}
	store.sub.Constraints = []Constraint{{
		ID: "prints", Text: "The program prints a sorted line.", DeductionHundredths: 150, Position: 1,
	}}
	model := &scriptedModel{responses: []semantic.ModelResponse{{
		Content: `{"score":9}`, TokensKnown: true, InputTokens: 1, OutputTokens: 1,
	}}}
	outcome, err := service(store, model, func(context.Context, string) ([]byte, error) {
		return []byte(`{"version":1,"status":"ok","references":[]}`), nil
	}).Run(context.Background(), store.sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 2 || len(store.saved) != 1 {
		t.Fatalf("calls %d saved %+v", model.calls, store.saved)
	}
	got := store.saved[0]
	if got.Verdict != VerdictUnsure || got.UnsureCause != string(semantic.CauseSchema) || got.DeductionHundredths != 150 {
		t.Fatalf("finding = %+v", got)
	}
	if outcome.Findings[0].DeductionHundredths == 900 {
		t.Fatal("model score became the deduction")
	}
}

func TestElevenPassesSampleTwoAndDoNotCallTheModel(t *testing.T) {
	store := &memoryStore{sub: baseSubmission()}
	store.sub.Constraints = nil
	for i := 0; i < 11; i++ {
		store.sub.Constraints = append(store.sub.Constraints, Constraint{
			ID: "p" + string(rune('a'+i)), Text: "Do not use `missing`.", DeductionHundredths: 10, Position: i + 1,
		})
	}
	model := &scriptedModel{}
	outcome, err := service(store, model, func(context.Context, string) ([]byte, error) {
		return []byte(`{"version":1,"status":"ok","references":[]}`), nil
	}).Run(context.Background(), store.sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 0 || len(store.saved) != 11 {
		t.Fatalf("calls %d saved %d", model.calls, len(store.saved))
	}
	required := 0
	for _, finding := range store.saved {
		if finding.Verdict != VerdictPass || finding.SourceKind != SourceHardRule {
			t.Fatalf("finding = %+v", finding)
		}
		if finding.RequiredReview {
			required++
		}
	}
	if required != 2 || outcome.ModelCalls != 0 {
		t.Fatalf("required passes = %d, want 2", required)
	}
}

func TestStaleRevisionIsNotStored(t *testing.T) {
	store := &memoryStore{sub: baseSubmission(), stale: true}
	model := &scriptedModel{responses: []semantic.ModelResponse{{
		Content:     `{"results":[{"constraintId":"prints","verdict":"pass","evidence":[]}]}`,
		TokensKnown: true, InputTokens: 1, OutputTokens: 1,
	}}}
	raw := analyzerDocument(t, hardrule.Ref{
		Kind: "call", Name: "sort", QualifiedName: "std::sort", Header: "algorithm", Resolved: true, Line: 2, Column: 3,
	})
	outcome, err := service(store, model, func(context.Context, string) ([]byte, error) {
		return raw, nil
	}).Run(context.Background(), store.sub.ID)
	if !errors.Is(err, ErrStale) || !outcome.Stale || outcome.Persisted || len(store.saved) != 0 {
		t.Fatalf("err %v outcome %+v saved %d", err, outcome, len(store.saved))
	}
	if model.calls != 1 {
		t.Fatalf("semantic call count = %d, want 1 before the stale save", model.calls)
	}
}

func TestSourceHashMismatchDoesNotCallTheModel(t *testing.T) {
	store := &memoryStore{sub: baseSubmission()}
	store.sub.SourceSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	model := &scriptedModel{}
	calls := 0
	_, err := service(store, model, func(context.Context, string) ([]byte, error) {
		calls++
		return nil, errors.New("analyzer should not run")
	}).Run(context.Background(), store.sub.ID)
	if err == nil || calls != 0 || model.calls != 0 || store.saveCalls != 0 {
		t.Fatalf("err %v analyzer %d model %d saves %d", err, calls, model.calls, store.saveCalls)
	}
}

func TestAnalyzeSourceRejectsAnEmptyBinary(t *testing.T) {
	_, err := AnalyzeSource(context.Background(), "", nil, "int main(){return 0;}\n")
	if err == nil {
		t.Fatal("empty analyzer binary must fail before a process starts")
	}
}

func service(store *memoryStore, model *scriptedModel, analyze AnalyzeFunc) *Service {
	return &Service{
		Store:   store,
		Analyze: analyze,
		Checker: &semantic.Checker{Model: model, Store: semantic.NewMemoryStore(), Timeout: time.Second},
		Price: semantic.Price{
			Known: true, MaxInputTokens: 100, MaxOutputTokens: 20,
			InputMicroUSDPerToken: 1, OutputMicroUSDPerToken: 1,
		},
	}
}

func baseSubmission() Submission {
	source := pipelineSource
	return Submission{
		ID: "sub-1", LabID: "lab-1", AuthorUserID: "author-1", Source: source, SourceSHA256: SourceHash(source),
		JudgeVerdict: "AC", Generation: 1, TestRevision: 1, ConstraintRevision: 1, Counted: true,
		Statement: "Print a sorted line.",
		Constraints: []Constraint{
			{ID: "ban-sort", Text: "Do not use `std::sort`.", DeductionHundredths: 300, Position: 1},
			{ID: "prints", Text: "The program prints a sorted line.", DeductionHundredths: 150, Position: 2},
		},
	}
}

func analyzerDocument(t *testing.T, refs ...hardrule.Ref) []byte {
	t.Helper()
	if refs == nil {
		refs = []hardrule.Ref{}
	}
	raw, err := json.Marshal(hardrule.Result{Version: 1, Status: "ok", References: refs})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func jsonContains(raw, needle string) bool {
	return len(raw) >= len(needle) && (raw == needle || len(needle) == 0 || contains(raw, needle))
}

func contains(raw, needle string) bool {
	return len(raw) >= len(needle) && (func() bool { return stringIndex(raw, needle) >= 0 })()
}

func stringIndex(raw, needle string) int {
	for i := 0; i+len(needle) <= len(raw); i++ {
		if raw[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

type memoryStore struct {
	sub       Submission
	saved     []Finding
	saveCalls int
	stale     bool
}

func (m *memoryStore) Load(context.Context, string) (Submission, error) {
	return m.sub, nil
}

func (m *memoryStore) Save(_ context.Context, _ Submission, findings []Finding) error {
	m.saveCalls++
	if m.stale {
		return ErrStale
	}
	m.saved = append([]Finding(nil), findings...)
	return nil
}

type scriptedModel struct {
	responses []semantic.ModelResponse
	calls     int
	users     []string
}

func (m *scriptedModel) Complete(_ context.Context, req semantic.CompletionRequest) (semantic.ModelResponse, error) {
	m.calls++
	m.users = append(m.users, req.UserJSON)
	if len(m.responses) == 0 {
		return semantic.ModelResponse{}, errors.New("no scripted response")
	}
	resp := m.responses[0]
	if len(m.responses) > 1 {
		m.responses = m.responses[1:]
	}
	return resp, nil
}
