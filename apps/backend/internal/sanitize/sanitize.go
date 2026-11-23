// Package sanitize prepares untrusted C++ source and free-text fields for
// bounded semantic model calls. Comment removal is lexer-aware: markers
// inside string, character and raw-string literals are preserved, while
// every real comment is replaced by spaces with newlines kept so the
// original line mapping survives. Known account names, emails and IDs are
// replaced across outbound text. Ambiguous input (an unterminated block
// comment, string or raw literal) is reported as unsafe so the caller
// becomes unsure and transmits nothing.
//
// The package is a pure function over in-memory strings: it never reads
// files, environment variables, credentials or the network.
package sanitize

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
)

// UnsafeReason enumerates why sanitization refused the input.
type UnsafeReason string

// Refusal reasons. Callers map every one to the sanitization-unsafe
// unsure cause.
const (
	ReasonUnterminatedBlockComment UnsafeReason = "unterminated block comment"
	ReasonUnterminatedString       UnsafeReason = "unterminated string literal"
	ReasonUnterminatedChar         UnsafeReason = "unterminated character literal"
	ReasonUnterminatedRawString    UnsafeReason = "unterminated raw string literal"
	ReasonInvalidUTF8              UnsafeReason = "invalid UTF-8"
)

// ErrInvalidUTF8 reports non-UTF-8 input before any transformation.
var ErrInvalidUTF8 = errors.New("sanitize: input is not valid UTF-8")

// RedactedIdentity replaces a known account string in outbound text.
const RedactedIdentity = "account_redacted"

// Result is the sanitized text plus what the transformation did.
type Result struct {
	Source   string
	Unsafe   bool
	Reason   UnsafeReason
	Removed  int // bytes replaced by spaces from comments
	Redacted int // occurrences of known identities replaced
}

// Text redacts known identities from a single free-text field (statement
// or constraint text). It never removes content: those fields are not
// code and an unmatched quote there must not refuse the check.
func Text(value string, identities []string) (Result, error) {
	if !utf8.ValidString(value) {
		return Result{}, ErrInvalidUTF8
	}
	sanitized, redacted := redact(value, identities)
	return Result{Source: sanitized, Redacted: redacted}, nil
}

// Source strips comments from C++17 source lexer-aware and redacts known
// identities. A successful result has exactly the original number of
// lines, so checker evidence lines remain valid. Unsafe results carry the
// reason and no partially transformed text.
func Source(code string, identities []string) (Result, error) {
	if !utf8.ValidString(code) {
		return Result{}, ErrInvalidUTF8
	}
	withoutComments, removed, reason := stripComments(code)
	if reason != "" {
		return Result{Unsafe: true, Reason: reason}, nil
	}
	sanitized, redacted := redact(withoutComments, identities)
	return Result{Source: sanitized, Removed: removed, Redacted: redacted}, nil
}

// redact replaces every occurrence of each identity, longest first, with
// a fixed placeholder that contains no newline. Identities shorter than
// four bytes are ignored: too-short strings would mangle ordinary code.
func redact(value string, identities []string) (string, int) {
	candidates := make([]string, 0, len(identities))
	for _, identity := range identities {
		if len(identity) >= 4 {
			candidates = append(candidates, identity)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return len(candidates[i]) > len(candidates[j]) })
	redacted := 0
	for _, identity := range candidates {
		count := strings.Count(value, identity)
		if count == 0 {
			continue
		}
		value = strings.ReplaceAll(value, identity, RedactedIdentity)
		redacted += count
	}
	return value, redacted
}

// lexState walks C++ source. Raw strings carry their delimiter so the
// closing sequence is exact.
type lexState int

const (
	stateNormal lexState = iota
	stateBlockComment
	stateString
	stateChar
	stateRawString
	stateLineComment
)

// stripComments replaces comments with spaces, preserving line breaks.
func stripComments(code string) (string, int, UnsafeReason) {
	out := []byte(code)
	removed := 0
	state := stateNormal
	rawDelimiter := ""
	escaped := false

	commentByte := func(index int) {
		if out[index] != '\n' && out[index] != '\r' {
			out[index] = ' '
			removed++
		}
	}

	for i := 0; i < len(code); i++ {
		c := code[i]
		switch state {
		case stateNormal:
			if c == '/' && i+1 < len(code) && code[i+1] == '/' {
				commentByte(i)
				commentByte(i + 1)
				i++
				state = stateLineComment
				continue
			}
			if c == '/' && i+1 < len(code) && code[i+1] == '*' {
				commentByte(i)
				commentByte(i + 1)
				i++
				state = stateBlockComment
				continue
			}
			if c == '"' {
				state = stateString
				escaped = false
				continue
			}
			if c == '\'' {
				state = stateChar
				escaped = false
				continue
			}
			if c == 'R' && i+1 < len(code) && code[i+1] == '"' {
				delimiter, end, ok := parseRawDelimiter(code, i+2)
				if !ok {
					return "", 0, ReasonUnterminatedRawString
				}
				rawDelimiter = delimiter
				state = stateRawString
				i = end
				continue
			}
		case stateLineComment:
			if c == '\n' {
				state = stateNormal
				continue
			}
			commentByte(i)
		case stateBlockComment:
			if c == '*' && i+1 < len(code) && code[i+1] == '/' {
				commentByte(i)
				commentByte(i + 1)
				i++
				state = stateNormal
				continue
			}
			commentByte(i)
		case stateString:
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' || c == '\n' {
				state = stateNormal
				if c == '\n' {
					return "", 0, ReasonUnterminatedString
				}
			}
		case stateChar:
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '\'' {
				state = stateNormal
			}
			if c == '\n' {
				return "", 0, ReasonUnterminatedChar
			}
		case stateRawString:
			closing := ")" + rawDelimiter + "\""
			if c == ')' && strings.HasPrefix(code[i:], closing) {
				state = stateNormal
				i += len(closing) - 1
			}
		}
	}

	switch state {
	case stateBlockComment:
		return "", 0, ReasonUnterminatedBlockComment
	case stateString:
		return "", 0, ReasonUnterminatedString
	case stateChar:
		return "", 0, ReasonUnterminatedChar
	case stateRawString:
		return "", 0, ReasonUnterminatedRawString
	}
	return string(out), removed, ""
}

// parseRawDelimiter reads a raw-string delimiter starting just after R".
// It returns the delimiter and the index of the opening parenthesis.
func parseRawDelimiter(code string, start int) (string, int, bool) {
	for i := start; i < len(code) && i-start <= 16; i++ {
		switch code[i] {
		case '(':
			return code[start:i], i, true
		case ')', '\\', ' ', '\n':
			return "", 0, false
		}
	}
	return "", 0, false
}
