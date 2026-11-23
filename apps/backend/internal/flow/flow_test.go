package flow_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/checker/hardrule"
	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

const source = `#include <algorithm>
#include <iostream>
#include <vector>
int main() {
  std::vector<int> values{3, 1, 2};
  std::sort(values.begin(), values.end());
  std::cout << "sorted\n";
}
`

// TestMergedPipeline runs the four merged parts in one sequence: durable
// store, sandboxed compile and run, a hard-rule ban, then a semantic check
// that is not asked to judge that ban. HTTP admission and the worker queue
// are not part of this path.
func TestMergedPipeline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := openFlowStore(t, ctx)
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("initialize store: %v", err)
	}

	compiled := compileAndRun(t, ctx)
	if compiled.ExitCode != 0 || string(compiled.Stdout) != "sorted\n" {
		t.Fatalf("sandbox stdout = %q exit %d, want sorted", compiled.Stdout, compiled.ExitCode)
	}

	ban := "Do not use `std::sort`"
	decision := decideBan(t, ctx, ban)
	if !decision.HardRule || decision.Verdict != schema.VerdictViolation || len(decision.Evidence) == 0 {
		t.Fatalf("hard rule = %+v, want a violation with evidence", decision)
	}

	model := &recordingModel{body: `{"results":[{"constraintId":"semantic-print","verdict":"pass","evidence":[]}]}`}
	checker := &semantic.Checker{
		Model:   model,
		Store:   semantic.NewMemoryStore(),
		Timeout: 2 * time.Second,
	}
	semanticResult, err := checker.Check(ctx, semantic.Input{
		LabID:     "lab-flow",
		Statement: "Print the word sorted.",
		Constraints: []semantic.Constraint{{
			ID:   "semantic-print",
			Text: "The program prints sorted.",
		}},
		Source: source,
		Price: semantic.Price{
			Known:                  true,
			MaxInputTokens:         1000,
			MaxOutputTokens:        100,
			InputMicroUSDPerToken:  1,
			OutputMicroUSDPerToken: 1,
		},
	})
	if err != nil {
		t.Fatalf("semantic check: %v", err)
	}
	if model.calls != 1 {
		t.Fatalf("model calls = %d, want 1", model.calls)
	}
	if strings.Contains(model.lastUser, "ban-sort") || strings.Contains(model.lastUser, "Do not use") {
		t.Fatalf("semantic request included the ban constraint: %s", model.lastUser)
	}
	if len(semanticResult.Findings) != 1 || semanticResult.Findings[0].Verdict != semantic.VerdictPass {
		t.Fatalf("semantic findings = %+v", semanticResult.Findings)
	}

	var submissionID string
	err = store.Within(ctx, func(ctx context.Context, tx *postgres.Tx) error {
		var userID, classID, labID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (email, display_name, role, password_hash)
			VALUES ('flow@synthetic.leetkob.local', 'Flow Student', 'student', 'not-a-login')
			RETURNING id`).Scan(&userID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO classes (name) VALUES ('Flow') RETURNING id`).Scan(&classID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO labs (
				class_id, created_by, title, statement, opens_at, deadline, cpu_seconds, memory_mib)
			VALUES ($1, $2, 'flow', 'Print sorted.', now(), now() + interval '1 day', 2, 256)
			RETURNING id`, classID, userID).Scan(&labID); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `
			INSERT INTO lab_revisions (
				lab_id, revision, title, statement, opens_at, deadline, cpu_seconds, memory_mib,
				compile_seconds, wall_multiplier, max_processes, max_output_bytes, max_source_bytes,
				test_revision, constraint_revision, generation)
			SELECT id, 1, title, statement, opens_at, deadline, cpu_seconds, memory_mib,
				compile_seconds, wall_multiplier, max_processes, max_output_bytes, max_source_bytes,
				test_revision, constraint_revision, generation
			FROM labs WHERE id = $1`, labID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO submissions (
				lab_id, author_user_id, source, source_sha256, state, received_at,
				generation, test_revision, constraint_revision, judge_verdict)
			VALUES ($1, $2, $3, 'flow-source', 'judged', now(), 1, 1, 1, 'AC')
			RETURNING id`, labID, userID, source).Scan(&submissionID); err != nil {
			return err
		}
		var findingID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO findings (
				submission_id, lab_id, constraint_id, generation, test_revision, constraint_revision,
				source_sha256, source_kind, verdict, deduction_hundredths, required_review)
			VALUES ($1, $2, 'ban-sort', 1, 1, 1, 'flow-source', 'hard-rule', 'violation', 0, true)
			RETURNING id`, submissionID, labID).Scan(&findingID); err != nil {
			return err
		}
		return tx.Exec(ctx, `
			INSERT INTO finding_evidence (finding_id, position, line, evidence_text)
			VALUES ($1, 1, $2, $3)`, findingID, decision.Evidence[0].Line, decision.Evidence[0].Text)
	})
	if err != nil {
		t.Fatalf("persist flow result: %v", err)
	}

	var verdict, evidence string
	err = store.Within(ctx, func(ctx context.Context, tx *postgres.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT s.judge_verdict, e.evidence_text
			FROM submissions s
			JOIN findings f ON f.submission_id = s.id
			JOIN finding_evidence e ON e.finding_id = f.id
			WHERE s.id = $1`, submissionID).Scan(&verdict, &evidence)
	})
	if err != nil {
		t.Fatalf("read flow result: %v", err)
	}
	if verdict != "AC" || !strings.Contains(evidence, "std::sort") {
		t.Fatalf("stored verdict %q evidence %q", verdict, evidence)
	}
}

