package worker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/identity"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

// The integration tests use a disposable pinned PostgreSQL container (or
// TEST_DATABASE_URL) exactly like the store suite. The stop/submit/restart
// proof additionally requires the real sandbox image; without Docker or the
// image it skips with an explicit reason rather than pretending to judge.
const workerPostgresImage = "postgres@sha256:eb63372c1e05c755f55fa30cbd07aecc443676fc6c0fa90b9002d01537841e1a"

const (
	workerClassID       = "11111111-aaaa-4111-8111-111111111111"
	workerClassBID      = "11111111-bbbb-4111-8111-111111111111"
	workerTAID          = "22222222-aaaa-4222-8222-222222222222"
	workerTATwoID       = "22222222-bbbb-4222-8222-222222222222"
	workerStudentID     = "33333333-aaaa-4333-8333-333333333333"
	workerStudentTwoID  = "33333333-bbbb-4333-8333-333333333333"
	workerLabID         = "44444444-aaaa-4444-8444-444444444444"
	workerSource        = "#include <iostream>\nint main(){long long a=0,b=0;std::cin>>a>>b;std::cout<<a+b<<\"\\n\";}\n"
	workerTestPassword  = "worker-test-password"
	workerPollInterval  = 20 * time.Millisecond
	workerJudgeDeadline = 3 * time.Minute
)

func workerRandomHex(t *testing.T, bytes int) string {
	t.Helper()
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatalf("random: %v", err)
	}
	return hex.EncodeToString(buffer)
}

func workerReplaceDatabase(url, database string) string {
	question := strings.Index(url, "?")
	base, query := url, ""
	if question >= 0 {
		base, query = url[:question], url[question:]
	}
	slash := strings.LastIndex(base, "/")
	if slash < 0 {
		return url
	}
	return base[:slash+1] + database + query
}

