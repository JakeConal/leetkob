package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/checker/hardrule"
	"github.com/JakeConal/leetkob/backend/internal/checking"
	"github.com/JakeConal/leetkob/backend/internal/review"
	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

const reviewSource = "int main() { // cohort\n  std::sort(a, a + n);\n}\n"

func TestPostgresCheckAndReview(t *testing.T) {
	store, ctx := openSchema(t)
	fx := seedReviewLab(t, ctx, store)
	model := &pgModel{body: `{"results":[{"constraintId":"prints","verdict":"violation","evidence":[{"line":1,"text":"int main() { "}]}]}`, in: 10, out: 5}
	analyzerCalls := 0
	raw, err := json.Marshal(hardrule.Result{Version: 1, Status: "ok", References: []hardrule.Ref{{
		Kind: "call", Name: "sort", QualifiedName: "std::sort", Header: "algorithm", Resolved: true, Line: 2, Column: 3,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	svc := &checking.Service{
		Store: &FindingRepo{Store: store},
		Analyze: func(context.Context, string) ([]byte, error) {
			analyzerCalls++
			return raw, nil
		},
		Checker: &semantic.Checker{Model: model, Store: &Budget{Store: store}, Timeout: time.Second},
		Price: semantic.Price{
			Known: true, MaxInputTokens: 100, MaxOutputTokens: 20,
			InputMicroUSDPerToken: 1, OutputMicroUSDPerToken: 1,
		},
	}

	wa, err := svc.Run(ctx, fx.waSubmission)
	if err != nil {
		t.Fatal(err)
	}
	if wa.Persisted || wa.ModelCalls != 0 || model.calls.Load() != 0 || analyzerCalls != 0 {
		t.Fatalf("non-AC outcome %+v model %d analyzer %d", wa, model.calls.Load(), analyzerCalls)
	}
	if countRows(t, ctx, store, `SELECT count(*) FROM findings WHERE submission_id = $1`, fx.waSubmission) != 0 {
		t.Fatal("non-AC submission stored findings")
	}

	ac, err := svc.Run(ctx, fx.acSubmission)
	if err != nil {
		t.Fatal(err)
	}
	if !ac.Persisted || ac.ModelCalls != 1 || model.calls.Load() != 1 || analyzerCalls != 1 {
		t.Fatalf("AC outcome %+v model %d analyzer %d", ac, model.calls.Load(), analyzerCalls)
	}
	if strings.Contains(model.body, "Do not use") {
		t.Fatal("script includes the ban text")
	}
	assertFinding(t, ctx, store, fx.acSubmission, "ban-sort", "hard-rule", "violation", 300, 2, "  std::sort(a, a + n);")
	assertFinding(t, ctx, store, fx.acSubmission, "prints", "semantic", "violation", 150, 1, "int main() { // cohort")
	charged, err := (&Budget{Store: store}).Charged(ctx, fx.labID)
	if err != nil || charged != 15 || charged > semantic.LabCapMicroUSD {
		t.Fatalf("charged = %d err %v", charged, err)
	}

	repo := &ReviewRepo{Store: store}
	student, err := repo.Access(ctx, fx.labID, fx.student)
	if err != nil || review.Allowed(student) {
		t.Fatalf("student access %+v err %v", student, err)
	}
	other, err := repo.Access(ctx, fx.labID, fx.otherTA)
	if err != nil || review.Allowed(other) {
		t.Fatalf("other TA access %+v err %v", other, err)
	}
	removed, err := repo.Access(ctx, fx.labID, fx.removedTA)
	if err != nil || review.Allowed(removed) || !removed.UserExists {
		t.Fatalf("removed TA access %+v err %v", removed, err)
	}
	assigned, err := repo.Access(ctx, fx.labID, fx.ta)
	if err != nil || !review.Allowed(assigned) {
		t.Fatalf("assigned access %+v err %v", assigned, err)
	}

	page, err := repo.List(ctx, fx.labID, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if !page.ProcessingPending || page.ReviewPending != 2 || len(page.Items) != 2 {
		t.Fatalf("queue = %+v", page)
	}
	again, err := repo.List(ctx, fx.labID, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Items) != 2 || again.Items[0].RequiredReview != page.Items[0].RequiredReview || again.Items[1].RequiredReview != page.Items[1].RequiredReview {
		t.Fatal("queue refresh changed the required sample")
	}
	var sawHard, sawSemantic bool
	for _, item := range page.Items {
		if item.Source != reviewSource || item.Deduction == 0 || item.Decision != nil {
			t.Fatalf("item = %+v", item)
		}
		switch item.ConstraintID {
		case "ban-sort":
			sawHard = item.SourceKind == "hard-rule" && item.CheckerVerdict == "violation" && item.Evidence[0].Text == "  std::sort(a, a + n);" && item.Deduction == 300
		case "prints":
			sawSemantic = item.SourceKind == "semantic" && item.CheckerVerdict == "violation" && item.Evidence[0].Text == "int main() { // cohort" && item.Deduction == 150
		}
	}
	if !sawHard || !sawSemantic {
		t.Fatalf("queue missing checker fields: %+v", page.Items)
	}

	var findingID string
	if err := store.pool.QueryRow(ctx, `SELECT id::text FROM findings WHERE submission_id = $1 AND constraint_id = 'prints'`, fx.acSubmission).Scan(&findingID); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 24, 1, 2, 3, 0, time.UTC)
	decided, err := repo.Decide(ctx, review.DecisionRequest{
		LabID: fx.labID, FindingID: findingID, ActorID: fx.ta, ExpectedRevision: 1,
		Verdict: "pass", Reason: "The printed line is acceptable.",
		IdempotencyKey: "decide-prints-1", Fingerprint: "fingerprint-prints-1", Now: when,
	})
	if err != nil || decided.Stale != nil || decided.Finding.CheckerVerdict != "violation" || decided.Finding.Revision != 2 || decided.Finding.SourceKind != "semantic" {
		t.Fatalf("decision %+v err %v", decided, err)
	}
	staleDecision, err := repo.Decide(ctx, review.DecisionRequest{
		LabID: fx.labID, FindingID: findingID, ActorID: fx.ta, ExpectedRevision: 1,
		Verdict: "pass", Reason: "The printed line is acceptable.",
		IdempotencyKey: "decide-prints-2", Fingerprint: "fingerprint-prints-2", Now: when,
	})
	if err != nil || staleDecision.Stale == nil || staleDecision.Stale.Expected != 1 || staleDecision.Stale.Current != 2 {
		t.Fatalf("stale %+v err %v", staleDecision, err)
	}
	if _, err := repo.Decide(ctx, review.DecisionRequest{
		LabID: fx.labID, FindingID: findingID, ActorID: fx.student, ExpectedRevision: 2,
		Verdict: "pass", Reason: "I disagree with the checker.",
		IdempotencyKey: "decide-student-1", Fingerprint: "fingerprint-student", Now: when,
	}); !errors.Is(err, review.ErrNotAssigned) {
		t.Fatalf("student decision err = %v", err)
	}
	assertFinding(t, ctx, store, fx.acSubmission, "prints", "semantic", "violation", 150, 1, "int main() { // cohort")
	var revision int
	if err := store.pool.QueryRow(ctx, `SELECT revision FROM findings WHERE id = $1`, findingID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || countRows(t, ctx, store, `SELECT count(*) FROM decisions WHERE finding_id = $1`, findingID) != 1 {
		t.Fatalf("revision %d", revision)
	}

	if _, err := store.pool.Exec(ctx, `UPDATE submissions SET constraint_revision = 2 WHERE id = $1`, fx.acSubmission); err != nil {
		t.Fatal(err)
	}
	current := checking.Submission{
		ID: fx.acSubmission, LabID: fx.labID, AuthorUserID: fx.student, Source: reviewSource,
		SourceSHA256: checking.SourceHash(reviewSource), JudgeVerdict: "AC", Generation: 1, TestRevision: 1, ConstraintRevision: 2,
	}
	if err := (&FindingRepo{Store: store}).Save(ctx, current, []checking.Finding{{
		ConstraintID: "prints", SourceKind: checking.SourceSemantic, Verdict: checking.VerdictPass,
		DeductionHundredths: 150, Revision: 1, RequiredReview: true,
	}}); err != nil {
		t.Fatal(err)
	}
	staleSnap := current
	staleSnap.ConstraintRevision = 1
	err = (&FindingRepo{Store: store}).Save(ctx, staleSnap, []checking.Finding{{
		ConstraintID: "stale-ban", SourceKind: checking.SourceHardRule, Verdict: checking.VerdictViolation,
		DeductionHundredths: 300, Revision: 1, RequiredReview: true,
		Evidence: []checking.Evidence{{Line: 2, Text: "  std::sort(a, a + n);"}},
	}})
	if !errors.Is(err, checking.ErrStale) {
		t.Fatalf("stale save err = %v", err)
	}
	if countRows(t, ctx, store, `SELECT count(*) FROM findings WHERE constraint_id = 'stale-ban'`) != 0 {
		t.Fatal("stale revision was stored")
	}
	active, err := repo.List(ctx, fx.labID, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(active.Items) != 1 || active.Items[0].ConstraintID != "prints" || active.Items[0].CheckerVerdict != "pass" {
		t.Fatalf("active set = %+v", active.Items)
	}
}

func TestPostgresConcurrentDecisionsOneCommits(t *testing.T) {
	store, ctx := openSchema(t)
	fx := seedReviewLab(t, ctx, store)
	var findingID string
	err := store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO findings (
				submission_id, lab_id, constraint_id, generation, test_revision, constraint_revision,
				source_sha256, source_kind, verdict, deduction_hundredths, required_review, revision)
			VALUES ($1, $2, 'ban-sort', 1, 1, 1, $3, 'hard-rule', 'violation', 300, true, 1)
			RETURNING id::text`, fx.acSubmission, fx.labID, checking.SourceHash(reviewSource)).Scan(&findingID)
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &ReviewRepo{Store: store}
	start := make(chan struct{})
	var wg sync.WaitGroup
	type outcome struct {
		result review.DecisionResult
		err    error
	}
	results := make([]outcome, 2)
	actors := []string{fx.ta, fx.secondTA}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			result, err := repo.Decide(ctx, review.DecisionRequest{
				LabID: fx.labID, FindingID: findingID, ActorID: actors[i], ExpectedRevision: 1,
				Verdict: "pass", Reason: "Reviewed the same revision.",
				IdempotencyKey: "concurrent-decision-" + string(rune('a'+i)), Fingerprint: "fingerprint-" + string(rune('a'+i)),
				Now: time.Date(2026, 11, 24, 1, 2, 3, 0, time.UTC),
			})
			results[i] = outcome{result: result, err: err}
		}(i)
	}
	close(start)
	wg.Wait()
	commits, conflicts := 0, 0
	for _, result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		switch {
		case result.result.Stale == nil && result.result.Finding.Revision == 2:
			commits++
		case result.result.Stale != nil && result.result.Stale.Expected == 1 && result.result.Stale.Current == 2:
			conflicts++
		default:
			t.Fatalf("result %+v", result.result)
		}
	}
	if commits != 1 || conflicts != 1 {
		t.Fatalf("commits %d conflicts %d", commits, conflicts)
	}
	var kind, verdict string
	var deduction, revision int
	if err := store.pool.QueryRow(ctx, `SELECT source_kind, verdict, deduction_hundredths, revision FROM findings WHERE id = $1`, findingID).Scan(&kind, &verdict, &deduction, &revision); err != nil {
		t.Fatal(err)
	}
	if kind != "hard-rule" || verdict != "violation" || deduction != 300 || revision != 2 {
		t.Fatalf("provenance kind %s verdict %s deduction %d revision %d", kind, verdict, deduction, revision)
	}
	if countRows(t, ctx, store, `SELECT count(*) FROM decisions WHERE finding_id = $1`, findingID) != 1 {
		t.Fatal("decision was overwritten")
	}
}

type reviewFixture struct {
	labID, classID, ta, secondTA, otherTA, removedTA, student, acSubmission, waSubmission string
}

func seedReviewLab(t *testing.T, ctx context.Context, store *Store) reviewFixture {
	t.Helper()
	fx := reviewFixture{
		labID: newUUID(t), classID: newUUID(t), ta: newUUID(t), secondTA: newUUID(t),
		otherTA: newUUID(t), removedTA: newUUID(t), student: newUUID(t),
		acSubmission: newUUID(t), waSubmission: newUUID(t),
	}
	otherClass := newUUID(t)
	waStudent := newUUID(t)
	queuedStudent := newUUID(t)
	received := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	err := store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		users := []struct{ id, name, role string }{
			{fx.ta, "TA One", "ta"},
			{fx.secondTA, "TA Two", "ta"},
			{fx.otherTA, "Other TA", "ta"},
			{fx.removedTA, "Former TA", "ta"},
			{fx.student, "Ada Lovelace", "student"},
			{waStudent, "Wrong Answer", "student"},
			{queuedStudent, "Queued Student", "student"},
		}
		for _, user := range users {
			if err := tx.Exec(ctx, `
				INSERT INTO users (id, email, display_name, role, password_hash)
				VALUES ($1, $2, $3, $4, 'not-a-login')`, user.id, user.id+"@example.test", user.name, user.role); err != nil {
				return err
			}
		}
		if err := tx.Exec(ctx, `INSERT INTO classes (id, name) VALUES ($1, 'A'), ($2, 'B')`, fx.classID, otherClass); err != nil {
			return err
		}
		for _, member := range []string{fx.ta, fx.secondTA, fx.student, waStudent} {
			if err := tx.Exec(ctx, `INSERT INTO memberships (user_id, class_id) VALUES ($1, $2)`, member, fx.classID); err != nil {
				return err
			}
		}
		if err := tx.Exec(ctx, `INSERT INTO memberships (user_id, class_id) VALUES ($1, $2)`, fx.otherTA, otherClass); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `INSERT INTO memberships (user_id, class_id, removed_at) VALUES ($1, $2, $3)`, fx.removedTA, fx.classID, received); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `
			INSERT INTO labs (id, class_id, created_by, title, statement, opens_at, deadline, cpu_seconds, memory_mib)
			VALUES ($1, $2, $3, 'Sort', 'Print a sorted line.', $4, $5, 1, 64)`,
			fx.labID, fx.classID, fx.ta, received.Add(-time.Hour), received.Add(24*time.Hour)); err != nil {
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
			FROM labs WHERE id = $1`, fx.labID); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `
			INSERT INTO lab_constraints (lab_id, revision, constraint_id, position, body, deduction_hundredths)
			VALUES ($1, 1, 'ban-sort', 1, 'Do not use `+"`"+`std::sort`+"`"+`.', 300),
			       ($1, 1, 'prints', 2, 'The program prints a sorted line.', 150)`, fx.labID); err != nil {
			return err
		}
		if err := insertSubmission(ctx, tx, fx.acSubmission, fx.labID, fx.student, reviewSource, "AC", received); err != nil {
			return err
		}
		if err := insertSubmission(ctx, tx, fx.waSubmission, fx.labID, waStudent, reviewSource, "WA", received.Add(time.Minute)); err != nil {
			return err
		}
		return tx.Exec(ctx, `
			INSERT INTO submissions (
				lab_id, author_user_id, source, source_sha256, state, received_at, generation, test_revision, constraint_revision)
			VALUES ($1, $2, 'int main(){return 0;}', $3, 'queued', $4, 1, 1, 1)`,
			fx.labID, queuedStudent, checking.SourceHash("int main(){return 0;}"), received)
	})
	if err != nil {
		t.Fatalf("seed review lab: %v", err)
	}
	return fx
}

func insertSubmission(ctx context.Context, tx *Tx, id, labID, author, source, verdict string, received time.Time) error {
	var seq int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO submissions (
			id, lab_id, author_user_id, source, source_sha256, state, received_at,
			generation, test_revision, constraint_revision, judge_verdict)
		VALUES ($1, $2, $3, $4, $5, 'judged', $6, 1, 1, 1, $7)
		RETURNING server_seq`,
		id, labID, author, source, checking.SourceHash(source), received, verdict).Scan(&seq); err != nil {
		return err
	}
	return tx.Exec(ctx, `
		INSERT INTO counted_selections (lab_id, author_user_id, submission_id, generation, received_at, server_seq)
		VALUES ($1, $2, $3, 1, $4, $5)`, labID, author, id, received, seq)
}

func assertFinding(t *testing.T, ctx context.Context, store *Store, submissionID, constraintID, kind, verdict string, deduction, line int, text string) {
	t.Helper()
	var gotKind, gotVerdict, gotText string
	var gotDeduction, gotRevision, gotLine int
	var cause *string
	err := store.pool.QueryRow(ctx, `
		SELECT f.source_kind, f.verdict, f.deduction_hundredths, f.revision, f.unsure_cause, e.line, e.evidence_text
		FROM findings f
		JOIN finding_evidence e ON e.finding_id = f.id
		WHERE f.submission_id = $1 AND f.constraint_id = $2`, submissionID, constraintID).Scan(
		&gotKind, &gotVerdict, &gotDeduction, &gotRevision, &cause, &gotLine, &gotText)
	if err != nil {
		t.Fatal(err)
	}
	if gotKind != kind || gotVerdict != verdict || gotDeduction != deduction || gotRevision < 1 || gotLine != line || gotText != text || cause != nil {
		t.Fatalf("finding %s kind %s verdict %s deduction %d revision %d line %d text %q cause %v", constraintID, gotKind, gotVerdict, gotDeduction, gotRevision, gotLine, gotText, cause)
	}
}
