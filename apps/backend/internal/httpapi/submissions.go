package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
)

// MaxSubmissionSourceBytes is the documented 65,536-byte source bound.
const MaxSubmissionSourceBytes = 65536

// maxSubmissionBodyBytes bounds the JSON envelope around the source.
const maxSubmissionBodyBytes = MaxSubmissionSourceBytes + 4096

var submissionUUIDPattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// SubmissionAcceptor is the durable acceptance boundary implemented by the
// PostgreSQL store.
type SubmissionAcceptor interface {
	AcceptSubmission(ctx context.Context, command postgres.AcceptCommand) (postgres.AcceptResult, error)
}

type submissionHandlers struct {
	auth        *auth.Service
	submissions SubmissionAcceptor
	webOrigin   string
}

type submissionRequest struct {
	Source string `json:"source"`
}

type submissionResponse struct {
	ID         string `json:"id"`
	LabID      string `json:"labId"`
	State      string `json:"state"`
	ReceivedAt string `json:"receivedAt"`
}

// create handles POST /v2/labs/{labId}/submissions.
func (h *submissionHandlers) create(w http.ResponseWriter, r *http.Request) {
	if !originAllowed(h.webOrigin, w, r) {
		return
	}
	identity, ok := resolveIdentity(r.Context(), h.auth, w, r)
	if !ok {
		return
	}
	if !requireRole(w, r, identity, auth.RoleStudent) {
		return
	}
	csrfValid, verifyErr := h.auth.VerifyCSRF(r.Context(), identity, r.Header.Get("X-CSRF-Token"))
	if verifyErr != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	if !csrfValid {
		writeRoleRequiredProblem(w, r)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !idempotencyKeyValid(key) {
		writeInvalidRequestProblem(w, r)
		return
	}
	labID := r.PathValue("labId")
	if !submissionUUIDPattern.MatchString(labID) {
		writeInvalidRequestProblem(w, r)
		return
	}

	var request submissionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSubmissionBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if isBodyTooLarge(err) {
			writePayloadTooLargeProblem(w, r)
			return
		}
		writeInvalidRequestProblem(w, r)
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeInvalidRequestProblem(w, r)
		return
	}
	if request.Source == "" || len(request.Source) > MaxSubmissionSourceBytes {
		writeInvalidRequestProblem(w, r)
		return
	}

	digest := sha256.Sum256([]byte(request.Source))
	result, err := h.submissions.AcceptSubmission(r.Context(), postgres.AcceptCommand{
		ActorID:        identity.User.ID,
		LabID:          labID,
		Source:         request.Source,
		IdempotencyKey: key,
		Fingerprint:    hex.EncodeToString(digest[:]),
		ReceivedAt:     time.Now().UTC(),
	})
	if err != nil {
		var window *postgres.SubmissionWindowClosedError
		switch {
		case errors.As(err, &window):
			writeProblemWithExtensions(w, http.StatusConflict, "conflict", "Conflict",
				"The submission window for this lab is closed.", r.URL.Path,
				map[string]any{
					"conflictKind": "submission-window-closed",
					"deadline":     window.Deadline.UTC().Format(time.RFC3339Nano),
				})
		case errors.Is(err, postgres.ErrIdempotencyContentConflict):
			writeProblemWithExtensions(w, http.StatusConflict, "conflict", "Conflict",
				"The idempotency key was already used with different request content.", r.URL.Path,
				map[string]any{"conflictKind": "idempotency-content-mismatch"})
		case errors.Is(err, postgres.ErrLabNotAccepting):
			writeConcealedNotFoundProblem(w, r)
		default:
			writeInternalErrorProblem(w, r)
		}
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSONResponse(w, r, http.StatusAccepted, submissionResponse{
		ID: result.SubmissionID, LabID: result.LabID, State: result.State,
		ReceivedAt: result.ReceivedAt.UTC().Format(time.RFC3339Nano),
	})
}

// ensureJSONEOF rejects trailing JSON content after the decoded value.
func ensureJSONEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("trailing JSON value")
}

func idempotencyKeyValid(key string) bool {
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

// isBodyTooLarge reports whether the bounded reader refused the body.
func isBodyTooLarge(err error) bool {
	var maxBytes *http.MaxBytesError
	return errors.As(err, &maxBytes) || strings.Contains(err.Error(), "request body too large")
}

func writePayloadTooLargeProblem(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusRequestEntityTooLarge, "payload-too-large", "Payload too large",
		"The request body exceeds the accepted size limit.", r.URL.Path)
}
