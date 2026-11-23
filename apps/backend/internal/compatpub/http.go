package compatpub

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
)

const (
	uuidPattern      = `^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	keyPattern       = `^[\x21-\x7e]{8,128}$`
	routeNotFoundMsg = "No route matches this request."
)

var (
	uuidRe = regexp.MustCompile(uuidPattern)
	keyRe  = regexp.MustCompile(keyPattern)
)

// HandlerDeps wires the HTTP boundary with injected adapters.
type HandlerDeps struct {
	Secret      string
	NowSeconds  func() int64
	Labs        LabRepository
	Idempotency IdempotencyRepository
	Clock       Clock
}

// NewMux registers the legacy publication route and a catch-all that
// mirrors the PA#2 route-not-found problem for every other method/path.
func NewMux(deps HandlerDeps) *http.ServeMux {
	service := NewPublicationService(deps.Labs, deps.Idempotency, deps.Clock)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /labs/{labId}/publish", func(w http.ResponseWriter, r *http.Request) {
		publish(w, r, service, deps)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, routeNotFound(r))
	})
	return mux
}

func publish(w http.ResponseWriter, r *http.Request, service *PublicationService, deps HandlerDeps) {
	instance := r.URL.Path
	if r.URL.RawQuery != "" {
		writeProblem(w, routeNotFound(r))
		return
	}

	// A duplicated Authorization or Idempotency-Key header has no single
	// value and is rejected like the Node array-header case.
	if len(r.Header.Values("Authorization")) > 1 {
		writeProblem(w, authenticationRequired(instance))
		return
	}
	identity, ok := VerifyAuthorizationHeader(r.Header.Get("Authorization"), deps.Secret, deps.NowSeconds())
	if !ok {
		writeProblem(w, authenticationRequired(instance))
		return
	}

	keyValues := r.Header.Values("Idempotency-Key")
	idempotencyKey := ""
	if len(keyValues) == 1 {
		idempotencyKey = keyValues[0]
	}
	bodyPresent := hasBody(r)
	if !uuidRe.MatchString(r.PathValue("labId")) || len(keyValues) != 1 || !keyRe.MatchString(idempotencyKey) || bodyPresent {
		writeProblem(w, invalidRequest(instance))
		return
	}

	response, err := service.Publish(PublishInput{
		Identity:       *identity,
		LabID:          r.PathValue("labId"),
		IdempotencyKey: idempotencyKey,
		Instance:       instance,
	})
	if err != nil {
		writeProblem(w, internalErrorResponse(instance))
		return
	}
	writeStored(w, response)
}

// hasBody reports whether any body bytes are present; `{}`, `null` and
// whitespace all count as bodies per the contract.
func hasBody(r *http.Request) bool {
	if r.ContentLength > 0 {
		return true
	}
	if r.ContentLength < 0 {
		buffer := make([]byte, 1)
		n, _ := io.ReadFull(r.Body, buffer)
		return n > 0
	}
	return false
}

func routeNotFound(r *http.Request) StoredResponse {
	return problemResponse("route-not-found", 404, "Route not found", routeNotFoundMsg, r.URL.Path)
}

func authenticationRequired(instance string) StoredResponse {
	return problemResponse("authentication-required", 401, "Authentication required",
		"Provide a valid, unexpired bearer token.", instance)
}

func invalidRequest(instance string) StoredResponse {
	return problemResponse("invalid-request", 400, "Invalid request",
		"Use a UUID labId, an 8-128 character visible-ASCII Idempotency-Key, and an empty body.", instance)
}

func writeProblem(w http.ResponseWriter, response StoredResponse) {
	writeStored(w, response)
}

func writeStored(w http.ResponseWriter, response StoredResponse) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", response.ContentType)
	w.WriteHeader(response.Status)
	_ = json.NewEncoder(w).Encode(response.Body)
}
