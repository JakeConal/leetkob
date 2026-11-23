package hardrule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
)

func TestCompliantTrapsAreNotViolations(t *testing.T) {
	own := readFixture(t, "own_namespace.cpp")
	ownLine := lineContaining(t, own, "marker-own-sort")
	comment := readFixture(t, "comment_and_string.cpp")
	swap := readFixture(t, "swap_only.cpp")
	swapLine := lineContaining(t, swap, "marker-swap")

	cases := []struct {
		name       string
		constraint string
		source     string
		result     Result
	}{
		{
			name:       "student namespace sort",
			constraint: "Do not call `std::sort`.",
			source:     own,
			result: okResult(Ref{
				Kind: "call", Name: "sort", QualifiedName: "student::sort",
				Resolved: true, UserDeclared: true, Line: ownLine, Column: 3,
			}),
		},
		{
			name:       "unqualified ban ignores student sort",
			constraint: "Do not use `sort`.",
			source:     own,
			result: okResult(Ref{
				Kind: "call", Name: "sort", QualifiedName: "student::sort",
				Resolved: true, UserDeclared: true, Line: ownLine, Column: 3,
			}),
		},
		{
			name:       "banned name only in a comment or string",
			constraint: "Do not call `std::sort`.",
			source:     comment,
			result:     okResult(),
		},
		{
			name:       "algorithm included only for swap",
			constraint: "Do not call `std::sort`.",
			source:     swap,
			result: okResult(
				Ref{Kind: "include", Name: "algorithm", Header: "algorithm", Resolved: true, Line: 1, Column: 1},
				Ref{Kind: "call", Name: "swap", QualifiedName: "std::swap", Header: "algorithm", Resolved: true, Line: swapLine, Column: 3},
			),
		},
		{
			name:       "header ban allows swap-only include",
			constraint: "Do not use `<algorithm>`.",
			source:     swap,
			result: okResult(
				Ref{Kind: "include", Name: "algorithm", Header: "algorithm", Resolved: true, Line: 1, Column: 1},
				Ref{Kind: "call", Name: "swap", QualifiedName: "std::__1::swap", Header: "algorithm", Resolved: true, Line: swapLine, Column: 3},
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.constraint, tc.source, tc.result)
			if !got.HardRule {
				t.Fatal("ban list must be decided by the hard rule")
			}
			if got.Verdict != schema.VerdictPass {
				t.Fatalf("verdict = %s (%s), want pass", got.Verdict, got.Reason)
			}
			if len(got.Evidence) != 0 {
				t.Fatalf("pass evidence = %#v, want none", got.Evidence)
			}
		})
	}
}

func TestIndirectionIsViolationAtOriginalLine(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		constraint string
		marker     string
		ref        Ref
		absent     string
	}{
		{
			name:       "namespace alias",
			file:       "namespace_alias.cpp",
			constraint: "Do not call `std::sort`.",
			marker:     "marker-alias",
			ref:        Ref{Kind: "call", Name: "sort", QualifiedName: "std::__1::sort", Header: "algorithm", Resolved: true, Column: 3},
		},
		{
			name:       "using declaration",
			file:       "using_decl.cpp",
			constraint: "Do not use `std::sort`.",
			marker:     "marker-using",
			ref:        Ref{Kind: "call", Name: "sort", QualifiedName: "std::sort", Header: "algorithm", Resolved: true, Column: 3},
		},
		{
			name:       "macro",
			file:       "macro_call.cpp",
			constraint: "Do not call `std::sort`.",
			marker:     "marker-macro",
			ref:        Ref{Kind: "call", Name: "sort", QualifiedName: "std::sort", Header: "algorithm", Resolved: true, Column: 3},
		},
		{
			name:       "wrapper",
			file:       "wrapper.cpp",
			constraint: "Do not call `std::sort`.",
			marker:     "marker-wrapper",
			absent:     "marker-wrapper-call",
			ref:        Ref{Kind: "reference", Name: "sort", QualifiedName: "std::sort", Header: "algorithm", Resolved: true, Column: 3},
		},
		{
			name:       "stable_sort through using",
			file:       "stable_using.cpp",
			constraint: "Do not call `std::sort` or `std::stable_sort`.",
			marker:     "marker-stable",
			ref:        Ref{Kind: "using", Name: "stable_sort", QualifiedName: "std::stable_sort", Header: "algorithm", Resolved: true, Column: 1},
		},
		{
			name:       "c library qsort",
			file:       "qsort_call.cpp",
			constraint: "Do not call `qsort`.",
			marker:     "marker-qsort",
			ref:        Ref{Kind: "call", Name: "qsort", QualifiedName: "qsort", Header: "cstdlib", Resolved: true, Column: 3},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := readFixture(t, tc.file)
			line := lineContaining(t, source, tc.marker)
			tc.ref.Line = line
			got := Decide(tc.constraint, source, okResult(tc.ref))
			assertViolationAt(t, source, got, line)
			text := evidenceText(t, got, line)
			if tc.name == "namespace alias" && strings.Contains(text, "std::sort") {
				t.Fatalf("alias evidence %q matched the banned spelling instead of the original call", text)
			}
			if tc.name == "macro" && strings.Contains(text, "std::sort") {
				t.Fatalf("macro evidence %q matched the banned spelling instead of the invocation line", text)
			}
			if tc.absent != "" {
				other := lineContaining(t, source, tc.absent)
				for _, item := range got.Evidence {
					if item.Line == other {
						t.Fatalf("evidence cited line %d, which is not the banned call", other)
					}
				}
			}
			prediction := schema.Prediction{
				Solution:   "fixture/s01",
				Constraint: "c-ban",
				Verdict:    got.Verdict,
				Evidence:   got.Evidence,
			}
			if err := schema.ValidatePrediction(prediction); err != nil {
				t.Fatalf("prediction schema: %v", err)
			}
		})
	}
}

