// Package model defines the PA#3 semantic-check port and the guarded
// standard-library HTTP adapter that talks to the configured model
// provider.
//
// The package owns only transport and validation. Callers sanitize the
// source before building a Request (issue #38 task 5.2), reserve priced
// spend before every dispatch (task 5.4) and wire the port into the worker
// and checker (task 5.5). The adapter never reads tests, labels, student
// metadata or TA deductions and never returns a score or grade.
//
// Tests substitute an httptest server or an injected Doer and must never
// contact a real provider.
package model

import (
	"context"
	"errors"
	"time"
)

// ErrModelOff is returned when MODEL_OFF=1. Offline operation must not
// create a provider client, read a credential or dispatch a request.
var ErrModelOff = errors.New("model: MODEL_OFF=1 forbids creating or using a provider client")

// ErrMissingAPIKey is returned when a dispatch is attempted without a
// configured provider credential. The key is read lazily so offline and
// non-dispatching paths never touch it.
var ErrMissingAPIKey = errors.New("model: provider API key is not configured")

// ErrInvalidRequest reports a caller error before any dispatch: the
// request is empty, unbounded, or has ambiguous constraint identifiers.
// It is not a model failure and never maps to an unsure verdict.
var ErrInvalidRequest = errors.New("model: invalid semantic check request")

// Verdict is the only set of outcomes a semantic check may report. A
// deduction, score or grade is never a verdict.
type Verdict string

// Allowed verdicts. They mirror docs/contract/schemas/checker-prediction.v1.json.
const (
	VerdictPass      Verdict = "pass"
	VerdictViolation Verdict = "violation"
	VerdictUnsure    Verdict = "unsure"
)

// UnsureCause explains why a dispatch or validation failure produced
// unsure. Model-declared unsure carries no transport cause.
type UnsureCause string

// Distinct unsure causes required by design decision 7 and PA3-CHECK-04/05.
const (
	CauseTimeout         UnsureCause = "timeout"
	CauseProviderFailure UnsureCause = "provider-failure"
	CauseSchemaInvalid   UnsureCause = "schema-invalid"
	CauseEvidenceInvalid UnsureCause = "evidence-invalid"
)

// Constraint is one requested constraint. The adapter sends only its ID
// and text; TA-configured deductions remain server-side and are never
// part of a provider request.
type Constraint struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Evidence is one original-source citation. Line is one-based in the
// sanitized source supplied with the request; the sanitizer preserves
// newlines and therefore the original line mapping. Text must be present
// on that line.
type Evidence struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Request is one bounded semantic check. Source MUST already be
// sanitized by task 5.2 (comments and known student identities removed,
// newlines preserved). The adapter does not sanitize and does not accept
// unsanitized source; unsafe sanitization is expected to become unsure
// before this port is called.
type Request struct {
	Statement   string
	Constraints []Constraint
	Source      string
}

// ConstraintResult is the validated outcome for one requested
// constraint. Evidence is empty for a clean pass. Cause is set only when
// the adapter produced unsure after a dispatch or validation failure; a
// model-declared unsure leaves it empty.
type ConstraintResult struct {
	ConstraintID string
	Verdict      Verdict
	Evidence     []Evidence
	Cause        UnsureCause
}

// Result is the bounded outcome of one Check call. It carries just enough
// provenance for task 5.5 to record the restricted call evidence; it
// never carries prompts, raw provider output or credentials.
type Result struct {
	Constraints []ConstraintResult
	// Cause is the failure that made every constraint unsure. Empty on a
	// validated model response, including a model-declared unsure.
	Cause UnsureCause
	// ProviderRequestID is the provider's own request identifier when the
	// provider returned one.
	ProviderRequestID string
	// Model is the configured model name.
	Model string
	// InputTokens and OutputTokens aggregate every dispatch, including the
	// schema-repair retry, and are what task 5.4 settles.
	InputTokens  int
	OutputTokens int
	// Attempts is the number of provider dispatches, 1 or 2.
	Attempts int
	// Elapsed is the wall time of the whole Check call.
	Elapsed time.Duration
}

// SemanticChecker checks every requested constraint in one bounded call.
//
// Implementations MUST NOT dispatch when MODEL_OFF=1. They MUST return
// ErrInvalidRequest for caller errors, and an unsure Result (with a
// distinct Cause and a nil error) for timeout, provider failure and
// strict schema/evidence failure.
type SemanticChecker interface {
	Check(ctx context.Context, request Request) (Result, error)
}
