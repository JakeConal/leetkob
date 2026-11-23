package analyzer

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

type trapExpectation struct {
	verdict Verdict
	lines   []int
}

type trapCase struct {
	name   string
	source string
	banned []string
	want   map[string]trapExpectation
}

// src joins explicit lines and adds the trailing newline a submitted
// source normally has, so test expectations refer to real one-based lines.
func src(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func hardRuleTrapCases() []trapCase {
	qualified := src(
		"#include <algorithm>",
		"#include <vector>",
		"",
		"void f(std::vector<int>& values) {",
		"  std::sort(values.begin(), values.end());",
		"}",
	)
	return []trapCase{
		{
			name:   "qualified std::sort call",
			source: qualified,
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictViolation, []int{5}}},
		},
		{
			name:   "unqualified ban resolves std::sort",
			source: qualified,
			banned: []string{"sort"},
			want:   map[string]trapExpectation{"sort": {VerdictViolation, []int{5}}},
		},
		{
			name: "comment-only mention",
			source: src(
				"// std::sort must not be used",
				"#include <vector>",
				"void f(std::vector<int>& values) { (void)values; }",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictPass, nil}},
		},
		{
			name: "string-literal mention",
			source: src(
				"#include <string>",
				"int main() {",
				"  std::string text = \"std::sort is banned\";",
				"  return static_cast<int>(text.size());",
				"}",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictPass, nil}},
		},
		{
			name: "raw-string mention",
			source: src(
				"#include <string>",
				"int main() {",
				"  std::string text = R\"(std::sort)\";",
				"  return static_cast<int>(text.size());",
				"}",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictPass, nil}},
		},
		{
			name: "algorithm included only for swap",
			source: src(
				"#include <algorithm>",
				"#include <vector>",
				"",
				"int main() {",
				"  int a = 1;",
				"  int b = 2;",
				"  std::swap(a, b);",
				"  return a + b;",
				"}",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictPass, nil}},
		},
		{
			name: "student namespace sort",
			source: src(
				"namespace sort {",
				"int value = 1;",
				"}",
				"int main() { return sort::value; }",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictPass, nil}},
		},
		{
			name: "student namespace sort against unqualified ban",
			source: src(
				"namespace sort {",
				"int value = 1;",
				"}",
				"int main() { return sort::value; }",
			),
			banned: []string{"sort"},
			want:   map[string]trapExpectation{"sort": {VerdictPass, nil}},
		},
		{
			name: "student-defined sort function",
			source: src(
				"void sort(int* begin, int* end) { (void)begin; (void)end; }",
				"int main() {",
				"  int values[2] = {2, 1};",
				"  sort(values, values + 2);",
				"}",
			),
			banned: []string{"sort"},
			want:   map[string]trapExpectation{"sort": {VerdictPass, nil}},
		},
		{
			name: "namespace alias indirection",
			source: src(
				"#include <algorithm>",
				"#include <vector>",
				"namespace alg = std;",
				"void f(std::vector<int>& values) {",
				"  alg::sort(values.begin(), values.end());",
				"}",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictViolation, []int{5}}},
		},
		{
			name: "using-declaration indirection",
			source: src(
				"#include <algorithm>",
				"#include <vector>",
				"using std::sort;",
				"void f(std::vector<int>& values) {",
				"  sort(values.begin(), values.end());",
				"}",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictViolation, []int{5}}},
		},
		{
			name: "using-directive indirection",
			source: src(
				"#include <algorithm>",
				"#include <vector>",
				"using namespace std;",
				"void f(vector<int>& values) {",
				"  sort(values.begin(), values.end());",
				"}",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictViolation, []int{5}}},
		},
		{
			name: "function-like macro indirection",
			source: src(
				"#include <algorithm>",
				"#include <vector>",
				"#define SORT_RANGE(v) std::sort((v).begin(), (v).end())",
				"void f(std::vector<int>& values) {",
				"  SORT_RANGE(values);",
				"}",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictViolation, []int{5}}},
		},
		{
			name: "object-like macro indirection",
			source: src(
				"#include <algorithm>",
				"#include <vector>",
				"#define SORT std::sort",
				"void f(std::vector<int>& values) {",
				"  SORT(values.begin(), values.end());",
				"}",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictViolation, []int{5}}},
		},
		{
			name: "unexpanded macro definition is not a use",
			source: src(
				"#include <algorithm>",
				"#define SORT std::sort",
				"int main() { return 0; }",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictPass, nil}},
		},
		{
			name: "wrapper function indirection",
			source: src(
				"#include <algorithm>",
				"#include <vector>",
				"void runSort(std::vector<int>& values) {",
				"  std::sort(values.begin(), values.end());",
				"}",
				"void f(std::vector<int>& values) { runSort(values); }",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictViolation, []int{4}}},
		},
		{
			name: "disabled preprocessor branch",
			source: src(
				"#include <algorithm>",
				"#include <vector>",
				"#if 0",
				"void f(std::vector<int>& values) { std::sort(values.begin(), values.end()); }",
				"#endif",
				"int main() { return 0; }",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictPass, nil}},
		},
		{
			name: "different namespace with same function name",
			source: src(
				"namespace other { void sort(); }",
				"void f() { other::sort(); }",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictPass, nil}},
		},
		{
			name:   "multiple names decide independently",
			source: qualified,
			banned: []string{"std::sort", "std::stable_sort", "sort"},
			want: map[string]trapExpectation{
				"std::sort":        {VerdictViolation, []int{5}},
				"std::stable_sort": {VerdictPass, nil},
				"sort":             {VerdictViolation, []int{5}},
			},
		},
		{
			name: "unresolved qualified call is unsure",
			source: src(
				"#include <algorithm>",
				"#include <vector>",
				"void f(std::vector<int>& values) {",
				"  unknown::sort(values.begin(), values.end());",
				"}",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictUnsure, nil}},
		},
		{
			name: "missing include is unsure rather than invented pass",
			source: src(
				"#include <leetkob_missing_header>",
				"#include <algorithm>",
				"int main() { return 0; }",
			),
			banned: []string{"std::sort"},
			want:   map[string]trapExpectation{"std::sort": {VerdictUnsure, nil}},
		},
	}
}

