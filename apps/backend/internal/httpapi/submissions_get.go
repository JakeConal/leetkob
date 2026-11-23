package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
)

// SubmissionReader is the safe durable read boundary for stored judge
// results. LabClassID is used only for current authorization.
type SubmissionReader interface {
	GetSubmissionForRead(ctx context.Context, submissionID string) (postgres.SubmissionView, error)
	LabClassID(ctx context.Context, labID string) (string, error)
	ListSubmissionsForLab(ctx context.Context, labID string, limit, offset int) ([]postgres.SubmissionView, error)
	ListSubmissionsForStudent(ctx context.Context, labID, studentID string, limit, offset int) ([]postgres.SubmissionView, error)
}

type submissionReadHandlers struct {
	auth  *auth.Service
	reads SubmissionReader
}

type judgeTestView struct {
	ID      string `json:"id"`
	Verdict string `json:"verdict"`
	TimeMs  int    `json:"timeMs"`
}

type judgeView struct {
	Verdict string          `json:"verdict"`
	Tests   []judgeTestView `json:"tests"`
}

// submissionReadView is a positive allowlist: identity, state, receipt time
// and per-test verdicts only. Hidden inputs, expected outputs, checker
// findings and deductions have no field here.
type submissionReadView struct {
	ID         string     `json:"id"`
	LabID      string     `json:"labId"`
	State      string     `json:"state"`
	ReceivedAt string     `json:"receivedAt"`
	Judge      *judgeView `json:"judge,omitempty"`
}

type submissionListView struct {
	Items      []submissionReadView `json:"items"`
	NextCursor *string              `json:"nextCursor"`
}

// get handles GET /v2/submissions/{submissionId}. Only the owner or an
// assigned TA of the submission's class may read it. Every other caller,
// including another student or another class's TA, receives the same
// concealed not-found response as an absent identifier.
func (h *submissionReadHandlers) get(w http.ResponseWriter, r *http.Request) {
	identity, ok := resolveIdentity(r.Context(), h.auth, w, r)
	if !ok {
		return
	}
	submissionID := r.PathValue("submissionId")
	if !isUUID(submissionID) {
		writeInvalidRequestProblem(w, r)
		return
	}
	view, err := h.reads.GetSubmissionForRead(r.Context(), submissionID)
	if err != nil {
		if errors.Is(err, postgres.ErrSubmissionNotFound) {
			writeConcealedNotFoundProblem(w, r)
			return
		}
		writeInternalErrorProblem(w, r)
		return
	}
	allowed, err := h.canRead(r.Context(), identity, view)
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	if !allowed {
		writeConcealedNotFoundProblem(w, r)
		return
	}
	writeJSONResponse(w, r, http.StatusOK, readViewOf(view))
}

// list handles GET /v2/labs/{labId}/submissions. A student sees only their
// own submissions; the assigned TA sees the lab's class submissions. The
// ownership filter is applied in the store before pagination.
func (h *submissionReadHandlers) list(w http.ResponseWriter, r *http.Request) {
	identity, ok := resolveIdentity(r.Context(), h.auth, w, r)
	if !ok {
		return
	}
	labID := r.PathValue("labId")
	if !isUUID(labID) {
		writeInvalidRequestProblem(w, r)
		return
	}
	classID, err := h.reads.LabClassID(r.Context(), labID)
	if err != nil {
		if errors.Is(err, postgres.ErrLabNotFound) {
			writeConcealedNotFoundProblem(w, r)
			return
		}
		writeInternalErrorProblem(w, r)
		return
	}
	member, err := memberOfClass(r.Context(), h.auth, identity.User.ID, classID)
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	if !member {
		// Absent, another class's and non-member labs are indistinguishable.
		writeConcealedNotFoundProblem(w, r)
		return
	}
	page, ok := parsePage(w, r)
	if !ok {
		return
	}

	var views []postgres.SubmissionView
	switch identity.User.Role {
	case auth.RoleStudent:
		views, err = h.reads.ListSubmissionsForStudent(r.Context(), labID, identity.User.ID,
			page.limit+1, page.offset)
	case auth.RoleTA:
		views, err = h.reads.ListSubmissionsForLab(r.Context(), labID, page.limit+1, page.offset)
	default:
		// Admin is not an implicit TA.
		writeRoleRequiredProblem(w, r)
		return
	}
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}
	hasMore := len(views) > page.limit
	if hasMore {
		views = views[:page.limit]
	}
	items := make([]submissionReadView, 0, len(views))
	for _, view := range views {
		items = append(items, readViewOf(view))
	}
	var cursor *string
	if hasMore {
		cursor = nextCursor(page.offset + page.limit)
	}
	writeJSONResponse(w, r, http.StatusOK, submissionListView{Items: items, NextCursor: cursor})
}

// canRead applies current server-owned ownership and class membership. A
// caller who fails is concealed as a missing resource.
func (h *submissionReadHandlers) canRead(ctx context.Context, identity auth.Identity, view postgres.SubmissionView) (bool, error) {
	if view.StudentID == identity.User.ID {
		return true, nil
	}
	if identity.User.Role != auth.RoleTA {
		return false, nil
	}
	return memberOfClass(ctx, h.auth, identity.User.ID, view.ClassID)
}

func readViewOf(view postgres.SubmissionView) submissionReadView {
	projected := submissionReadView{
		ID:         view.ID,
		LabID:      view.LabID,
		State:      view.State,
		ReceivedAt: formatTime(view.ReceivedAt),
	}
	if view.Verdict == nil || view.State != "judged" {
		return projected
	}
	tests := make([]judgeTestView, 0, len(view.Tests))
	for _, test := range view.Tests {
		tests = append(tests, judgeTestView{ID: test.TestID, Verdict: test.Verdict, TimeMs: test.TimeMs})
	}
	projected.Judge = &judgeView{Verdict: *view.Verdict, Tests: tests}
	return projected
}
