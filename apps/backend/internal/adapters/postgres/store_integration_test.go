package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pinnedPostgresImage is the dependency-inventory pin: PostgreSQL 18.7
// (Debian 18.7-1.pgdg13+5) resolved to its immutable digest on
// 2026-11-23. TEST_POSTGRES_IMAGE may override it for a local disposable
// image only.
const pinnedPostgresImage = "postgres@sha256:eb63372c1e05c755f55fa30cbd07aecc443676fc6c0fa90b9002d01537841e1a"

func postgresImage() string {
	if override := os.Getenv("TEST_POSTGRES_IMAGE"); override != "" {
		return override
	}
	return pinnedPostgresImage
}

const testPassword = "leetkob-test-password"

// startTestDatabase provisions an isolated database per test and returns
// a pool connected to it. Each call starts (or reuses) a disposable
// PostgreSQL container unless TEST_DATABASE_URL points at an existing
// instance, in which case a randomly named database is created there.
func startTestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		adminURL = startDisposableContainer(t)
	}

	name := "leetkob_test_" + randomHex(t, 8)
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	admin.Close()

	testURL := replaceDatabase(adminURL, name)
	pool, err := New(ctx, testURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, err := pgxpool.New(context.Background(), adminURL)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return pool.pool
}

func startDisposableContainer(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("no TEST_DATABASE_URL and docker is unavailable")
	}
	name := "leetkob-pg-" + randomHex(t, 6)
	run := exec.Command("docker", "run", "-d", "--rm",
		"--name", name,
		"-e", "POSTGRES_PASSWORD="+testPassword,
		"-p", "127.0.0.1::5432",
		postgresImage(),
	)
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("start postgres container: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})

	portOut, err := exec.Command("docker", "port", name, "5432/tcp").CombinedOutput()
	if err != nil {
		t.Fatalf("read container port: %v\n%s", err, portOut)
	}
	// Output form: 127.0.0.1:49153
	address := strings.TrimSpace(string(portOut))
	if idx := strings.LastIndex(address, ":"); idx >= 0 {
		address = address[idx+1:]
	}
	url := fmt.Sprintf("postgres://postgres:%s@127.0.0.1:%s/postgres?sslmode=disable", testPassword, address)

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		pool, err := pgxpool.New(context.Background(), url)
		if err == nil {
			if err = pool.Ping(context.Background()); err == nil {
				pool.Close()
				return url
			}
			pool.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("postgres container %s did not become ready", name)
	return ""
}

func replaceDatabase(url string, database string) string {
	question := strings.Index(url, "?")
	base := url
	query := ""
	if question >= 0 {
		base = url[:question]
		query = url[question:]
	}
	slash := strings.LastIndex(base, "/")
	if slash < 0 {
		return url
	}
	return base[:slash+1] + database + query
}

func randomHex(t *testing.T, bytes int) string {
	t.Helper()
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatalf("random: %v", err)
	}
	return hex.EncodeToString(buffer)
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := startTestDatabase(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func testLab(id string, classID string, creator string) LabRecord {
	return LabRecord{
		ID:          id,
		ClassID:     classID,
		Title:       "Sorting lab",
		Statement:   "Implement merge sort.",
		OpensAt:     time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC),
		Deadline:    time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
		Revision:    1,
		CPUSeconds:  1,
		MemoryMiB:   256,
		ModelNotice: "The semantic constraint is reviewed by a model; a TA approves any deduction.",
		CreatedBy:   creator,
		Constraints: []ConstraintRecord{
			{ID: "C1", Text: "Do not call `std::sort`.", DeductionCents: 300},
			{ID: "C2", Text: "Implement merge sort.", DeductionCents: 400},
		},
		Tests: []TestRecord{
			{ID: "01", Input: "3\n3 1 2\n", ExpectedOutput: "1 2 3\n", Sample: true},
			{ID: "02", Input: "1\n5\n", ExpectedOutput: "5\n", Sample: false},
		},
	}
}

const (
	classID   = "11111111-1111-4111-8111-111111111111"
	taID      = "22222222-2222-4222-8222-222222222222"
	studentID = "33333333-3333-4333-8333-333333333333"
)

func seedIdentity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`INSERT INTO classes (id, name) VALUES ($1, 'Class A')`,
		`INSERT INTO users (id, email, display_name, role, password_hash) VALUES ($1, 'ta@example.test', 'TA One', 'ta', 'argon2id$placeholder')`,
		`INSERT INTO users (id, email, display_name, role, password_hash) VALUES ($1, 'student@example.test', 'Student One', 'student', 'argon2id$placeholder')`,
		`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2)`,
	}
	if _, err := pool.Exec(ctx, statements[0], classID); err != nil {
		t.Fatalf("seed class: %v", err)
	}
	if _, err := pool.Exec(ctx, statements[1], taID); err != nil {
		t.Fatalf("seed ta: %v", err)
	}
	if _, err := pool.Exec(ctx, statements[2], studentID); err != nil {
		t.Fatalf("seed student: %v", err)
	}
	if _, err := pool.Exec(ctx, statements[3], classID, taID); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
}

