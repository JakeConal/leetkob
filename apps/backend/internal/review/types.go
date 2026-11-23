// Package review is the assigned teaching assistant's private queue and the
// append-only decision record. Checker provenance stays on the finding.
// Nothing in this package publishes a mark.
package review

import (
	"context"
	"time"
)

// Pair is one submission/constraint result used to choose review obligations.
type Pair struct {
	ConstraintID string
	Verdict      string
}

// Evidence is one original source line. Text is the submitted line, not a
// model paraphrase.
type Evidence struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Deduction is a TA-configured deduction in hundredths of a point.
// 300 is 3.00. The model never chooses this value.
type Deduction int

// MarshalJSON writes the fixed two-decimal contract number.
func (d Deduction) MarshalJSON() ([]byte, error) {
	if d < 0 || d > 1000 {
		return nil, errDeduction
	}
	whole := int(d) / 100
	frac := int(d) % 100
	return []byte(itoa(whole) + "." + twoDigits(frac)), nil
}

// TADecision is one explicit TA choice. Revision is the finding revision
// after this decision, which is the value the next request must send.
type TADecision struct {
	Verdict   string    `json:"verdict"`
	Reason    string    `json:"reason"`
	Evidence  *Evidence `json:"evidence,omitempty"`
	Revision  int       `json:"revision"`
	DecidedBy string    `json:"decidedBy"`
	DecidedAt string    `json:"decidedAt"`
}

// Finding is one current queue row. Checker fields are not updated when a
// decision is appended.
type Finding struct {
	ID                 string      `json:"id"`
	SubmissionID       string      `json:"submissionId"`
	LabID              string      `json:"labId"`
	ConstraintID       string      `json:"constraintId"`
	ConstraintRevision int         `json:"constraintRevision"`
	Revision           int         `json:"revision"`
	SourceKind         string      `json:"sourceKind"`
	CheckerVerdict     string      `json:"checkerVerdict"`
	UnsureCause        string      `json:"unsureCause,omitempty"`
	Evidence           []Evidence  `json:"evidence"`
	Deduction          Deduction   `json:"deduction"`
	RequiredReview     bool        `json:"requiredReview"`
	Source             string      `json:"source"`
	Decision           *TADecision `json:"decision,omitempty"`
}

// Page is one assigned-TA queue response.
type Page struct {
	Items             []Finding `json:"items"`
	NextCursor        *string   `json:"nextCursor"`
	ProcessingPending bool      `json:"processingPending"`
	ReviewPending     int       `json:"reviewPending"`
}

// Access is the server's view of a user and a lab. Role comes from the
// user record, never from a request header.
type Access struct {
	UserExists bool
	LabExists  bool
	Role       string
	Member     bool
}

// Allowed reports whether this user is a current TA of the lab's class.
func Allowed(access Access) bool {
	return access.UserExists && access.LabExists && access.Role == "ta" && access.Member
}

// DecisionRequest is one explicit decision. Evidence is required only when
// Verdict is violation.
type DecisionRequest struct {
	LabID            string
	FindingID        string
	ActorID          string
	ExpectedRevision int
	Verdict          string
	Reason           string
	Evidence         *Evidence
	IdempotencyKey   string
	Fingerprint      string
	Now              time.Time
}

// DecisionResult is the outcome of a committed decision attempt.
// Stale and Mismatch are business results. A non-nil error means nothing
// was committed.
type DecisionResult struct {
	Finding  Finding
	Replay   bool
	Stale    *StaleRevision
	Mismatch bool
}

// Store is the queue and decision port. HTTP and the postgres adapter both
// use it. Reads must return the stored sample, not draw a new one.
type Store interface {
	Access(ctx context.Context, labID, userID string) (Access, error)
	List(ctx context.Context, labID string, limit int, cursor string) (Page, error)
	Decide(ctx context.Context, req DecisionRequest) (DecisionResult, error)
}
