package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/review"
)

const maxDecisionBody = 8192

var (
	uuidPattern        = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	idempotencyPattern = regexp.MustCompile(`^[!-~]{8,128}$`)
)

// Identity is the authenticated user. The role is read from the user
// record by the review store, not from this value or from a header.
type Identity struct {
	UserID string
	CSRF   string
}

// Authenticator resolves the session cookie. Implementations must ignore
// caller-supplied role headers.
type Authenticator interface {
	Identify(r *http.Request) (Identity, bool)
}

// ReviewsHandler serves the assigned-TA queue and decision route.
// It does not register a production Publish route.
type ReviewsHandler struct {
	Auth      Authenticator
	Reviews   review.Store
	WebOrigin string
	Now       func() time.Time
}

type decisionBody struct {
	ExpectedRevision int              `json:"expectedRevision"`
	Verdict          string           `json:"verdict"`
	Reason           string           `json:"reason"`
	Evidence         *review.Evidence `json:"evidence"`
}

type contractProblem struct {
	Type             string `json:"type"`
	Title            string `json:"title"`
	Status           int    `json:"status"`
	Detail           string `json:"detail"`
	Instance         string `json:"instance"`
	ConflictKind     string `json:"conflictKind,omitempty"`
	ExpectedRevision *int   `json:"expectedRevision,omitempty"`
	CurrentRevision  *int   `json:"currentRevision,omitempty"`
}

// ServeHTTP implements http.Handler.
func (h *ReviewsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	labID, findingID, decision, ok := parseReviewPath(r.URL.Path)
	if !ok {
		writeNotFound(w, r.URL.Path)
		return
	}
	if !validUUID(labID) || (decision && !validUUID(findingID)) {
		writeInvalid(w, r.URL.Path)
		return
	}
	if decision {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r.URL.Path)
			return
		}
		h.decide(w, r, labID, findingID)
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r.URL.Path)
		return
	}
	h.list(w, r, labID)
}

func (h *ReviewsHandler) list(w http.ResponseWriter, r *http.Request, labID string) {
	identity, ok := h.identify(w, r)
	if !ok {
		return
	}
	if !h.allow(w, r, labID, identity.UserID) {
		return
	}
	limit, cursor, ok := listQuery(w, r)
	if !ok {
		return
	}
	if h.Reviews == nil {
		writeInternal(w, r.URL.Path)
		return
	}
	page, err := h.Reviews.List(r.Context(), labID, limit, cursor)
	if err != nil {
		writeStoreError(w, r.URL.Path, err)
		return
	}
	if page.Items == nil {
		page.Items = []review.Finding{}
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *ReviewsHandler) decide(w http.ResponseWriter, r *http.Request, labID, findingID string) {
	identity, ok := h.identify(w, r)
	if !ok {
		return
	}
	if !h.allow(w, r, labID, identity.UserID) {
		return
	}
	if !tokenEqual(r.Header.Get("X-CSRF-Token"), identity.CSRF) {
		writeRole(w, r.URL.Path)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && h.WebOrigin != "" && origin != h.WebOrigin {
		writeRole(w, r.URL.Path)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !idempotencyPattern.MatchString(key) {
		writeInvalid(w, r.URL.Path)
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	var decoded decisionBody
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decoded); err != nil {
		writeInvalid(w, r.URL.Path)
		return
	}
	var trailing struct{}
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeInvalid(w, r.URL.Path)
		return
	}
	if h.Reviews == nil {
		writeInternal(w, r.URL.Path)
		return
	}
	sum := sha256.Sum256(body)
	result, err := h.Reviews.Decide(r.Context(), review.DecisionRequest{
		LabID:            labID,
		FindingID:        findingID,
		ActorID:          identity.UserID,
		ExpectedRevision: decoded.ExpectedRevision,
		Verdict:          decoded.Verdict,
		Reason:           decoded.Reason,
		Evidence:         decoded.Evidence,
		IdempotencyKey:   key,
		Fingerprint:      hex.EncodeToString(sum[:]),
		Now:              h.now(),
	})
	if err != nil {
		writeStoreError(w, r.URL.Path, err)
		return
	}
	if result.Mismatch {
		writeConflict(w, r.URL.Path, "idempotency-content-mismatch", "The idempotency key was already used with different request content.", nil, nil)
		return
	}
	if result.Stale != nil {
		writeConflict(w, r.URL.Path, "stale-revision", "The item changed before this request was applied.", &result.Stale.Expected, &result.Stale.Current)
		return
	}
	writeJSON(w, http.StatusOK, result.Finding)
}

func (h *ReviewsHandler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now().UTC()
}

func (h *ReviewsHandler) identify(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	if h.Auth == nil {
		writeAuth(w, r.URL.Path)
		return Identity{}, false
	}
	identity, ok := h.Auth.Identify(r)
	if !ok || !validUUID(identity.UserID) {
		writeAuth(w, r.URL.Path)
		return Identity{}, false
	}
	return identity, true
}

func (h *ReviewsHandler) allow(w http.ResponseWriter, r *http.Request, labID, userID string) bool {
	if h.Reviews == nil {
		writeInternal(w, r.URL.Path)
		return false
	}
	access, err := h.Reviews.Access(r.Context(), labID, userID)
	if err != nil {
		writeInternal(w, r.URL.Path)
		return false
	}
	if !access.UserExists {
		writeAuth(w, r.URL.Path)
		return false
	}
	if !review.Allowed(access) {
		writeNotFound(w, r.URL.Path)
		return false
	}
	return true
}

func listQuery(w http.ResponseWriter, r *http.Request) (int, string, bool) {
	query := r.URL.Query()
	for key, values := range query {
		if key != "limit" && key != "cursor" {
			writeInvalid(w, r.URL.Path)
			return 0, "", false
		}
		if len(values) > 1 {
			writeInvalid(w, r.URL.Path)
			return 0, "", false
		}
	}
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeInvalid(w, r.URL.Path)
			return 0, "", false
		}
		limit = parsed
	}
	return limit, query.Get("cursor"), true
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	media := r.Header.Get("Content-Type")
	if media != "application/json" && !strings.HasPrefix(media, "application/json;") {
		writeInvalid(w, r.URL.Path)
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDecisionBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(w, http.StatusRequestEntityTooLarge, "payload-too-large", "Payload too large", "The request body exceeds the accepted size limit.", r.URL.Path)
			return nil, false
		}
		writeInvalid(w, r.URL.Path)
		return nil, false
	}
	if len(body) == 0 {
		writeInvalid(w, r.URL.Path)
		return nil, false
	}
	return body, true
}