func TestDirectCallAndHeaderUseAreViolations(t *testing.T) {
	source := readFixture(t, "direct_call.cpp")
	line := lineContaining(t, source, "marker-direct")
	result := okResult(Ref{
		Kind: "call", Name: "sort", QualifiedName: "std::sort", Header: "algorithm",
		Resolved: true, Line: line, Column: 3,
	})
	for _, constraint := range []string{"Do not call `std::sort`.", "Do not use `<algorithm>`."} {
		got := Decide(constraint, source, result)
		assertViolationAt(t, source, got, line)
	}
}

func TestSwapIsViolationWhenSwapItselfIsBanned(t *testing.T) {
	source := readFixture(t, "swap_only.cpp")
	line := lineContaining(t, source, "marker-swap")
	got := Decide("Do not call `std::swap`.", source, okResult(Ref{
		Kind: "call", Name: "swap", QualifiedName: "std::swap", Header: "algorithm",
		Resolved: true, Line: line, Column: 3,
	}))
	assertViolationAt(t, source, got, line)
}

func TestUnresolvedNameIsUnsure(t *testing.T) {
	source := "void f() {\n  sort(values);\n}\n"
	got := Decide("Do not call `std::sort`.", source, okResult(Ref{
		Kind: "call", Name: "sort", Resolved: false, Line: 2, Column: 3,
	}))
	if !got.HardRule || got.Verdict != schema.VerdictUnsure {
		t.Fatalf("decision = %+v, want hard-rule unsure", got)
	}
	if len(got.Evidence) != 0 {
		t.Fatalf("unsure invented evidence %#v", got.Evidence)
	}
}

func TestKnownViolationStandsBesideUnresolvedName(t *testing.T) {
	source := readFixture(t, "direct_call.cpp")
	line := lineContaining(t, source, "marker-direct")
	got := Decide("Do not call `std::sort`.", source, okResult(
		Ref{Kind: "call", Name: "sort", QualifiedName: "std::sort", Header: "algorithm", Resolved: true, Line: line, Column: 3},
		Ref{Kind: "call", Name: "sort", Resolved: false, Line: line, Column: 3},
	))
	assertViolationAt(t, source, got, line)
}

func TestUnsupportedAnalysisIsUnsure(t *testing.T) {
	source := readFixture(t, "direct_call.cpp")
	line := lineContaining(t, source, "marker-direct")
	banned := "Do not call `std::sort`."
	hit := Ref{Kind: "call", Name: "sort", QualifiedName: "std::sort", Resolved: true, Line: line, Column: 3}

	cases := []Result{
		{Version: 1, Status: "unsure", Reason: "diagnostics", References: []Ref{hit}},
		{Version: 2, Status: "ok", References: []Ref{hit}},
		{Version: 1, Status: "ok", References: tooMany(hit)},
	}
	for _, result := range cases {
		got := Decide(banned, source, result)
		if !got.HardRule || got.Verdict != schema.VerdictUnsure {
			t.Fatalf("result %+v decision %+v, want unsure", result.Status, got)
		}
		if len(got.Evidence) != 0 {
			t.Fatalf("unsupported analysis invented evidence")
		}
	}
}

