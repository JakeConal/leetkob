package semantic

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

// The schema-failure and timeout tests are the two limits from issue #38
// task 5.3. They use a fake model and must stay free of network calls.

func TestInvalidSchemaBecomesUnsureWithoutUnboundedRetry(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	fake := &scriptedModel{responses: []ModelResponse{
		{
			Content:      `{"results":[{"constraintId":"c1","verdict":"pass","evidence":[],"score":9}],"deduction":1}`,
			TokensKnown:  true,
			InputTokens:  3,
			OutputTokens: 1,
		},
		{Content: `{"grade":100}`, TokensKnown: true, InputTokens: 3, OutputTokens: 1},
		{Content: `{"results":[]}`, TokensKnown: true, InputTokens: 1, OutputTokens: 1},
	}}
	res, err := (&Checker{Model: fake, Store: store, Timeout: time.Second}).Check(context.Background(), sampleInput())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := fake.calls(); got != 2 {
		t.Fatalf("model calls = %d, want 2 (one schema-repair retry, never a third)", got)
	}
	if res.ModelCalls != 2 {
		t.Fatalf("ModelCalls = %d, want 2", res.ModelCalls)
	}
	assertUnsure(t, res, "c1", CauseSchema)
	if !fake.got[1].Repair || fake.got[0].Repair {
		t.Fatalf("repair flags = %v, %v, want false then true", fake.got[0].Repair, fake.got[1].Repair)
	}
	assertCharged(t, store, "lab-1", 8)
}

func TestTimeoutBecomesUnsureWithoutNetwork(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	fake := &scriptedModel{block: true}
	start := time.Now()
	res, err := (&Checker{
		Model:   fake,
		Store:   store,
		Timeout: 30 * time.Millisecond,
	}).Check(context.Background(), sampleInput())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("deadline was not enforced: took %s", elapsed)
	}
	if got := fake.calls(); got != 1 {
		t.Fatalf("model calls = %d, want 1 (timeout is not a schema retry)", got)
	}
	assertUnsure(t, res, "c1", CauseTimeout)
	// Usage never arrived, so the conservative reservation stays charged.
	assertCharged(t, store, "lab-1", samplePrice().mustMax())
}

func TestDuplicateMissingAndUnknownConstraintsFailSchema(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	in := sampleInput()
	in.Constraints = []Constraint{{ID: "c1", Text: "keep it linear"}, {ID: "c2", Text: "free allocations"}}
	fake := &scriptedModel{responses: []ModelResponse{
		{Content: `{"results":[{"constraintId":"c1","verdict":"pass","evidence":[]},{"constraintId":"c1","verdict":"violation","evidence":[{"line":1,"text":"int main(){return 0;}"}]}]}`, TokensKnown: true, InputTokens: 1, OutputTokens: 1},
		{Content: `{"results":[{"constraintId":"c9","verdict":"pass","evidence":[]}]}`, TokensKnown: true, InputTokens: 1, OutputTokens: 1},
		{Content: passJSON("c1", "c2"), TokensKnown: true, InputTokens: 1, OutputTokens: 1},
	}}
	res, err := (&Checker{Model: fake, Store: store, Timeout: time.Second}).Check(context.Background(), in)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if fake.calls() != 2 {
		t.Fatalf("calls = %d, want 2", fake.calls())
	}
	if len(res.Findings) != 2 {
		t.Fatalf("findings = %d, want one per requested constraint", len(res.Findings))
	}
	for _, f := range res.Findings {
		if f.Verdict != VerdictUnsure || f.UnsureCause != CauseSchema {
			t.Fatalf("finding %+v, want unsure schema-invalid", f)
		}
	}
}

