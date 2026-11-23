package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JakeConal/leetkob/backend/internal/review"
)

// Identifiers used only by the review integration fixtures.
const (
	reviewClassA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	reviewClassB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	reviewTAA1   = "a1a1a1a1-a1a1-4a1a-8a1a-a1a1a1a1a1a1"
	reviewTAA2   = "a2a2a2a2-a2a2-4a2a-8a2a-a2a2a2a2a2a2"
	reviewTAB    = "b1b1b1b1-b1b1-4b1b-8b1b-b1b1b1b1b1b1"
	reviewStu    = "c1c1c1c1-c1c1-4c1c-8c1c-c1c1c1c1c1c1"
	reviewStu2   = "c2c2c2c2-c2c2-4c2c-8c2c-c2c2c2c2c2c2"
	reviewLabA   = "1a1a1a1a-1a1a-4a1a-8a1a-1a1a1a1a1a1a"
	reviewLabB   = "1b1b1b1b-1b1b-4b1b-8b1b-1b1b1b1b1b1b"

	reviewSource = "int main() {\n    stable_sort(a, a + n);\n    return 0;\n}\n"
)

type reviewWorld struct {
	store *Store
	pool  *pgxpool.Pool
}

// newReviewWorld seeds two classes, three TAs, one student and one lab per
// class. No findings or submissions are created.
func newReviewWorld(t *testing.T) reviewWorld {
	t.Helper()
	pool := migratedPool(t)
	ctx := context.Background()

	seed := []struct {
		statement string
		args      []any
	}{
		{`INSERT INTO classes (id, name) VALUES ($1, 'Review Class A')`, []any{reviewClassA}},
		{`INSERT INTO classes (id, name) VALUES ($1, 'Review Class B')`, []any{reviewClassB}},
		{`INSERT INTO users (id, email, display_name, role, password_hash)
		  VALUES ($1, 'ta-a1@example.test', 'TA A1', 'ta', 'argon2id$placeholder')`, []any{reviewTAA1}},
		{`INSERT INTO users (id, email, display_name, role, password_hash)
		  VALUES ($1, 'ta-a2@example.test', 'TA A2', 'ta', 'argon2id$placeholder')`, []any{reviewTAA2}},
		{`INSERT INTO users (id, email, display_name, role, password_hash)
		  VALUES ($1, 'ta-b@example.test', 'TA B', 'ta', 'argon2id$placeholder')`, []any{reviewTAB}},
		{`INSERT INTO users (id, email, display_name, role, password_hash)
		  VALUES ($1, 'student-r@example.test', 'Student R', 'student', 'argon2id$placeholder')`, []any{reviewStu}},
		{`INSERT INTO users (id, email, display_name, role, password_hash)
		  VALUES ($1, 'student-r2@example.test', 'Student R2', 'student', 'argon2id$placeholder')`, []any{reviewStu2}},
		{`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2)`, []any{reviewClassA, reviewTAA1}},
		{`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2)`, []any{reviewClassA, reviewTAA2}},
		{`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2)`, []any{reviewClassA, reviewStu}},
		{`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2)`, []any{reviewClassA, reviewStu2}},
		{`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2)`, []any{reviewClassB, reviewTAB}},
	}
	for _, item := range seed {
		if _, err := pool.Exec(ctx, item.statement, item.args...); err != nil {
			t.Fatalf("seed review identity: %v", err)
		}
	}

	for _, lab := range []struct {
		id, classID, creator string
	}{
		{reviewLabA, reviewClassA, reviewTAA1},
		{reviewLabB, reviewClassB, reviewTAB},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO labs (id, class_id, title, statement, opens_at, deadline,
				revision, cpu_seconds, memory_mib, model_notice, created_by)
			VALUES ($1, $2, 'Review lab', 'Implement merge sort.',
				'2026-11-22T00:00:00Z', '2026-11-25T15:00:00Z',
				1, 1, 256, 'A TA reviews every finding.', $3)`,
			lab.id, lab.classID, lab.creator,
		); err != nil {
			t.Fatalf("seed review lab: %v", err)
		}
	}
	return reviewWorld{store: &Store{pool: pool}, pool: pool}
}

var reviewReceipt int

// seedReviewSubmission inserts one submission; superseded work is stored with
// counted=false exactly as the acceptance transaction leaves it.
func seedReviewSubmission(t *testing.T, pool *pgxpool.Pool, id, labID, studentID string, counted bool, state, source string) {
	t.Helper()
	digest := sha256.Sum256([]byte(source))
	reviewReceipt++
	var verdict, judgedAt any
	if state == "judged" {
		verdict = "AC"
		judgedAt = time.Date(2026, 11, 23, 13, 0, 0, 0, time.UTC)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO submissions (id, lab_id, student_id, source, source_sha256, state,
			received_at, receipt_seq, generation, counted, verdict, judged_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, nextval('submission_receipt_seq'), 1, $8, $9, $10)`,
		id, labID, studentID, source, digest[:], state,
		time.Date(2026, 11, 23, 12, reviewReceipt, 0, 0, time.UTC), counted, verdict, judgedAt,
	); err != nil {
		t.Fatalf("seed review submission: %v", err)
	}
}

