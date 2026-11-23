package checker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/analyzer"
)

const execTestSource = "int main() {\n  std::sort(a, b);\n}\n"

func execReply(names ...string) string {
	items := make([]string, 0, len(names))
	for _, name := range names {
		items = append(items, fmt.Sprintf(`{"name":%q,"verdict":"pass","evidence":[]}`, name))
	}
	return fmt.Sprintf(`{"version":1,"results":[%s]}`, strings.Join(items, ","))
}

func TestExecAnalyzerDecodesReplyInRequestOrder(t *testing.T) {
	bundled := NewExecAnalyzer("/unused/leetkob-analyzer")
	bundled.run = func(_ context.Context, path string, stdin []byte) ([]byte, error) {
		if path != "/unused/leetkob-analyzer" {
			t.Fatalf("path = %q", path)
		}
		if !strings.Contains(string(stdin), `"bannedNames":["sort","std::sort"]`) {
			t.Fatalf("stdin = %s", stdin)
		}
		return []byte(execReply("std::sort", "sort")), nil
	}
	report, err := bundled.Analyze(context.Background(), analyzer.Request{
		Source:      execTestSource,
		BannedNames: []string{"sort", "sort", "std::sort"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("results = %+v, want two unique names", report.Results)
	}
	if report.Results[0].Name != "sort" || report.Results[1].Name != "std::sort" {
		t.Fatalf("results = %+v, want request order", report.Results)
	}
}

func TestExecAnalyzerRejectsInvalidReply(t *testing.T) {
	tests := []struct {
		name  string
		reply string
	}{
		{"unknown field", `{"version":1,"results":[],"extra":1}`},
		{"wrong version", `{"version":2,"results":[{"name":"sort","verdict":"pass","evidence":[]}]}`},
		{"missing result", `{"version":1,"results":[]}`},
		{"duplicate result", `{"version":1,"results":[{"name":"sort","verdict":"pass","evidence":[]},{"name":"sort","verdict":"pass","evidence":[]}]}`},
		{"unknown verdict", `{"version":1,"results":[{"name":"sort","verdict":"maybe","evidence":[]}]}`},
		{"violation without evidence", `{"version":1,"results":[{"name":"sort","verdict":"violation","evidence":[]}]}`},
		{"fabricated evidence", `{"version":1,"results":[{"name":"sort","verdict":"violation","evidence":[{"line":2,"text":"  std::stable_sort(a, b);"}]}]}`},
		{"evidence line out of range", `{"version":1,"results":[{"name":"sort","verdict":"violation","evidence":[{"line":99,"text":"x"}]}]}`},
		{"trailing data", execReply("sort") + `{"version":1}`},
		{"not json", `not json`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundled := NewExecAnalyzer("/unused/leetkob-analyzer")
			bundled.run = func(context.Context, string, []byte) ([]byte, error) {
				return []byte(test.reply), nil
			}
			_, err := bundled.Analyze(context.Background(), analyzer.Request{
				Source:      execTestSource,
				BannedNames: []string{"sort"},
			})
			if err == nil {
				t.Fatal("invalid reply was accepted")
			}
			if !errors.Is(err, analyzer.ErrInvalidReport) && !errors.Is(err, analyzer.ErrOutputTooLarge) {
				t.Fatalf("err = %v, want a strict report error", err)
			}
		})
	}
}

func TestExecAnalyzerRejectsMissingExecutableAndBadRequests(t *testing.T) {
	if _, err := NewExecAnalyzer("  ").Analyze(context.Background(), analyzer.Request{Source: "int main(){}", BannedNames: []string{"sort"}}); err == nil {
		t.Fatal("empty path was accepted")
	}
	bundled := NewExecAnalyzer("/unused/leetkob-analyzer")
	bundled.run = func(context.Context, string, []byte) ([]byte, error) {
		t.Fatal("invalid request reached the process boundary")
		return nil, nil
	}
	if _, err := bundled.Analyze(context.Background(), analyzer.Request{
		Source:      execTestSource,
		BannedNames: []string{"bad\x00name"},
	}); !errors.Is(err, analyzer.ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestExecAnalyzerMapsTimeout(t *testing.T) {
	bundled := NewExecAnalyzer("/unused/leetkob-analyzer")
	bundled.Timeout = 10 * time.Millisecond
	bundled.run = func(ctx context.Context, _ string, _ []byte) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := bundled.Analyze(context.Background(), analyzer.Request{
		Source:      execTestSource,
		BannedNames: []string{"sort"},
	})
	if !errors.Is(err, analyzer.ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

// TestExecAnalyzerChildProcessGetsNoSecrets proves the child process
// environment is rebuilt, not inherited. A script records its environment and
// returns a valid reply.
func TestExecAnalyzerChildProcessGetsNoSecrets(t *testing.T) {
	t.Setenv("MODEL_API_KEY", "live-key-must-not-leak")
	t.Setenv("DATASET_LABEL_SECRET", "label-must-not-leak")
	dir := t.TempDir()
	envCapture := filepath.Join(dir, "env.txt")
	script := filepath.Join(dir, "fake-analyzer.sh")
	content := "#!/bin/sh\nenv > " + envCapture + "\ncat >/dev/null\nprintf '%s' '" + execReply("sort") + "'\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := NewExecAnalyzer(script).Analyze(context.Background(), analyzer.Request{
		Source:      execTestSource,
		BannedNames: []string{"sort"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(report.Results) != 1 || report.Results[0].Verdict != analyzer.VerdictPass {
		t.Fatalf("report = %+v", report)
	}
	captured, err := os.ReadFile(envCapture)
	if err != nil {
		t.Fatalf("read captured environment: %v", err)
	}
	for _, secret := range []string{"MODEL_API_KEY", "DATASET_LABEL_SECRET", "live-key-must-not-leak", "label-must-not-leak"} {
		if strings.Contains(string(captured), secret) {
			t.Fatalf("child environment leaked %q:\n%s", secret, captured)
		}
	}
}
