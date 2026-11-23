package semantic

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPAdapterTimeoutDoesNotDial(t *testing.T) {
	called := false
	adapter := &HTTPAdapter{
		URL:   "https://model.example/v1/chat",
		Model: "configured-model",
		Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			called = true
			<-r.Context().Done()
			return nil, r.Context().Err()
		})},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := adapter.Complete(ctx, CompletionRequest{System: SystemPrompt, UserJSON: `{"source":"int main(){return 0;}"}`, MaxOutputTokens: 16})
	if !called {
		t.Fatal("adapter did not use the injected client")
	}
	if time.Since(start) > time.Second {
		t.Fatal("adapter ignored the deadline")
	}
	if err == nil || !errorsIsDeadline(err) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}

func TestHTTPAdapterDoesNotReadFileURLOrEnvironmentKey(t *testing.T) {
	dir := t.TempDir()
	const canary = "SYNTHETIC-CANARY-NOT-A-LIVE-KEY"
	path := filepath.Join(dir, "trusted-canary.h")
	if err := os.WriteFile(path, []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", canary)
	called := false
	adapter := &HTTPAdapter{
		URL:   "file://" + path,
		Token: "",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, os.ErrClosed
		})},
	}
	_, err := adapter.Complete(context.Background(), CompletionRequest{System: SystemPrompt, UserJSON: `{}`, MaxOutputTokens: 8})
	if err == nil {
		t.Fatal("file URL was accepted")
	}
	if called {
		t.Fatal("file URL reached the transport")
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatal("error included the canary")
	}

	var gotReq *http.Request
	var body string
	adapter = &HTTPAdapter{
		URL:   "https://model.example/v1/chat",
		Model: "configured-model",
		Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotReq = r
			raw, _ := io.ReadAll(r.Body)
			body = string(raw)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"id":"req_synthetic","choices":[{"message":{"content":"{}"}}]}`)),
				Header:     make(http.Header),
			}, nil
		})},
	}
	resp, err := adapter.Complete(context.Background(), CompletionRequest{
		System:          SystemPrompt,
		UserJSON:        `{"source":"int main(){return 0;}"}`,
		MaxOutputTokens: 32,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.TokensKnown {
		t.Fatal("omitted usage must stay unknown")
	}
	if resp.RequestID != "req_synthetic" {
		t.Fatalf("request id = %q", resp.RequestID)
	}
	if gotReq.Header.Get("Authorization") != "" {
		t.Fatal("empty token must not set Authorization")
	}
	if strings.Contains(body, canary) || strings.Contains(gotReq.URL.String(), canary) {
		t.Fatal("request attached the environment canary")
	}
	if !strings.Contains(body, "int main(){return 0;}") {
		t.Fatalf("body lost the data payload: %s", body)
	}
}

func errorsIsDeadline(err error) bool {
	return err != nil && (strings.Contains(err.Error(), context.DeadlineExceeded.Error()) || err == context.DeadlineExceeded)
}
