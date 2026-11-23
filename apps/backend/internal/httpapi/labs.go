package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
)

// LabStore is the durable port for safe lab reads and atomic creation.
type LabStore interface {
	CreateLab(ctx context.Context, lab postgres.LabRecord) error
	GetLabForClass(ctx context.Context, labID, classID string) (postgres.LabRecord, error)
	ListLabsByClass(ctx context.Context, classID string, limit, offset int) ([]postgres.LabRecord, error)
}

// IdempotencyStore persists completed mutation responses so a retry with the
// same key and content replays the original result.
type IdempotencyStore interface {
	FindIdempotency(ctx context.Context, actorID, operation, resourceID, key string) (postgres.IdempotencyRecord, error)
	SaveIdempotency(ctx context.Context, record postgres.IdempotencyRecord) error
}

const (
	createLabOperation = "create-lab"
	labBodyLimit       = 12 << 20 // 12 MiB bound for the whole JSON body
	maxLabTestPayload  = 10 << 20 // combined input/output payload cap
)

// labHandlers implements the PA#3 /v2 class and lab routes.
type labHandlers struct {
	auth      *auth.Service
	labs      LabStore
	idem      IdempotencyStore
	webOrigin string
}

// modelNotice explains model sanitization and mandatory human approval. It
// is fixed server text, never caller-supplied.
const modelNotice = "Semantic constraints are checked by a configured model on code stripped of comments " +
	"and known student identities. A human teaching assistant reviews every finding and approves any " +
	"deduction before it is released; model output never assigns a deduction by itself."

