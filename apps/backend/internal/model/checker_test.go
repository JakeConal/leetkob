package model_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/model"
)

func TestCheckValidPassViolationUnsure(t *testing.T) {
	const content = `{"results":[` +
		`{"constraintId":"C2","verdict":"violation","evidence":[{"line":3,"text":"std::sort"}]},` +
		`{"constraintId":"C3","verdict":"unsure","evidence":[]},` +
		`{"constraintId":"C1","verdict":"pass","evidence":[]}]}`

	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
			t.Errorf("authorization = %q, want bearer test key", got)
		}
		writeCompletion(t, w, "req-test", content)
	})
	checker := newChecker(t, server, model.Config{})

	result, err := checker.Check(context.Background(), testSourceRequest())
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if result.Cause != "" {
		t.Fatalf("cause = %q, want empty for a validated model response", result.Cause)
	}
	if result.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", result.Attempts)
	}
	if result.ProviderRequestID != "req-test" {
		t.Fatalf("provider request id = %q, want req-test", result.ProviderRequestID)
	}
	if result.Model != "test-model" {
		t.Fatalf("model = %q, want test-model", result.Model)
	}
	if result.InputTokens != 12 || result.OutputTokens != 7 {
		t.Fatalf("usage = %d/%d, want 12/7", result.InputTokens, result.OutputTokens)
	}
	if result.Elapsed <= 0 {
		t.Fatal("elapsed must be recorded")
	}
	if len(result.Constraints) != 3 {
		t.Fatalf("results = %d, want 3", len(result.Constraints))
	}

	// Results are reordered into request order regardless of model order.
	if got := result.Constraints[0]; got.ConstraintID != "C1" || got.Verdict != model.VerdictPass || len(got.Evidence) != 0 || got.Cause != "" {
		t.Fatalf("C1 result = %+v, want pass with no evidence and no cause", got)
	}
	if got := result.Constraints[1]; got.ConstraintID != "C2" || got.Verdict != model.VerdictViolation {
		t.Fatalf("C2 result = %+v, want violation", got)
	} else if len(got.Evidence) != 1 || got.Evidence[0].Line != 3 || got.Evidence[0].Text != "std::sort" {
		t.Fatalf("C2 evidence = %+v, want line 3 std::sort", got.Evidence)
	}
	if got := result.Constraints[2]; got.ConstraintID != "C3" || got.Verdict != model.VerdictUnsure {
		t.Fatalf("C3 result = %+v, want model-declared unsure", got)
	} else if got.Cause != "" {
		t.Fatalf("model-declared unsure cause = %q, want empty", got.Cause)
	}
}