func startWorkerPostgresContainer(t *testing.T) string {
	t.Helper()
	name := "leetkob-worker-pg-" + workerRandomHex(t, 6)
	run := exec.Command("docker", "run", "-d", "--rm",
		"--name", name,
		"-e", "POSTGRES_PASSWORD="+workerTestPassword,
		"-p", "127.0.0.1::5432",
		workerPostgresImage,
	)
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("start postgres container: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })

	portOut, err := exec.Command("docker", "port", name, "5432/tcp").CombinedOutput()
	if err != nil {
		t.Fatalf("read container port: %v\n%s", err, portOut)
	}
	address := strings.TrimSpace(string(portOut))
	if index := strings.LastIndex(address, ":"); index >= 0 {
		address = address[index+1:]
	}
	url := fmt.Sprintf("postgres://postgres:%s@127.0.0.1:%s/postgres?sslmode=disable", workerTestPassword, address)

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

// startWorkerTestDatabase provisions one disposable migrated database. It
// skips only when neither TEST_DATABASE_URL nor Docker is available.
func startWorkerTestDatabase(t *testing.T) *postgres.Store {
	t.Helper()
	ctx := context.Background()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skip("no TEST_DATABASE_URL and docker is unavailable")
		}
		if err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").Run(); err != nil {
			t.Skipf("docker daemon unavailable: %v", err)
		}
		adminURL = startWorkerPostgresContainer(t)
	}
	name := "leetkob_worker_" + workerRandomHex(t, 8)
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	admin.Close()

	store, err := postgres.New(ctx, workerReplaceDatabase(adminURL, name))
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
		cleanup, err := pgxpool.New(context.Background(), adminURL)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	if err := postgres.Migrate(ctx, store.Pool()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

func seedWorkerLab(t *testing.T, store *postgres.Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	hash, err := identity.HashPassword(workerTestPassword)
	if err != nil {
		t.Fatalf("hash test password: %v", err)
	}
	statements := []string{
		`INSERT INTO classes (id, name) VALUES ($1, 'Worker Class A')`,
		`INSERT INTO classes (id, name) VALUES ($1, 'Worker Class B')`,
		`INSERT INTO users (id, email, display_name, role, password_hash) VALUES ($1, 'worker-ta@example.test', 'Worker TA', 'ta', $2)`,
		`INSERT INTO users (id, email, display_name, role, password_hash) VALUES ($1, 'worker-ta2@example.test', 'Other Worker TA', 'ta', $2)`,
		`INSERT INTO users (id, email, display_name, role, password_hash) VALUES ($1, 'worker-student@example.test', 'Worker Student', 'student', $2)`,
		`INSERT INTO users (id, email, display_name, role, password_hash) VALUES ($1, 'worker-student2@example.test', 'Other Worker Student', 'student', $2)`,
		`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2)`,
	}
	if _, err := store.Pool().Exec(ctx, statements[0], workerClassID); err != nil {
		t.Fatalf("seed class A: %v", err)
	}
	if _, err := store.Pool().Exec(ctx, statements[1], workerClassBID); err != nil {
		t.Fatalf("seed class B: %v", err)
	}
	for _, user := range []struct {
		id    string
		query string
	}{
		{workerTAID, statements[2]},
		{workerTATwoID, statements[3]},
		{workerStudentID, statements[4]},
		{workerStudentTwoID, statements[5]},
	} {
		if _, err := store.Pool().Exec(ctx, user.query, user.id, hash); err != nil {
			t.Fatalf("seed user %s: %v", user.id, err)
		}
	}
	for _, membership := range []struct{ classID, userID string }{
		{workerClassID, workerTAID},
		{workerClassBID, workerTATwoID},
		{workerClassID, workerStudentID},
		{workerClassID, workerStudentTwoID},
	} {
		if _, err := store.Pool().Exec(ctx, statements[6], membership.classID, membership.userID); err != nil {
			t.Fatalf("seed membership: %v", err)
		}
	}
	lab := postgres.LabRecord{
		ID:          workerLabID,
		ClassID:     workerClassID,
		Title:       "Sum lab",
		Statement:   "Read two integers and print their sum.",
		OpensAt:     now.Add(-time.Hour),
		Deadline:    now.Add(time.Hour),
		Revision:    1,
		CPUSeconds:  1,
		MemoryMiB:   256,
		ModelNotice: "Worker integration lab.",
		CreatedBy:   workerTAID,
		Constraints: []postgres.ConstraintRecord{
			{ID: "C1", Text: "Do not call `std::sort`.", DeductionCents: 300},
		},
		Tests: []postgres.TestRecord{
			{ID: "01", Input: "1 2\n", ExpectedOutput: "3\n", Sample: true},
			{ID: "02", Input: "10 20\n", ExpectedOutput: "30\n", Sample: false},
		},
	}
	if err := store.CreateLab(ctx, lab); err != nil {
		t.Fatalf("seed lab: %v", err)
	}
}

func acceptWorkerSubmission(t *testing.T, store *postgres.Store, key, source string) (submissionID, jobID string) {
	t.Helper()
	digest := sha256.Sum256([]byte(source))
	result, err := store.AcceptSubmission(context.Background(), postgres.AcceptCommand{
		ActorID:        workerStudentID,
		LabID:          workerLabID,
		Source:         source,
		IdempotencyKey: key,
		Fingerprint:    hex.EncodeToString(digest[:]),
		ReceivedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("accept submission: %v", err)
	}
	if err := store.Pool().QueryRow(context.Background(),
		`SELECT id FROM jobs WHERE submission_id = $1`, result.SubmissionID,
	).Scan(&jobID); err != nil {
		t.Fatalf("read job: %v", err)
	}
	return result.SubmissionID, jobID
}

func workerSandboxImage(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("real sandbox image requires docker: %v", err)
	}
	if err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").Run(); err != nil {
		t.Skipf("docker daemon unavailable, cannot run the real sandbox: %v", err)
	}
	image := os.Getenv("TEST_SANDBOX_IMAGE")
	if image == "" {
		image = sandbox.DefaultImage
	}
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("sandbox image %q is not present; build apps/backend/sandbox or set TEST_SANDBOX_IMAGE to run this proof", image)
	}
	return image
}

func newSandboxJudge(t *testing.T, image string) *sandbox.Controller {
	t.Helper()
	cfg := sandbox.DefaultConfig()
	cfg.Image = image
	cfg.WorkRoot = t.TempDir()
	controller, err := sandbox.New(cfg)
	if err != nil {
		t.Fatalf("sandbox controller: %v", err)
	}
	return controller
}