// seedReviewFinding inserts one finding with required_review=false so the
// queue-selection rules, not the fixture, must derive the obligation.
func seedReviewFinding(t *testing.T, pool *pgxpool.Pool, id, submissionID, labID, constraintID, sourceKind, verdict string, unsureCause *string, evidenceJSON string, deductionCents int) {
	t.Helper()
	if evidenceJSON == "" {
		evidenceJSON = "[]"
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO findings (id, submission_id, lab_id, constraint_id, constraint_revision,
			source_kind, checker_verdict, unsure_cause, evidence, deduction_cents, required_review)
		VALUES ($1, $2, $3, $4, 1, $5, $6, $7, $8::jsonb, $9, false)`,
		id, submissionID, labID, constraintID, sourceKind, verdict, unsureCause, evidenceJSON, deductionCents,
	); err != nil {
		t.Fatalf("seed review finding: %v", err)
	}
}

func countSampled(t *testing.T, pool *pgxpool.Pool, labID string) int {
	t.Helper()
	var sampled int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM findings f
		JOIN submissions s ON s.id = f.submission_id
		WHERE f.lab_id = $1 AND s.counted AND f.checker_verdict = 'pass' AND f.sampled`,
		labID,
	).Scan(&sampled); err != nil {
		t.Fatalf("count sampled: %v", err)
	}
	return sampled
}

func sampledIDs(items []review.Finding) []string {
	ids := []string{}
	for _, finding := range items {
		if finding.Sampled {
			ids = append(ids, finding.ID)
		}
	}
	return ids
}

func equalIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := map[string]int{}
	for _, id := range left {
		seen[id]++
	}
	for _, id := range right {
		seen[id]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}

// PA3-REVIEW-02: the required pass sample has exactly ceil(10%) membership
// for 0, 1, 10 and 11 pass pairs, and refreshing an unchanged queue returns
// the same persisted membership.
func TestReviewQueueSampleSizeAndRefreshStability(t *testing.T) {
	world := newReviewWorld(t)
	ctx := context.Background()
	seedReviewSubmission(t, world.pool, "50000000-0000-4000-8000-000000000001", reviewLabA, reviewStu, true, "judged", reviewSource)

	readSample := func() (int, []string) {
		t.Helper()
		queue, err := world.store.ListReviewQueue(ctx, ReviewQueueQuery{
			LabID: reviewLabA, ActorID: reviewTAA1, Limit: 200,
		})
		if err != nil {
			t.Fatalf("list queue: %v", err)
		}
		return len(sampledIDs(queue.Items)), sampledIDs(queue.Items)
	}

	steps := []struct {
		passPairs int
		want      int
	}{
		{passPairs: 0, want: 0},
		{passPairs: 1, want: 1},
		{passPairs: 10, want: 1},
		{passPairs: 11, want: 2},
	}
	inserted := 0
	for _, step := range steps {
		for inserted < step.passPairs {
			inserted++
			seedReviewFinding(t, world.pool,
				fmt.Sprintf("60000000-0000-4000-8000-%012d", inserted),
				"50000000-0000-4000-8000-000000000001", reviewLabA,
				fmt.Sprintf("C%02d", inserted), "model", review.VerdictPass, nil, "", 100)
		}
		got, _ := readSample()
		if got != step.want {
			t.Fatalf("%d pass pairs sampled %d, want %d", step.passPairs, got, step.want)
		}
		if persisted := countSampled(t, world.pool, reviewLabA); persisted != step.want {
			t.Fatalf("%d pass pairs persisted %d sampled, want %d", step.passPairs, persisted, step.want)
		}
		t.Logf("pass pairs=%d sampled=%d", step.passPairs, got)
	}

	_, first := readSample()
	_, second := readSample()
	if !equalIDs(first, second) {
		t.Fatalf("unchanged refresh redrew the sample: %v -> %v", first, second)
	}
	if len(first) != 2 {
		t.Fatalf("11 pass pairs sample = %v, want 2", first)
	}
}

