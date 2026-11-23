package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/access"
	"github.com/JakeConal/leetkob/backend/internal/labs"
)

const (
	detailInvalid  = "The request is not valid for this operation."
	detailAuth     = "Authentication is required to access this resource."
	detailRole     = "Your account is not permitted to perform this operation."
	detailNotFound = "The requested resource does not exist."
	detailRate     = "Too many attempts. Retry after the indicated delay."
	detailConflict = "The idempotency key was already used with different request content."
	detailTooLarge = "The request body exceeds the accepted size limit."
	detailInternal = "An unexpected error occurred. The request can be safely retried."
	maxLoginBody   = 8 << 10
	maxLabBody     = 10<<20 + 64<<10 + 1<<20
)

// Dependencies is the set Register wires into the /v2 session and lab routes.
// Now is the server clock. WebOrigin is the only Origin accepted on a
// mutation; an empty Origin is allowed. Session cookies are always Secure.
// Current browsers still store a Secure cookie on http://localhost and
// http://127.0.0.1.
type Dependencies struct {
	Accounts  access.Repository
	Labs      labs.Repository
	Now       func() time.Time
	WebOrigin string
}

type api struct {
	deps Dependencies
}

// Register mounts password sessions, class listing and TA lab authoring.
// Caller role headers are not read. Routes that are not registered here
// receive the concealed not-found problem, including unsupported lab
// revision methods.
func Register(mux *http.ServeMux, deps Dependencies) {
	h := &api{deps: deps}
	mux.HandleFunc("POST /v2/auth/login", h.login)
	mux.HandleFunc("GET /v2/auth/me", h.me)
	mux.HandleFunc("POST /v2/auth/logout", h.logout)
	mux.HandleFunc("GET /v2/classes", h.listClasses)
	mux.HandleFunc("GET /v2/classes/{classId}/labs", h.listLabs)
	mux.HandleFunc("POST /v2/classes/{classId}/labs", h.createLab)
	mux.HandleFunc("GET /v2/labs/{labId}", h.getLab)
	mux.HandleFunc("/", h.notFound)
}

func (h *api) now() time.Time {
	if h.deps.Now != nil {
		return h.deps.Now().UTC()
	}
	return time.Now().UTC()
}

func (h *api) login(w http.ResponseWriter, r *http.Request) {
	if !singleOrigin(r, h.deps.WebOrigin) {
		writeRole(w, r)
		return
	}
	if !jsonMedia(r.Header.Get("Content-Type")) {
		writeInvalid(w, r)
		return
	}
	body, tooBig, err := readBounded(w, r, maxLoginBody)
	if tooBig {
		writeTooLarge(w, r)
		return
	}
	if err != nil {
		writeInvalid(w, r)
		return
	}
	email, password, ok := parseLogin(body)
	if !ok {
		writeInvalid(w, r)
		return
	}
	result, err := access.Login(r.Context(), h.deps.Accounts, access.LoginInput{
		Email:      email,
		Password:   password,
		RemoteHost: remoteHost(r),
		Origin:     r.Header.Get("Origin"),
		WebOrigin:  h.deps.WebOrigin,
		Now:        h.now(),
	})
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	setSessionCookie(w, result.CookieToken, result.ExpiresAt)
	writeJSON(w, http.StatusOK, userSessionBody{
		User: userBody{
			ID:          result.User.ID,
			DisplayName: result.User.DisplayName,
			Role:        result.User.Role,
		},
		CSRFToken: result.CSRF,
		ExpiresAt: labs.FormatInstant(result.ExpiresAt),
	})
}

func (h *api) me(w http.ResponseWriter, r *http.Request) {
	authn, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, currentUserBody{
		User:    userFrom(authn.Principal),
		Classes: classesFrom(authn.Principal.Classes),
	})
}

func (h *api) logout(w http.ResponseWriter, r *http.Request) {
	authn, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.allowMutation(w, r, authn.CSRFHash) {
		return
	}
	cookie, err := r.Cookie(access.SessionCookieName)
	if err != nil {
		writeAuth(w, r)
		return
	}
	if err := access.Logout(r.Context(), h.deps.Accounts, cookie.Value, h.now()); err != nil {
		writeAccessError(w, r, err)
		return
	}
	clearSessionCookie(w)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *api) listClasses(w http.ResponseWriter, r *http.Request) {
	authn, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	limit, cursor, err := parseListQuery(r.URL.Query())
	if err != nil {
		writeInvalid(w, r)
		return
	}
	page, next, err := pageClasses(authn.Principal.Classes, limit, cursor)
	if err != nil {
		writeInvalid(w, r)
		return
	}
	writeJSON(w, http.StatusOK, classListBody{Items: classesFrom(page), NextCursor: next})
}