func TestMigrateIsCleanIdempotentAndAdditive(t *testing.T) {
	pool := startTestDatabase(t)
	ctx := context.Background()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	versions, err := AppliedVersions(ctx, pool)
	if err != nil {
		t.Fatalf("applied versions: %v", err)
	}
	if len(versions) != 1 || versions[0] != 1 {
		t.Fatalf("versions after first migrate = %v, want [1]", versions)
	}

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate must be a no-op: %v", err)
	}
	versions, err = AppliedVersions(ctx, pool)
	if err != nil {
		t.Fatalf("applied versions after second migrate: %v", err)
	}
	if len(versions) != 1 || versions[0] != 1 {
		t.Fatalf("versions after second migrate = %v, want [1]", versions)
	}

	for _, table := range []string{
		"schema_migrations", "users", "sessions", "classes", "memberships",
		"labs", "lab_constraints", "lab_tests", "submissions", "idempotency_keys",
		"jobs", "findings", "decisions", "budget_reservations",
	} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table,
		).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %s missing after migration", table)
		}
	}
}

func TestCreateLabPersistsAtomicallyAndReadsBack(t *testing.T) {
	pool := migratedPool(t)
	seedIdentity(t, pool)

	lab := testLab("44444444-4444-4444-8444-444444444444", classID, taID)
	if err := (&Store{pool: pool}).CreateLab(context.Background(), lab); err != nil {
		t.Fatalf("create lab: %v", err)
	}

	read, err := (&Store{pool: pool}).GetLab(context.Background(), lab.ID)
	if err != nil {
		t.Fatalf("read lab: %v", err)
	}
	if read.Title != lab.Title || read.Revision != 1 {
		t.Fatalf("lab header mismatch: %+v", read)
	}
	if len(read.Constraints) != 2 || read.Constraints[0].ID != "C1" || read.Constraints[0].DeductionCents != 300 {
		t.Fatalf("constraints mismatch: %+v", read.Constraints)
	}
	if len(read.Tests) != 2 || read.Tests[0].ID != "01" || !read.Tests[0].Sample || read.Tests[1].Sample {
		t.Fatalf("tests mismatch: %+v", read.Tests)
	}
}

func TestCreateLabRollsBackEveryRowOnInterruptedWrite(t *testing.T) {
	pool := migratedPool(t)
	seedIdentity(t, pool)
	ctx := context.Background()

	// A duplicate test id violates the primary key partway through the
	// tests insert, after the lab and constraint rows were written.
	lab := testLab("55555555-5555-4555-8555-555555555555", classID, taID)
	lab.Tests = append(lab.Tests, TestRecord{ID: "01", Input: "x", ExpectedOutput: "y"})
	if err := (&Store{pool: pool}).CreateLab(ctx, lab); err == nil {
		t.Fatal("expected the duplicate test insert to fail")
	}

	if _, err := (&Store{pool: pool}).GetLab(ctx, lab.ID); !errors.Is(err, ErrLabNotFound) {
		t.Fatalf("lab must not exist after rollback, got %v", err)
	}
	var constraintCount, testCount int
	if err := pool.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM lab_constraints WHERE lab_id = $1),
		        (SELECT count(*) FROM lab_tests WHERE lab_id = $1)`, lab.ID,
	).Scan(&constraintCount, &testCount); err != nil {
		t.Fatalf("count orphan rows: %v", err)
	}
	if constraintCount != 0 || testCount != 0 {
		t.Fatalf("orphan rows after rollback: constraints=%d tests=%d", constraintCount, testCount)
	}

	// The schema also refuses an invalid schedule (deadline not after
	// opensAt) as a whole-transaction failure.
	invalid := testLab("66666666-6666-4666-8666-666666666666", classID, taID)
	invalid.OpensAt = invalid.Deadline
	if err := (&Store{pool: pool}).CreateLab(ctx, invalid); err == nil {
		t.Fatal("expected the schedule check to fail")
	}
	if _, err := (&Store{pool: pool}).GetLab(ctx, invalid.ID); !errors.Is(err, ErrLabNotFound) {
		t.Fatalf("invalid lab must not exist, got %v", err)
	}
}

func TestLabSurvivesPoolRestart(t *testing.T) {
	pool := migratedPool(t)
	seedIdentity(t, pool)
	ctx := context.Background()

	lab := testLab("77777777-7777-4777-8777-777777777777", classID, taID)
	if err := (&Store{pool: pool}).CreateLab(ctx, lab); err != nil {
		t.Fatalf("create lab: %v", err)
	}

	// Close the pool: a new connection sees only durable state.
	pool.Close()
	reopened, err := New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatalf("reopen pool: %v", err)
	}
	defer reopened.Close()
	if err := Migrate(ctx, reopened.pool); err != nil {
		t.Fatalf("idempotent migrate after restart: %v", err)
	}

	read, err := reopened.GetLab(ctx, lab.ID)
	if err != nil {
		t.Fatalf("read after restart: %v", err)
	}
	if read.ID != lab.ID || len(read.Tests) != 2 || len(read.Constraints) != 2 {
		t.Fatalf("restarted read mismatch: %+v", read)
	}
}
