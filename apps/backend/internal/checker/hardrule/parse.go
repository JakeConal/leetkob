package hardrule

import "errors"

// ErrNotImplemented marks the tests-first stub. The analyzer consumer
// replaces it; callers must not treat this as a violation.
var ErrNotImplemented = errors.New("hardrule: analyzer consumer is not implemented")

// ParseResult decodes an analyzer-result.v1 document. Unknown fields,
// the wrong version, and over-bound reference lists are errors.
func ParseResult(raw []byte) (Result, error) {
	return Result{}, ErrNotImplemented
}