// PA3-REVIEW-01/02: obligations are every violation/unsure plus the sampled
// pass pair; superseded submissions and their findings are excluded.
func TestReviewQueueObligationsAndSupersededExclusion(t *testing.T) {
	world := newReviewWorld(t)
	ctx := context.Background()
	seedReviewSubmission(t, world.pool, "51000000-0000-4000-8000-000000000001", reviewLabA, reviewStu, true, "judged", reviewSource)
	seedReviewSubmission(t, world.pool, "51000000-0000-4000-8000-000000000002", reviewLabA, reviewStu, false, "judged", "int main() { return 0; }")

	seedReviewFinding(t, world.pool, "61000000-0000-4000-8000-000000000001",
		"51000000-0000-4000-8000-000000000001", reviewLabA, "C1", "hard-rule", review.VerdictViolation,
		nil, `[{"line":2,"text":"    stable_sort(a, a + n);"}]`, 300)
	timeout := "timeout"
	seedReviewFinding(t, world.pool, "61000000-0000-4000-8000-000000000002",
		"51000000-0000-4000-8000-000000000001", reviewLabA, "C2", "model", review.VerdictUnsure,
		&timeout, "[]", 400)
	seedReviewFinding(t, world.pool, "61000000-0000-4000-8000-000000000003",
		"51000000-0000-4000-8000-000000000001", reviewLabA, "C3", "model", review.VerdictPass, nil, "", 100)
	seedReviewFinding(t, world.pool, "61000000-0000-4000-8000-000000000004",
		"51000000-0000-4000-8000-000000000001", reviewLabA, "C4", "model", review.VerdictPass, nil, "", 100)
	// Superseded submission: a violation that must never become an obligation.
	seedReviewFinding(t, world.pool, "61000000-0000-4000-8000-000000000005",
		"51000000-0000-4000-8000-000000000002", reviewLabA, "C1", "model", review.VerdictViolation,
		nil, `[{"line":1,"text":"int main() { return 0; }"}]`, 300)

	queue, err := world.store.ListReviewQueue(ctx, ReviewQueueQuery{
		LabID: reviewLabA, ActorID: reviewTAA1, Limit: 200,
	})
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(queue.Items) != 4 {
		t.Fatalf("queue items = %d, want 4 current findings", len(queue.Items))
	}
	required := 0
	for _, item := range queue.Items {
		if item.SubmissionID == "51000000-0000-4000-8000-000000000002" {
			t.Fatalf("superseded finding %s leaked into the queue", item.ID)
		}
		if item.RequiredReview() {
			required++
		}
	}
	// Two violations/unsure plus exactly one of the two pass pairs.
	if required != 3 || len(sampledIDs(queue.Items)) != 1 {
		t.Fatalf("required=%d sampled=%v, want 3 obligations and one sampled pass",
			required, sampledIDs(queue.Items))
	}
	if queue.ReviewPending != 3 {
		t.Fatalf("reviewPending = %d, want 3", queue.ReviewPending)
	}
	if queue.ProcessingPending {
		t.Fatal("judged current submission must not report processingPending")
	}

	// A queued superseded submission still does not block on processing; a
	// queued *counted* submission does.
	seedReviewSubmission(t, world.pool, "51000000-0000-4000-8000-000000000003", reviewLabA, reviewStu2, true, "queued", "int main() { return 0; }")
	queue, err = world.store.ListReviewQueue(ctx, ReviewQueueQuery{
		LabID: reviewLabA, ActorID: reviewTAA1, Limit: 200,
	})
	if err != nil {
		t.Fatalf("list queue with queued counted submission: %v", err)
	}
	if !queue.ProcessingPending {
		t.Fatal("queued counted submission must report processingPending")
	}

	// One explicit decision resolves one obligation; the checker provenance
	// and the number of findings stay unchanged.
	if _, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: "61000000-0000-4000-8000-000000000001",
		ActorID: reviewTAA1, ExpectedRevision: 1, Verdict: review.VerdictPass,
		Reason: "The call is inside a comment.", DecidedAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("record decision: %v", err)
	}
	queue, err = world.store.ListReviewQueue(ctx, ReviewQueueQuery{
		LabID: reviewLabA, ActorID: reviewTAA1, Limit: 200,
	})
	if err != nil {
		t.Fatalf("list queue after decision: %v", err)
	}
	if queue.ReviewPending != 2 {
		t.Fatalf("reviewPending after decision = %d, want 2", queue.ReviewPending)
	}
}

