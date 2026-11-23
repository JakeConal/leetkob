// Package analyzer is the Go boundary around the isolated libclang C++
// analyzer image. The Go backend never links libclang and never uses
// CGO: it writes one bounded JSON request to a disposable container and
// validates the bounded JSON reply before any hard-rule verdict is used.
//
// A failed, timed-out or malformed analysis is a tooling error. Callers
// must map it to unsure, never to a violation. Fallback provides that
// behaviour in-process when Docker is unavailable.
package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ProtocolVersion is the only analyzer wire version this package accepts.
const ProtocolVersion = 1

// Bounds from the PA#3 contract and this package's isolation budget.
const (
	// MaxSourceBytes matches the lab's 65,536-byte source limit.
	MaxSourceBytes = 65536
	// MaxBannedNames bounds one request. A lab has at most 20 constraints,
	// each contributing a small number of backticked names.
	MaxBannedNames = 64
	// MaxNameBytes bounds one banned name.
	MaxNameBytes = 256
	// MaxInputBytes bounds the encoded request, including JSON escaping.
	MaxInputBytes = 256 << 10
	// MaxOutputBytes bounds the analyzer reply. The source itself is only
	// 64 KiB, so a larger reply is invalid rather than useful.
	MaxOutputBytes = 2 << 20
	// MaxEvidencePerResult bounds cited original lines per name.
	MaxEvidencePerResult = 8
	// MaxEvidenceTextBytes matches checker-prediction.v1.json.
	MaxEvidenceTextBytes = 4096
	// DefaultTimeout bounds one container invocation.
	DefaultTimeout = 20 * time.Second
)

// Analyzer decides banned-name pairs from C++ source. Implementations
// return an error rather than a guessed verdict when analysis cannot
// complete; callers must map errors to unsure.
type Analyzer interface {
	Analyze(ctx context.Context, req Request) (Report, error)
}

// Request is the analyzer input. BannedNames are the backticked names
// extracted from one or more ban-list constraints.
type Request struct {
	Source      string
	BannedNames []string
}

// Report is the analyzer output, one result per unique banned name in the
// order requested.
type Report struct {
	Version int      `json:"version"`
	Results []Result `json:"results"`
	// Cause is set only by in-process fallbacks and diagnostics. It is
	// not part of the wire protocol and is never a verdict.
	Cause string `json:"-"`
}

// Result is the verdict for one banned name.
type Result struct {
	Name     string     `json:"name"`
	Verdict  Verdict    `json:"verdict"`
	Evidence []Evidence `json:"evidence"`
}

// Verdict is the analyzer result for one banned name.
type Verdict string

const (
	// VerdictViolation means the AST contains a resolved reference to the
	// banned declaration and original-line evidence was supplied.
	VerdictViolation Verdict = "violation"
	// VerdictPass means analysis completed and no matching reference was
	// found.
	VerdictPass Verdict = "pass"
	// VerdictUnsure means the analyzer could not resolve the name
	// confidently. It is never a violation.
	VerdictUnsure Verdict = "unsure"
)

// Valid reports whether v is in the analyzer wire enum.
func (v Verdict) Valid() bool {
	switch v {
	case VerdictViolation, VerdictPass, VerdictUnsure:
		return true
	default:
		return false
	}
}

// Evidence is an original source line cited for a violation.
type Evidence struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Errors returned by this package. All are tooling conditions that a
// hard-rule caller must turn into unsure, never into a deduction.
var (
	ErrInvalidRequest    = errors.New("analyzer: invalid request")
	ErrInputTooLarge     = errors.New("analyzer: encoded request exceeds input bound")
	ErrOutputTooLarge    = errors.New("analyzer: reply exceeds output bound")
	ErrInvalidReport     = errors.New("analyzer: invalid reply")
	ErrContainerFailed   = errors.New("analyzer: container invocation failed")
	ErrTimeout           = errors.New("analyzer: analysis timed out")
	ErrDockerUnavailable = errors.New("analyzer: docker is unavailable")
)

