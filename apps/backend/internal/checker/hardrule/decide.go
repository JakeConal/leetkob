package hardrule

import (
	"strings"

	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
)

// Decide applies one constraint to a parsed analyzer result.
// source is read only to copy an evidence line verbatim. Banned names come
// from backtick pairs in the constraint, matched against resolved
// declarations. Submission text is not searched.
func Decide(constraintText, source string, result Result) Decision {
	if !isBanList(constraintText) {
		return Decision{}
	}
	names, namesOK := bannedNames(constraintText)
	if !namesOK || len(names) == 0 {
		return Decision{HardRule: true, Verdict: schema.VerdictUnsure, Reason: "ban-list"}
	}
	if len(source) > MaxSourceBytes {
		return Decision{HardRule: true, Verdict: schema.VerdictUnsure, Reason: "source-too-large"}
	}
	if result.Version != 1 || result.Status != "ok" || len(result.References) > MaxReferences {
		reason := result.Reason
		if reason == "" {
			reason = "analyzer-result"
		}
		return Decision{HardRule: true, Verdict: schema.VerdictUnsure, Reason: reason}
	}

	evidence := make([]schema.Evidence, 0)
	seen := map[int]struct{}{}
	unresolved := false
	for _, ref := range result.References {
		matched := false
		for _, name := range names {
			if referenceMatches(name, ref) {
				matched = true
				break
			}
		}
		if matched {
			text, ok := originalLine(source, ref.Line)
			if !ok {
				return Decision{HardRule: true, Verdict: schema.VerdictUnsure, Reason: "evidence-line"}
			}
			if _, exists := seen[ref.Line]; !exists {
				seen[ref.Line] = struct{}{}
				evidence = append(evidence, schema.Evidence{Line: ref.Line, Text: text})
			}
			continue
		}
		for _, name := range names {
			if unresolvedBlocks(name, ref) {
				unresolved = true
				break
			}
		}
	}
	if len(evidence) > 0 {
		sortEvidence(evidence)
		return Decision{HardRule: true, Verdict: schema.VerdictViolation, Evidence: evidence}
	}
	if unresolved {
		return Decision{HardRule: true, Verdict: schema.VerdictUnsure, Reason: "unresolved"}
	}
	return Decision{HardRule: true, Verdict: schema.VerdictPass, Evidence: []schema.Evidence{}}
}

// Evaluate parses a versioned analyzer document and decides the constraint.
// A document that does not parse is unsure for a ban list and is never a
// violation. A constraint that is not a ban list is left undecided.
func Evaluate(constraintText, source string, raw []byte) Decision {
	if !isBanList(constraintText) {
		return Decision{}
	}
	result, err := ParseResult(raw)
	if err != nil {
		return Decision{HardRule: true, Verdict: schema.VerdictUnsure, Reason: "analyzer-result"}
	}
	return Decide(constraintText, source, result)
}

func isBanList(text string) bool {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	return strings.HasPrefix(trimmed, "Do not use") || strings.HasPrefix(trimmed, "Do not call")
}

// bannedNames returns backtick contents. The boolean is false when a
// backtick is unclosed; that constraint cannot be decided by syntax.
func bannedNames(text string) ([]string, bool) {
	var names []string
	for i := 0; i < len(text); i++ {
		if text[i] != '`' {
			continue
		}
		end := strings.IndexByte(text[i+1:], '`')
		if end < 0 {
			return nil, false
		}
		name := strings.TrimSpace(text[i+1 : i+1+end])
		if name != "" {
			names = append(names, name)
		}
		i += end + 1
	}
	return names, true
}

func referenceMatches(ban string, ref Ref) bool {
	if !ref.Resolved || ref.Line < 1 {
		return false
	}
	if isHeader(ban) {
		if ref.Kind == "include" || ref.Header != headerToken(ban) {
			return false
		}
		base := stripTemplate(normalizeQualified(ref.QualifiedName))
		// Including this header only to call std::swap is not a use of the
		// banned header's other facilities.
		return base != "" && base != "std::swap"
	}
	base := stripTemplate(normalizeQualified(ref.QualifiedName))
	if base == "" || ref.UserDeclared {
		return false
	}
	if strings.Contains(ban, "::") {
		return base == stripTemplate(normalizeQualified(ban))
	}
	return lastComponent(base) == ban
}

func unresolvedBlocks(ban string, ref Ref) bool {
	if ref.Resolved || ref.Name == "" || isHeader(ban) {
		return false
	}
	want := ban
	if strings.Contains(ban, "::") {
		want = lastComponent(stripTemplate(normalizeQualified(ban)))
	}
	return ref.Name == want
}

func isHeader(ban string) bool {
	return strings.HasPrefix(ban, "<") && strings.HasSuffix(ban, ">") && len(ban) > 2
}

func headerToken(ban string) string {
	return ban[1 : len(ban)-1]
}

// normalizeQualified drops libc++ and libstdc++ inline namespaces so a
// resolved std::__1::sort declaration matches the banned name std::sort.
func normalizeQualified(name string) string {
	if name == "" {
		return ""
	}
	parts := strings.Split(name, "::")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if len(kept) > 0 && kept[len(kept)-1] == "std" && (part == "__1" || part == "__2" || part == "__cxx11") {
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, "::")
}

func stripTemplate(name string) string {
	if strings.Contains(name, "operator") {
		return name
	}
	if index := strings.IndexByte(name, '<'); index >= 0 {
		return name[:index]
	}
	return name
}

func lastComponent(name string) string {
	if index := strings.LastIndex(name, "::"); index >= 0 {
		return name[index+2:]
	}
	return name
}

func originalLine(source string, line int) (string, bool) {
	parts := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if line < 1 || line > len(parts) {
		return "", false
	}
	text := strings.TrimRight(parts[line-1], "\r")
	if text == "" || len(text) > 4096 {
		return "", false
	}
	return text, true
}

func sortEvidence(items []schema.Evidence) {
	for i := 1; i < len(items); i++ {
		item := items[i]
		j := i
		for j > 0 && items[j-1].Line > item.Line {
			items[j] = items[j-1]
			j--
		}
		items[j] = item
	}
}