// PA3-REVIEW-01: another class's TA receives the concealed not-found error
// for both the queue and an individual decision.
func TestReviewQueueConcealsOtherClasses(t *testing.T) {
	world := newReviewWorld(t)
	ctx := context.Background()
	seedReviewSubmission(t, world.pool, "52000000-0000-4000-8000-000000000001", reviewLabA, reviewStu, true, "judged", reviewSource)
	seedReviewFinding(t, world.pool, "62000000-0000-4000-8000-000000000001",
		"52000000-0000-4000-8000-000000000001", reviewLabA, "C1", "model", review.VerdictViolation,
		nil, "[]", 300)

	if _, err := world.store.ListReviewQueue(ctx, ReviewQueueQuery{
		LabID: reviewLabA, ActorID: reviewTAA1, Limit: 50,
	}); err != nil {
		t.Fatalf("assigned TA queue: %v", err)
	}
	if _, err := world.store.ListReviewQueue(ctx, ReviewQueueQuery{
		LabID: reviewLabA, ActorID: reviewTAB, Limit: 50,
	}); !errors.Is(err, ErrReviewNotFound) {
		t.Fatalf("other-class TA queue err = %v, want ErrReviewNotFound", err)
	}
	if _, err := world.store.ListReviewQueue(ctx, ReviewQueueQuery{
		LabID: "1c1c1c1c-1c1c-4c1c-8c1c-1c1c1c1c1c1c", ActorID: reviewTAA1, Limit: 50,
	}); !errors.Is(err, ErrReviewNotFound) {
		t.Fatalf("unknown lab err = %v, want ErrReviewNotFound", err)
	}
	if _, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: "62000000-0000-4000-8000-000000000001",
		ActorID: reviewTAB, ExpectedRevision: 1, Verdict: review.VerdictPass,
		Reason: "not my class", DecidedAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	}); !errors.Is(err, ErrReviewNotFound) {
		t.Fatalf("other-class decision err = %v, want ErrReviewNotFound", err)
	}
}

