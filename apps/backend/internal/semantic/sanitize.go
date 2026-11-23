package semantic

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const redactionPlaceholder = "[removed]"

const (
	kindString = iota
	kindChar
	kindRaw
)

// literalOpeners are longest-prefix-first so u8R" wins over u8" and ".
var literalOpeners = []struct {
	prefix string
	kind   int
}{
	{"u8R\"", kindRaw},
	{"uR\"", kindRaw},
	{"UR\"", kindRaw},
	{"LR\"", kindRaw},
	{"u8\"", kindString},
	{"R\"", kindRaw},
	{"u\"", kindString},
	{"U\"", kindString},
	{"L\"", kindString},
	{"u'", kindChar},
	{"U'", kindChar},
	{"L'", kindChar},
	{"\"", kindString},
	{"'", kindChar},
}

// Sanitize strips C++ comments with a lexer, redacts known identities from
// every outbound field, and builds the JSON data payload. The system
// prompt stays the fixed instruction text. An error means the transform
// was ambiguous; the caller must not transmit any of this input.
func Sanitize(in SanitizeInput) (Sanitized, error) {
	terms, err := prepareTerms(in.RedactionTerms)
	if err != nil {
		return Sanitized{}, err
	}
	for _, constraint := range in.Constraints {
		for _, term := range terms {
			if strings.Contains(constraint.ID, term) || strings.Contains(SystemPrompt, term) || strings.Contains(RepairNote, term) {
				return Sanitized{}, ErrUnsafeTransform
			}
		}
	}
	stripped, err := stripComments(in.Source)
	if err != nil {
		return Sanitized{}, err
	}
	if strings.Count(stripped, "\n") != strings.Count(in.Source, "\n") {
		return Sanitized{}, ErrUnsafeTransform
	}
	statement, err := redact(in.Statement, terms)
	if err != nil {
		return Sanitized{}, err
	}
	constraints := make([]Constraint, len(in.Constraints))
	copy(constraints, in.Constraints)
	for i := range constraints {
		constraints[i].Text, err = redact(constraints[i].Text, terms)
		if err != nil {
			return Sanitized{}, err
		}
	}
	redactedSource, err := redact(stripped, terms)
	if err != nil {
		return Sanitized{}, err
	}
	original := splitLines(in.Source)
	sanitized := splitLines(redactedSource)
	if len(original) != len(sanitized) {
		return Sanitized{}, ErrUnsafeTransform
	}
	payload := outboundPayload{
		Statement:   statement,
		Constraints: make([]outboundConstraint, len(constraints)),
		Source:      numberLines(sanitized),
	}
	for i, constraint := range constraints {
		payload.Constraints[i] = outboundConstraint{ID: constraint.ID, Text: constraint.Text}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Sanitized{}, ErrUnsafeTransform
	}
	userJSON := string(raw)
	for _, term := range terms {
		if strings.Contains(statement, term) || strings.Contains(userJSON, term) || strings.Contains(SystemPrompt, term) {
			return Sanitized{}, ErrUnsafeTransform
		}
		for _, constraint := range constraints {
			if strings.Contains(constraint.Text, term) {
				return Sanitized{}, ErrUnsafeTransform
			}
		}
	}
	return Sanitized{
		Statement:      statement,
		Constraints:    constraints,
		OriginalLines:  original,
		SanitizedLines: sanitized,
		System:         SystemPrompt,
		UserJSON:       userJSON,
	}, nil
}

type outboundPayload struct {
	Statement   string               `json:"statement"`
	Constraints []outboundConstraint `json:"constraints"`
	Source      string               `json:"source"`
}

type outboundConstraint struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func prepareTerms(terms []string) ([]string, error) {
	seen := make(map[string]struct{}, len(terms))
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		if term == "" || strings.ContainsAny(term, "\r\n") {
			return nil, ErrUnsafeTransform
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out, nil
}

func redact(s string, terms []string) (string, error) {
	for _, term := range terms {
		s = strings.ReplaceAll(s, term, redactionPlaceholder)
	}
	for _, term := range terms {
		if strings.Contains(s, term) {
			return "", ErrUnsafeTransform
		}
	}
	return s, nil
}

func numberLines(lines []string) string {
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%d|%s\n", i+1, line)
	}
	return b.String()
}

func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	s = strings.TrimSuffix(s, "\n")
	parts := strings.Split(s, "\n")
	for i, part := range parts {
		parts[i] = strings.TrimSuffix(part, "\r")
	}
	return parts
}

func stripComments(src string) (string, error) {
	for i := 0; i+1 < len(src); i++ {
		// Phase-2 line splicing would change which physical line a token
		// occupies, so the original-line mapping would be ambiguous.
		if src[i] == '\\' && (src[i+1] == '\n' || src[i+1] == '\r') {
			return "", ErrUnsafeTransform
		}
	}
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); {
		if width, kind, ok := matchOpener(src, i); ok {
			n, err := consumeLiteral(src, i, width, kind, &b)
			if err != nil {
				return "", err
			}
			i = n
			continue
		}
		if src[i] == '/' && i+1 < len(src) && src[i+1] == '/' {
			i += 2
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if src[i] == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			closed := false
			for i < len(src) {
				if src[i] == '\n' {
					b.WriteByte('\n')
					i++
					continue
				}
				if src[i] == '*' && i+1 < len(src) && src[i+1] == '/' {
					i += 2
					closed = true
					break
				}
				i++
			}
			if !closed {
				return "", ErrUnsafeTransform
			}
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String(), nil
}

func matchOpener(src string, i int) (int, int, bool) {
	if i > 0 && isIdentByte(src[i-1]) && src[i] != '"' && src[i] != '\'' {
		return 0, 0, false
	}
	for _, opener := range literalOpeners {
		if strings.HasPrefix(src[i:], opener.prefix) {
			return len(opener.prefix), opener.kind, true
		}
	}
	return 0, 0, false
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func consumeLiteral(src string, i, width, kind int, b *strings.Builder) (int, error) {
	if kind == kindRaw {
		return consumeRaw(src, i, width, b)
	}
	quote := byte('"')
	if kind == kindChar {
		quote = '\''
	}
	return consumeQuoted(src, i, width, quote, b)
}

func consumeQuoted(src string, i, width int, quote byte, b *strings.Builder) (int, error) {
	j := i + width
	b.WriteString(src[i:j])
	for j < len(src) {
		if src[j] == '\\' {
			if j+1 >= len(src) {
				return 0, ErrUnsafeTransform
			}
			b.WriteByte('\\')
			b.WriteByte(src[j+1])
			j += 2
			continue
		}
		if src[j] == '\n' {
			return 0, ErrUnsafeTransform
		}
		b.WriteByte(src[j])
		j++
		if src[j-1] == quote {
			return j, nil
		}
	}
	return 0, ErrUnsafeTransform
}

func consumeRaw(src string, i, width int, b *strings.Builder) (int, error) {
	j := i + width
	delimStart := j
	for j < len(src) && src[j] != '(' {
		switch src[j] {
		case ' ', ')', '\\', '\t', '\v', '\f', '\r', '\n':
			return 0, ErrUnsafeTransform
		}
		j++
		if j-delimStart > 16 {
			return 0, ErrUnsafeTransform
		}
	}
	if j >= len(src) {
		return 0, ErrUnsafeTransform
	}
	delim := src[delimStart:j]
	j++ // skip '('
	closer := ")" + delim + "\""
	rel := strings.Index(src[j:], closer)
	if rel < 0 {
		return 0, ErrUnsafeTransform
	}
	end := j + rel + len(closer)
	b.WriteString(src[i:end])
	return end, nil
}