func TestMalformedAnalyzerDocumentIsUnsure(t *testing.T) {
	source := readFixture(t, "comment_and_string.cpp")
	got := Evaluate("Do not call `std::sort`.", source, []byte(`{"version":1,"status":"ok","references":[],"extra":1}`))
	if !got.HardRule || got.Verdict != schema.VerdictUnsure {
		t.Fatalf("decision = %+v, want unsure", got)
	}
	plain := Evaluate("Explain the ordering you chose.", source, []byte(`{`))
	if plain.HardRule || plain.Verdict == schema.VerdictViolation {
		t.Fatalf("non-ban malformed document decision = %+v", plain)
	}
}

func TestNonBanConstraintIsNotAHardRule(t *testing.T) {
	source := readFixture(t, "direct_call.cpp")
	line := lineContaining(t, source, "marker-direct")
	got := Decide("Explain why the ordering is stable.", source, okResult(Ref{
		Kind: "call", Name: "sort", QualifiedName: "std::sort", Resolved: true, Line: line, Column: 3,
	}))
	if got.HardRule || got.Verdict == schema.VerdictViolation {
		t.Fatalf("semantic constraint decision = %+v", got)
	}
	embedded := Decide("Students should know: Do not call `std::sort`.", source, okResult(Ref{
		Kind: "call", Name: "sort", QualifiedName: "std::sort", Resolved: true, Line: line, Column: 3,
	}))
	if embedded.HardRule {
		t.Fatal("a ban-list prefix must be at the start of the constraint")
	}
}

func TestBanListWithoutNamesIsUnsure(t *testing.T) {
	got := Decide("Do not call the library routine.", "int main(){return 0;}\n", okResult())
	if !got.HardRule || got.Verdict != schema.VerdictUnsure {
		t.Fatalf("decision = %+v, want unsure", got)
	}
	open := Decide("Do not use `std::sort.", "int main(){return 0;}\n", okResult())
	if !open.HardRule || open.Verdict != schema.VerdictUnsure {
		t.Fatalf("unclosed name decision = %+v, want unsure", open)
	}
}

func TestParseResultRejectsUnknownFields(t *testing.T) {
	if _, err := ParseResult([]byte(`{"version":1,"status":"ok","references":[]}`)); err != nil {
		t.Fatalf("valid document: %v", err)
	}
	if _, err := ParseResult([]byte(`{"version":1,"status":"ok","references":[],"verdict":"violation"}`)); err == nil {
		t.Fatal("unknown field must be rejected")
	}
}

func okResult(refs ...Ref) Result {
	if refs == nil {
		refs = []Ref{}
	}
	return Result{Version: 1, Status: "ok", References: refs}
}

func tooMany(seed Ref) []Ref {
	refs := make([]Ref, MaxReferences+1)
	for i := range refs {
		refs[i] = seed
	}
	return refs
}

func assertViolationAt(t *testing.T, source string, got Decision, line int) {
	t.Helper()
	if !got.HardRule {
		t.Fatal("expected a hard-rule decision")
	}
	if got.Verdict != schema.VerdictViolation {
		t.Fatalf("verdict = %s (%s), want violation", got.Verdict, got.Reason)
	}
	text := evidenceText(t, got, line)
	want, ok := originalLine(source, line)
	if !ok || text != want {
		t.Fatalf("evidence text = %q, want original line %q", text, want)
	}
}

func evidenceText(t *testing.T, got Decision, line int) string {
	t.Helper()
	for _, item := range got.Evidence {
		if item.Line == line {
			return item.Text
		}
	}
	t.Fatalf("no evidence at line %d in %#v", line, got.Evidence)
	return ""
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func lineContaining(t *testing.T, source, marker string) int {
	t.Helper()
	for i, line := range strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		if strings.Contains(line, marker) {
			return i + 1
		}
	}
	t.Fatalf("marker %q not found", marker)
	return 0
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
