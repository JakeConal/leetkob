package submit

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
)

const (
	problemNamespace = "https://leetkob.example/problems/"
	sessionCookie    = "leetkob_session"
	maxBodyBytes     = MaxSourceBytes*6 + 4096
)

// Authenticator resolves an opaque session cookie. A failure is generic.
type Authenticator interface {
	Authenticate(ctx context.Context, sessionToken string) (Principal, error)
}

// Handler serves the published submission routes.
type Handler struct {
	Service   *Service
	Auth      Authenticator
	WebOrigin string
}

// Register attaches the submission routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v2/labs/{labId}/submissions", h.ServeLabSubmissions)
	mux.HandleFunc("/v2/submissions/{submissionId}", h.ServeSubmission)
}

// ServeLabSubmissions handles GET and POST /v2/labs/{labId}/submissions.
func (h *Handler) ServeLabSubmissions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.create(w, r)
	case http.MethodGet:
		h.list(w, r)
	default:
		writeProblem(w, http.StatusMethodNotAllowed, "route-not-found", "Route not found", "The requested route does not exist.", r.URL.Path, nil)
	}
}

// ServeSubmission handles GET /v2/submissions/{submissionId}.
func (h *Handler) ServeSubmission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProblem(w, http.StatusMethodNotAllowed, "route-not-found", "Route not found", "The requested route does not exist.", r.URL.Path, nil)
		return
	}
	principal, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	submissionID := r.PathValue("submissionId")
	if !ValidUUID(submissionID) || !ValidUUID(principal.UserID) {
		writeInvalid(w, r)
		return
	}
	view, err := h.Service.Repo.GetSubmission(r.Context(), principal.UserID, submissionID, h.Service.now())
	if err != nil {
		h.writeRepoErr(w, r, err)
		return
	}
	writeView(w, http.StatusOK, view)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !originOK(r, h.WebOrigin) || !csrfOK(r, principal) {
		writeForbidden(w, r)
		return
	}
	labID := r.PathValue("labId")
	key := r.Header.Get("Idempotency-Key")
	if !ValidUUID(labID) || !ValidUUID(principal.UserID) || !ValidIdempotencyKey(key) {
		writeInvalid(w, r)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeInvalid(w, r)
		return
	}
	lease, err := h.Service.Begin(r.Context(), principal.UserID, labID)
	if err != nil {
		h.writeRepoErr(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		_ = h.Service.Repo.AbortAdmission(r.Context(), lease.ID, h.Service.now())
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			writeProblem(w, http.StatusRequestEntityTooLarge, "payload-too-large", "Payload too large", "The request body exceeds the accepted size limit.", r.URL.Path, nil)
			return
		}
		writeInvalid(w, r)
		return
	}
	// Receipt is the arrival of the complete bounded body, before parsing.
	received := h.Service.now()
	out, err := h.Service.Complete(r.Context(), AcceptIn{
		UserID:         principal.UserID,
		LabID:          labID,
		LeaseID:        lease.ID,
		ReceivedAt:     received,
		IdempotencyKey: key,
	}, body)
	if err != nil {
		h.writeRepoErr(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(out.Status)
	_, _ = w.Write(out.Body)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	labID := r.PathValue("labId")
	if !ValidUUID(labID) || !ValidUUID(principal.UserID) {
		writeInvalid(w, r)
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeInvalid(w, r)
		return
	}
	page, err := h.Service.Repo.ListSubmissions(r.Context(), ListQuery{
		UserID: principal.UserID,
		LabID:  labID,
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
		Now:    h.Service.now(),
	})
	if err != nil {
		h.writeRepoErr(w, r, err)
		return
	}
	items := make([]submissionBody, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, toBody(item))
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	writeJSON(w, http.StatusOK, struct {
		Items      []submissionBody `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}{Items: items, NextCursor: next})
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (Principal, bool) {
	if h == nil || h.Service == nil || h.Service.Repo == nil || h.Auth == nil {
		writeProblem(w, http.StatusInternalServerError, "internal-error", "Internal server error", "An unexpected error occurred. The request can be safely retried.", r.URL.Path, nil)
		return Principal{}, false
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeUnauthenticated(w, r)
		return Principal{}, false
	}
	principal, err := h.Auth.Authenticate(r.Context(), cookie.Value)
	if err != nil || principal.UserID == "" {
		writeUnauthenticated(w, r)
		return Principal{}, false
	}
	return principal, true
}

func (h *Handler) writeRepoErr(w http.ResponseWriter, r *http.Request, err error) {
	var closed WindowClosed
	switch {
	case errors.As(err, &closed):
		writeProblem(w, http.StatusConflict, "conflict", "Conflict", "The submission window for this lab is closed.", r.URL.Path, map[string]string{
			"conflictKind": "submission-window-closed",
			"deadline":     FormatTime(closed.Deadline),
		})
	case errors.As(err, new(IdempotencyMismatch)):
		writeProblem(w, http.StatusConflict, "conflict", "Conflict", "The idempotency key was already used with different request content.", r.URL.Path, map[string]string{
			"conflictKind": "idempotency-content-mismatch",
		})
	case errors.Is(err, ErrNotFound):
		writeProblem(w, http.StatusNotFound, "resource-not-found", "Resource not found", "The requested resource does not exist.", r.URL.Path, nil)
	case errors.Is(err, ErrForbidden):
		writeForbidden(w, r)
	case errors.Is(err, ErrInvalid):
		writeInvalid(w, r)
	case errors.Is(err, ErrLeaseExpired):
		writeProblem(w, http.StatusServiceUnavailable, "unavailable", "Service unavailable", "The service is temporarily unavailable. The request can be safely retried.", r.URL.Path, nil)
	default:
		writeProblem(w, http.StatusInternalServerError, "internal-error", "Internal server error", "An unexpected error occurred. The request can be safely retried.", r.URL.Path, nil)
	}
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		return 0, ErrInvalid
	}
	return n, nil
}

func originOK(r *http.Request, allowed string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return allowed != "" && origin == allowed
}

func csrfOK(r *http.Request, principal Principal) bool {
	got := r.Header.Get("X-CSRF-Token")
	if got == "" {
		return false
	}
	if len(principal.CSRFHash) == sha256.Size {
		sum := sha256.Sum256([]byte(got))
		return subtle.ConstantTimeCompare(sum[:], principal.CSRFHash) == 1
	}
	if len(got) != len(principal.CSRFToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(principal.CSRFToken)) == 1
}

type submissionBody struct {
	ID         string     `json:"id"`
	LabID      string     `json:"labId"`
	State      string     `json:"state"`
	ReceivedAt string     `json:"receivedAt"`
	Judge      *judgeBody `json:"judge,omitempty"`
}

type judgeBody struct {
	Verdict string     `json:"verdict"`
	Tests   []testBody `json:"tests"`
}

type testBody struct {
	ID      string `json:"id"`
	Verdict string `json:"verdict"`
	TimeMS  int    `json:"timeMs"`
}

func toBody(view View) submissionBody {
	body := submissionBody{
		ID:         view.ID,
		LabID:      view.LabID,
		State:      view.State,
		ReceivedAt: FormatTime(view.ReceivedAt),
	}
	if view.Judge != nil {
		tests := make([]testBody, 0, len(view.Judge.Tests))
		for _, test := range view.Judge.Tests {
			tests = append(tests, testBody{ID: test.ID, Verdict: test.Verdict, TimeMS: test.TimeMS})
		}
		body.Judge = &judgeBody{Verdict: view.Judge.Verdict, Tests: tests}
	}
	return body
}

func writeView(w http.ResponseWriter, status int, view View) {
	writeJSON(w, status, toBody(view))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func writeInvalid(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusBadRequest, "invalid-request", "Invalid request", "The request is not valid for this operation.", r.URL.Path, nil)
}

func writeUnauthenticated(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusUnauthorized, "authentication-required", "Authentication required", "Authentication is required to access this resource.", r.URL.Path, nil)
}

func writeForbidden(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusForbidden, "role-required", "Role required", "Your account is not permitted to perform this operation.", r.URL.Path, nil)
}

func writeProblem(w http.ResponseWriter, status int, typeSuffix, title, detail, instance string, extra map[string]string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	payload := map[string]any{
		"type":     problemNamespace + typeSuffix,
		"title":    title,
		"status":   status,
		"detail":   detail,
		"instance": instance,
	}
	for key, value := range extra {
		payload[key] = value
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