// PA3-REVIEW-04: two TAs decide from the same previous revision; only one
// commits and the other receives the stale revision instead of overwriting.
func TestReviewDecisionTwoTAsStaleConflict(t *testing.T) {
	world := newReviewWorld(t)
	ctx := context.Background()
	seedReviewSubmission(t, world.pool, "53000000-0000-4000-8000-000000000001", reviewLabA, reviewStu, true, "judged", reviewSource)
	seedReviewFinding(t, world.pool, "63000000-0000-4000-8000-000000000001",
		"53000000-0000-4000-8000-000000000001", reviewLabA, "C1", "model", review.VerdictUnsure,
		ptr("timeout"), "[]", 300)

	seenByTA1, err := world.store.ListReviewQueue(ctx, ReviewQueueQuery{LabID: reviewLabA, ActorID: reviewTAA1, Limit: 50})
	if err != nil {
		t.Fatalf("TA1 read: %v", err)
	}
	seenByTA2, err := world.store.ListReviewQueue(ctx, ReviewQueueQuery{LabID: reviewLabA, ActorID: reviewTAA2, Limit: 50})
	if err != nil {
		t.Fatalf("TA2 read: %v", err)
	}
	revision := seenByTA1.Items[0].Revision
	if revision != seenByTA2.Items[0].Revision {
		t.Fatalf("TA revisions differ before deciding: %d vs %d", revision, seenByTA2.Items[0].Revision)
	}

	decidedAt := func(minute int) time.Time { return time.Date(2026, 11, 24, 9, minute, 0, 0, time.UTC) }
	first, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: "63000000-0000-4000-8000-000000000001",
		ActorID: reviewTAA1, ExpectedRevision: revision, Verdict: review.VerdictPass,
		Reason: "The timeout is infrastructure, not a violation.", DecidedAt: decidedAt(1),
	})
	if err != nil {
		t.Fatalf("first decision: %v", err)
	}
	if first.Revision != revision+1 || first.Decision == nil || first.Decision.DecidedBy != reviewTAA1 {
		t.Fatalf("first decision result = %+v", first)
	}

	_, err = world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: "63000000-0000-4000-8000-000000000001",
		ActorID: reviewTAA2, ExpectedRevision: revision, Verdict: review.VerdictViolation,
		Reason:    "I believe this is a violation.",
		Evidence:  &review.Evidence{Line: 2, Text: "    stable_sort(a, a + n);"},
		DecidedAt: decidedAt(2),
	})
	var stale *StaleRevisionError
	if !errors.As(err, &stale) {
		t.Fatalf("second decision err = %v, want StaleRevisionError", err)
	}
	if stale.Expected != revision || stale.Current != revision+1 {
		t.Fatalf("stale = %+v, want expected %d current %d", stale, revision, revision+1)
	}

	var decisions, newestRevision int
	if err := world.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM decisions WHERE finding_id = $1),
		       (SELECT revision FROM findings WHERE id = $1)`,
		"63000000-0000-4000-8000-000000000001",
	).Scan(&decisions, &newestRevision); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if decisions != 1 || newestRevision != revision+1 {
		t.Fatalf("after conflict: decisions=%d revision=%d", decisions, newestRevision)
	}
	// The conflicting request must not have advanced the finding a second time.
	if _, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: "63000000-0000-4000-8000-000000000001",
		ActorID: reviewTAA2, ExpectedRevision: revision, Verdict: review.VerdictPass,
		Reason: "still stale", DecidedAt: decidedAt(3),
	}); !errors.As(err, &stale) {
		t.Fatalf("repeat stale err = %v", err)
	}
}

// PA3-REVIEW-03/04: decisions append audit rows and preserve checker
// provenance; evidence must match the original source line.
func TestReviewDecisionAuditAndOriginalEvidence(t *testing.T) {
	world := newReviewWorld(t)
	ctx := context.Background()
	seedReviewSubmission(t, world.pool, "54000000-0000-4000-8000-000000000001", reviewLabA, reviewStu, true, "judged", reviewSource)
	originalEvidence := `[{"line":2,"text":"    stable_sort(a, a + n);"}]`
	seedReviewFinding(t, world.pool, "64000000-0000-4000-8000-000000000001",
		"54000000-0000-4000-8000-000000000001", reviewLabA, "C1", "model", review.VerdictViolation,
		nil, originalEvidence, 300)
	findingID := "64000000-0000-4000-8000-000000000001"

	// Wrong line text, wrong line and missing evidence for a violation are
	// rejected without writing a decision or advancing the revision.
	badEvidence := []*review.Evidence{
		{Line: 2, Text: "stable_sort(a, a + n);"},
		{Line: 2, Text: "    stable_sort(a, a + n); "},
		{Line: 9, Text: "stable_sort(a, a + n);"},
	}
	for _, evidence := range badEvidence {
		_, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
			LabID: reviewLabA, FindingID: findingID, ActorID: reviewTAA1,
			ExpectedRevision: 1, Verdict: review.VerdictViolation,
			Reason: "banned call", Evidence: evidence,
			DecidedAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
		})
		var invalid *review.ValidationError
		if !errors.As(err, &invalid) {
			t.Fatalf("evidence %+v err = %v, want ValidationError", evidence, err)
		}
	}
	if _, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: findingID, ActorID: reviewTAA1,
		ExpectedRevision: 1, Verdict: review.VerdictViolation,
		Reason: "banned call", DecidedAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	}); err == nil {
		t.Fatal("violation without evidence was accepted")
	}
	var decisions, revision int
	if err := world.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM decisions WHERE finding_id = $1),
		       (SELECT revision FROM findings WHERE id = $1)`, findingID,
	).Scan(&decisions, &revision); err != nil {
		t.Fatalf("read after invalid evidence: %v", err)
	}
	if decisions != 0 || revision != 1 {
		t.Fatalf("invalid evidence wrote decisions=%d revision=%d", decisions, revision)
	}

	// A matching original line commits revision 2 with the TA's audit actor.
	first, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: findingID, ActorID: reviewTAA1,
		ExpectedRevision: 1, Verdict: review.VerdictViolation,
		Reason: "Calls stable_sort inside the merge step.",
		Evidence: &review.Evidence{
			Line: 2, Text: "    stable_sort(a, a + n);",
		},
		DecidedAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("matching decision: %v", err)
	}
	if first.Revision != 2 || first.Decision == nil || first.Decision.Evidence == nil ||
		first.Decision.Evidence.Line != 2 || first.Decision.Revision != 2 {
		t.Fatalf("decision result = %+v", first)
	}
	if first.Decision.Reason != "Calls stable_sort inside the merge step." ||
		first.Decision.DecidedBy != reviewTAA1 {
		t.Fatalf("audit actor/reason = %+v", first.Decision)
	}

	// A superseding explicit pass decision appends a second row; it never
	// updates the first and never changes the checker evidence/deduction.
	second, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: findingID, ActorID: reviewTAA2,
		ExpectedRevision: 2, Verdict: review.VerdictPass,
		Reason:    "The TA config disables this constraint for this lab.",
		DecidedAt: time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("second decision: %v", err)
	}
	if second.Revision != 3 || second.Decision == nil ||
		second.Decision.Verdict != review.VerdictPass || second.Decision.DecidedBy != reviewTAA2 ||
		second.Decision.Evidence != nil {
		t.Fatalf("second decision = %+v", second)
	}

	var rows int
	if err := world.pool.QueryRow(ctx, `SELECT count(*) FROM decisions WHERE finding_id = $1`, findingID).Scan(&rows); err != nil {
		t.Fatalf("count decisions: %v", err)
	}
	if rows != 2 {
		t.Fatalf("audit rows = %d, want 2 append-only rows", rows)
	}
	var verdicts []string
	rowsCursor, err := world.pool.Query(ctx, `
		SELECT verdict FROM decisions WHERE finding_id = $1 ORDER BY finding_revision`, findingID)
	if err != nil {
		t.Fatalf("history query: %v", err)
	}
	for rowsCursor.Next() {
		var verdict string
		if err := rowsCursor.Scan(&verdict); err != nil {
			t.Fatalf("scan history: %v", err)
		}
		verdicts = append(verdicts, verdict)
	}
	rowsCursor.Close()
	if len(verdicts) != 2 || verdicts[0] != review.VerdictViolation || verdicts[1] != review.VerdictPass {
		t.Fatalf("history = %v", verdicts)
	}

	var checkerVerdict, sourceKind string
	var storedEvidenceJSON []byte
	var deduction int
	if err := world.pool.QueryRow(ctx, `
		SELECT checker_verdict, source_kind, evidence, deduction_cents
		FROM findings WHERE id = $1`, findingID,
	).Scan(&checkerVerdict, &sourceKind, &storedEvidenceJSON, &deduction); err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	if checkerVerdict != review.VerdictViolation || sourceKind != "model" || deduction != 300 {
		t.Fatalf("checker provenance changed: verdict=%s source=%s deduction=%d",
			checkerVerdict, sourceKind, deduction)
	}
	var storedEvidence []review.Evidence
	if err := json.Unmarshal(storedEvidenceJSON, &storedEvidence); err != nil {
		t.Fatalf("decode provenance evidence: %v", err)
	}
	if len(storedEvidence) != 1 || storedEvidence[0].Line != 2 ||
		storedEvidence[0].Text != "    stable_sort(a, a + n);" {
		t.Fatalf("checker evidence changed: %+v", storedEvidence)
	}

	// The queue surfaces the latest decision and no remaining obligation.
	queue, err := world.store.ListReviewQueue(ctx, ReviewQueueQuery{LabID: reviewLabA, ActorID: reviewTAA1, Limit: 50})
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(queue.Items) != 1 || queue.Items[0].Decision == nil ||
		queue.Items[0].Decision.Verdict != review.VerdictPass ||
		queue.Items[0].DeductionCents != 300 {
		t.Fatalf("queue after decisions = %+v", queue.Items)
	}
	if queue.ReviewPending != 0 {
		t.Fatalf("reviewPending = %d after a decision, want 0", queue.ReviewPending)
	}
}

