package model_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/model"
)

func TestModelOffRefusesConstructor(t *testing.T) {
	t.Setenv("MODEL_OFF", "1")

	checker, err := model.NewHTTPChecker(model.Config{
		BaseURL:         "http://127.0.0.1:1",
		Model:           "test-model",
		Timeout:         time.Second,
		MaxOutputTokens: 16,
	})
	if !errors.Is(err, model.ErrModelOff) {
		t.Fatalf("err = %v, want ErrModelOff", err)
	}
	if checker != nil {
		t.Fatal("offline construction must not produce a provider client")
	}
}

func TestModelOffRefusesDispatch(t *testing.T) {
	var calls atomic.Int32
	server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeCompletion(t, w, "req-test", `{"results":[]}`)
	})
	checker := newChecker(t, server, model.Config{})

	// MODEL_OFF can be selected after construction by the checker's
	// selection point; dispatch must still be refused.
	t.Setenv("MODEL_OFF", "1")
	if _, err := checker.Check(context.Background(), testSourceRequest()); !errors.Is(err, model.ErrModelOff) {
		t.Fatalf("err = %v, want ErrModelOff", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("provider calls = %d, want 0", calls.Load())
	}
}

func TestAPIKeyIsReadLazily(t *testing.T) {
	var reads atomic.Int32
	server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCompletion(t, w, "req-test", `{"results":[{"constraintId":"C1","verdict":"pass","evidence":[]}]}`)
	})
	cfg := model.Config{BaseURL: server.URL, Model: "test-model", Timeout: 2 * time.Second, MaxOutputTokens: 64}

	checker, err := model.NewHTTPChecker(
		cfg,
		model.WithHTTPClient(guardDoer{t: t, base: server.URL, next: server.Client()}),
		model.WithAPIKeySource(func() (string, error) {
			reads.Add(1)
			return testAPIKey, nil
		}),
	)
	if err != nil {
		t.Fatalf("new checker: %v", err)
	}
	if reads.Load() != 0 {
		t.Fatalf("credential reads after construction = %d, want 0", reads.Load())
	}

	if _, err := checker.Check(context.Background(), oneConstraintRequest()); err != nil {
		t.Fatalf("check: %v", err)
	}
	if reads.Load() != 1 {
		t.Fatalf("credential reads after one check = %d, want 1", reads.Load())
	}
}

func TestMissingAPIKeyDoesNotDispatch(t *testing.T) {
	t.Setenv(model.EnvAPIKey, "")
	var calls atomic.Int32
	server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeCompletion(t, w, "req-test", `{"results":[]}`)
	})
	cfg := model.Config{BaseURL: server.URL, Model: "test-model", Timeout: 2 * time.Second, MaxOutputTokens: 64}

	checker, err := model.NewHTTPChecker(
		cfg,
		model.WithHTTPClient(guardDoer{t: t, base: server.URL, next: server.Client()}),
	)
	if err != nil {
		t.Fatalf("new checker: %v", err)
	}
	if _, err := checker.Check(context.Background(), testSourceRequest()); !errors.Is(err, model.ErrMissingAPIKey) {
		t.Fatalf("err = %v, want ErrMissingAPIKey", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("provider calls = %d, want 0", calls.Load())
	}
}

func TestInvalidRequestIsRejectedBeforeKeyReadOrDispatch(t *testing.T) {
	var calls atomic.Int32
	var keyReads atomic.Int32
	server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeCompletion(t, w, "req-test", `{"results":[{"constraintId":"C1","verdict":"pass","evidence":[]}]}`)
	})
	cfg := model.Config{BaseURL: server.URL, Model: "test-model", Timeout: 2 * time.Second, MaxOutputTokens: 64}
	checker, err := model.NewHTTPChecker(
		cfg,
		model.WithHTTPClient(guardDoer{t: t, base: server.URL, next: server.Client()}),
		model.WithAPIKeySource(func() (string, error) {
			keyReads.Add(1)
			return testAPIKey, nil
		}),
	)
	if err != nil {
		t.Fatalf("new checker: %v", err)
	}

	duplicateIDs := testSourceRequest()
	duplicateIDs.Constraints = []model.Constraint{
		{ID: "C1", Text: "Do not call std::sort."},
		{ID: "C1", Text: "Implement merge sort."},
	}
	manyConstraints := testSourceRequest()
	manyConstraints.Constraints = make([]model.Constraint, 0, 21)
	for i := 0; i < 21; i++ {
		manyConstraints.Constraints = append(manyConstraints.Constraints, model.Constraint{
			ID:   "C" + strings.Repeat("x", i+1),
			Text: "constraint text",
		})
	}

	cases := []struct {
		name    string
		request model.Request
	}{
		{name: "empty statement", request: model.Request{Constraints: testSourceRequest().Constraints, Source: testSource}},
		{name: "empty source", request: model.Request{Statement: "statement", Constraints: testSourceRequest().Constraints}},
		{name: "no constraints", request: model.Request{Statement: "statement", Source: testSource}},
		{name: "duplicate constraint ids", request: duplicateIDs},
		{name: "too many constraints", request: manyConstraints},
		{
			name: "source over the byte bound",
			request: model.Request{
				Statement:   "statement",
				Constraints: testSourceRequest().Constraints,
				Source:      strings.Repeat("a", model.MaxSourceBytes+1),
			},
		},
		{
			name: "statement over the byte bound",
			request: model.Request{
				Statement:   strings.Repeat("a", model.MaxStatementBytes+1),
				Constraints: testSourceRequest().Constraints,
				Source:      testSource,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := checker.Check(context.Background(), tc.request); !errors.Is(err, model.ErrInvalidRequest) {
				t.Fatalf("err = %v, want ErrInvalidRequest", err)
			}
		})
	}

	if calls.Load() != 0 {
		t.Fatalf("provider calls = %d, want 0", calls.Load())
	}
	if keyReads.Load() != 0 {
		t.Fatalf("credential reads = %d, want 0", keyReads.Load())
	}
}
