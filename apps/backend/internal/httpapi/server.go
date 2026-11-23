package httpapi

import (
	"net/http"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
)

// Deps wires the API server with its injected boundaries. Auth is optional
// so the process can run storeless during bring-up. Labs and Idempotency are
// wired from the same durable store in production and from fakes in tests.
type Deps struct {
	Pinger           postgres.Pinger
	Auth             *auth.Service
	Labs             LabStore
	Idempotency      IdempotencyStore
	Submissions      SubmissionAcceptor
	SubmissionReader SubmissionReader
	Reviews          ReviewStore
	WebOrigin        string
	SecureCookies    bool
}

// NewServer builds the route table: the health contract plus, when
// configured, the /v2 session, class and lab routes. Unknown routes return
// the documented route-not-found problem.
func NewServer(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/healthz", &HealthHandler{Pinger: deps.Pinger})
	if deps.Auth != nil {
		handlers := &sessionHandlers{auth: deps.Auth, webOrigin: deps.WebOrigin, secure: deps.SecureCookies}
		mux.HandleFunc("POST /v2/auth/login", handlers.login)
		mux.HandleFunc("GET /v2/auth/me", handlers.me)
		mux.HandleFunc("POST /v2/auth/logout", handlers.logout)
		mux.HandleFunc("GET /v2/classes", handlers.listClasses)

		if deps.Labs != nil {
			labs := &labHandlers{
				auth:      deps.Auth,
				labs:      deps.Labs,
				idem:      deps.Idempotency,
				webOrigin: deps.WebOrigin,
			}
			mux.HandleFunc("GET /v2/classes/{classId}/labs", labs.listLabs)
			mux.HandleFunc("GET /v2/labs/{labId}", labs.getLab)
			if deps.Idempotency != nil {
				mux.HandleFunc("POST /v2/classes/{classId}/labs", labs.createLab)
			}
			// Lab revision/editing routes are deliberately not implemented in
			// this slice. Registering explicit fallbacks keeps unsupported
			// mutations on a known path at the documented route-not-found
			// problem instead of ServeMux's plain-text 405.
			for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
				mux.HandleFunc(method+" /v2/labs/{labId}", routeNotFound)
			}
		}
		if deps.Submissions != nil {
			submissions := &submissionHandlers{
				auth:        deps.Auth,
				submissions: deps.Submissions,
				webOrigin:   deps.WebOrigin,
			}
			mux.HandleFunc("POST /v2/labs/{labId}/submissions", submissions.create)
		}
		if deps.SubmissionReader != nil {
			reads := &submissionReadHandlers{auth: deps.Auth, reads: deps.SubmissionReader}
			mux.HandleFunc("GET /v2/submissions/{submissionId}", reads.get)
			mux.HandleFunc("GET /v2/labs/{labId}/submissions", reads.list)
		}
		if deps.Reviews != nil {
			reviews := &reviewHandlers{
				auth:      deps.Auth,
				reviews:   deps.Reviews,
				idem:      deps.Idempotency,
				webOrigin: deps.WebOrigin,
			}
			mux.HandleFunc("GET /v2/labs/{labId}/reviews", reviews.list)
			if deps.Idempotency != nil {
				mux.HandleFunc("POST /v2/labs/{labId}/reviews/{findingId}/decision", reviews.decide)
			}
		}
	}
	mux.HandleFunc("/", routeNotFound)
	return mux
}