func startDurableWorker(t *testing.T, queue Queue, inputs Inputs, judge Judge, id string, leaseFor time.Duration) (context.CancelFunc, <-chan error) {
	t.Helper()
	instance, err := New(Options{
		Queue: queue, Inputs: inputs, Judge: judge, Logger: quietLogger(),
		WorkerID: id, PollInterval: workerPollInterval, LeaseFor: leaseFor,
		MaxFailures: 2, ShutdownTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- instance.Run(ctx) }()
	t.Cleanup(cancel)
	return cancel, done
}

func stopDurableWorker(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker run: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("worker did not stop")
	}
}

func waitForJudged(t *testing.T, store *postgres.Store, submissionID string) postgres.SubmissionView {
	t.Helper()
	deadline := time.Now().Add(workerJudgeDeadline)
	for time.Now().Before(deadline) {
		view, err := store.GetSubmissionForRead(context.Background(), submissionID)
		if err == nil && view.State == "judged" {
			return view
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("submission %s did not reach judged", submissionID)
	return postgres.SubmissionView{}
}

func countRows(t *testing.T, store *postgres.Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := store.Pool().QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

// PA3-JUDGE-03: the worker is stopped, a submission is accepted, and a
// restarted worker judges it. Exactly one verdict set is stored, and a
// duplicate delivery of that result is refused by the fence.
func TestStopSubmitRestartStoresExactlyOneVerdictSet(t *testing.T) {
	image := workerSandboxImage(t)
	store := startWorkerTestDatabase(t)
	seedWorkerLab(t, store)
	controller := newSandboxJudge(t, image)

	// No worker is running: acceptance must still commit a durable job.
	submissionID, jobID := acceptWorkerSubmission(t, store, "worker-stop-key-1", workerSource)
	queued, err := store.GetSubmissionForRead(context.Background(), submissionID)
	if err != nil {
		t.Fatalf("read queued submission: %v", err)
	}
	if queued.State != "queued" || queued.Verdict != nil {
		t.Fatalf("submission before restart = %+v, want queued without a verdict", queued)
	}
	if state := countRows(t, store, `SELECT count(*) FROM jobs WHERE id = $1 AND state = 'pending'`, jobID); state != 1 {
		t.Fatal("acceptance did not leave a durable pending job")
	}

	cancel, done := startDurableWorker(t, store, store, controller, "worker-restart-1", 2*time.Minute)
	view := waitForJudged(t, store, submissionID)
	stopDurableWorker(t, cancel, done)

	if view.Verdict == nil || *view.Verdict != "AC" {
		t.Fatalf("judged verdict = %v, want AC", view.Verdict)
	}
	if len(view.Tests) != 2 {
		t.Fatalf("judged tests = %+v, want two", view.Tests)
	}
	if got := countRows(t, store, `SELECT count(*) FROM submission_test_results WHERE submission_id = $1`, submissionID); got != 2 {
		t.Fatalf("stored test results = %d, want exactly 2", got)
	}
	if got := countRows(t, store, `SELECT count(*) FROM jobs WHERE submission_id = $1 AND state = 'done'`, submissionID); got != 1 {
		t.Fatalf("done jobs = %d, want exactly 1", got)
	}

	// Duplicate delivery of the same attempt cannot store a second set.
	var attempt int
	if err := store.Pool().QueryRow(context.Background(),
		`SELECT attempt FROM jobs WHERE id = $1`, jobID).Scan(&attempt); err != nil {
		t.Fatalf("read attempt: %v", err)
	}
	duplicate := postgres.JudgeOutcome{Verdict: "WA", Tests: []postgres.TestOutcome{{TestID: "01", Verdict: "WA"}}}
	if err := store.CommitJudgeResult(context.Background(), jobID, attempt,
		duplicate, time.Now().UTC()); !errors.Is(err, postgres.ErrStaleFence) {
		t.Fatalf("duplicate delivery err = %v, want ErrStaleFence", err)
	}
	if got := countRows(t, store, `SELECT count(*) FROM submission_test_results WHERE submission_id = $1`, submissionID); got != 2 {
		t.Fatalf("duplicate delivery changed the stored set: %d rows", got)
	}

	// Restarting the worker finds no work and cannot duplicate effects.
	restartCancel, restartDone := startDurableWorker(t, store, store, controller, "worker-restart-2", 2*time.Minute)
	time.Sleep(500 * time.Millisecond)
	stopDurableWorker(t, restartCancel, restartDone)

	if got := countRows(t, store, `SELECT count(*) FROM submission_test_results WHERE submission_id = $1`, submissionID); got != 2 {
		t.Fatalf("restart changed the stored test set: %d rows", got)
	}
	if got := countRows(t, store, `SELECT count(*) FROM jobs WHERE submission_id = $1`, submissionID); got != 1 {
		t.Fatalf("restart created another job: %d", got)
	}
	var attemptAfter int
	if err := store.Pool().QueryRow(context.Background(),
		`SELECT attempt FROM jobs WHERE id = $1`, jobID).Scan(&attemptAfter); err != nil {
		t.Fatalf("read attempt after restart: %v", err)
	}
	if attemptAfter != attempt {
		t.Fatalf("restart re-leased a finished job: attempt %d -> %d", attempt, attemptAfter)
	}
}

type commitRecord struct {
	attempt int
	err     error
}

// commitRecorder observes the fence decision without changing store
// behavior. It is how the late-attempt proof knows the old worker tried to
// commit.
type commitRecorder struct {
	Queue
	mu      sync.Mutex
	records []commitRecord
}

func (c *commitRecorder) CommitJudgeResult(ctx context.Context, jobID string, attempt int, outcome postgres.JudgeOutcome, now time.Time) error {
	err := c.Queue.CommitJudgeResult(ctx, jobID, attempt, outcome, now)
	c.mu.Lock()
	c.records = append(c.records, commitRecord{attempt: attempt, err: err})
	c.mu.Unlock()
	return err
}

func (c *commitRecorder) snapshot() []commitRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]commitRecord(nil), c.records...)
}

