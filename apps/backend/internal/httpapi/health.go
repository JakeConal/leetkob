// Package httpapi holds HTTP route handlers for the Go backend. Routes
// follow the published PA#3 contract (docs/contract/pa3-openapi.json) and
// return RFC 9457 problems pinned by docs/contract/fixtures/pa3-problems.json.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
)

// ProblemNamespace is the absolute URI prefix for all PA#3 problem types.
const ProblemNamespace = "https://leetkob.example/problems/"

// healthResponse is the success body of GET /healthz.
type healthResponse struct {
	Status string `json:"status"`
}

// healthProblem mirrors the unavailable fixture body with an
// occurrence-specific instance.
type healthProblem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

// HealthHandler reports service health. Without a configured store it
// reports the process itself healthy; with a store it reports 503 using
// the contract's unavailable problem when the store cannot be reached.
type HealthHandler struct {
	Pinger postgres.Pinger
}

// ServeHTTP implements http.Handler.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeProblem(w, http.StatusMethodNotAllowed, "route-not-found", "Route not found", "The requested route does not exist.", r.URL.Path)
		return
	}
	if h.Pinger != nil {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.Pinger.Ping(pingCtx); err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "unavailable", "Service unavailable", "The service is temporarily unavailable. The request can be safely retried.", r.URL.Path)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(healthResponse{Status: "ok"}); err != nil {
		// The body could not be written; nothing further to do.
		_ = err
	}
}

func writeProblem(w http.ResponseWriter, status int, typeSuffix, title, detail, instance string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	body := healthProblem{
		Type:     ProblemNamespace + typeSuffix,
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		_ = fmt.Errorf("httpapi: encode problem: %w", err)
	}
}
