package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JakeConal/leetkob/backend/internal/analyzer"
)

// ExecAnalyzer runs the analyzer executable bundled in the checker image as
// a direct child process. It never uses a Docker socket, a shell, a network
// namespace or a credential. The wire protocol is the same bounded JSON the
// isolated analyzer image speaks; see apps/backend/analyzer/README.md.
type ExecAnalyzer struct {
	// Path is the bundled analyzer executable.
	Path string
	// Timeout bounds one process invocation. Zero selects the analyzer
	// package default.
	Timeout time.Duration

	run func(ctx context.Context, path string, stdin []byte) ([]byte, error)
}

// NewExecAnalyzer returns an analyzer backed by the executable at path.
func NewExecAnalyzer(path string) *ExecAnalyzer {
	return &ExecAnalyzer{Path: path, Timeout: analyzer.DefaultTimeout}
}

// Analyze implements analyzer.Analyzer. Every failure is returned as an
// error so the caller records unsure; no partial or guessed verdict set is
// ever produced.
func (e *ExecAnalyzer) Analyze(ctx context.Context, request analyzer.Request) (analyzer.Report, error) {
	if strings.TrimSpace(e.Path) == "" {
		return analyzer.Report{}, fmt.Errorf("%w: no bundled analyzer path", analyzer.ErrDockerUnavailable)
	}
	if err := analyzer.ValidateRequest(request); err != nil {
		return analyzer.Report{}, err
	}
	names := normalizeNames(request.BannedNames)
	input, err := encodeAnalyzerRequest(request.Source, names)
	if err != nil {
		return analyzer.Report{}, err
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = analyzer.DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	run := e.run
	if run == nil {
		run = runExecProcess
	}
	raw, err := run(runCtx, e.Path, input)
	if err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return analyzer.Report{}, fmt.Errorf("%w: %v", analyzer.ErrTimeout, runCtx.Err())
		}
		return analyzer.Report{}, err
	}
	return decodeAnalyzerReply(raw, request.Source, names)
}

// runExecProcess executes one bounded child process. The environment is
// rebuilt from scratch so no provider key, dataset label or Docker socket
// variable can reach the analyzer through the environment.
func runExecProcess(ctx context.Context, path string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path)
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"TMPDIR=/tmp",
		"LC_ALL=C",
	}
	stdout := &execBoundedBuffer{limit: analyzer.MaxOutputBytes}
	stderr := &execCappedBuffer{limit: 8 << 10}
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		// Stderr is intentionally not included: libclang diagnostics can
		// quote student source.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%w: analyzer exit code %d", analyzer.ErrContainerFailed, exitErr.ExitCode())
		}
		return nil, fmt.Errorf("%w: %v", analyzer.ErrContainerFailed, err)
	}
	if stdout.overflow {
		return nil, fmt.Errorf("%w: maximum is %d bytes", analyzer.ErrOutputTooLarge, analyzer.MaxOutputBytes)
	}
	return stdout.Bytes(), nil
}

func normalizeNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	unique := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		unique = append(unique, name)
	}
	return unique
}

type execAnalyzerRequest struct {
	Source      string   `json:"source"`
	BannedNames []string `json:"bannedNames"`
}

func encodeAnalyzerRequest(source string, names []string) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(execAnalyzerRequest{Source: source, BannedNames: names}); err != nil {
		return nil, fmt.Errorf("%w: encode analyzer request: %v", analyzer.ErrInvalidRequest, err)
	}
	input := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	if len(input) > analyzer.MaxInputBytes {
		return nil, fmt.Errorf("%w: %d bytes, maximum is %d", analyzer.ErrInputTooLarge, len(input), analyzer.MaxInputBytes)
	}
	return input, nil
}

type execAnalyzerReply struct {
	Version int                `json:"version"`
	Results []execAnalyzerItem `json:"results"`
}

type execAnalyzerItem struct {
	Name     string              `json:"name"`
	Verdict  analyzer.Verdict    `json:"verdict"`
	Evidence []analyzer.Evidence `json:"evidence"`
}

