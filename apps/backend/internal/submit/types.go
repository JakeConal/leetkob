// Package submit accepts a bounded C++ submission for an enrolled student
// and an open lab. The source, counted selection, judge job, and replay
// record commit in one transaction. The HTTP acknowledgement is written
// only after that commit returns.
package submit

import (
	"context"
	"errors"
	"time"
)

const (
	// AdmissionLease bounds an incomplete body. A new submission whose
	// bytes finish at or after this lease is not accepted.
	AdmissionLease = 30 * time.Second
	// MaxSourceBytes is the contract's 65,536-byte source ceiling.
	MaxSourceBytes = 65536
)

var (
	// ErrNotFound hides a missing lab, a lab that is not open yet, and a
	// caller who is not currently enrolled.
	ErrNotFound = errors.New("submit: not found")
	// ErrForbidden is a wrong role or a rejected CSRF/origin proof.
	ErrForbidden = errors.New("submit: forbidden")
	// ErrInvalid is a malformed request.
	ErrInvalid = errors.New("submit: invalid")
	// ErrLeaseExpired means the body finished after the admission lease.
	ErrLeaseExpired = errors.New("submit: admission lease expired")
	// ErrReplayRace is a concurrent insert of the same idempotency key.
	// The caller retries the read once the other transaction commits.
	ErrReplayRace = errors.New("submit: replay race")
)

// WindowClosed is a new submission whose complete body arrived at or after
// the lab deadline.
type WindowClosed struct {
	Deadline time.Time
}

func (WindowClosed) Error() string { return "submit: submission window closed" }

// IdempotencyMismatch is the same key with different source content.
type IdempotencyMismatch struct{}

func (IdempotencyMismatch) Error() string { return "submit: idempotency mismatch" }

// Principal is the authenticated account. Authorization uses the user id
// against current membership and the stored role, not a caller header.
type Principal struct {
	UserID    string
	CSRFToken string
	// CSRFHash is the stored digest. When it is set, the header is checked
	// against the hash. CSRFToken remains for tests that supply the proof
	// itself.
	CSRFHash []byte
}

// Admission is a committed lease taken before the body is consumed.
type Admission struct {
	ID        string
	UserID    string
	LabID     string
	ExpiresAt time.Time
}

// AcceptIn is one complete submission body.
type AcceptIn struct {
	UserID         string
	LabID          string
	LeaseID        string
	ReceivedAt     time.Time
	IdempotencyKey string
	Source         string
}

// AcceptOut is the durable acknowledgement. Body is the exact JSON that
// was committed in the replay row.
type AcceptOut struct {
	Status int
	Body   []byte
}

// TestView is one student-visible test result. It has no hidden I/O.
type TestView struct {
	ID      string
	Verdict string
	TimeMS  int
}

// JudgeView is present only after a student verdict is stored.
type JudgeView struct {
	Verdict string
	Tests   []TestView
}

// View is a safe submission projection.
type View struct {
	ID         string
	LabID      string
	State      string
	ReceivedAt time.Time
	Judge      *JudgeView
}

// Page is one list page. NextCursor is empty when the page is the last.
type Page struct {
	Items      []View
	NextCursor string
}

// ListQuery is an authorized list of one lab's submissions.
type ListQuery struct {
	UserID string
	LabID  string
	Limit  int
	Cursor string
	Now    time.Time
}

// Repository is the durable store. Implementations commit Accept before
// returning a successful AcceptOut.
type Repository interface {
	BeginAdmission(ctx context.Context, userID, labID string, now, expires time.Time) (Admission, error)
	AbortAdmission(ctx context.Context, leaseID string, now time.Time) error
	Accept(ctx context.Context, in AcceptIn) (AcceptOut, error)
	GetSubmission(ctx context.Context, userID, submissionID string, now time.Time) (View, error)
	ListSubmissions(ctx context.Context, q ListQuery) (Page, error)
}