func parseReviewPath(path string) (labID, findingID string, decision, ok bool) {
	parts := strings.Split(path, "/")
	if len(parts) < 5 || parts[0] != "" || parts[1] != "v2" || parts[2] != "labs" || parts[4] != "reviews" {
		return "", "", false, false
	}
	if len(parts) == 5 {
		return parts[3], "", false, true
	}
	if len(parts) == 7 && parts[6] == "decision" {
		return parts[3], parts[5], true, true
	}
	return "", "", false, false
}

func validUUID(value string) bool {
	return uuidPattern.MatchString(value)
}

func tokenEqual(got, want string) bool {
	if want == "" {
		return false
	}
	sumGot := sha256.Sum256([]byte(got))
	sumWant := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(sumGot[:], sumWant[:]) == 1
}

func writeStoreError(w http.ResponseWriter, instance string, err error) {
	switch {
	case errors.Is(err, review.ErrInvalid), errors.Is(err, review.ErrBadCursor):
		writeInvalid(w, instance)
	case errors.Is(err, review.ErrNotFound), errors.Is(err, review.ErrNotAssigned):
		writeNotFound(w, instance)
	default:
		writeInternal(w, instance)
	}
}

func writeAuth(w http.ResponseWriter, instance string) {
	writeProblem(w, http.StatusUnauthorized, "authentication-required", "Authentication required", "Authentication is required to access this resource.", instance)
}

func writeRole(w http.ResponseWriter, instance string) {
	writeProblem(w, http.StatusForbidden, "role-required", "Role required", "Your account is not permitted to perform this operation.", instance)
}

func writeNotFound(w http.ResponseWriter, instance string) {
	writeProblem(w, http.StatusNotFound, "resource-not-found", "Resource not found", "The requested resource does not exist.", instance)
}

func writeInvalid(w http.ResponseWriter, instance string) {
	writeProblem(w, http.StatusBadRequest, "invalid-request", "Invalid request", "The request is not valid for this operation.", instance)
}

func writeInternal(w http.ResponseWriter, instance string) {
	writeProblem(w, http.StatusInternalServerError, "internal-error", "Internal server error", "An unexpected error occurred. The request can be safely retried.", instance)
}

func writeMethodNotAllowed(w http.ResponseWriter, instance string) {
	writeProblem(w, http.StatusMethodNotAllowed, "route-not-found", "Route not found", "The requested route does not exist.", instance)
}

func writeConflict(w http.ResponseWriter, instance, kind, detail string, expected, current *int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusConflict)
	body := contractProblem{
		Type:             ProblemNamespace + "conflict",
		Title:            "Conflict",
		Status:           http.StatusConflict,
		Detail:           detail,
		Instance:         instance,
		ConflictKind:     kind,
		ExpectedRevision: expected,
		CurrentRevision:  current,
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}