// decodeAnalyzerReply applies the same strict reply rules as the Go analyzer
// boundary: exact version, one result per requested name, valid verdicts,
// original-line evidence for violations and no fabricated citations.
func decodeAnalyzerReply(raw []byte, source string, names []string) (analyzer.Report, error) {
	if len(raw) > analyzer.MaxOutputBytes {
		return analyzer.Report{}, fmt.Errorf("%w: %d bytes, maximum is %d", analyzer.ErrOutputTooLarge, len(raw), analyzer.MaxOutputBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var reply execAnalyzerReply
	if err := decoder.Decode(&reply); err != nil {
		return analyzer.Report{}, fmt.Errorf("%w: decode: %v", analyzer.ErrInvalidReport, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return analyzer.Report{}, fmt.Errorf("%w: trailing data", analyzer.ErrInvalidReport)
	}
	if reply.Version != analyzer.ProtocolVersion {
		return analyzer.Report{}, fmt.Errorf("%w: version %d, want %d", analyzer.ErrInvalidReport, reply.Version, analyzer.ProtocolVersion)
	}
	if len(reply.Results) != len(names) {
		return analyzer.Report{}, fmt.Errorf("%w: %d results for %d requested names", analyzer.ErrInvalidReport, len(reply.Results), len(names))
	}
	lines := sourceLines(source)
	byName := make(map[string]execAnalyzerItem, len(reply.Results))
	for _, result := range reply.Results {
		if _, duplicate := byName[result.Name]; duplicate {
			return analyzer.Report{}, fmt.Errorf("%w: duplicate result for %q", analyzer.ErrInvalidReport, result.Name)
		}
		byName[result.Name] = result
	}
	results := make([]analyzer.Result, 0, len(names))
	for _, name := range names {
		result, ok := byName[name]
		if !ok {
			return analyzer.Report{}, fmt.Errorf("%w: missing result for %q", analyzer.ErrInvalidReport, name)
		}
		if !result.Verdict.Valid() {
			return analyzer.Report{}, fmt.Errorf("%w: result %q has verdict %q", analyzer.ErrInvalidReport, name, result.Verdict)
		}
		evidence := result.Evidence
		if evidence == nil {
			evidence = []analyzer.Evidence{}
		}
		if len(evidence) > analyzer.MaxEvidencePerResult {
			return analyzer.Report{}, fmt.Errorf("%w: result %q cites %d lines, maximum is %d", analyzer.ErrInvalidReport, name, len(evidence), analyzer.MaxEvidencePerResult)
		}
		if result.Verdict == analyzer.VerdictViolation && len(evidence) == 0 {
			return analyzer.Report{}, fmt.Errorf("%w: violation %q has no evidence", analyzer.ErrInvalidReport, name)
		}
		for i, item := range evidence {
			if err := validateAnalyzerEvidence(item, lines); err != nil {
				return analyzer.Report{}, fmt.Errorf("%w: result %q evidence[%d]: %v", analyzer.ErrInvalidReport, name, i, err)
			}
		}
		results = append(results, analyzer.Result{Name: name, Verdict: result.Verdict, Evidence: evidence})
	}
	return analyzer.Report{Version: analyzer.ProtocolVersion, Results: results}, nil
}

func validateAnalyzerEvidence(item analyzer.Evidence, lines []string) error {
	if item.Line < 1 || item.Line > len(lines) {
		return fmt.Errorf("line %d is outside the %d-line source", item.Line, len(lines))
	}
	if item.Text == "" {
		return errors.New("text is empty")
	}
	if len(item.Text) > analyzer.MaxEvidenceTextBytes {
		return fmt.Errorf("text is %d bytes, maximum is %d", len(item.Text), analyzer.MaxEvidenceTextBytes)
	}
	if !utf8.ValidString(item.Text) {
		return errors.New("text is not valid UTF-8")
	}
	original := lines[item.Line-1]
	if item.Text == original {
		return nil
	}
	if len(original) > analyzer.MaxEvidenceTextBytes && item.Text == truncateUTF8(original, analyzer.MaxEvidenceTextBytes) {
		return nil
	}
	return fmt.Errorf("text does not match original line %d", item.Line)
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

type execBoundedBuffer struct {
	limit    int
	buf      bytes.Buffer
	overflow bool
}

func (b *execBoundedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > b.limit {
		b.overflow = true
		return 0, analyzer.ErrOutputTooLarge
	}
	return b.buf.Write(p)
}

func (b *execBoundedBuffer) Bytes() []byte { return b.buf.Bytes() }

type execCappedBuffer struct {
	limit int
	buf   bytes.Buffer
}

func (b *execCappedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.buf.Write(p)
		} else {
			_, _ = b.buf.Write(p[:remaining])
		}
	}
	return len(p), nil
}
