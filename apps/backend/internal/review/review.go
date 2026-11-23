// Package review holds the PA#3 rules for the assigned TA's private finding
// queue and for revision-bound TA decisions. It is pure domain logic: it
// imports no storage, HTTP or authentication package, and every clock or
// persistence effect is supplied by the caller.
package review

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Checker verdict values stored on a finding.
const (
	VerdictPass      = "pass"
	VerdictViolation = "violation"
	VerdictUnsure    = "unsure"
)

// TA decision verdict values. An unsure finding has no default decision, so
// only the two explicit choices are accepted.
const (
	DecisionPass      = VerdictPass
	DecisionViolation = VerdictViolation
)

// SamplePercent is the documented pass-pair sample rate.
const SamplePercent = 10

// Request bounds from the published PA#3 contract.
const (
	MaxReasonChars    = 2000
	MaxEvidenceText   = 4096
	MaxEvidenceLineNo = 1 << 30
)

// Evidence is one original-line citation from the submitted source. Line is
// one-based; Text is the exact line content.
type Evidence struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Decision is one append-only TA decision stored separately from the
// immutable checker provenance.
type Decision struct {
	Verdict   string
	Reason    string
	Evidence  *Evidence
	Revision  int
	DecidedBy string
	DecidedAt time.Time
}

// Finding is the TA-visible projection of one checker constraint verdict.
// Source is the original submitted source and never leaves the TA queue.
type Finding struct {
	ID                 string
	SubmissionID       string
	LabID              string
	ConstraintID       string
	ConstraintRevision int
	SourceKind         string
	Source             string
	CheckerVerdict     string
	UnsureCause        *string
	Evidence           []Evidence
	DeductionCents     int
	Sampled            bool
	Revision           int
	CreatedAt          time.Time
	Decision           *Decision
}

// RequiredReview reports whether this finding is a current obligation.
// Every violation and unsure requires review, as does the persisted pass
// sample. The decision history never removes an obligation by itself: the
// queue reports pending work separately.
func (f Finding) RequiredReview() bool {
	return f.CheckerVerdict == VerdictViolation ||
		f.CheckerVerdict == VerdictUnsure ||
		f.Sampled
}

// SampleSize returns ceil(passPairs * SamplePercent / 100): the stable
// random sample size for a lab's current counted pass pairs.
func SampleSize(passPairs int) int {
	if passPairs <= 0 {
		return 0
	}
	return (passPairs*SamplePercent + 99) / 100
}

// SampleIDs selects the persisted pass sample from the current counted pass
// finding IDs.
//
// Membership is a pure function of each immutable finding ID: IDs are ranked
// by SHA-256(id) ascending (ties broken by the ID text), and the first
// ceil(10%) are sampled. Hashing gives each pair an independent uniform rank,
// so the sample is random with respect to marking order, while the absence of
// mutable RNG state means refreshing an unchanged queue can never redraw it.
// New findings can change the membership of a *changed* queue; queue reads
// persist this selection and never call an RNG.
func SampleIDs(passIDs []string) []string {
	size := SampleSize(len(passIDs))
	if size == 0 {
		return nil
	}
	ranked := append([]string(nil), passIDs...)
	digests := make(map[string][sha256.Size]byte, len(ranked))
	for _, id := range ranked {
		digests[id] = sha256.Sum256([]byte(id))
	}
	sort.Slice(ranked, func(i, j int) bool {
		left, right := digests[ranked[i]], digests[ranked[j]]
		for k := 0; k < sha256.Size; k++ {
			if left[k] != right[k] {
				return left[k] < right[k]
			}
		}
		return ranked[i] < ranked[j]
	})
	return ranked[:size]
}

// SampledSet returns the sampled IDs as a set for membership checks.
func SampledSet(passIDs []string) map[string]bool {
	selected := SampleIDs(passIDs)
	set := make(map[string]bool, len(selected))
	for _, id := range selected {
		set[id] = true
	}
	return set
}

// ValidationError is one bounded request-field rejection. HTTP maps it to the
// documented 400 invalid-request problem; storage adapters return it without
// writing anything.
type ValidationError struct {
	Field  string
	Detail string
}

// Error implements error.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("review: invalid %s: %s", e.Field, e.Detail)
}

func invalid(field, detail string) *ValidationError {
	return &ValidationError{Field: field, Detail: detail}
}

// ValidateDecisionRequest checks the fields a TA supplies independently of
// the stored finding: explicit verdict, nonempty bounded reason and bounded
// evidence shape. It never invents a default for an unsure finding.
func ValidateDecisionRequest(verdict, reason string, evidence *Evidence) error {
	if verdict != DecisionPass && verdict != DecisionViolation {
		return invalid("verdict", "must be pass or violation")
	}
	if strings.TrimSpace(reason) == "" {
		return invalid("reason", "must not be blank")
	}
	if utf8.RuneCountInString(reason) > MaxReasonChars {
		return invalid("reason", fmt.Sprintf("must be at most %d characters", MaxReasonChars))
	}
	if evidence != nil {
		if evidence.Line < 1 || evidence.Line > MaxEvidenceLineNo {
			return invalid("evidence.line", "must be a positive line number")
		}
		if evidence.Text == "" || utf8.RuneCountInString(evidence.Text) > MaxEvidenceText {
			return invalid("evidence.text", fmt.Sprintf("must be between 1 and %d characters", MaxEvidenceText))
		}
	}
	if verdict == DecisionViolation && evidence == nil {
		return invalid("evidence", "is required for a violation decision")
	}
	return nil
}

// CanonicalEvidence validates a violation citation against the original
// submission source and returns the source line text. The cited line must
// exist and the text must match that line exactly; only CRLF line endings are
// normalized, so a TA cannot confirm a deduction from different code. This
// keeps stored decision evidence an original-line citation rather than free
// text.
func CanonicalEvidence(source string, evidence Evidence) (Evidence, error) {
	lines := sourceLines(source)
	if evidence.Line < 1 || evidence.Line > len(lines) {
		return Evidence{}, invalid("evidence.line", "does not exist in the submitted source")
	}
	line := lines[evidence.Line-1]
	if evidence.Text != line {
		return Evidence{}, invalid("evidence.text", "does not match the cited source line")
	}
	return Evidence{Line: evidence.Line, Text: line}, nil
}

// sourceLines splits source into one-based line content, normalizing CRLF and
// ignoring the synthetic empty element after a final newline.
func sourceLines(source string) []string {
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// MarshalEvidence serializes one optional evidence value for the decisions
// evidence column. A nil evidence is stored as NULL, not an empty object.
func MarshalEvidence(evidence *Evidence) ([]byte, error) {
	if evidence == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}
