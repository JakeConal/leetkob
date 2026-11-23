package hardrule

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const maxDocumentBytes = 1 << 20

// ParseResult decodes an analyzer-result.v1 document. Unknown fields, the
// wrong version, and over-bound reference lists are errors. Callers turn
// that error into unsure; they do not invent a violation.
func ParseResult(raw []byte) (Result, error) {
	if len(raw) == 0 || len(raw) > maxDocumentBytes {
		return Result{}, fmt.Errorf("hardrule: analyzer document size is out of bounds")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var result Result
	if err := dec.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("hardrule: analyzer document: %w", err)
	}
	if dec.More() {
		return Result{}, fmt.Errorf("hardrule: analyzer document has trailing data")
	}
	if result.Version != 1 {
		return Result{}, fmt.Errorf("hardrule: unsupported analyzer version %d", result.Version)
	}
	if result.Status != "ok" && result.Status != "unsure" {
		return Result{}, fmt.Errorf("hardrule: unsupported analyzer status %q", result.Status)
	}
	if len(result.Reason) > 64 {
		return Result{}, fmt.Errorf("hardrule: analyzer reason is too long")
	}
	if len(result.References) > MaxReferences {
		return Result{}, fmt.Errorf("hardrule: analyzer reference bound exceeded")
	}
	for _, ref := range result.References {
		switch ref.Kind {
		case "call", "using", "include", "reference":
		default:
			return Result{}, fmt.Errorf("hardrule: unknown reference kind %q", ref.Kind)
		}
		if ref.Line < 1 || ref.Column < 1 {
			return Result{}, fmt.Errorf("hardrule: reference location is not in the submitted file")
		}
		if len(ref.Name) > 256 || len(ref.QualifiedName) > 512 || len(ref.Header) > 256 {
			return Result{}, fmt.Errorf("hardrule: reference field is too long")
		}
	}
	if result.References == nil {
		result.References = []Ref{}
	}
	return result, nil
}
