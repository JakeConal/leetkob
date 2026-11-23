package checker

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Prediction is one line of the JSONL file written by
// `checker --data <dir> --predictions <file>`. It mirrors
// docs/contract/schemas/checker-prediction.v1.json exactly:
//
//	{"solution":"...","constraint":"...","verdict":"pass|violation|unsure","evidence":[{"line":1,"text":"..."}]}
//
// Evidence is always present in the encoded line, including when it is
// empty, because the schema requires the property even for unsure rows.
type Prediction struct {
	Solution   string     `json:"solution"`
	Constraint string     `json:"constraint"`
	Verdict    Verdict    `json:"verdict"`
	Evidence   []Evidence `json:"evidence"`
}

// Verdict is the checker result for one solution/constraint pair.
type Verdict string

const (
	// VerdictPass means the submitted source complies with the constraint.
	VerdictPass Verdict = "pass"
	// VerdictViolation means deterministic rules or the semantic model
	// found a violation and supplied original-line evidence.
	VerdictViolation Verdict = "violation"
	// VerdictUnsure means the checker cannot decide. It is never a
	// deduction and has no default TA verdict.
	VerdictUnsure Verdict = "unsure"
)

// Valid reports whether v is one of the schema's verdict enum values.
func (v Verdict) Valid() bool {
	switch v {
	case VerdictPass, VerdictViolation, VerdictUnsure:
		return true
	default:
		return false
	}
}

// Evidence is one original source line cited by a checker prediction.
// Line is one-based; Text is the original line content.
type Evidence struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// ErrInvalidPrediction marks a prediction that cannot be written under
// docs/contract/schemas/checker-prediction.v1.json.
var ErrInvalidPrediction = errors.New("checker: invalid prediction")

// Schema bounds from checker-prediction.v1.json.
const (
	maxPredictionLabelLength = 256
	maxEvidenceTextLength    = 4096
)

// Validate checks p against the contract schema and the evidence rule
// that a violation must cite at least one original source line. Callers
// should validate before writing JSONL; a failure is a checker bug, not a
// reason to emit a deduction.
func (p Prediction) Validate() error {
	if err := validateLabel("solution", p.Solution); err != nil {
		return err
	}
	if err := validateLabel("constraint", p.Constraint); err != nil {
		return err
	}
	if !p.Verdict.Valid() {
		return fmt.Errorf("%w: verdict %q is not pass, violation or unsure", ErrInvalidPrediction, p.Verdict)
	}
	if p.Verdict == VerdictViolation && len(p.Evidence) == 0 {
		return fmt.Errorf("%w: violation %q/%q has no original-line evidence", ErrInvalidPrediction, p.Solution, p.Constraint)
	}
	for i, e := range p.Evidence {
		if e.Line < 1 {
			return fmt.Errorf("%w: evidence[%d] line %d is not one-based", ErrInvalidPrediction, i, e.Line)
		}
		if text := []rune(e.Text); len(text) == 0 || len(text) > maxEvidenceTextLength {
			return fmt.Errorf("%w: evidence[%d] text length %d is outside 1-%d", ErrInvalidPrediction, i, len(text), maxEvidenceTextLength)
		}
		if !utf8.ValidString(e.Text) {
			return fmt.Errorf("%w: evidence[%d] text is not valid UTF-8", ErrInvalidPrediction, i)
		}
	}
	return nil
}

func validateLabel(field, value string) error {
	if value == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidPrediction, field)
	}
	if utf8.RuneCountInString(value) > maxPredictionLabelLength {
		return fmt.Errorf("%w: %s exceeds %d characters", ErrInvalidPrediction, field, maxPredictionLabelLength)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidPrediction, field)
	}
	return nil
}

// MarshalJSON encodes a prediction with evidence as an empty array when
// the caller left it nil. Without this, a pass/unsure prediction would
// encode `"evidence":null`, which the published schema rejects.
func (p Prediction) MarshalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	type wirePrediction struct {
		Solution   string     `json:"solution"`
		Constraint string     `json:"constraint"`
		Verdict    Verdict    `json:"verdict"`
		Evidence   []Evidence `json:"evidence"`
	}
	evidence := p.Evidence
	if evidence == nil {
		evidence = []Evidence{}
	}
	return json.Marshal(wirePrediction{
		Solution:   p.Solution,
		Constraint: p.Constraint,
		Verdict:    p.Verdict,
		Evidence:   evidence,
	})
}

// Ban-list constraint prefixes from docs/contract/pa3-api.md. Matching is
// case-insensitive so a lowercase authoring slip still becomes a hard
// rule rather than an accidental model dispatch.
const (
	banUsePrefix  = "do not use "
	banCallPrefix = "do not call "
)

// MaxBannedNamesPerConstraint bounds generic extraction from one
// constraint text. It is a safety limit, not a contract field.
const MaxBannedNamesPerConstraint = 64

// IsBanListConstraint reports whether text is a hard-rule constraint
// according to the contract: after leading whitespace it starts with
// "Do not use " or "Do not call " (matched case-insensitively).
func IsBanListConstraint(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(trimmed, banUsePrefix) || strings.HasPrefix(trimmed, banCallPrefix)
}

// BannedNames extracts backticked names from a ban-list constraint. It is
// deliberately generic: it knows only the two contract prefixes and
// backtick quoting, never a development problem's answer. A text that is
// not a ban list, or that names nothing in backticks, returns nil.
//
// Names are trimmed, deduplicated in first-seen order, and bounded to
// MaxBannedNamesPerConstraint entries. An overlong or empty backticked
// fragment is ignored rather than guessed at; the caller can treat a
// missing decision as unsure.
func BannedNames(text string) []string {
	if !IsBanListConstraint(text) {
		return nil
	}
	trimmed := strings.TrimSpace(text)
	var names []string
	seen := make(map[string]struct{})
	for offset := 0; offset < len(trimmed); {
		open := strings.IndexByte(trimmed[offset:], '`')
		if open < 0 {
			break
		}
		open += offset
		closeOffset := strings.IndexByte(trimmed[open+1:], '`')
		if closeOffset < 0 {
			break
		}
		closeIndex := open + 1 + closeOffset
		name := strings.TrimSpace(trimmed[open+1 : closeIndex])
		offset = closeIndex + 1
		if name == "" || len(name) > 256 {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
		if len(names) == MaxBannedNamesPerConstraint {
			break
		}
	}
	return names
}