// authenticate loads the session, then the role and memberships from the
// store. X-Role, X-Class-Ids and any other caller role header are ignored.
func (h *api) authenticate(w http.ResponseWriter, r *http.Request) (access.Authenticated, bool) {
	cookie, err := r.Cookie(access.SessionCookieName)
	if err != nil || cookie.Value == "" {
		writeAuth(w, r)
		return access.Authenticated{}, false
	}
	authn, err := access.Load(r.Context(), h.deps.Accounts, cookie.Value, h.now())
	if err != nil {
		writeAccessError(w, r, err)
		return access.Authenticated{}, false
	}
	return authn, true
}

func (h *api) allowMutation(w http.ResponseWriter, r *http.Request, csrfHash []byte) bool {
	if !singleOrigin(r, h.deps.WebOrigin) {
		writeRole(w, r)
		return false
	}
	values := r.Header.Values("X-CSRF-Token")
	if len(values) != 1 || !access.ProofMatches(values[0], csrfHash) {
		writeRole(w, r)
		return false
	}
	return true
}

func (h *api) notFound(w http.ResponseWriter, r *http.Request) {
	writeNotFound(w, r)
}

type userBody struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type userSessionBody struct {
	User      userBody `json:"user"`
	CSRFToken string   `json:"csrfToken"`
	ExpiresAt string   `json:"expiresAt"`
}

type classBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type currentUserBody struct {
	User    userBody    `json:"user"`
	Classes []classBody `json:"classes"`
}

type classListBody struct {
	Items      []classBody `json:"items"`
	NextCursor *string     `json:"nextCursor"`
}

func userFrom(p access.Principal) userBody {
	return userBody{ID: p.User.ID, DisplayName: p.User.DisplayName, Role: p.User.Role}
}

func classesFrom(classes []access.Class) []classBody {
	out := make([]classBody, 0, len(classes))
	for _, class := range classes {
		out = append(out, classBody{ID: class.ID, Name: class.Name})
	}
	return out
}

func parseLogin(raw []byte) (string, string, bool) {
	if err := rejectDuplicateJSON(raw); err != nil {
		return "", "", false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := dec.Decode(&body); err != nil {
		return "", "", false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return "", "", false
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(raw, &present); err != nil || len(present) != 2 {
		return "", "", false
	}
	if _, ok := present["email"]; !ok {
		return "", "", false
	}
	if _, ok := present["password"]; !ok {
		return "", "", false
	}
	if !validEmail(body.Email) || len(body.Password) < 1 || len(body.Password) > 128 {
		return "", "", false
	}
	return body.Email, body.Password, true
}

func validEmail(value string) bool {
	if len(value) < 3 || len(value) > 254 || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	local, domain, ok := strings.Cut(value, "@")
	if !ok || strings.Contains(local, "@") || local == "" || domain == "" || !strings.Contains(domain, ".") {
		return false
	}
	return true
}

func rejectDuplicateJSON(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return access.ErrInvalid
	}
	seen := map[string]struct{}{}
	for dec.More() {
		keyTok, err := dec.Token()
		key, ok := keyTok.(string)
		if err != nil || !ok {
			return access.ErrInvalid
		}
		if _, dup := seen[key]; dup {
			return access.ErrInvalid
		}
		seen[key] = struct{}{}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return access.ErrInvalid
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') {
		return access.ErrInvalid
	}
	if len(bytes.TrimSpace(raw[dec.InputOffset():])) != 0 {
		return access.ErrInvalid
	}
	return nil
}

func parseListQuery(values map[string][]string) (int, string, error) {
	limit := 50
	cursor := ""
	for key, items := range values {
		switch key {
		case "limit":
			if len(items) != 1 {
				return 0, "", access.ErrInvalid
			}
			n, err := strconv.Atoi(items[0])
			if err != nil || n < 1 || n > 100 {
				return 0, "", access.ErrInvalid
			}
			limit = n
		case "cursor":
			if len(items) != 1 || items[0] == "" {
				return 0, "", access.ErrInvalid
			}
			cursor = items[0]
		default:
			return 0, "", access.ErrInvalid
		}
	}
	return limit, cursor, nil
}

func pageClasses(classes []access.Class, limit int, cursor string) ([]access.Class, *string, error) {
	start := 0
	if cursor != "" {
		raw, err := decodeClassCursor(cursor)
		if err != nil {
			return nil, nil, err
		}
		found := -1
		for i, class := range classes {
			if class.ID == raw {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, nil, access.ErrInvalid
		}
		start = found + 1
	}
	if start > len(classes) {
		start = len(classes)
	}
	end := start + limit
	var next *string
	if end < len(classes) {
		encoded := encodeClassCursor(classes[end-1].ID)
		next = &encoded
		end = start + limit
	} else {
		end = len(classes)
	}
	page := append([]access.Class{}, classes[start:end]...)
	return page, next, nil
}

func encodeClassCursor(id string) string {
	return strconv.FormatInt(int64(len(id)), 16) + "." + id
}

func decodeClassCursor(token string) (string, error) {
	sizeText, id, ok := strings.Cut(token, ".")
	if !ok || id == "" {
		return "", access.ErrInvalid
	}
	size, err := strconv.ParseInt(sizeText, 16, 64)
	if err != nil || int(size) != len(id) {
		return "", access.ErrInvalid
	}
	return id, nil
}

func singleOrigin(r *http.Request, configured string) bool {
	values := r.Header.Values("Origin")
	if len(values) > 1 {
		return false
	}
	origin := ""
	if len(values) == 1 {
		origin = values[0]
	}
	return access.OriginAllowed(origin, configured)
}

func jsonMedia(header string) bool {
	if header == "" {
		return false
	}
	media, params, err := mime.ParseMediaType(header)
	if err != nil || media != "application/json" {
		return false
	}
	if charset, ok := params["charset"]; ok && !strings.EqualFold(charset, "utf-8") {
		return false
	}
	return true
}

func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		if r.RemoteAddr == "" {
			return "unknown"
		}
		return r.RemoteAddr
	}
	return host
}