func compileAndRun(t *testing.T, ctx context.Context) sandbox.Observation {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker is not available; merged flow was not run: %v", err)
	}
	controller := sandbox.New()
	compiled, err := controller.Compile(ctx, []byte(source))
	if err != nil {
		t.Fatalf("sandbox compile: %v", err)
	}
	if !compiled.OK {
		t.Fatalf("sandbox compile failed: %s", compiled.Stderr)
	}
	policy, err := sandbox.JudgeRunPolicy(2, 256)
	if err != nil {
		t.Fatalf("run policy: %v", err)
	}
	observed, err := controller.Execute(ctx, compiled.Binary, policy, nil)
	if err != nil {
		t.Fatalf("sandbox execute: %v", err)
	}
	return observed
}

func decideBan(t *testing.T, ctx context.Context, constraint string) hardrule.Decision {
	t.Helper()
	binary := buildAnalyzer(t)
	args, err := hardrule.CompilerArgs(ctx)
	if err != nil {
		t.Fatalf("compiler args: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "main.cpp")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	// Temp directories on macOS sit behind a symlink. The analyzer only
	// keeps references whose file matches the real path it was given.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	raw, err := hardrule.RunAnalyzer(ctx, binary, path, args)
	if err != nil {
		t.Fatalf("analyzer: %v", err)
	}
	return hardrule.Evaluate(constraint, source, raw)
}

func buildAnalyzer(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate flow test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	src := filepath.Join(root, "native", "cpp-analyzer", "analyzer.c")
	libdir, err := libclangDir()
	if err != nil {
		t.Skipf("libclang is not available; merged flow was not run: %v", err)
	}
	clang, err := commandPath("clang")
	if err != nil {
		t.Skipf("clang is not available; merged flow was not run: %v", err)
	}
	out := filepath.Join(t.TempDir(), "cpp-analyzer")
	cmd := exec.Command(clang, "-std=c11", src, "-o", out, "-lclang", "-L"+libdir, "-Wl,-rpath,"+libdir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compile analyzer: %v\n%s", err, output)
	}
	return out
}

func commandPath(name string) (string, error) {
	found, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(found)
}

func libclangDir() (string, error) {
	clang, err := commandPath("clang")
	if err != nil {
		return "", err
	}
	if out, err := exec.Command(clang, "-print-file-name=libclang.dylib").Output(); err == nil {
		path := strings.TrimSpace(string(out))
		if filepath.IsAbs(path) {
			return filepath.Dir(path), nil
		}
	}
	candidate := "/Library/Developer/CommandLineTools/usr/lib/libclang.dylib"
	if _, err := os.Stat(candidate); err == nil {
		return filepath.Dir(candidate), nil
	}
	return "", os.ErrNotExist
}

func openFlowStore(t *testing.T, ctx context.Context) *postgres.Store {
	t.Helper()
	maintenance := strings.TrimSpace(os.Getenv("POSTGRES_URL"))
	if maintenance == "" {
		maintenance = "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"
	}
	dial, err := net.DialTimeout("tcp", "127.0.0.1:5432", 250*time.Millisecond)
	if err != nil && os.Getenv("POSTGRES_URL") == "" {
		t.Skip("PostgreSQL is not reachable; merged flow was not run")
	}
	if dial != nil {
		_ = dial.Close()
	}

	admin, err := pgx.Connect(ctx, maintenance)
	if err != nil {
		t.Fatalf("connect maintenance database: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	name := "leetkob_flow_" + randomHex(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create flow database: %v", err)
	}
	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(dropCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	})

	cfg, err := pgx.ParseConfig(maintenance)
	if err != nil {
		t.Fatalf("parse maintenance URL: %v", err)
	}
	cfg.Database = name
	store, err := postgres.New(ctx, cfg.ConnString())
	if err != nil {
		t.Fatalf("connect flow database: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

func randomHex(t *testing.T) string {
	t.Helper()
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(buf[:])
}

type recordingModel struct {
	body     string
	calls    int
	lastUser string
}

func (m *recordingModel) Complete(_ context.Context, req semantic.CompletionRequest) (semantic.ModelResponse, error) {
	m.calls++
	m.lastUser = req.UserJSON
	if strings.TrimSpace(m.body) == "" {
		return semantic.ModelResponse{}, errors.New("empty script")
	}
	return semantic.ModelResponse{Content: m.body, TokensKnown: true, InputTokens: 10, OutputTokens: 5, RequestID: "flow-fake"}, nil
}
