package model_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/model"
)

const testAPIKey = "test-provider-key"

// testSource has five original lines, including a trailing newline that
// must not create a phantom sixth line.
const testSource = "#include <algorithm>\nint main() {\n  std::sort(a, a + n);\n  return 0;\n}\n"

// blockedTransport is the package-wide network guard: any test that
// forgets to inject a fake transport fails instead of reaching a provider.
type blockedTransport struct{}

func (blockedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("model tests must not use the network: %s", request.URL.Redacted())
}

func TestMain(m *testing.M) {
	original := http.DefaultTransport
	http.DefaultTransport = blockedTransport{}
	// Tests that exercise offline mode set MODEL_OFF explicitly; an
	// ambient offline selection must not silently stop dispatch tests.
	_ = os.Unsetenv("MODEL_OFF")
	code := m.Run()
	http.DefaultTransport = original
	os.Exit(code)
}

// guardDoer only permits the local httptest provider configured for the
// current test. Any other destination fails the test immediately.
type guardDoer struct {
	t    *testing.T
	base string
	next model.Doer
}

func (guard guardDoer) Do(request *http.Request) (*http.Response, error) {
	if !strings.HasPrefix(request.URL.String(), guard.base) {
		guard.t.Errorf("test attempted a network request outside the fake provider: %s", request.URL)
		return nil, errors.New("test network access blocked")
	}
	return guard.next.Do(request)
}

func newServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func newChecker(t *testing.T, server *httptest.Server, cfg model.Config) *model.HTTPChecker {
	t.Helper()
	if cfg.BaseURL == "" {
		cfg.BaseURL = server.URL
	}
	if cfg.Model == "" {
		cfg.Model = "test-model"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.MaxOutputTokens == 0 {
		cfg.MaxOutputTokens = 256
	}
	checker, err := model.NewHTTPChecker(
		cfg,
		model.WithHTTPClient(guardDoer{t: t, base: server.URL, next: server.Client()}),
		model.WithAPIKeySource(func() (string, error) { return testAPIKey, nil }),
	)
	if err != nil {
		t.Fatalf("new checker: %v", err)
	}
	return checker
}

// writeCompletion writes a provider envelope with the given model
// content. The envelope itself is provider-owned; only the content is
// strictly validated by the adapter.
func writeCompletion(t *testing.T, w http.ResponseWriter, requestID, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	payload := map[string]any{
		"id": requestID,
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
		"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 7},
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Errorf("encode completion: %v", err)
	}
}

// constantCompletion serves the same model content on every attempt.
func constantCompletion(t *testing.T, content string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeCompletion(t, w, "req-test", content)
	}
}

func testSourceRequest() model.Request {
	return model.Request{
		Statement: "Implement merge sort.",
		Constraints: []model.Constraint{
			{ID: "C1", Text: "Do not call std::sort."},
			{ID: "C2", Text: "Implement merge sort."},
			{ID: "C3", Text: "Keep the output stable."},
		},
		Source: testSource,
	}
}

func oneConstraintRequest() model.Request {
	request := testSourceRequest()
	request.Constraints = request.Constraints[:1]
	return request
}