func readBounded(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	body, err := io.ReadAll(r.Body)
	if err == nil {
		return body, false, nil
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return nil, true, err
	}
	return nil, false, err
}

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     access.SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires.UTC(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     access.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeAccessError(w http.ResponseWriter, r *http.Request, err error) {
	var limited access.RateLimitError
	switch {
	case errors.As(err, &limited):
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Retry-After", strconv.Itoa(limited.RetryAfter))
		writeProblem(w, http.StatusTooManyRequests, "rate-limited", "Rate limited", detailRate, r.URL.Path)
	case errors.Is(err, access.ErrUnauthenticated):
		writeAuth(w, r)
	case errors.Is(err, access.ErrForbidden):
		writeRole(w, r)
	case errors.Is(err, access.ErrNotFound):
		writeNotFound(w, r)
	case errors.Is(err, access.ErrInvalid), errors.Is(err, labs.ErrInvalid):
		writeInvalid(w, r)
	case errors.Is(err, labs.ErrConflict):
		writeConflict(w, r)
	case errors.Is(err, labs.ErrTooLarge):
		writeTooLarge(w, r)
	default:
		writeInternal(w, r)
	}
}

func writeAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeProblem(w, http.StatusUnauthorized, "authentication-required", "Authentication required", detailAuth, r.URL.Path)
}

func writeRole(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeProblem(w, http.StatusForbidden, "role-required", "Role required", detailRole, r.URL.Path)
}

func writeNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeProblem(w, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound, r.URL.Path)
}

func writeInvalid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeProblem(w, http.StatusBadRequest, "invalid-request", "Invalid request", detailInvalid, r.URL.Path)
}

func writeConflict(w http.ResponseWriter, r *http.Request) {
	writeProblemExtra(w, http.StatusConflict, "conflict", "Conflict", detailConflict, r.URL.Path, map[string]any{
		"conflictKind": "idempotency-content-mismatch",
	})
}

func writeTooLarge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeProblem(w, http.StatusRequestEntityTooLarge, "payload-too-large", "Payload too large", detailTooLarge, r.URL.Path)
}

func writeInternal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeProblem(w, http.StatusInternalServerError, "internal-error", "Internal server error", detailInternal, r.URL.Path)
}

func writeProblemExtra(w http.ResponseWriter, status int, suffix, title, detail, instance string, extra map[string]any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	body := map[string]any{
		"type":     ProblemNamespace + suffix,
		"title":    title,
		"status":   status,
		"detail":   detail,
		"instance": instance,
	}
	for key, value := range extra {
		body[key] = value
	}
	_ = json.NewEncoder(w).Encode(body)
}