// createLab handles POST /v2/classes/{classId}/labs.
func (h *labHandlers) createLab(w http.ResponseWriter, r *http.Request) {
	identity, ok := resolveIdentity(r.Context(), h.auth, w, r)
	if !ok {
		return
	}
	if !originAllowed(h.webOrigin, w, r) {
		return
	}
	if !requireRole(w, r, identity, auth.RoleTA) {
		return
	}
	classID := r.PathValue("classId")
	if !isUUID(classID) {
		writeInvalidRequestProblem(w, r)
		return
	}
	member, err := memberOfClass(r.Context(), h.auth, identity.User.ID, classID)
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	if !member {
		// Another TA's class is indistinguishable from an absent class.
		writeConcealedNotFoundProblem(w, r)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !validIdempotencyKey(key) {
		writeInvalidRequestProblem(w, r)
		return
	}
	csrfValid, err := h.auth.VerifyCSRF(r.Context(), identity, r.Header.Get("X-CSRF-Token"))
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	if !csrfValid {
		writeRoleRequiredProblem(w, r)
		return
	}

	body, ok := readLabBody(w, r)
	if !ok {
		return
	}
	fingerprint := requestFingerprint(body)

	record, err := h.idem.FindIdempotency(r.Context(), identity.User.ID, createLabOperation, classID, key)
	switch {
	case err == nil:
		if record.Fingerprint != fingerprint {
			writeProblemWithExtensions(w, http.StatusConflict, "conflict", "Conflict",
				"The idempotency key was already used with different request content.",
				r.URL.Path, map[string]any{"conflictKind": "idempotency-content-mismatch"})
			return
		}
		writeRawResponse(w, record.Status, record.ContentType, record.Body)
		return
	case errors.Is(err, postgres.ErrIdempotencyNotFound):
		// Continue with the original creation.
	default:
		writeInternalErrorProblem(w, r)
		return
	}

	lab, ok := parseLabCreate(body, classID, identity.User.ID)
	if !ok {
		writeInvalidRequestProblem(w, r)
		return
	}
	if err := h.labs.CreateLab(r.Context(), lab); err != nil {
		writeInternalErrorProblem(w, r)
		return
	}

	payload := marshalLab(lab)
	if payload == nil {
		writeInternalErrorProblem(w, r)
		return
	}
	if err := h.idem.SaveIdempotency(r.Context(), postgres.IdempotencyRecord{
		ActorID:        identity.User.ID,
		Operation:      createLabOperation,
		ResourceID:     classID,
		IdempotencyKey: key,
		Fingerprint:    fingerprint,
		Status:         http.StatusCreated,
		ContentType:    "application/json; charset=utf-8",
		Body:           payload,
	}); err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	writeRawResponse(w, http.StatusCreated, "application/json; charset=utf-8", payload)
}

// listLabs handles GET /v2/classes/{classId}/labs.
func (h *labHandlers) listLabs(w http.ResponseWriter, r *http.Request) {
	identity, ok := resolveIdentity(r.Context(), h.auth, w, r)
	if !ok {
		return
	}
	classID := r.PathValue("classId")
	if !isUUID(classID) {
		writeInvalidRequestProblem(w, r)
		return
	}
	member, err := memberOfClass(r.Context(), h.auth, identity.User.ID, classID)
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	if !member {
		writeConcealedNotFoundProblem(w, r)
		return
	}
	page, ok := parsePage(w, r)
	if !ok {
		return
	}

	labs, err := h.labs.ListLabsByClass(r.Context(), classID, page.limit+1, page.offset)
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	hasMore := len(labs) > page.limit
	if hasMore {
		labs = labs[:page.limit]
	}
	items := make([]labSummaryView, 0, len(labs))
	for _, lab := range labs {
		items = append(items, labSummary(lab))
	}
	var cursor *string
	if hasMore {
		cursor = nextCursor(page.offset + page.limit)
	}
	writeJSONResponse(w, r, http.StatusOK, labListView{Items: items, NextCursor: cursor})
}

// getLab handles GET /v2/labs/{labId} and projects only sample tests.
func (h *labHandlers) getLab(w http.ResponseWriter, r *http.Request) {
	identity, ok := resolveIdentity(r.Context(), h.auth, w, r)
	if !ok {
		return
	}
	labID := r.PathValue("labId")
	if !isUUID(labID) {
		writeInvalidRequestProblem(w, r)
		return
	}
	classes, err := h.auth.Classes(r.Context(), identity.User.ID)
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	for _, class := range classes {
		lab, err := h.labs.GetLabForClass(r.Context(), labID, class.ID)
		if err == nil {
			writeJSONResponse(w, r, http.StatusOK, labDetail(lab))
			return
		}
		if !errors.Is(err, postgres.ErrLabNotFound) {
			writeInternalErrorProblem(w, r)
			return
		}
	}
	// Absent, another class's, and non-member labs are indistinguishable.
	writeConcealedNotFoundProblem(w, r)
}

func readLabBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, labBodyLimit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(w, http.StatusRequestEntityTooLarge, "payload-too-large", "Payload too large",
				"The request body exceeds the accepted size limit.", r.URL.Path)
			return nil, false
		}
		writeInvalidRequestProblem(w, r)
		return nil, false
	}
	return body, true
}

func requestFingerprint(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

// validIdempotencyKey enforces 8-128 visible ASCII characters exactly.
func validIdempotencyKey(key string) bool {
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '!' || key[i] > '~' {
			return false
		}
	}
	return true
}

// isUUID accepts the canonical hyphenated UUID form used by the contract.
func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') && !(c >= 'A' && c <= 'F') {
				return false
			}
		}
	}
	return true
}

// newUUID returns a random RFC 4122 version 4 identifier.
func newUUID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80

	const hexDigits = "0123456789abcdef"
	out := make([]byte, 36)
	positions := []int{8, 13, 18, 23}
	offset := 0
	for i := 0; i < 36; i++ {
		if offset < len(positions) && i == positions[offset] {
			out[i] = '-'
			offset++
			continue
		}
		out[i] = hexDigits[buffer[(i-offset)/2]>>4]
		if (i-offset)%2 == 1 {
			out[i] = hexDigits[buffer[(i-offset)/2]&0x0f]
		}
	}
	return string(out), nil
}

// formatTime renders RFC 3339 UTC with milliseconds, as pinned by the
// problem/response fixture notes.
func formatTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000Z")
}
