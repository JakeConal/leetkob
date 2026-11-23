package hardrule

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
)

func TestNativeAnalyzerTrapsAndIndirection(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang is not on PATH; native libclang analyzer was not executed")
	}
	binary := buildAnalyzer(t)
	args, err := CompilerArgs(context.Background())
	if err != nil {
		t.Fatalf("compiler arguments: %v", err)
	}

	cases := []struct {
		file       string
		constraint string
		marker     string
		absent     string
		want       schema.Verdict
	}{
		{"own_namespace.cpp", "Do not call `std::sort`.", "", "", schema.VerdictPass},
		{"own_namespace.cpp", "Do not use `sort`.", "", "", schema.VerdictPass},
		{"comment_and_string.cpp", "Do not call `std::sort`.", "", "", schema.VerdictPass},
		{"swap_only.cpp", "Do not call `std::sort`.", "", "", schema.VerdictPass},
		{"swap_only.cpp", "Do not use `<algorithm>`.", "", "", schema.VerdictPass},
		{"swap_only.cpp", "Do not call `std::swap`.", "marker-swap", "", schema.VerdictViolation},
		{"namespace_alias.cpp", "Do not call `std::sort`.", "marker-alias", "", schema.VerdictViolation},
		{"using_decl.cpp", "Do not use `std::sort`.", "marker-using", "", schema.VerdictViolation},
		{"macro_call.cpp", "Do not call `std::sort`.", "marker-macro", "", schema.VerdictViolation},
		{"wrapper.cpp", "Do not call `std::sort`.", "marker-wrapper", "marker-wrapper-call", schema.VerdictViolation},
		{"direct_call.cpp", "Do not call `std::sort`.", "marker-direct", "", schema.VerdictViolation},
		{"direct_call.cpp", "Do not use `<algorithm>`.", "marker-direct", "", schema.VerdictViolation},
		{"stable_using.cpp", "Do not call `std::stable_sort`.", "marker-stable", "", schema.VerdictViolation},
		{"qsort_call.cpp", "Do not call `qsort`.", "marker-qsort", "", schema.VerdictViolation},
	}

	for _, tc := range cases {
		t.Run(tc.file+"/"+tc.constraint, func(t *testing.T) {
			path := filepath.Join("testdata", tc.file)
			source := readFixture(t, tc.file)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			raw, err := RunAnalyzer(ctx, binary, path, args)
			if err != nil {
				t.Fatalf("analyzer: %v", err)
			}
			got := Evaluate(tc.constraint, source, raw)
			if !got.HardRule {
				t.Fatalf("analyzer document was not a hard-rule decision: %s", raw)
			}
			if got.Verdict != tc.want {
				t.Fatalf("verdict = %s (%s), want %s\n%s", got.Verdict, got.Reason, tc.want, raw)
			}
			if tc.want == schema.VerdictPass {
				if len(got.Evidence) != 0 {
					t.Fatalf("pass evidence = %#v", got.Evidence)
				}
				return
			}
			line := lineContaining(t, source, tc.marker)
			assertViolationAt(t, source, got, line)
			if tc.absent != "" {
				other := lineContaining(t, source, tc.absent)
				for _, item := range got.Evidence {
					if item.Line == other {
						t.Fatalf("evidence cited the wrapper call at line %d instead of the banned call", other)
					}
				}
			}
			if tc.file == "namespace_alias.cpp" || tc.file == "macro_call.cpp" {
				text := evidenceText(t, got, line)
				if containsStdSortSpelling(text) {
					t.Fatalf("evidence %q is a raw spelling match, not the original indirect call", text)
				}
			}
		})
	}
}

func containsStdSortSpelling(text string) bool {
	return strings.Contains(text, "std::sort") || strings.Contains(text, "std :: sort")
}

func buildAnalyzer(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate native test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	src := filepath.Join(root, "native", "cpp-analyzer", "analyzer.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("analyzer source %s: %v", src, err)
	}
	libdir, err := libclangDir()
	if err != nil {
		t.Fatalf("libclang: %v", err)
	}
	out := filepath.Join(t.TempDir(), "cpp-analyzer")
	cmd := exec.Command("clang", "-std=c11", src, "-o", out, "-lclang", "-L"+libdir, "-Wl,-rpath,"+libdir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compile analyzer: %v\n%s", err, output)
	}
	return out
}

func libclangDir() (string, error) {
	if out, err := exec.Command("clang", "-print-file-name=libclang.dylib").Output(); err == nil {
		path := string(trimSpaceBytes(out))
		if len(path) > 0 && filepath.IsAbs(path) {
			return filepath.Dir(path), nil
		}
	}
	candidates := []string{
		"/Library/Developer/CommandLineTools/usr/lib/libclang.dylib",
		"/usr/lib/llvm-19/lib/libclang.so",
		"/usr/lib/llvm-19/lib/libclang.so.1",
		"/usr/lib/x86_64-linux-gnu/libclang-19.so",
		"/usr/lib/llvm-18/lib/libclang.so",
		"/usr/lib/libclang.so",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Dir(candidate), nil
		}
	}
	return "", os.ErrNotExist
}

func trimSpaceBytes(in []byte) []byte {
	start, end := 0, len(in)
	for start < end && (in[start] == ' ' || in[start] == '\n' || in[start] == '\r' || in[start] == '\t') {
		start++
	}
	for end > start && (in[end-1] == ' ' || in[end-1] == '\n' || in[end-1] == '\r' || in[end-1] == '\t') {
		end--
	}
	return in[start:end]
}
