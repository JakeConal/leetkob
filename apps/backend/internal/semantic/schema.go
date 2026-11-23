package semantic

import (
	"encoding/json"
	"io"
	"strings"
)

type wireOutput struct {
	Results []wireResult `json:"results"`
}

type wireResult struct {
	ConstraintID string          `json:"constraintId"`
	Verdict      string          `json:"verdict"`
	Evidence     *[]wireEvidence `json:"evidence"`
}

type wireEvidence struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// parseModelOutput enforces one result per requested constraint. Evidence
// text must match the original line or the sanitized line the model saw;
// stored text is always the original line.
func parseModelOutput(raw string, constraints []Constraint, original, sanitized []string) ([]Finding, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var out wireOutput
	if err := dec.Decode(&out); err != nil {
		return nil, ErrSchemaInvalid
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, ErrSchemaInvalid
	}
	want := make(map[string]struct{}, len(constraints))
	for _, constraint := range constraints {
		if _, dup := want[constraint.ID]; dup {
			return nil, ErrSchemaInvalid
		}
		want[constraint.ID] = struct{}{}
	}
	got := make(map[string]wireResult, len(out.Results))
	for _, result := range out.Results {
		if _, ok := want[result.ConstraintID]; !ok {
			return nil, ErrSchemaInvalid
		}
		if _, dup := got[result.ConstraintID]; dup {
			return nil, ErrSchemaInvalid
		}
		if result.Evidence == nil {
			return nil, ErrSchemaInvalid
		}
		switch result.Verdict {
		case string(VerdictPass):
			if len(*result.Evidence) != 0 {
				return nil, ErrSchemaInvalid
			}
		case string(VerdictViolation):
			if len(*result.Evidence) == 0 {
				return nil, ErrSchemaInvalid
			}
		case string(VerdictUnsure):
		default:
			return nil, ErrSchemaInvalid
		}
		got[result.ConstraintID] = result
	}
	if len(got) != len(want) {
		return nil, ErrSchemaInvalid
	}
	findings := make([]Finding, len(constraints))
	for i, constraint := range constraints {
		result := got[constraint.ID]
		finding := Finding{ConstraintID: constraint.ID, Verdict: Verdict(result.Verdict)}
		if finding.Verdict == VerdictPass {
			findings[i] = finding
			continue
		}
		evidence := make([]Evidence, 0, len(*result.Evidence))
		for _, item := range *result.Evidence {
			if !evidenceMatches(item.Text, item.Line, original, sanitized) {
				return nil, ErrEvidenceMismatch
			}
			evidence = append(evidence, Evidence{Line: item.Line, Text: original[item.Line-1]})
		}
		finding.Evidence = evidence
		findings[i] = finding
	}
	return findings, nil
}

func evidenceMatches(text string, line int, original, sanitized []string) bool {
	if line < 1 || line > len(original) {
		return false
	}
	cited := strings.TrimSpace(text)
	if cited == strings.TrimSpace(original[line-1]) {
		return true
	}
	return line <= len(sanitized) && cited == strings.TrimSpace(sanitized[line-1])
}
