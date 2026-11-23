package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

func seedAcceptanceLab(t *testing.T) (labID string, store *Store) {
	t.Helper()
	pool := migratedPool(t)
	seedIdentity(t, pool)
	// The shared identity seed enrolls only the TA; submissions require
	// the student to be a current member of the lab's class.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		classID, studentID); err != nil {
		t.Fatalf("enroll student: %v", err)
	}
	lab := testLab("abcdabcd-abcd-4bcd-8bcd-abcdabcdabcd", classID, taID)
	if err := (&Store{pool: pool}).CreateLab(context.Background(), lab); err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	return lab.ID, &Store{pool: pool}
}

func acceptCommand(labID, key, source string, receivedAt time.Time) AcceptCommand {
	digest := sha256.Sum256([]byte(source))
	return AcceptCommand{
		ActorID:        studentID,
		LabID:          labID,
		Source:         source,
		IdempotencyKey: key,
		Fingerprint:    hex.EncodeToString(digest[:]),
		ReceivedAt:     receivedAt,
	}
}

func TestAcceptSubmissionCommitsEverythingAtomically(t *testing.T) {
	labID, store := seedAcceptanceLab(t)
	ctx := context.Background()
	receivedAt := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	result, err := store.AcceptSubmission(ctx, acceptCommand(labID, "accept-key-0001", "int main(){}", receivedAt))
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if result.State != "queued" || result.SubmissionID == "" || result.Replayed {
		t.Fatalf("result = %+v", result)
	}

	var counted bool
	var generation int
	var state string
	if err := store.pool.QueryRow(ctx, `
		SELECT counted, generation, state FROM submissions WHERE id = $1`, result.SubmissionID,
	).Scan(&counted, &generation, &state); err != nil {
		t.Fatalf("read submission: %v", err)
	}
	if !counted || generation != 1 || state != "queued" {
		t.Fatalf("submission counted=%v generation=%d state=%s", counted, generation, state)
	}

	var jobRevision int
	var jobState string
	if err := store.pool.QueryRow(ctx,
		`SELECT test_revision, state FROM jobs WHERE submission_id = $1`, result.SubmissionID,
	).Scan(&jobRevision, &jobState); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if jobRevision != 1 || jobState != "pending" {
		t.Fatalf("job revision=%d state=%s", jobRevision, jobState)
	}

	var replayCount int
	if err := store.pool.QueryRow(ctx, `
		SELECT count(*) FROM idempotency_keys
		WHERE actor_id = $1 AND operation = 'create-submission' AND resource_id = $2
		  AND idempotency_key = 'accept-key-0001'`, studentID, labID,
	).Scan(&replayCount); err != nil {
		t.Fatalf("read replay: %v", err)
	}
	if replayCount != 1 {
		t.Fatalf("replay rows = %d", replayCount)
	}
}

func TestAcceptReplaySurvivesDeadlineAndContentChangeConflicts(t *testing.T) {
	labID, store := seedAcceptanceLab(t)
	ctx := context.Background()
	inside := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	afterDeadline := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

	first, err := store.AcceptSubmission(ctx, acceptCommand(labID, "accept-key-0002", "int main(){}", inside))
	if err != nil {
		t.Fatalf("first accept: %v", err)
	}

	// Same key and content after the deadline replays the stored result.
	replay, err := store.AcceptSubmission(ctx, acceptCommand(labID, "accept-key-0002", "int main(){}", afterDeadline))
	if err != nil {
		t.Fatalf("post-deadline replay: %v", err)
	}
	if !replay.Replayed || replay.SubmissionID != first.SubmissionID {
		t.Fatalf("replay = %+v, want stored %s", replay, first.SubmissionID)
	}

	// Different content under the same key is a conflict, never a replay.
	_, err = store.AcceptSubmission(ctx, acceptCommand(labID, "accept-key-0002", "int main(){return 1;}", afterDeadline))
	if !errors.Is(err, ErrIdempotencyContentConflict) {
		t.Fatalf("changed content err = %v, want ErrIdempotencyContentConflict", err)
	}

	var submissions int
	if err := store.pool.QueryRow(ctx,
		`SELECT count(*) FROM submissions WHERE lab_id = $1 AND student_id = $2`, labID, studentID,
	).Scan(&submissions); err != nil {
		t.Fatalf("count submissions: %v", err)
	}
	if submissions != 1 {
		t.Fatalf("submissions = %d, want only the accepted one", submissions)
	}
}

