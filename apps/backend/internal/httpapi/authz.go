package httpapi

import (
	"context"
	"net/http"

	"github.com/JakeConal/leetkob/backend/internal/auth"
)

// resolveIdentity authenticates the session cookie or writes the documented
// unauthenticated problem. The returned identity is always derived from the
// server-owned session; caller-supplied role or class headers are never read.
func resolveIdentity(ctx context.Context, service *auth.Service, w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		writeUnauthenticatedProblem(w, r)
		return auth.Identity{}, false
	}
	identity, err := service.Authenticate(ctx, cookie.Value)
	if err != nil {
		writeUnauthenticatedProblem(w, r)
		return auth.Identity{}, false
	}
	return identity, true
}

// requireRole enforces the current server-owned role. A wrong role is a
// general authorization failure (403), never a concealed not-found.
func requireRole(w http.ResponseWriter, r *http.Request, identity auth.Identity, role auth.Role) bool {
	if identity.User.Role != role {
		writeRoleRequiredProblem(w, r)
		return false
	}
	return true
}

// memberOfClass reports whether the caller's current memberships include the
// class. Membership is read from the server store on every request, so a
// removed membership takes effect even while the session remains valid.
func memberOfClass(ctx context.Context, service *auth.Service, userID, classID string) (bool, error) {
	classes, err := service.Classes(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, class := range classes {
		if class.ID == classID {
			return true, nil
		}
	}
	return false, nil
}

// originAllowed rejects a differing Origin before any mutation state changes.
// A missing header is allowed for same-origin clients that omit it; the CSRF
// proof still gates every mutation.
func originAllowed(webOrigin string, w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" && webOrigin != "" && origin != webOrigin {
		writeRoleRequiredProblem(w, r)
		return false
	}
	return true
}

func writeUnauthenticatedProblem(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusUnauthorized, "authentication-required", "Authentication required",
		"Authentication is required to access this resource.", r.URL.Path)
}

func writeRoleRequiredProblem(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusForbidden, "role-required", "Role required",
		"Your account is not permitted to perform this operation.", r.URL.Path)
}

func writeInvalidRequestProblem(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusBadRequest, "invalid-request", "Invalid request",
		"The request is not valid for this operation.", r.URL.Path)
}

func writeConcealedNotFoundProblem(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusNotFound, "resource-not-found", "Resource not found",
		"The requested resource does not exist.", r.URL.Path)
}

func writeInternalErrorProblem(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusInternalServerError, "internal-error", "Internal server error",
		"An unexpected error occurred. The request can be safely retried.", r.URL.Path)
}
