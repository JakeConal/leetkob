package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
	"github.com/JakeConal/leetkob/backend/internal/review"
)

// ReviewStore is the durable boundary for the assigned-TA queue and
// revision-bound decisions. Both operations re-check class membership in the
// store so another class's queue stays concealed.
type ReviewStore interface {
	ListReviewQueue(ctx context.Context, query postgres.ReviewQueueQuery) (postgres.ReviewQueue, error)
	RecordReviewDecision(ctx context.Context, command postgres.DecisionCommand) (review.Finding, error)
}

// maxReviewDecisionBodyBytes bounds the JSON decision envelope: a 2000
// character reason plus one 4096 character evidence citation.
const maxReviewDecisionBodyBytes = 16 << 10

const decideFindingOperation = "decide-finding"

// reviewHandlers implements the PA#3 /v2 review routes.
type reviewHandlers struct {
	auth      *auth.Service
	reviews   ReviewStore
	idem      IdempotencyStore
	webOrigin string
}

type evidenceView struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

type decisionView struct {
	Verdict   string        `json:"verdict"`
	Reason    string        `json:"reason"`
	Evidence  *evidenceView `json:"evidence,omitempty"`
	Revision  int           `json:"revision"`
	DecidedBy string        `json:"decidedBy"`
	DecidedAt string        `json:"decidedAt"`
}

// findingView is the private TA projection from the published Finding
// schema. It deliberately carries no student-facing fields.
type findingView struct {
	ID                 string         `json:"id"`
	SubmissionID       string         `json:"submissionId"`
	LabID              string         `json:"labId"`
	ConstraintID       string         `json:"constraintId"`
	ConstraintRevision int            `json:"constraintRevision"`
	Source             string         `json:"source"`
	CheckerVerdict     string         `json:"checkerVerdict"`
	UnsureCause        *string        `json:"unsureCause,omitempty"`
	Evidence           []evidenceView `json:"evidence"`
	Deduction          float64        `json:"deduction"`
	RequiredReview     bool           `json:"requiredReview"`
	Decision           *decisionView  `json:"decision,omitempty"`
}

type reviewQueueView struct {
	Items             []findingView `json:"items"`
	NextCursor        *string       `json:"nextCursor"`
	ProcessingPending bool          `json:"processingPending"`
	ReviewPending     int           `json:"reviewPending"`
}

type decisionEvidenceRequest struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

type reviewDecisionRequest struct {
	ExpectedRevision int                      `json:"expectedRevision"`
	Verdict          string                   `json:"verdict"`
	Reason           string                   `json:"reason"`
	Evidence         *decisionEvidenceRequest `json:"evidence"`
}

// list handles GET /v2/labs/{labId}/reviews.
func (h *reviewHandlers) list(w http.ResponseWriter, r *http.Request) {
	identity, ok := resolveIdentity(r.Context(), h.auth, w, r)
	if !ok {
		return
	}
	if !requireRole(w, r, identity, auth.RoleTA) {
		return
	}
	labID := r.PathValue("labId")
	if !isUUID(labID) {
		writeInvalidRequestProblem(w, r)
		return
	}
	page, ok := parsePage(w, r)
	if !ok {
		return
	}

	queue, err := h.reviews.ListReviewQueue(r.Context(), postgres.ReviewQueueQuery{
		LabID:   labID,
		ActorID: identity.User.ID,
		Limit:   page.limit + 1,
		Offset:  page.offset,
	})
	if err != nil {
		if errors.Is(err, postgres.ErrReviewNotFound) {
			writeConcealedNotFoundProblem(w, r)
			return
		}
		writeInternalErrorProblem(w, r)
		return
	}

	items := queue.Items
	hasMore := len(items) > page.limit
	if hasMore {
		items = items[:page.limit]
	}
	views := make([]findingView, 0, len(items))
	for _, finding := range items {
		views = append(views, reviewFindingView(finding))
	}
	var cursor *string
	if hasMore {
		cursor = nextCursor(page.offset + page.limit)
	}
	writeJSONResponse(w, r, http.StatusOK, reviewQueueView{
		Items:             views,
		NextCursor:        cursor,
		ProcessingPending: queue.ProcessingPending,
		ReviewPending:     queue.ReviewPending,
	})
}

