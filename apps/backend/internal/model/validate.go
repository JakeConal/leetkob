package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// outputKind classifies a model response after strict decoding. Schema
// problems are structural and may use the single repair retry; evidence
// problems are content failures and never retry.
type outputKind int

const (
	outputValid outputKind = iota
	outputSchemaInvalid
	outputEvidenceInvalid
)

// wireOutput is the only accepted model response object. Unknown fields,
// including score, deduction, points, grade and mark, are rejected by the
// decoder.
type wireOutput struct {
	Results []wireResult `json:"results"`
}

type wireResult struct {
	ConstraintID string      `json:"constraintId"`
	Verdict      Verdict     `json:"verdict"`
	Evidence     *[]Evidence `json:"evidence"`
}

// sourceLines splits source with preserved original line numbering. The
// sanitizer from task 5.2 preserves newlines, so the same one-based
// numbering applies to the original submission. A single trailing newline
// does not create a phantom line.
func sourceLines(source string) []string {
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

// validateRequest bounds caller input before any credential or provider
// access. It mirrors the approved lab limits so a malformed request can
// never become an unbounded prompt.
func validateRequest(request Request) error {
	if strings.TrimSpace(request.Statement) == "" {
		return fmt.Errorf("%w: statement is required", ErrInvalidRequest)
	}
	if len(request.Statement) > MaxStatementBytes {
		return fmt.Errorf("%w: statement exceeds %d bytes", ErrInvalidRequest, MaxStatementBytes)
	}
	if request.Source == "" {
		return fmt.Errorf("%w: sanitized source is required", ErrInvalidRequest)
	}
	if len(request.Source) > MaxSourceBytes {
		return fmt.Errorf("%w: source exceeds %d bytes", ErrInvalidRequest, MaxSourceBytes)
	}
	if len(request.Constraints) == 0 {
		return fmt.Errorf("%w: at least one constraint is required", ErrInvalidRequest)
	}
	if len(request.Constraints) > MaxConstraints {
		return fmt.Errorf("%w: at most %d constraints are allowed", ErrInvalidRequest, MaxConstraints)
	}
	seen := make(map[string]struct{}, len(request.Constraints))
	for _, constraint := range request.Constraints {
		if strings.TrimSpace(constraint.ID) == "" {
			return fmt.Errorf("%w: constraint ID is required", ErrInvalidRequest)
		}
		if _, duplicate := seen[constraint.ID]; duplicate {
			return fmt.Errorf("%w: duplicate constraint ID %q", ErrInvalidRequest, constraint.ID)
		}
		seen[constraint.ID] = struct{}{}
		if strings.TrimSpace(constraint.Text) == "" {
			return fmt.Errorf("%w: constraint %q has no text", ErrInvalidRequest, constraint.ID)
		}
		if utf8.RuneCountInString(constraint.Text) > MaxConstraintTextRune {
			return fmt.Errorf("%w: constraint %q text exceeds %d characters", ErrInvalidRequest, constraint.ID, MaxConstraintTextRune)
		}
	}
	return nil
}

// validateModelOutput strictly decodes one model response and validates
// it against the requested constraint set and the supplied source lines.
// It returns results in request order for deterministic storage.
func validateModelOutput(content string, request Request, lines []string) ([]ConstraintResult, outputKind, string) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()

	var output wireOutput
	if err := decoder.Decode(&output); err != nil {
		return nil, outputSchemaInvalid, "malformed JSON, wrong field type or unknown field: " + err.Error()
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, outputSchemaInvalid, "trailing data after the result object"
	}

	if output.Results == nil {
		return nil, outputSchemaInvalid, "results is required and must be an array"
	}
	if len(output.Results) == 0 {
		return nil, outputSchemaInvalid, "results must not be empty"
	}

	type slot struct {
		result ConstraintResult
		filled bool
	}
	slots := make([]slot, len(request.Constraints))
	index := make(map[string]int, len(request.Constraints))
	for i, constraint := range request.Constraints {
		index[constraint.ID] = i
	}

	for _, wire := range output.Results {
		slotIndex, requested := index[wire.ConstraintID]
		if !requested {
			return nil, outputSchemaInvalid, fmt.Sprintf("unrequested constraint ID %q", wire.ConstraintID)
		}
		if slots[slotIndex].filled {
			return nil, outputSchemaInvalid, fmt.Sprintf("duplicate result for constraint %q", wire.ConstraintID)
		}
		switch wire.Verdict {
		case VerdictPass, VerdictViolation, VerdictUnsure:
		default:
			return nil, outputSchemaInvalid, fmt.Sprintf("invalid verdict for constraint %q", wire.ConstraintID)
		}
		if wire.Evidence == nil {
			return nil, outputSchemaInvalid, fmt.Sprintf("evidence must be an array for constraint %q", wire.ConstraintID)
		}
		evidence, kind, detail := validateEvidence(*wire.Evidence, lines)
		if kind != outputValid {
			return nil, kind, detail
		}
		if wire.Verdict == VerdictViolation && len(evidence) == 0 {
			return nil, outputEvidenceInvalid, fmt.Sprintf("violation for constraint %q has no evidence", wire.ConstraintID)
		}
		slots[slotIndex] = slot{
			result: ConstraintResult{
				ConstraintID: wire.ConstraintID,
				Verdict:      wire.Verdict,
				Evidence:     evidence,
			},
			filled: true,
		}
	}

	results := make([]ConstraintResult, len(slots))
	for i := range slots {
		if !slots[i].filled {
			return nil, outputSchemaInvalid, fmt.Sprintf("missing result for constraint %q", request.Constraints[i].ID)
		}
		results[i] = slots[i].result
	}
	return results, outputValid, ""
}

// validateEvidence requires every cited line to exist and every text to
// appear on that line. Trimmed text is compared against the source line;
// the original text is retained in the result.
func validateEvidence(evidence []Evidence, lines []string) ([]Evidence, outputKind, string) {
	validated := make([]Evidence, 0, len(evidence))
	for i, item := range evidence {
		if item.Line < 1 || item.Line > len(lines) {
			return nil, outputEvidenceInvalid, fmt.Sprintf("evidence %d references line %d, which does not exist", i, item.Line)
		}
		text := strings.TrimSpace(item.Text)
		if text == "" {
			return nil, outputEvidenceInvalid, fmt.Sprintf("evidence %d has no text", i)
		}
		if utf8.RuneCountInString(item.Text) > MaxEvidenceTextRune {
			return nil, outputEvidenceInvalid, fmt.Sprintf("evidence %d text exceeds %d characters", i, MaxEvidenceTextRune)
		}
		if !strings.Contains(lines[item.Line-1], text) {
			return nil, outputEvidenceInvalid, fmt.Sprintf("evidence %d text is not present on line %d", i, item.Line)
		}
		validated = append(validated, item)
	}
	return validated, outputValid, ""
}
