package review

import (
	"context"
	"sync"
	"testing"
	"time"
)

const (
	decisionLab      = "11111111-1111-4111-8111-111111111111"
	decisionClass    = "22222222-2222-4222-8222-222222222222"
	decisionFinding  = "33333333-3333-4333-8333-333333333333"
	decisionTA       = "44444444-4444-4444-8444-444444444444"
	decisionOtherTA  = "55555555-5555-4555-8555-555555555555"
	decisionSource   = "int main() {\n  std::sort(a, a + n);\n}\n"
	decisionEvidence = "  std::sort(a, a + n);"
)

func TestApplyDecisionKeepsCheckerProvenance(t *testing.T) {
	current := sampleFinding()
	next, err := ApplyDecision(current, DecisionRequest{
		ActorID:          decisionTA,
		ExpectedRevision: 1,
		Verdict:          "violation",
		Reason:           "The call is std::sort.",
		Evidence:         &Evidence{Line: 2, Text: decisionEvidence},
		Now:              time.Date(2026, 11, 24, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision != 2 || next.Decision == nil || next.Decision.Revision != 2 {
		t.Fatalf("revision = %d decision %#v", next.Revision, next.Decision)
	}
	if next.Decision.DecidedAt != "2026-11-24T01:02:03.000Z" || next.Decision.DecidedBy != decisionTA {
		t.Fatalf("audit = %+v", next.Decision)
	}
	assertProvenance(t, current, next)
	if next.Decision.Evidence == nil || next.Decision.Evidence.Text != decisionEvidence {
		t.Fatalf("decision evidence = %+v", next.Decision.Evidence)
	}
}

func TestUnsureHasNoDefaultAndViolationNeedsTheOriginalLine(t *testing.T) {
	current := sampleFinding()
	current.CheckerVerdict = "unsure"
	current.UnsureCause = "provider-timeout"
	current.Evidence = []Evidence{}
	if _, err := ApplyDecision(current, DecisionRequest{ActorID: decisionTA, ExpectedRevision: 1, Reason: "Because."}); err == nil {
		t.Fatal("missing verdict was accepted")
	}
	if _, err := ApplyDecision(current, DecisionRequest{ActorID: decisionTA, ExpectedRevision: 1, Verdict: "unsure", Reason: "Leave it."}); err == nil {
		t.Fatal("unsure was accepted as a decision")
	}
	if _, err := ApplyDecision(current, DecisionRequest{ActorID: decisionTA, ExpectedRevision: 1, Verdict: "violation", Reason: "Because."}); err == nil {
		t.Fatal("violation without evidence was accepted")
	}
	if _, err := ApplyDecision(current, DecisionRequest{
		ActorID: decisionTA, ExpectedRevision: 1, Verdict: "violation", Reason: "Because.",
		Evidence: &Evidence{Line: 2, Text: "not the original line"},
	}); err == nil {
		t.Fatal("invented evidence was accepted")
	}
	next, err := ApplyDecision(current, DecisionRequest{
		ActorID: decisionTA, ExpectedRevision: 1, Verdict: "pass", Reason: "The timeout is not a violation.",
		Now: time.Date(2026, 11, 24, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.CheckerVerdict != "unsure" || next.UnsureCause != "provider-timeout" || next.Decision == nil || next.Decision.Verdict != "pass" {
		t.Fatalf("unsure was overwritten: %+v", next)
	}
	if len(next.Evidence) != 0 {
		t.Fatalf("checker evidence changed: %+v", next.Evidence)
	}
}

func TestSecondDecisionOnTheSameRevisionConflicts(t *testing.T) {
	store := newDecisionMemory(t)
	first, err := store.Decide(context.Background(), passRequest(decisionTA, "same-revision-a", "fingerprint-a"))
	if err != nil || first.Stale != nil || first.Finding.Revision != 2 {
		t.Fatalf("first = %+v err %v", first, err)
	}
	second, err := store.Decide(context.Background(), passRequest(decisionOtherTA, "same-revision-b", "fingerprint-b"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Stale == nil || second.Stale.Expected != 1 || second.Stale.Current != 2 {
		t.Fatalf("second = %+v, want conflict current 2", second.Stale)
	}
	if store.DecisionCount(decisionFinding) != 1 {
		t.Fatalf("decisions = %d, want 1", store.DecisionCount(decisionFinding))
	}
	page, err := store.List(context.Background(), decisionLab, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].CheckerVerdict != "violation" || page.Items[0].SourceKind != "hard-rule" {
		t.Fatalf("queue = %+v", page.Items)
	}
	if page.Items[0].Deduction != 300 || page.Items[0].Evidence[0].Text != decisionEvidence {
		t.Fatalf("provenance changed: %+v", page.Items[0])
	}
}

func TestConcurrentDecisionsOneCommits(t *testing.T) {
	store := newDecisionMemory(t)
	start := make(chan struct{})
	var wg sync.WaitGroup
	type outcome struct {
		result DecisionResult
		err    error
	}
	results := make([]outcome, 2)
	actors := []string{decisionTA, decisionOtherTA}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			result, err := store.Decide(context.Background(), passRequest(actors[i], "concurrent-key-"+string(rune('a'+i)), "fingerprint-"+string(rune('a'+i))))
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
		case result.result.Stale != nil && result.result.Stale.Current == 2 && result.result.Stale.Expected == 1:
			conflicts++
		default:
			t.Fatalf("unexpected result %+v", result.result)
		}
	}
	if commits != 1 || conflicts != 1 {
		t.Fatalf("commits = %d conflicts = %d", commits, conflicts)
	}
	if store.DecisionCount(decisionFinding) != 1 {
		t.Fatalf("decisions = %d, want 1", store.DecisionCount(decisionFinding))
	}
}

func TestQueueReadDoesNotRedrawThePassSample(t *testing.T) {
	store := NewMemory()
	store.PutUser(decisionTA, "ta")
	store.PutLab(decisionLab, decisionClass, false)
	store.PutMember(decisionClass, decisionTA)
	store.PutFinding(Finding{
		ID: decisionFinding, LabID: decisionLab, SubmissionID: "66666666-6666-4666-8666-666666666666",
		ConstraintID: "pass-one", ConstraintRevision: 1, Revision: 1, SourceKind: "semantic",
		CheckerVerdict: "pass", Evidence: []Evidence{}, Deduction: 100, RequiredReview: false, Source: decisionSource,
	}, 1, 1, true)
	store.PutFinding(Finding{
		ID: "77777777-7777-4777-8777-777777777777", LabID: decisionLab, SubmissionID: "66666666-6666-4666-8666-666666666666",
		ConstraintID: "must-review", ConstraintRevision: 1, Revision: 1, SourceKind: "hard-rule",
		CheckerVerdict: "violation", Evidence: []Evidence{{Line: 2, Text: decisionEvidence}}, Deduction: 300,
		RequiredReview: true, Source: decisionSource,
	}, 1, 2, true)
	first, err := store.List(context.Background(), decisionLab, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.List(context.Background(), decisionLab, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || len(second.Items) != 2 {
		t.Fatalf("items = %d and %d", len(first.Items), len(second.Items))
	}
	for i := range first.Items {
		if first.Items[i].ConstraintID != second.Items[i].ConstraintID || first.Items[i].RequiredReview != second.Items[i].RequiredReview {
			t.Fatalf("refresh changed %+v -> %+v", first.Items[i], second.Items[i])
		}
	}
	var passRequired bool
	for _, item := range second.Items {
		if item.CheckerVerdict == "pass" {
			passRequired = item.RequiredReview
		}
	}
	if passRequired {
		t.Fatal("reading the queue redrew the pass sample")
	}
}

func TestReplayReturnsTheOriginalDecision(t *testing.T) {
	store := newDecisionMemory(t)
	req := passRequest(decisionTA, "replay-key-0001", "fingerprint-same")
	first, err := store.Decide(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Decide(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Replay || again.Finding.Revision != first.Finding.Revision || store.DecisionCount(decisionFinding) != 1 {
		t.Fatalf("replay = %+v count %d", again, store.DecisionCount(decisionFinding))
	}
	changed := req
	changed.Fingerprint = "fingerprint-other"
	changed.Reason = "A different reason."
	mismatch, err := store.Decide(context.Background(), changed)
	if err != nil || !mismatch.Mismatch || store.DecisionCount(decisionFinding) != 1 {
		t.Fatalf("mismatch = %+v err %v count %d", mismatch, err, store.DecisionCount(decisionFinding))
	}
}

func newDecisionMemory(t *testing.T) *Memory {
	t.Helper()
	store := NewMemory()
	store.PutUser(decisionTA, "ta")
	store.PutUser(decisionOtherTA, "ta")
	store.PutLab(decisionLab, decisionClass, false)
	store.PutMember(decisionClass, decisionTA)
	store.PutMember(decisionClass, decisionOtherTA)
	store.PutFinding(sampleFinding(), 1, 1, true)
	return store
}

func sampleFinding() Finding {
	return Finding{
		ID: decisionFinding, LabID: decisionLab, SubmissionID: "66666666-6666-4666-8666-666666666666",
		ConstraintID: "ban-sort", ConstraintRevision: 1, Revision: 1, SourceKind: "hard-rule",
		CheckerVerdict: "violation", Evidence: []Evidence{{Line: 2, Text: decisionEvidence}},
		Deduction: 300, RequiredReview: true, Source: decisionSource,
	}
}

func passRequest(actor, key, fingerprint string) DecisionRequest {
	return DecisionRequest{
		LabID: decisionLab, FindingID: decisionFinding, ActorID: actor, ExpectedRevision: 1,
		Verdict: "pass", Reason: "The ban does not apply to this line.",
		IdempotencyKey: key, Fingerprint: fingerprint,
		Now: time.Date(2026, 11, 24, 1, 2, 3, 0, time.UTC),
	}
}

func assertProvenance(t *testing.T, before, after Finding) {
	t.Helper()
	if after.SourceKind != before.SourceKind || after.CheckerVerdict != before.CheckerVerdict ||
		after.UnsureCause != before.UnsureCause || after.Deduction != before.Deduction ||
		after.Source != before.Source || !sameEvidence(after.Evidence, before.Evidence) {
		t.Fatalf("provenance changed\nbefore %+v\nafter %+v", before, after)
	}
}