// decide handles POST /v2/labs/{labId}/reviews/{findingId}/decision.
func (h *reviewHandlers) decide(w http.ResponseWriter, r *http.Request) {
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
	labID := r.PathValue("labId")
	findingID := r.PathValue("findingId")
	if !isUUID(labID) || !isUUID(findingID) {
		writeInvalidRequestProblem(w, r)
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

	body, request, ok := readDecisionBody(w, r)
	if !ok {
		return
	}
	if request.ExpectedRevision < 1 {
		writeInvalidRequestProblem(w, r)
		return
	}
	var evidence *review.Evidence
	if request.Evidence != nil {
		evidence = &review.Evidence{Line: request.Evidence.Line, Text: request.Evidence.Text}
	}
	if err := review.ValidateDecisionRequest(request.Verdict, request.Reason, evidence); err != nil {
		writeInvalidRequestProblem(w, r)
		return
	}

	digest := sha256.Sum256(body)
	fingerprint := hex.EncodeToString(digest[:])
	record, err := h.idem.FindIdempotency(r.Context(), identity.User.ID, decideFindingOperation, findingID, key)
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
		// Continue with the original decision.
	default:
		writeInternalErrorProblem(w, r)
		return
	}

	finding, err := h.reviews.RecordReviewDecision(r.Context(), postgres.DecisionCommand{
		LabID:            labID,
		FindingID:        findingID,
		ActorID:          identity.User.ID,
		ExpectedRevision: request.ExpectedRevision,
		Verdict:          request.Verdict,
		Reason:           request.Reason,
		Evidence:         evidence,
		DecidedAt:        time.Now().UTC(),
	})
	if err != nil {
		var stale *postgres.StaleRevisionError
		var invalid *review.ValidationError
		switch {
		case errors.As(err, &stale):
			writeProblemWithExtensions(w, http.StatusConflict, "conflict", "Conflict",
				"The item changed before this request was applied.", r.URL.Path,
				map[string]any{
					"conflictKind":     "stale-revision",
					"expectedRevision": stale.Expected,
					"currentRevision":  stale.Current,
				})
		case errors.As(err, &invalid):
			writeInvalidRequestProblem(w, r)
		case errors.Is(err, postgres.ErrReviewNotFound):
			writeConcealedNotFoundProblem(w, r)
		default:
			writeInternalErrorProblem(w, r)
		}
		return
	}

	payload, err := json.Marshal(reviewFindingView(finding))
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	if err := h.idem.SaveIdempotency(r.Context(), postgres.IdempotencyRecord{
		ActorID:        identity.User.ID,
		Operation:      decideFindingOperation,
		ResourceID:     findingID,
		IdempotencyKey: key,
		Fingerprint:    fingerprint,
		Status:         http.StatusOK,
		ContentType:    "application/json; charset=utf-8",
		Body:           payload,
	}); err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	writeRawResponse(w, http.StatusOK, "application/json; charset=utf-8", payload)
}

// readDecisionBody reads one bounded JSON object and rejects unknown members
// and trailing values.
func readDecisionBody(w http.ResponseWriter, r *http.Request) ([]byte, reviewDecisionRequest, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReviewDecisionBodyBytes))
	if err != nil {
		if isBodyTooLarge(err) {
			writePayloadTooLargeProblem(w, r)
			return nil, reviewDecisionRequest{}, false
		}
		writeInvalidRequestProblem(w, r)
		return nil, reviewDecisionRequest{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request reviewDecisionRequest
	if err := decoder.Decode(&request); err != nil {
		writeInvalidRequestProblem(w, r)
		return nil, reviewDecisionRequest{}, false
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeInvalidRequestProblem(w, r)
		return nil, reviewDecisionRequest{}, false
	}
	return body, request, true
}

func reviewFindingView(finding review.Finding) findingView {
	view := findingView{
		ID:                 finding.ID,
		SubmissionID:       finding.SubmissionID,
		LabID:              finding.LabID,
		ConstraintID:       finding.ConstraintID,
		ConstraintRevision: finding.ConstraintRevision,
		Source:             finding.Source,
		CheckerVerdict:     finding.CheckerVerdict,
		UnsureCause:        finding.UnsureCause,
		Evidence:           evidenceViews(finding.Evidence),
		Deduction:          float64(finding.DeductionCents) / 100,
		RequiredReview:     finding.RequiredReview(),
	}
	if finding.Decision != nil {
		decision := &decisionView{
			Verdict:   finding.Decision.Verdict,
			Reason:    finding.Decision.Reason,
			Revision:  finding.Decision.Revision,
			DecidedBy: finding.Decision.DecidedBy,
			DecidedAt: finding.Decision.DecidedAt.UTC().Format(time.RFC3339Nano),
		}
		if finding.Decision.Evidence != nil {
			evidence := evidenceView{Line: finding.Decision.Evidence.Line, Text: finding.Decision.Evidence.Text}
			decision.Evidence = &evidence
		}
		view.Decision = decision
	}
	return view
}

func evidenceViews(evidence []review.Evidence) []evidenceView {
	views := make([]evidenceView, 0, len(evidence))
	for _, item := range evidence {
		views = append(views, evidenceView{Line: item.Line, Text: item.Text})
	}
	return views
}
