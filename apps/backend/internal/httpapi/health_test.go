package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
)

func TestHealthWithoutStoreIsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	(&HealthHandler{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("status body = %q, want ok", body.Status)
	}
}

func TestHealthWithReachableStoreIsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	h := &HealthHandler{Pinger: postgres.PingFunc(func(context.Context) error { return nil })}
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestHealthWithUnreachableStoreIsUnavailableProblem(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	h := &HealthHandler{Pinger: postgres.PingFunc(func(context.Context) error { return errors.New("connection refused") })}
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}
	var problem struct {
		Type     string `json:"type"`
		Title    string `json:"title"`
		Status   int    `json:"status"`
		Detail   string `json:"detail"`
		Instance string `json:"instance"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if problem.Type != "https://leetkob.example/problems/unavailable" {
		t.Fatalf("type = %q, want unavailable problem", problem.Type)
	}
	if problem.Status != http.StatusServiceUnavailable {
		t.Fatalf("problem status = %d, want 503", problem.Status)
	}
	if problem.Instance != "/healthz" {
		t.Fatalf("instance = %q, want request path", problem.Instance)
	}
}

func TestHealthRejectsNonGet(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/healthz", nil)

		(&HealthHandler{}).ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want 405", method, rec.Code)
		}
		var problem struct {
			Type string `json:"type"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
			t.Fatalf("%s decode: %v", method, err)
		}
		if problem.Type != "https://leetkob.example/problems/route-not-found" {
			t.Fatalf("%s type = %q, want route-not-found", method, problem.Type)
		}
	}
}