// Fallback is the pure-Go analyzer used when Docker cannot run. It
// decides every name as unsure with a visible cause and never invents a
// violation. It is the MODEL_OFF-safe behaviour for hard-rule tooling
// failures.
type Fallback struct {
	// Reason explains why the fallback was selected, for the caller's
	// finding record. It is never sent to a student.
	Reason string
}

// Analyze implements Analyzer.
func (f Fallback) Analyze(_ context.Context, req Request) (Report, error) {
	names, err := validateRequest(req)
	if err != nil {
		return Report{}, err
	}
	cause := f.Reason
	if cause == "" {
		cause = "fallback analyzer selected; no libclang result"
	}
	return UnsureReport(names, cause), nil
}

// UnsureReport builds a bounded all-unsure report in requested order.
// Duplicate names are collapsed because the wire protocol returns one
// result per name.
func UnsureReport(names []string, cause string) Report {
	unique := uniqueNames(names)
	results := make([]Result, 0, len(unique))
	for _, name := range unique {
		results = append(results, Result{Name: name, Verdict: VerdictUnsure, Evidence: []Evidence{}})
	}
	return Report{Version: ProtocolVersion, Results: results, Cause: cause}
}

// ValidateRequest checks bounds before any container starts.
func ValidateRequest(req Request) error {
	_, err := validateRequest(req)
	return err
}

func validateRequest(req Request) ([]string, error) {
	if !utf8.ValidString(req.Source) {
		return nil, fmt.Errorf("%w: source is not valid UTF-8", ErrInvalidRequest)
	}
	if len(req.Source) > MaxSourceBytes {
		return nil, fmt.Errorf("%w: source is %d bytes, maximum is %d", ErrInvalidRequest, len(req.Source), MaxSourceBytes)
	}
	if len(req.BannedNames) > MaxBannedNames {
		return nil, fmt.Errorf("%w: %d banned names, maximum is %d", ErrInvalidRequest, len(req.BannedNames), MaxBannedNames)
	}
	names := make([]string, 0, len(req.BannedNames))
	for _, raw := range req.BannedNames {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, fmt.Errorf("%w: banned name is empty", ErrInvalidRequest)
		}
		if len(name) > MaxNameBytes {
			return nil, fmt.Errorf("%w: banned name %q exceeds %d bytes", ErrInvalidRequest, name, MaxNameBytes)
		}
		if !utf8.ValidString(name) {
			return nil, fmt.Errorf("%w: banned name is not valid UTF-8", ErrInvalidRequest)
		}
		for _, r := range name {
			if unicode.IsControl(r) {
				return nil, fmt.Errorf("%w: banned name contains a control character", ErrInvalidRequest)
			}
		}
		names = append(names, name)
	}
	return uniqueNames(names), nil
}

func uniqueNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	unique := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		unique = append(unique, name)
	}
	return unique
}

type wireRequest struct {
	Source      string   `json:"source"`
	BannedNames []string `json:"bannedNames"`
}

func encodeRequest(req Request, names []string) ([]byte, error) {
	wire := wireRequest{Source: req.Source, BannedNames: names}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(wire); err != nil {
		return nil, fmt.Errorf("%w: encode: %v", ErrInvalidRequest, err)
	}
	// Encoder.Encode appends a newline; it is harmless for the C++
	// parser but remove it so the byte bound describes the request.
	input := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	if len(input) > MaxInputBytes {
		return nil, fmt.Errorf("%w: %d bytes, maximum is %d", ErrInputTooLarge, len(input), MaxInputBytes)
	}
	return input, nil
}

type wireReport struct {
	Version int          `json:"version"`
	Results []wireResult `json:"results"`
}

type wireResult struct {
	Name     string     `json:"name"`
	Verdict  Verdict    `json:"verdict"`
	Evidence []Evidence `json:"evidence"`
}