type blockingJudge struct {
	started chan struct{}
	release chan struct{}
	result  sandbox.JudgeResult
}

func (b *blockingJudge) Judge(ctx context.Context, _ sandbox.JudgeRequest) (sandbox.JudgeResult, error) {
	close(b.started)
	select {
	case <-b.release:
		return b.result, nil
	case <-ctx.Done():
		return sandbox.JudgeResult{}, ctx.Err()
	}
}

// PA3-JUDGE-04: a worker loses its lease, a replacement attempt takes over,
// and the old attempt's result cannot replace or duplicate the committed
// current result.
func TestLateAttemptAfterLeaseExpiryCannotCommit(t *testing.T) {
	store := startWorkerTestDatabase(t)
	seedWorkerLab(t, store)
	submissionID, jobID := acceptWorkerSubmission(t, store, "worker-late-key-1", workerSource)

	queue := &commitRecorder{Queue: store}
	judge := &blockingJudge{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result: sandbox.JudgeResult{
			Verdict: sandbox.VerdictRan,
			Compile: sandbox.CompileResult{Succeeded: true},
			Tests: []sandbox.TestResult{
				{ID: "01", Verdict: sandbox.VerdictRan, Tokens: []string{"3"}},
				{ID: "02", Verdict: sandbox.VerdictRan, Tokens: []string{"30"}},
			},
		},
	}
	cancel, done := startDurableWorker(t, queue, store, judge, "worker-old", 400*time.Millisecond)
	defer cancel()

	select {
	case <-judge.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the old worker did not start judging")
	}
	// Let the old lease expire, then let a replacement claim attempt 2. The
	// poll tolerates scheduler/container delay while still proving natural
	// lease expiry: only an expired or pending row can be re-leased.
	var replacement []postgres.LeasedJob
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		candidates, leaseErr := store.LeaseJobs(context.Background(), "worker-new", 1,
			time.Now().UTC(), time.Minute)
		if leaseErr != nil {
			t.Fatalf("replacement lease: %v", leaseErr)
		}
		if len(candidates) == 1 {
			replacement = candidates
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(replacement) != 1 || replacement[0].JobID != jobID || replacement[0].Attempt != 2 {
		var state string
		var attempt int
		var expires *time.Time
		_ = store.Pool().QueryRow(context.Background(),
			`SELECT state, attempt, lease_expires_at FROM jobs WHERE id = $1`, jobID).
			Scan(&state, &attempt, &expires)
		t.Fatalf("replacement lease = %+v, want the same job at attempt 2 (state=%s attempt=%d expires=%v)",
			replacement, state, attempt, expires)
	}

	// The old attempt now returns a successful-looking result.
	close(judge.release)
	deadline = time.Now().Add(5 * time.Second)
	for {
		records := queue.snapshot()
		if len(records) > 0 {
			if records[0].attempt != 1 {
				t.Fatalf("old worker committed with attempt %d, want 1", records[0].attempt)
			}
			if !errors.Is(records[0].err, postgres.ErrStaleFence) {
				t.Fatalf("old attempt commit err = %v, want ErrStaleFence", records[0].err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("old worker never attempted to commit")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := countRows(t, store, `SELECT count(*) FROM submission_test_results WHERE submission_id = $1`, submissionID); got != 0 {
		t.Fatalf("late attempt stored %d result rows", got)
	}

	if err := store.CommitJudgeResult(context.Background(), jobID, 2, postgres.JudgeOutcome{
		Verdict: "AC",
		Tests: []postgres.TestOutcome{
			{TestID: "01", Verdict: "AC", TimeMs: 2},
			{TestID: "02", Verdict: "AC", TimeMs: 3},
		},
	}, time.Now().UTC()); err != nil {
		t.Fatalf("replacement commit: %v", err)
	}
	if got := countRows(t, store, `SELECT count(*) FROM submission_test_results WHERE submission_id = $1`, submissionID); got != 2 {
		t.Fatalf("replacement stored %d result rows, want 2", got)
	}
	view, err := store.GetSubmissionForRead(context.Background(), submissionID)
	if err != nil {
		t.Fatalf("read replacement result: %v", err)
	}
	if view.Verdict == nil || *view.Verdict != "AC" || len(view.Tests) != 2 {
		t.Fatalf("replacement view = %+v, want AC with two tests", view)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("old worker did not stop")
	}
}

// Two workers may race for one queued submission; exclusive leases and the
// fence must still leave exactly one stored verdict set.
func TestTwoWorkersJudgeOneSubmissionExactlyOnce(t *testing.T) {
	image := workerSandboxImage(t)
	store := startWorkerTestDatabase(t)
	seedWorkerLab(t, store)
	controller := newSandboxJudge(t, image)
	submissionID, _ := acceptWorkerSubmission(t, store, "worker-race-key-1", workerSource)

	cancelOne, doneOne := startDurableWorker(t, store, store, controller, "worker-race-1", 2*time.Minute)
	cancelTwo, doneTwo := startDurableWorker(t, store, store, controller, "worker-race-2", 2*time.Minute)
	view := waitForJudged(t, store, submissionID)
	stopDurableWorker(t, cancelOne, doneOne)
	stopDurableWorker(t, cancelTwo, doneTwo)

	if view.Verdict == nil || *view.Verdict != "AC" || len(view.Tests) != 2 {
		t.Fatalf("judged view = %+v, want AC with two tests", view)
	}
	if got := countRows(t, store, `SELECT count(*) FROM submission_test_results WHERE submission_id = $1`, submissionID); got != 2 {
		t.Fatalf("stored test results = %d, want exactly 2", got)
	}
	if got := countRows(t, store, `SELECT count(*) FROM jobs WHERE submission_id = $1`, submissionID); got != 1 {
		t.Fatalf("jobs = %d, want exactly 1", got)
	}
	var attempt int
	if err := store.Pool().QueryRow(context.Background(),
		`SELECT attempt FROM jobs WHERE submission_id = $1`, submissionID).Scan(&attempt); err != nil {
		t.Fatalf("read attempt: %v", err)
	}
	if attempt != 1 {
		t.Fatalf("one queued job was leased %d times, want 1", attempt)
	}
}
