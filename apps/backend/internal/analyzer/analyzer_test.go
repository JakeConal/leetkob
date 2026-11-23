package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const testSource = "int main() {\n  std::sort(values, values + 2);\n}\n"

func testReply(name, verdict, evidenceLineText string) string {
	if evidenceLineText == "" {
		return fmt.Sprintf(`{"version":1,"results":[{"name":%q,"verdict":%q,"evidence":[]}]}`, name, verdict)
	}
	return fmt.Sprintf(`{"version":1,"results":[{"name":%q,"verdict":%q,"evidence":[{"line":2,"text":%q}]}]}`, name, verdict, evidenceLineText)
}

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want error
	}{
		{
			name: "valid empty source",
			req:  Request{},
		},
		{
			name: "valid names",
			req:  Request{Source: testSource, BannedNames: []string{"sort", "std::sort"}},
		},
		{
			name: "invalid UTF-8 source",
			req:  Request{Source: string([]byte{0xff})},
			want: ErrInvalidRequest,
		},
		{
			name: "source too large",
			req:  Request{Source: strings.Repeat("x", MaxSourceBytes+1)},
			want: ErrInvalidRequest,
		},
		{
			name: "empty name",
			req:  Request{Source: testSource, BannedNames: []string{"  "}},
			want: ErrInvalidRequest,
		},
		{
			name: "control character in name",
			req:  Request{Source: testSource, BannedNames: []string{"std::sort\x01"}},
			want: ErrInvalidRequest,
		},
		{
			name: "name too long",
			req:  Request{Source: testSource, BannedNames: []string{strings.Repeat("n", MaxNameBytes+1)}},
			want: ErrInvalidRequest,
		},
		{
			name: "too many names",
			req:  Request{Source: testSource, BannedNames: manyNames(MaxBannedNames + 1)},
			want: ErrInvalidRequest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequest(test.req)
			if test.want == nil {
				if err != nil {
					t.Fatalf("ValidateRequest: %v", err)
				}
				return
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("ValidateRequest error = %v, want %v", err, test.want)
			}
		})
	}
}

func manyNames(count int) []string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("name%d", i)
	}
	return names
}

