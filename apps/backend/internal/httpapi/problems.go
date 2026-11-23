package httpapi

import (
	"encoding/json"
	"net/http"
)

// ProblemNamespace is the absolute URI prefix for all PA#3 problem types.
const ProblemNamespace = "https://leetkob.example/problems/"

// problemBody is the shared RFC 9457 body. Extension members are added by
// the caller when a fixture pins them.
type problemBody struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

// writeProblem emits one problem response with the documented media type
// and no-store caching.
func writeProblem(w http.ResponseWriter, status int, typeSuffix, title, detail, instance string) {
	writeProblemWithExtensions(w, status, typeSuffix, title, detail, instance, nil)
}

// writeProblemWithExtensions adds the fixture-pinned extension members
// (for example conflictKind) to the shared RFC 9457 body.
func writeProblemWithExtensions(w http.ResponseWriter, status int, typeSuffix, title, detail, instance string, extensions map[string]any) {
	body := map[string]any{
		"type":     ProblemNamespace + typeSuffix,
		"title":    title,
		"status":   status,
		"detail":   detail,
		"instance": instance,
	}
	for key, value := range extensions {
		body[key] = value
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// routeNotFound writes the route-not-found problem for unknown routes.
func routeNotFound(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusNotFound, "route-not-found", "Route not found",
		"No route matches this request.", r.URL.Path)
}
