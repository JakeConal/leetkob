// Package schema holds the shared checker documents used by the offline
// marking command and the online worker. Prediction lines follow the
// published checker-prediction.v1 contract. Analyzer results follow
// analyzer-result.v1, the bounded document exchanged with the native
// libclang program. Neither type is a student HTTP body.
package schema

import "fmt"

// Verdict is the checker outcome for one solution/constraint pair.
type Verdict string

const (
	// VerdictPass means the constraint holds.
	VerdictPass Verdict = "pass"
	// VerdictViolation means the constraint is broken, with original-line evidence.
	VerdictViolation Verdict = "violation"
	// VerdictUnsure means the checker could not decide. It is never a deduction.
	VerdictUnsure Verdict = "unsure"
)

// Evidence is one original source line cited by a prediction.
type Evidence struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Prediction is one JSONL object from checker-prediction.v1.json.
type Prediction struct {
	Solution   string     `json:"solution"`
	Constraint string     `json:"constraint"`
	Verdict    Verdict    `json:"verdict"`
	Evidence   []Evidence `json:"evidence"`
}

const (
	maxIDLen         = 256
	maxEvidenceText  = 4096
	maxEvidenceItems = 64
)

// ValidatePrediction checks the published prediction bounds. An empty
// evidence list is valid so an unsure line can still be written.
func ValidatePrediction(p Prediction) error {
	if p.Solution == "" || len(p.Solution) > maxIDLen {
		return fmt.Errorf("schema: solution length out of bounds")
	}
	if p.Constraint == "" || len(p.Constraint) > maxIDLen {
		return fmt.Errorf("schema: constraint length out of bounds")
	}
	switch p.Verdict {
	case VerdictPass, VerdictViolation, VerdictUnsure:
	default:
		return fmt.Errorf("schema: unknown verdict %q", p.Verdict)
	}
	if len(p.Evidence) > maxEvidenceItems {
		return fmt.Errorf("schema: too many evidence lines")
	}
	for _, item := range p.Evidence {
		if item.Line < 1 {
			return fmt.Errorf("schema: evidence line must be positive")
		}
		if item.Text == "" || len(item.Text) > maxEvidenceText {
			return fmt.Errorf("schema: evidence text length out of bounds")
		}
	}
	return nil
}