func TestDockerAnalyzerHardRuleTraps(t *testing.T) {
	requireDocker(t)
	image := analyzerImage()
	requireImage(t, image)
	docker := NewDocker(image)
	docker.Timeout = 60 * time.Second

	for _, test := range hardRuleTrapCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			report, err := docker.Analyze(ctx, Request{Source: test.source, BannedNames: test.banned})
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			if len(report.Results) != len(test.banned) {
				t.Fatalf("results = %v, want %d", report.Results, len(test.banned))
			}
			lines := strings.Split(test.source, "\n")
			for i, name := range test.banned {
				want, ok := test.want[name]
				if !ok {
					t.Fatalf("test has no expectation for %q", name)
				}
				result := report.Results[i]
				if result.Name != name {
					t.Fatalf("results[%d].Name = %q, want %q", i, result.Name, name)
				}
				if result.Verdict != want.verdict {
					t.Fatalf("%q verdict = %q, want %q (evidence %v)", name, result.Verdict, want.verdict, result.Evidence)
				}
				if result.Verdict != VerdictViolation && len(result.Evidence) != 0 {
					t.Fatalf("%q gave evidence for %s: %v", name, result.Verdict, result.Evidence)
				}
				if result.Verdict == VerdictViolation && len(result.Evidence) == 0 {
					t.Fatalf("%q violation has no evidence", name)
				}
				gotLines := make([]int, 0, len(result.Evidence))
				for _, evidence := range result.Evidence {
					gotLines = append(gotLines, evidence.Line)
					if evidence.Line < 1 || evidence.Line > len(lines) {
						t.Fatalf("%q evidence line %d outside source", name, evidence.Line)
					}
					original := strings.TrimSuffix(lines[evidence.Line-1], "\r")
					if evidence.Text != original {
						t.Fatalf("%q evidence text %q != original line %q", name, evidence.Text, original)
					}
				}
				if !slices.Equal(gotLines, want.lines) {
					t.Fatalf("%q evidence lines = %v, want %v", name, gotLines, want.lines)
				}
			}
		})
	}
}

func analyzerImage() string {
	if image := os.Getenv("ANALYZER_IMAGE"); image != "" {
		return image
	}
	return DefaultImage
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").Run(); err != nil {
		t.Skipf("docker daemon is not available: %v", err)
	}
}

func requireImage(t *testing.T, image string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "image", "inspect", image).Run(); err != nil {
		t.Fatalf("analyzer image %q is not built; run: docker build -t %s apps/backend/analyzer", image, image)
	}
}
