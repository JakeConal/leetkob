package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func seedQueueJob(t *testing.T, pool *pgxpool.Pool) (jobID string, submissionID string) {
	t.Helper()
	ctx := context.Background()
	seedIdentity(t, pool)
	lab := testLab("88888888-8888-4888-8888-888888888888", classID, taID)
	if err := (&Store{pool: pool}).CreateLab(ctx, lab); err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	submissionID = "99999999-1111-4999-8999-111111111111"
	jobID = "99999999-2222-4999-8999-222222222222"
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO submissions (id, lab_id, student_id, source, source_sha256, state,
			received_at, receipt_seq, generation, counted)
		VALUES ($1, $2, $3, 'int main(){}', '\x00', 'queued', $4, 1, 1, true)`,
		submissionID, lab.ID, studentID, now); err != nil {
		t.Fatalf("seed submission: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (id, kind, submission_id, lab_id, generation, test_revision,
			constraint_revision, source_sha256, available_at)
		VALUES ($1, 'judge', $2, $3, 1, 1, 1, '\x00', $4)`,
		jobID, submissionID, lab.ID, now.Add(-time.Hour)); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return jobID, submissionID
}

func outcome() JudgeOutcome {
	return JudgeOutcome{
		Verdict: "AC",
		Tests: []TestOutcome{
			{TestID: "01", Verdict: "AC", TimeMs: 12},
			{TestID: "02", Verdict: "AC", TimeMs: 7},
		},
	}
}

func TestLeaseIsExclusiveAndCommitStoresOneVerdictSet(t *testing.T) {
	pool := migratedPool(t)
	jobID, submissionID := seedQueueJob(t, pool)
	store := &Store{pool: pool}
	ctx := context.Background()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

	leased, err := store.LeaseJobs(ctx, "worker-1", 10, now, time.Minute)
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if len(leased) != 1 || leased[0].JobID != jobID || leased[0].Attempt != 1 {
		t.Fatalf("lease = %+v, want one job attempt 1", leased)
	}

	again, err := store.LeaseJobs(ctx, "worker-2", 10, now, time.Minute)
	if err != nil {
		t.Fatalf("second lease: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("an active lease must not be handed to another worker: %+v", again)
	}

	if err := store.CommitJudgeResult(ctx, jobID, 1, outcome(), now.Add(time.Second)); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var state, verdict string
	if err := pool.QueryRow(ctx,
		`SELECT state, verdict FROM submissions WHERE id = $1`, submissionID,
	).Scan(&state, &verdict); err != nil {
		t.Fatalf("read submission: %v", err)
	}
	if state != "judged" || verdict != "AC" {
		t.Fatalf("submission = %s/%s, want judged/AC", state, verdict)
	}
	var testCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM submission_test_results WHERE submission_id = $1`, submissionID,
	).Scan(&testCount); err != nil {
		t.Fatalf("count results: %v", err)
	}
	if testCount != 2 {
		t.Fatalf("stored test results = %d, want 2", testCount)
	}

	// A duplicate delivery cannot store a second verdict set.
	if err := store.CommitJudgeResult(ctx, jobID, 1, JudgeOutcome{Verdict: "WA"}, now.Add(2*time.Second)); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("duplicate commit err = %v, want ErrStaleFence", err)
	}
	if err := pool.QueryRow(ctx, `SELECT verdict FROM submissions WHERE id = $1`, submissionID).Scan(&verdict); err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	if verdict != "AC" {
		t.Fatalf("duplicate delivery overwrote the verdict: %s", verdict)
	}
}

func TestExpiredLeaseReissuesAndOLDAttemptCannotCommit(t *testing.T) {
	pool := migratedPool(t)
	jobID, _ := seedQueueJob(t, pool)
	store := &Store{pool: pool}
	ctx := context.Background()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	lease := 30 * time.Second

	if _, err := store.LeaseJobs(ctx, "worker-1", 10, now, lease); err != nil {
		t.Fatalf("first lease: %v", err)
	}

	// The old worker is too slow: its lease expires and another worker
	// picks the same job up with a higher fence.
	reissued, err := store.LeaseJobs(ctx, "worker-2", 10, now.Add(lease+time.Second), lease)
	if err != nil {
		t.Fatalf("re-lease: %v", err)
	}
	if len(reissued) != 1 || reissued[0].Attempt != 2 {
		t.Fatalf("re-lease = %+v, want attempt 2", reissued)
	}

	if err := store.CommitJudgeResult(ctx, jobID, 1, outcome(), now.Add(lease+2*time.Second)); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("old attempt commit err = %v, want ErrStaleFence", err)
	}
	if err := store.CommitJudgeResult(ctx, jobID, 2, outcome(), now.Add(lease+3*time.Second)); err != nil {
		t.Fatalf("current attempt commit: %v", err)
	}
}

func TestLeasesSurviveRestartAndRetriesAreBounded(t *testing.T) {
	pool := migratedPool(t)
	jobID, _ := seedQueueJob(t, pool)
	ctx := context.Background()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	lease := 30 * time.Second
	store := &Store{pool: pool}

	leased, err := store.LeaseJobs(ctx, "worker-1", 10, now, lease)
	if err != nil || len(leased) != 1 {
		t.Fatalf("lease: %v %+v", err, leased)
	}

	// Worker restart: a fresh pool still sees the persisted lease.
	pool.Close()
	reopened, err := New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if err := Migrate(ctx, reopened.pool); err != nil {
		t.Fatalf("migrate after restart: %v", err)
	}

	// First infrastructure failure returns the job to the pool.
	if err := reopened.FailJob(ctx, jobID, leased[0].Attempt, now.Add(time.Second), 2); err != nil {
		t.Fatalf("first failure: %v", err)
	}

	// Second lease increments the fence; the second failure exhausts the
	// retry ceiling and becomes a visible failed job, never a verdict.
	reissued, err := reopened.LeaseJobs(ctx, "worker-2", 10, now.Add(2*time.Second), lease)
	if err != nil || len(reissued) != 1 || reissued[0].Attempt != 2 {
		t.Fatalf("second lease = %+v err=%v", reissued, err)
	}
	if err := reopened.FailJob(ctx, jobID, reissued[0].Attempt, now.Add(3*time.Second), 2); !errors.Is(err, ErrJobExhausted) {
		t.Fatalf("exhaustion err = %v, want ErrJobExhausted", err)
	}
	remaining, err := reopened.LeaseJobs(ctx, "worker-3", 10, now.Add(time.Minute), lease)
	if err != nil {
		t.Fatalf("lease after exhaustion: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("exhausted job must not be leased again: %+v", remaining)
	}
	var state string
	if err := reopened.pool.QueryRow(ctx, `SELECT state FROM jobs WHERE id = $1`, jobID).Scan(&state); err != nil {
		t.Fatalf("read job state: %v", err)
	}
	if state != "failed" {
		t.Fatalf("job state = %q, want failed", state)
	}
}