func TestAcceptRefusesOutsideTheWindow(t *testing.T) {
	labID, store := seedAcceptanceLab(t)
	ctx := context.Background()
	deadline := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)

	// Exact deadline equality is rejected: receivedAt < deadline.
	_, err := store.AcceptSubmission(ctx, acceptCommand(labID, "accept-key-0003", "int main(){}", deadline))
	var window *SubmissionWindowClosedError
	if !errors.As(err, &window) {
		t.Fatalf("deadline-equality err = %v, want SubmissionWindowClosedError", err)
	}
	if !window.Deadline.Equal(deadline) {
		t.Fatalf("window deadline = %s, want %s", window.Deadline, deadline)
	}

	// Before the lab opens is refused the same way.
	beforeOpen := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if _, err := store.AcceptSubmission(ctx, acceptCommand(labID, "accept-key-0004", "int main(){}", beforeOpen)); !errors.As(err, &window) {
		t.Fatalf("before-open err = %v, want SubmissionWindowClosedError", err)
	}

	var count int
	if err := store.pool.QueryRow(ctx,
		`SELECT count(*) FROM submissions WHERE lab_id = $1`, labID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("refused requests created %d submissions", count)
	}
}

func TestLaterSubmissionReplacesCountedSelection(t *testing.T) {
	labID, store := seedAcceptanceLab(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	first, err := store.AcceptSubmission(ctx, acceptCommand(labID, "accept-key-0005", "int main(){}", now))
	if err != nil {
		t.Fatalf("first accept: %v", err)
	}
	second, err := store.AcceptSubmission(ctx, acceptCommand(labID, "accept-key-0006", "int main(){return 1;}", now.Add(time.Minute)))
	if err != nil {
		t.Fatalf("second accept: %v", err)
	}

	var counted int
	var generation int
	if err := store.pool.QueryRow(ctx, `
		SELECT count(*), COALESCE(max(generation), 0) FROM submissions
		WHERE lab_id = $1 AND student_id = $2 AND counted`, labID, studentID,
	).Scan(&counted, &generation); err != nil {
		t.Fatalf("read counted: %v", err)
	}
	if counted != 1 || generation != 2 {
		t.Fatalf("counted = %d generation = %d, want exactly one at generation 2", counted, generation)
	}
	var firstCounted bool
	if err := store.pool.QueryRow(ctx,
		`SELECT counted FROM submissions WHERE id = $1`, first.SubmissionID,
	).Scan(&firstCounted); err != nil {
		t.Fatalf("read first: %v", err)
	}
	if firstCounted {
		t.Fatal("the superseded submission must not stay counted")
	}
	var jobGeneration int
	if err := store.pool.QueryRow(ctx,
		`SELECT generation FROM jobs WHERE submission_id = $1`, second.SubmissionID,
	).Scan(&jobGeneration); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if jobGeneration != 2 {
		t.Fatalf("job generation = %d, want 2", jobGeneration)
	}
}

func TestAcceptRejectsNonMembersWithConcealedReason(t *testing.T) {
	labID, store := seedAcceptanceLab(t)
	ctx := context.Background()
	command := acceptCommand(labID, "accept-key-0007", "int main(){}", time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC))
	command.ActorID = "99999999-9999-4999-8999-999999999999" // valid uuid, not a member

	if _, err := store.AcceptSubmission(ctx, command); !errors.Is(err, ErrLabNotAccepting) {
		t.Fatalf("non-member err = %v, want ErrLabNotAccepting", err)
	}
}

func TestAcceptRollsBackEveryRowOnInjectedFailure(t *testing.T) {
	labID, store := seedAcceptanceLab(t)
	ctx := context.Background()

	if _, err := store.pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION reject_jobs() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'injected job failure'; END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_jobs_trigger BEFORE INSERT ON jobs
		FOR EACH ROW EXECUTE FUNCTION reject_jobs();`); err != nil {
		t.Fatalf("install trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS reject_jobs_trigger ON jobs;
			DROP FUNCTION IF EXISTS reject_jobs();`)
	})

	_, err := store.AcceptSubmission(ctx, acceptCommand(labID, "accept-key-0008", "int main(){}", time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)))
	if err == nil {
		t.Fatal("expected the injected job failure to abort acceptance")
	}

	var submissions, replays, jobs int
	if err := store.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM submissions WHERE lab_id = $1::uuid),
		       (SELECT count(*) FROM idempotency_keys WHERE resource_id = $1::text AND operation = 'create-submission'),
		       (SELECT count(*) FROM jobs WHERE lab_id = $1::uuid)`, labID,
	).Scan(&submissions, &replays, &jobs); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if submissions != 0 || replays != 0 || jobs != 0 {
		t.Fatalf("partial rows after rollback: submissions=%d replays=%d jobs=%d", submissions, replays, jobs)
	}
}