// A pass decision does not require evidence and an unsure finding has no
// default verdict: the store only records what the TA explicitly supplied.
func TestReviewDecisionPassNeedsNoEvidenceAndUnsureHasNoDefault(t *testing.T) {
	world := newReviewWorld(t)
	ctx := context.Background()
	seedReviewSubmission(t, world.pool, "55000000-0000-4000-8000-000000000001", reviewLabA, reviewStu, true, "judged", reviewSource)
	seedReviewFinding(t, world.pool, "65000000-0000-4000-8000-000000000001",
		"55000000-0000-4000-8000-000000000001", reviewLabA, "C1", "model", review.VerdictPass, nil, "", 250)
	seedReviewFinding(t, world.pool, "65000000-0000-4000-8000-000000000002",
		"55000000-0000-4000-8000-000000000001", reviewLabA, "C2", "model", review.VerdictUnsure,
		ptr("timeout"), "[]", 400)

	queue, err := world.store.ListReviewQueue(ctx, ReviewQueueQuery{LabID: reviewLabA, ActorID: reviewTAA1, Limit: 50})
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	var unsure *review.Finding
	for i := range queue.Items {
		if queue.Items[i].CheckerVerdict == review.VerdictUnsure {
			unsure = &queue.Items[i]
		}
	}
	if unsure == nil || unsure.Decision != nil || !unsure.RequiredReview() {
		t.Fatalf("unsure finding has a default decision: %+v", unsure)
	}
	if unsure.UnsureCause == nil || *unsure.UnsureCause != "timeout" {
		t.Fatalf("unsure cause = %v", unsure.UnsureCause)
	}

	if _, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: unsure.ID, ActorID: reviewTAA1,
		ExpectedRevision: 1, Verdict: review.VerdictPass, Reason: "model timed out; source looks compliant",
		DecidedAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("explicit pass on unsure: %v", err)
	}

	// A pass verdict never needs evidence, and the configured deduction is
	// only ever the TA-configured value, not a model default.
	if _, err := world.store.RecordReviewDecision(ctx, DecisionCommand{
		LabID: reviewLabA, FindingID: "65000000-0000-4000-8000-000000000001", ActorID: reviewTAA1,
		ExpectedRevision: 1, Verdict: review.VerdictPass, Reason: "compliant",
		DecidedAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("pass without evidence: %v", err)
	}
	var deduction int
	if err := world.pool.QueryRow(ctx, `SELECT deduction_cents FROM findings WHERE id = $1`,
		"65000000-0000-4000-8000-000000000001").Scan(&deduction); err != nil {
		t.Fatalf("read deduction: %v", err)
	}
	if deduction != 250 {
		t.Fatalf("deduction = %d, want the TA-configured 250", deduction)
	}
}

func ptr(value string) *string { return &value }