func TestFallbackDecidesUnsureNeverViolation(t *testing.T) {
	fallback := Fallback{Reason: "docker unavailable for test"}
	report, err := fallback.Analyze(context.Background(), Request{
		Source:      testSource,
		BannedNames: []string{"std::sort", "sort"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.Version != ProtocolVersion {
		t.Fatalf("version = %d, want %d", report.Version, ProtocolVersion)
	}
	if report.Cause != "docker unavailable for test" {
		t.Fatalf("cause = %q", report.Cause)
	}
	if len(report.Results) != 2 {
		t.Fatalf("results = %v", report.Results)
	}
	for _, result := range report.Results {
		if result.Verdict != VerdictUnsure {
			t.Fatalf("%q verdict = %q, want unsure", result.Name, result.Verdict)
		}
		if len(result.Evidence) != 0 {
			t.Fatalf("%q gave evidence on unsure: %v", result.Name, result.Evidence)
		}
	}
}

func TestFallbackRejectsInvalidRequest(t *testing.T) {
	_, err := (Fallback{}).Analyze(context.Background(), Request{Source: string([]byte{0xff})})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestUnsureReportDeduplicatesAndPreservesOrder(t *testing.T) {
	report := UnsureReport([]string{"sort", "std::sort", "sort"}, "cause")
	if len(report.Results) != 2 {
		t.Fatalf("results = %v", report.Results)
	}
	if report.Results[0].Name != "sort" || report.Results[1].Name != "std::sort" {
		t.Fatalf("order = %v", report.Results)
	}
}

func TestDockerAnalyzerEncodesBoundedRequestAndReordersReply(t *testing.T) {
	var captured wireRequest
	docker := NewDocker("test-image")
	docker.run = func(_ context.Context, stdin []byte) ([]byte, error) {
		if len(stdin) > MaxInputBytes {
			t.Fatalf("stdin = %d bytes", len(stdin))
		}
		if err := json.Unmarshal(stdin, &captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		// Deliberately return results out of request order.
		return []byte(`{"version":1,"results":[` +
			`{"name":"std::sort","verdict":"violation","evidence":[{"line":2,"text":"  std::sort(values, values + 2);"}]},` +
			`{"name":"sort","verdict":"pass","evidence":[]}]}`), nil
	}
	report, err := docker.Analyze(context.Background(), Request{
		Source:      testSource,
		BannedNames: []string{"sort", " sort ", "std::sort"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if captured.Source != testSource {
		t.Fatalf("captured source differs")
	}
	if len(captured.BannedNames) != 2 || captured.BannedNames[0] != "sort" || captured.BannedNames[1] != "std::sort" {
		t.Fatalf("captured names = %v", captured.BannedNames)
	}
	if len(report.Results) != 2 {
		t.Fatalf("results = %v", report.Results)
	}
	if report.Results[0].Name != "sort" || report.Results[0].Verdict != VerdictPass {
		t.Fatalf("results[0] = %+v", report.Results[0])
	}
	if report.Results[1].Name != "std::sort" || report.Results[1].Verdict != VerdictViolation {
		t.Fatalf("results[1] = %+v", report.Results[1])
	}
}

func TestDockerAnalyzerRejectsFabricatedEvidence(t *testing.T) {
	tests := []struct {
		name  string
		reply string
	}{
		{
			name:  "text is not an original line",
			reply: testReply("std::sort", "violation", "invented evidence"),
		},
		{
			name:  "line is outside source",
			reply: `{"version":1,"results":[{"name":"std::sort","verdict":"violation","evidence":[{"line":99,"text":"std::sort"}]}]}`,
		},
		{
			name:  "partial prefix is not accepted",
			reply: testReply("std::sort", "violation", "  std::sort"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			docker := NewDocker("test-image")
			docker.run = func(context.Context, []byte) ([]byte, error) { return []byte(test.reply), nil }
			report, err := docker.Analyze(context.Background(), Request{
				Source:      testSource,
				BannedNames: []string{"std::sort"},
			})
			if !errors.Is(err, ErrInvalidReport) {
				t.Fatalf("err = %v, want ErrInvalidReport", err)
			}
			if len(report.Results) != 0 {
				t.Fatalf("partial results returned on invalid reply: %v", report.Results)
			}
		})
	}
}

func TestDockerAnalyzerRejectsMalformedReports(t *testing.T) {
	tests := []struct {
		name  string
		reply string
	}{
		{
			name:  "invalid JSON",
			reply: `{"version":1,"results":[`,
		},
		{
			name:  "unknown field",
			reply: `{"version":1,"results":[],"answer":"pass"}`,
		},
		{
			name:  "wrong version",
			reply: `{"version":2,"results":[]}`,
		},
		{
			name:  "missing result",
			reply: `{"version":1,"results":[]}`,
		},
		{
			name:  "unknown result name",
			reply: testReply("std::stable_sort", "pass", ""),
		},
		{
			name: "duplicate result",
			reply: `{"version":1,"results":[` +
				`{"name":"std::sort","verdict":"pass","evidence":[]},` +
				`{"name":"std::sort","verdict":"pass","evidence":[]}]}`,
		},
		{
			name:  "unknown verdict",
			reply: testReply("std::sort", "maybe", ""),
		},
		{
			name:  "violation without evidence",
			reply: testReply("std::sort", "violation", ""),
		},
		{
			name:  "trailing JSON",
			reply: testReply("std::sort", "pass", "") + `{}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			docker := NewDocker("test-image")
			docker.run = func(context.Context, []byte) ([]byte, error) { return []byte(test.reply), nil }
			_, err := docker.Analyze(context.Background(), Request{
				Source:      testSource,
				BannedNames: []string{"std::sort"},
			})
			if !errors.Is(err, ErrInvalidReport) {
				t.Fatalf("err = %v, want ErrInvalidReport", err)
			}
		})
	}
}

func TestDockerAnalyzerRejectsOversizedReply(t *testing.T) {
	docker := NewDocker("test-image")
	docker.run = func(context.Context, []byte) ([]byte, error) {
		return bytes.Repeat([]byte("x"), MaxOutputBytes+1), nil
	}
	_, err := docker.Analyze(context.Background(), Request{
		Source:      testSource,
		BannedNames: []string{"std::sort"},
	})
	if !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("err = %v, want ErrOutputTooLarge", err)
	}
}

func TestDockerAnalyzerRejectsOversizedEncodedInput(t *testing.T) {
	called := false
	docker := NewDocker("test-image")
	docker.run = func(context.Context, []byte) ([]byte, error) {
		called = true
		return nil, errors.New("must not run")
	}
	// NUL bytes are valid UTF-8 but expand to six bytes each in JSON.
	_, err := docker.Analyze(context.Background(), Request{
		Source:      strings.Repeat("\x00", MaxSourceBytes),
		BannedNames: []string{"std::sort"},
	})
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("err = %v, want ErrInputTooLarge", err)
	}
	if called {
		t.Fatal("runner was invoked despite an oversized request")
	}
}

func TestDockerAnalyzerTimeout(t *testing.T) {
	docker := NewDocker("test-image")
	docker.Timeout = 20 * time.Millisecond
	docker.run = func(ctx context.Context, _ []byte) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := docker.Analyze(context.Background(), Request{
		Source:      testSource,
		BannedNames: []string{"std::sort"},
	})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestDockerAnalyzerAcceptsCRLFOriginalLines(t *testing.T) {
	source := "int main() {\r\n  std::sort(values, values + 2);\r\n}\r\n"
	docker := NewDocker("test-image")
	docker.run = func(context.Context, []byte) ([]byte, error) {
		return []byte(testReply("std::sort", "violation", "  std::sort(values, values + 2);")), nil
	}
	report, err := docker.Analyze(context.Background(), Request{
		Source:      source,
		BannedNames: []string{"std::sort"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.Results[0].Evidence[0].Line != 2 {
		t.Fatalf("evidence = %v", report.Results[0].Evidence)
	}
}

func TestDockerAnalyzerDoesNotReturnPartialResultsOnRunnerError(t *testing.T) {
	docker := NewDocker("test-image")
	docker.run = func(context.Context, []byte) ([]byte, error) {
		return nil, ErrContainerFailed
	}
	report, err := docker.Analyze(context.Background(), Request{
		Source:      testSource,
		BannedNames: []string{"std::sort"},
	})
	if !errors.Is(err, ErrContainerFailed) {
		t.Fatalf("err = %v", err)
	}
	if len(report.Results) != 0 {
		t.Fatalf("partial results = %v", report.Results)
	}
}
