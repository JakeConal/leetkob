package httpapi

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/auth"
)

// SessionCookieName is the host-only HttpOnly session cookie.
const SessionCookieName = "leetkob_session"

const (
	maxLoginBodyBytes = 8 * 1024
	maxEmailLength    = 320
	maxPasswordLength = 128
)

// sessionHandlers implements the PA#3 /v2/auth routes.
type sessionHandlers struct {
	auth      *auth.Service
	webOrigin string
	secure    bool
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userView struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Role        auth.Role `json:"role"`
}

type classView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type loginResponse struct {
	User      userView  `json:"user"`
	CSRFToken string    `json:"csrfToken"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type meResponse struct {
	User    userView    `json:"user"`
	Classes []classView `json:"classes"`
}

// login handles POST /v2/auth/login.
func (h *sessionHandlers) login(w http.ResponseWriter, r *http.Request) {
	if !h.allowedOrigin(w, r) {
		return
	}
	var request loginRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil ||
		request.Email == "" || len(request.Email) > maxEmailLength ||
		request.Password == "" || len(request.Password) > maxPasswordLength {
		writeProblem(w, http.StatusBadRequest, "invalid-request", "Invalid request",
			"The request is not valid for this operation.", r.URL.Path)
		return
	}

	result, err := h.auth.Login(r.Context(), request.Email, request.Password, clientIP(r))
	if err != nil {
		var limited *auth.RateLimitedError
		switch {
		case errors.As(err, &limited):
			retryAfter := int(limited.RetryAfter / time.Second)
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeProblem(w, http.StatusTooManyRequests, "rate-limited", "Rate limited",
				"Too many attempts. Retry after the indicated delay.", r.URL.Path)
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeProblem(w, http.StatusUnauthorized, "authentication-required", "Authentication required",
				"Authentication is required to access this resource.", r.URL.Path)
		default:
			writeProblem(w, http.StatusInternalServerError, "internal-error", "Internal server error",
				"An unexpected error occurred. The request can be safely retried.", r.URL.Path)
		}
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    result.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  result.ExpiresAt,
	})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(loginResponse{
		User:      userView{ID: result.User.ID, DisplayName: result.User.DisplayName, Role: result.User.Role},
		CSRFToken: result.CSRFToken,
		ExpiresAt: result.ExpiresAt,
	})
}

// me handles GET /v2/auth/me.
func (h *sessionHandlers) me(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	classes, err := h.auth.Classes(r.Context(), identity.User.ID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal-error", "Internal server error",
			"An unexpected error occurred. The request can be safely retried.", r.URL.Path)
		return
	}
	views := make([]classView, 0, len(classes))
	for _, class := range classes {
		views = append(views, classView{ID: class.ID, Name: class.Name})
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(meResponse{
		User:    userView{ID: identity.User.ID, DisplayName: identity.User.DisplayName, Role: identity.User.Role},
		Classes: views,
	})
}

// logout handles POST /v2/auth/logout.
func (h *sessionHandlers) logout(w http.ResponseWriter, r *http.Request) {
	if !h.allowedOrigin(w, r) {
		return
	}
	identity, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	presented := r.Header.Get("X-CSRF-Token")
	valid, err := h.auth.VerifyCSRF(r.Context(), identity, presented)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal-error", "Internal server error",
			"An unexpected error occurred. The request can be safely retried.", r.URL.Path)
		return
	}
	if !valid {
		writeRoleRequiredProblem(w, r)
		return
	}
	if err := h.auth.Logout(r.Context(), identity); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal-error", "Internal server error",
			"An unexpected error occurred. The request can be safely retried.", r.URL.Path)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// authenticate resolves the session cookie or writes the 401 problem.
func (h *sessionHandlers) authenticate(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	return resolveIdentity(r.Context(), h.auth, w, r)
}

// allowedOrigin rejects a differing Origin before any state is read.
func (h *sessionHandlers) allowedOrigin(w http.ResponseWriter, r *http.Request) bool {
	return originAllowed(h.webOrigin, w, r)
}

// clientIP extracts the direct peer address; forwarded headers are not
// trusted for throttling.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
