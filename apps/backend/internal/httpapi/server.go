package httpapi

import (
	"net/http"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
)

// Deps wires the API server with its injected boundaries. Auth is optional
// so the process can run storeless during bring-up.
type Deps struct {
	Pinger        postgres.Pinger
	Auth          *auth.Service
	WebOrigin     string
	SecureCookies bool
}

// NewServer builds the route table: the health contract plus, when
// configured, the /v2 session routes. Unknown routes return the documented
// route-not-found problem.
func NewServer(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/healthz", &HealthHandler{Pinger: deps.Pinger})
	if deps.Auth != nil {
		handlers := &sessionHandlers{auth: deps.Auth, webOrigin: deps.WebOrigin, secure: deps.SecureCookies}
		mux.HandleFunc("POST /v2/auth/login", handlers.login)
		mux.HandleFunc("GET /v2/auth/me", handlers.me)
		mux.HandleFunc("POST /v2/auth/logout", handlers.logout)
	}
	mux.HandleFunc("/", routeNotFound)
	return mux
}