func decodeReport(raw []byte, source string, names []string) (Report, error) {
	if len(raw) > MaxOutputBytes {
		return Report{}, fmt.Errorf("%w: %d bytes, maximum is %d", ErrOutputTooLarge, len(raw), MaxOutputBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire wireReport
	if err := decoder.Decode(&wire); err != nil {
		return Report{}, fmt.Errorf("%w: decode: %v", ErrInvalidReport, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Report{}, err
	}
	if wire.Version != ProtocolVersion {
		return Report{}, fmt.Errorf("%w: version %d, want %d", ErrInvalidReport, wire.Version, ProtocolVersion)
	}
	if len(wire.Results) != len(names) {
		return Report{}, fmt.Errorf("%w: %d results for %d requested names", ErrInvalidReport, len(wire.Results), len(names))
	}
	lines := sourceLines(source)
	byName := make(map[string]wireResult, len(wire.Results))
	for _, result := range wire.Results {
		if _, duplicate := byName[result.Name]; duplicate {
			return Report{}, fmt.Errorf("%w: duplicate result for %q", ErrInvalidReport, result.Name)
		}
		byName[result.Name] = result
	}
	results := make([]Result, 0, len(names))
	for _, name := range names {
		result, ok := byName[name]
		if !ok {
			return Report{}, fmt.Errorf("%w: missing result for %q", ErrInvalidReport, name)
		}
		if !result.Verdict.Valid() {
			return Report{}, fmt.Errorf("%w: result %q has verdict %q", ErrInvalidReport, name, result.Verdict)
		}
		evidence := result.Evidence
		if evidence == nil {
			evidence = []Evidence{}
		}
		if len(evidence) > MaxEvidencePerResult {
			return Report{}, fmt.Errorf("%w: result %q cites %d lines, maximum is %d", ErrInvalidReport, name, len(evidence), MaxEvidencePerResult)
		}
		if result.Verdict == VerdictViolation && len(evidence) == 0 {
			return Report{}, fmt.Errorf("%w: violation %q has no evidence", ErrInvalidReport, name)
		}
		for i, item := range evidence {
			if err := validateEvidence(item, lines, name); err != nil {
				return Report{}, fmt.Errorf("%w: result %q evidence[%d]: %v", ErrInvalidReport, name, i, err)
			}
		}
		results = append(results, Result{Name: name, Verdict: result.Verdict, Evidence: evidence})
	}
	return Report{Version: ProtocolVersion, Results: results}, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("%w: trailing JSON value", ErrInvalidReport)
	}
	return fmt.Errorf("%w: trailing data: %v", ErrInvalidReport, err)
}

func validateEvidence(e Evidence, lines []string, name string) error {
	if e.Line < 1 || e.Line > len(lines) {
		return fmt.Errorf("line %d is outside the %d-line source", e.Line, len(lines))
	}
	if e.Text == "" {
		return errors.New("text is empty")
	}
	if len(e.Text) > MaxEvidenceTextBytes {
		return fmt.Errorf("text is %d bytes, maximum is %d", len(e.Text), MaxEvidenceTextBytes)
	}
	if !utf8.ValidString(e.Text) {
		return errors.New("text is not valid UTF-8")
	}
	original := lines[e.Line-1]
	if e.Text == original {
		return nil
	}
	// The C++ analyzer truncates a very long original line at a UTF-8
	// boundary. Accept exactly that truncation, never an arbitrary prefix.
	if len(original) > MaxEvidenceTextBytes && e.Text == truncateUTF8(original, MaxEvidenceTextBytes) {
		return nil
	}
	return fmt.Errorf("text does not match original line %d for %q", e.Line, name)
}

func sourceLines(source string) []string {
	parts := strings.Split(source, "\n")
	for i, line := range parts {
		parts[i] = strings.TrimSuffix(line, "\r")
	}
	return parts
}

func truncateUTF8(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}