func TestProviderRequestContainsOnlyAllowedFields(t *testing.T) {
	const content = `{"results":[{"constraintId":"C1","verdict":"pass","evidence":[]}]}`

	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}

		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode provider request: %v", err)
			return
		}
		allowed := []string{"model", "max_tokens", "temperature", "response_format", "messages"}
		if len(envelope) != len(allowed) {
			t.Errorf("provider request keys = %v, want exactly %v", keysOf(envelope), allowed)
		}
		for _, key := range allowed {
			if _, ok := envelope[key]; !ok {
				t.Errorf("provider request is missing %q", key)
			}
		}
		for _, forbidden := range []string{"deduction", "score", "grade", "mark", "tests", "labels"} {
			if strings.Contains(string(body), `"`+forbidden+`"`) {
				t.Errorf("provider request must not contain %q", forbidden)
			}
		}

		var messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(envelope["messages"], &messages); err != nil {
			t.Errorf("decode messages: %v", err)
			return
		}
		if len(messages) != 2 || messages[0].Role != "system" || messages[1].Role != "user" {
			t.Errorf("messages = %+v, want system then user", messages)
			return
		}

		var payload map[string]json.RawMessage
		if err := json.Unmarshal([]byte(messages[1].Content), &payload); err != nil {
			t.Errorf("decode prompt payload: %v", err)
			return
		}
		wantPayload := []string{"statement", "constraints", "source"}
		if len(payload) != len(wantPayload) {
			t.Errorf("prompt payload keys = %v, want exactly %v", keysOf(payload), wantPayload)
		}
		var constraints []map[string]json.RawMessage
		if err := json.Unmarshal(payload["constraints"], &constraints); err != nil {
			t.Errorf("decode constraints: %v", err)
			return
		}
		if len(constraints) != 1 || len(constraints[0]) != 2 {
			t.Errorf("constraint payload = %v, want only id and text", constraints)
		}
		var source string
		if err := json.Unmarshal(payload["source"], &source); err != nil || source != testSource {
			t.Errorf("source = %q (err %v), want the supplied sanitized source", source, err)
		}

		writeCompletion(t, w, "req-test", content)
	})
	checker := newChecker(t, server, model.Config{MaxOutputTokens: 256})

	if _, err := checker.Check(context.Background(), oneConstraintRequest()); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckRejectsInvalidModelOutput(t *testing.T) {
	validOne := `{"constraintId":"C1","verdict":"pass","evidence":[]}`
	twoConstraints := testSourceRequest()
	twoConstraints.Constraints = twoConstraints.Constraints[:2]

	cases := []struct {
		name      string
		request   model.Request
		content   string
		wantCause model.UnsureCause
		wantCalls int32
	}{
		{
			name:      "unknown score field",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"pass","evidence":[],"score":2}]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "unknown deduction field",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"pass","evidence":[],"deduction":4}]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "unknown grade field",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"pass","evidence":[],"grade":"A"}]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "unknown top-level field",
			request:   oneConstraintRequest(),
			content:   `{"results":[` + validOne + `],"score":2}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "duplicate constraint id",
			request:   oneConstraintRequest(),
			content:   `{"results":[` + validOne + `,` + validOne + `]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "missing constraint id",
			request:   twoConstraints,
			content:   `{"results":[` + validOne + `]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "unrequested constraint id",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C9","verdict":"pass","evidence":[]}]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "invalid verdict enum",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"maybe","evidence":[]}]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "wrong typed verdict",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":3,"evidence":[]}]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "missing evidence array",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"pass"}]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "wrong typed evidence line",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"violation","evidence":[{"line":"3","text":"std::sort"}]}]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "unknown evidence field",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"violation","evidence":[{"line":3,"text":"std::sort","score":1}]}]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "empty results",
			request:   oneConstraintRequest(),
			content:   `{"results":[]}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "null results",
			request:   oneConstraintRequest(),
			content:   `{"results":null}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "trailing json value",
			request:   oneConstraintRequest(),
			content:   `{"results":[` + validOne + `]}{}`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
		{
			name:      "fabricated line number",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"violation","evidence":[{"line":99,"text":"std::sort"}]}]}`,
			wantCause: model.CauseEvidenceInvalid,
			wantCalls: 1,
		},
		{
			name:      "line zero",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"violation","evidence":[{"line":0,"text":"std::sort"}]}]}`,
			wantCause: model.CauseEvidenceInvalid,
			wantCalls: 1,
		},
		{
			name:      "evidence text absent from the cited line",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"violation","evidence":[{"line":3,"text":"std::stable_sort"}]}]}`,
			wantCause: model.CauseEvidenceInvalid,
			wantCalls: 1,
		},
		{
			name:      "violation without evidence",
			request:   oneConstraintRequest(),
			content:   `{"results":[{"constraintId":"C1","verdict":"violation","evidence":[]}]}`,
			wantCause: model.CauseEvidenceInvalid,
			wantCalls: 1,
		},
		{
			name:      "non-object output",
			request:   oneConstraintRequest(),
			content:   `[{"constraintId":"C1"}]`,
			wantCause: model.CauseSchemaInvalid,
			wantCalls: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeCompletion(t, w, "req-test", tc.content)
			})
			checker := newChecker(t, server, model.Config{})

			result, err := checker.Check(context.Background(), tc.request)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if result.Cause != tc.wantCause {
				t.Fatalf("cause = %q, want %q", result.Cause, tc.wantCause)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Fatalf("provider calls = %d, want %d", got, tc.wantCalls)
			}
			if result.Attempts != int(tc.wantCalls) {
				t.Fatalf("attempts = %d, want %d", result.Attempts, tc.wantCalls)
			}
			if len(result.Constraints) != len(tc.request.Constraints) {
				t.Fatalf("results = %d, want %d", len(result.Constraints), len(tc.request.Constraints))
			}
			for _, constraint := range result.Constraints {
				if constraint.Verdict != model.VerdictUnsure {
					t.Fatalf("constraint %q verdict = %q, want unsure", constraint.ConstraintID, constraint.Verdict)
				}
				if constraint.Cause != tc.wantCause {
					t.Fatalf("constraint %q cause = %q, want %q", constraint.ConstraintID, constraint.Cause, tc.wantCause)
				}
			}
		})
	}
}