func TestFabricatedEvidenceBecomesUnsure(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	fake := &scriptedModel{responses: []ModelResponse{
		{Content: `{"results":[{"constraintId":"c1","verdict":"violation","evidence":[{"line":1,"text":"not in the original source"},{"line":40,"text":"int main(){return 0;}"}]}]}`, TokensKnown: true, InputTokens: 2, OutputTokens: 2},
		{Content: `{"results":[{"constraintId":"c1","verdict":"violation","evidence":[{"line":1,"text":"still invented"}]}]}`, TokensKnown: true, InputTokens: 2, OutputTokens: 2},
	}}
	res, err := (&Checker{Model: fake, Store: store, Timeout: time.Second}).Check(context.Background(), sampleInput())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if fake.calls() != 2 {
		t.Fatalf("calls = %d, want 2", fake.calls())
	}
	assertUnsure(t, res, "c1", CauseEvidence)
}

func TestSchemaRepairAcceptsTheSecondReply(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	fake := &scriptedModel{responses: []ModelResponse{
		{Content: `not json`, TokensKnown: true, InputTokens: 4, OutputTokens: 0},
		{Content: passJSON("c1"), TokensKnown: true, InputTokens: 4, OutputTokens: 1},
	}}
	res, err := (&Checker{Model: fake, Store: store, Timeout: time.Second}).Check(context.Background(), sampleInput())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if fake.calls() != 2 {
		t.Fatalf("calls = %d, want 2", fake.calls())
	}
	if len(res.Findings) != 1 || res.Findings[0].Verdict != VerdictPass || res.Findings[0].UnsureCause != "" {
		t.Fatalf("finding = %+v, want pass", res.Findings)
	}
	if len(res.Findings[0].Evidence) != 0 {
		t.Fatalf("pass evidence = %+v, want none", res.Findings[0].Evidence)
	}
}

func TestProviderFailureIsNotASchemaRetry(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	fake := &scriptedModel{errs: []error{errors.New("provider status 503")}}
	res, err := (&Checker{Model: fake, Store: store, Timeout: time.Second}).Check(context.Background(), sampleInput())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if fake.calls() != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls())
	}
	assertUnsure(t, res, "c1", CauseProvider)
	assertCharged(t, store, "lab-1", samplePrice().mustMax())
}

type scriptedModel struct {
	mu        sync.Mutex
	responses []ModelResponse
	errs      []error
	block     bool
	got       []CompletionRequest
}

func (m *scriptedModel) Complete(ctx context.Context, req CompletionRequest) (ModelResponse, error) {
	m.mu.Lock()
	m.got = append(m.got, req)
	n := len(m.got)
	m.mu.Unlock()
	if m.block {
		<-ctx.Done()
		return ModelResponse{}, ctx.Err()
	}
	if n-1 < len(m.errs) && m.errs[n-1] != nil {
		return ModelResponse{}, m.errs[n-1]
	}
	if n-1 >= len(m.responses) {
		return ModelResponse{}, errors.New("unexpected extra model call")
	}
	return m.responses[n-1], nil
}

func (m *scriptedModel) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.got)
}

func (m *scriptedModel) requests() []CompletionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CompletionRequest, len(m.got))
	copy(out, m.got)
	return out
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func blockNetwork(t *testing.T) {
	t.Helper()
	prev := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s", r.URL)
		return nil, errors.New("network disabled")
	})
	t.Cleanup(func() { http.DefaultTransport = prev })
}

func assertUnsure(t *testing.T, res Result, id string, cause UnsureCause) {
	t.Helper()
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %+v, want one", res.Findings)
	}
	f := res.Findings[0]
	if f.ConstraintID != id || f.Verdict != VerdictUnsure || f.UnsureCause != cause || len(f.Evidence) != 0 {
		t.Fatalf("finding = %+v, want %s unsure %s with no evidence", f, id, cause)
	}
}

func assertCharged(t *testing.T, store *MemoryStore, lab string, want int64) {
	t.Helper()
	got, err := store.Charged(context.Background(), lab)
	if err != nil {
		t.Fatalf("Charged: %v", err)
	}
	if got != want {
		t.Fatalf("charged = %d, want %d", got, want)
	}
}