func TestCheckRepairsOneSchemaFailure(t *testing.T) {
	var calls atomic.Int32
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempt := calls.Add(1)
		if attempt == 1 {
			writeCompletion(t, w, "req-1", `{"results":[]}`)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read repair body: %v", err)
			return
		}
		var envelope struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode repair body: %v", err)
			return
		}
		if len(envelope.Messages) != 4 {
			t.Errorf("repair messages = %d, want 4", len(envelope.Messages))
		} else {
			last := envelope.Messages[len(envelope.Messages)-1]
			if last.Role != "user" || !strings.Contains(last.Content, "corrected JSON object") {
				t.Errorf("repair instruction = %+v, want a corrected-object request", last)
			}
		}
		writeCompletion(t, w, "req-2", `{"results":[{"constraintId":"C1","verdict":"pass","evidence":[]}]}`)
	})
	checker := newChecker(t, server, model.Config{})

	result, err := checker.Check(context.Background(), oneConstraintRequest())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want exactly 2", calls.Load())
	}
	if result.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", result.Attempts)
	}
	if result.Cause != "" {
		t.Fatalf("cause = %q, want empty after a successful repair", result.Cause)
	}
	if got := result.Constraints[0]; got.Verdict != model.VerdictPass {
		t.Fatalf("repaired verdict = %q, want pass", got.Verdict)
	}
	if result.ProviderRequestID != "req-2" {
		t.Fatalf("provider request id = %q, want req-2", result.ProviderRequestID)
	}
}

func TestCheckStopsAfterOneRepair(t *testing.T) {
	var calls atomic.Int32
	server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeCompletion(t, w, "req-test", `{"results":[]}`)
	})
	checker := newChecker(t, server, model.Config{})

	result, err := checker.Check(context.Background(), testSourceRequest())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want exactly 2 (one repair)", calls.Load())
	}
	if result.Cause != model.CauseSchemaInvalid {
		t.Fatalf("cause = %q, want schema-invalid", result.Cause)
	}
	if result.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", result.Attempts)
	}
}

func TestCheckTimeoutBecomesUnsureWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	server := newServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		// Outlast the configured deadline. The server is not expected to
		// observe the client cancellation before this timer fires.
		time.Sleep(500 * time.Millisecond)
	})
	checker := newChecker(t, server, model.Config{Timeout: 150 * time.Millisecond})

	started := time.Now()
	result, err := checker.Check(context.Background(), testSourceRequest())
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("check took %s, want the configured 50ms deadline", elapsed)
	}
	if result.Cause != model.CauseTimeout {
		t.Fatalf("cause = %q, want timeout", result.Cause)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1 (timeouts are not retried)", calls.Load())
	}
	if result.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", result.Attempts)
	}
	for _, constraint := range result.Constraints {
		if constraint.Verdict != model.VerdictUnsure {
			t.Fatalf("constraint %q verdict = %q, want unsure", constraint.ConstraintID, constraint.Verdict)
		}
	}
}

func TestCheckProviderStatusBecomesUnsure(t *testing.T) {
	var calls atomic.Int32
	server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	})
	checker := newChecker(t, server, model.Config{})

	result, err := checker.Check(context.Background(), testSourceRequest())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Cause != model.CauseProviderFailure {
		t.Fatalf("cause = %q, want provider-failure", result.Cause)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", calls.Load())
	}
}

func TestCheckMalformedProviderEnvelopeBecomesUnsure(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"req-test"}`)
	})
	checker := newChecker(t, server, model.Config{})

	result, err := checker.Check(context.Background(), testSourceRequest())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Cause != model.CauseProviderFailure {
		t.Fatalf("cause = %q, want provider-failure", result.Cause)
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (function doerFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestCheckTransportErrorBecomesUnsure(t *testing.T) {
	var calls atomic.Int32
	checker, err := model.NewHTTPChecker(
		model.Config{
			BaseURL:         "http://127.0.0.1:1",
			Model:           "test-model",
			Timeout:         2 * time.Second,
			MaxOutputTokens: 64,
		},
		model.WithHTTPClient(doerFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("fake transport refused the connection")
		})),
		model.WithAPIKeySource(func() (string, error) { return testAPIKey, nil }),
	)
	if err != nil {
		t.Fatalf("new checker: %v", err)
	}

	result, err := checker.Check(context.Background(), testSourceRequest())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Cause != model.CauseProviderFailure {
		t.Fatalf("cause = %q, want provider-failure", result.Cause)
	}
	if calls.Load() != 1 {
		t.Fatalf("dispatch attempts = %d, want 1", calls.Load())
	}
}

func TestCanceledContextReturnsError(t *testing.T) {
	var calls atomic.Int32
	server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeCompletion(t, w, "req-test", `{"results":[]}`)
	})
	checker := newChecker(t, server, model.Config{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := checker.Check(ctx, testSourceRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("provider calls = %d, want 0", calls.Load())
	}
}

func keysOf(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
