package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/access"
	"github.com/JakeConal/leetkob/backend/internal/labs"
)

func TestLabWriteRollsBackAndKeepsHiddenTestsPrivate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	store := openIsolated(t, ctx)
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := access.Provision(ctx, store, access.Passwords{
		Admin: "admin-password-value", TeachingAssistantA: "ta-a-password-value",
		TeachingAssistantB: "ta-b-password-value", StudentA1: "student-a1-password",
		StudentA2: "student-a2-password",
	}); err != nil {
		t.Fatalf("provision: %v", err)
	}

	opens := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	deadline := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	bad := storedLab(newUUID(t), opens, deadline)
	bad.Constraints = []labs.Constraint{
		{ID: "C1", Text: "first", Deduction: 100},
		{ID: "C1", Text: "duplicate", Deduction: 200},
	}
	err := store.CreateLab(ctx, bad, labs.Replay{
		ActorID: access.TAAID, Operation: labs.CreateOperation, Resource: access.ClassAID,
		Key: "rollback-key1", Fingerprint: "fp", Status: 201, Body: []byte(`{"id":"ignored"}`),
	})
	if err == nil {
		t.Fatal("duplicate constraint id must fail the transaction")
	}
	for _, table := range []string{"labs", "lab_revisions", "lab_tests", "lab_constraints", "replays"} {
		if got := countRows(t, ctx, store, `SELECT count(*) FROM `+table); got != 0 {
			t.Fatalf("%s rows after rollback = %d", table, got)
		}
	}

	const hidden = "hidden-body-7f3a"
	good := storedLab(newUUID(t), opens, deadline)
	good.Tests = []labs.Test{
		{ID: "02", Input: "6\n5 -2 9\n", ExpectedOutput: "-2 5 9\n", Sample: true},
		{ID: "05", Input: hidden, ExpectedOutput: "secret-output", Sample: false},
	}
	if err := store.CreateLab(ctx, good, labs.Replay{
		ActorID: access.TAAID, Operation: labs.CreateOperation, Resource: access.ClassAID,
		Key: "create-key-0001", Fingerprint: "fp-good", Status: 201, Body: []byte(`{"id":"stored"}`),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	var compile, wall, processes, output, source, revision int
	var budget int64
	if err := store.pool.QueryRow(ctx, `
		SELECT compile_seconds, wall_multiplier, max_processes, max_output_bytes,
		       max_source_bytes, budget_cap_micros, current_revision
		FROM labs WHERE id = $1`, good.ID).Scan(&compile, &wall, &processes, &output, &source, &budget, &revision); err != nil {
		t.Fatalf("limits: %v", err)
	}
	if compile != 10 || wall != 3 || processes != 16 || output != 1<<20 || source != 65536 || budget != 250000 || revision != 1 {
		t.Fatalf("stored policy = compile %d wall %d processes %d output %d source %d budget %d revision %d", compile, wall, processes, output, source, budget, revision)
	}
	var hiddenCount int
	if err := store.pool.QueryRow(ctx, `
		SELECT count(*) FROM lab_tests WHERE lab_id = $1 AND sample = false AND input = $2`, good.ID, hidden).Scan(&hiddenCount); err != nil {
		t.Fatal(err)
	}
	if hiddenCount != 1 {
		t.Fatalf("hidden rows = %d", hiddenCount)
	}

	now := time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC)
	visible, found, err := store.ReadLab(ctx, access.TAAID, access.RoleTA, good.ID, now)
	if err != nil || !found {
		t.Fatalf("TA read before open: found %v err %v", found, err)
	}
	if len(visible.SampleTests) != 1 || strings.Contains(visible.SampleTests[0].Input, hidden) {
		t.Fatalf("TA read included hidden content: %#v", visible.SampleTests)
	}
	if _, found, err = store.ReadLab(ctx, access.StudentA1ID, access.RoleStudent, good.ID, now); err != nil || found {
		t.Fatalf("student read before open: found %v err %v", found, err)
	}
	if _, found, err = store.ReadLab(ctx, access.TABID, access.RoleTA, good.ID, now); err != nil || found {
		t.Fatalf("other TA read: found %v err %v", found, err)
	}
}

func storedLab(id string, opens, deadline time.Time) labs.StoredLab {
	return labs.StoredLab{
		ID: id, ClassID: access.ClassAID, CreatedBy: access.TAAID,
		Title: "sort-merge", Statement: "Merge the numbers.",
		OpensAt: opens, Deadline: deadline,
		CPUSeconds: 2, MemoryMiB: 256,
		CompileSeconds: labs.CompileSeconds, WallMultiplier: labs.WallMultiplier,
		MaxProcesses: labs.MaxProcesses, MaxOutputBytes: labs.MaxOutputBytes,
		MaxSourceBytes: labs.MaxSourceBytes, BudgetCapMicros: labs.BudgetCapMicros,
		Revision: 1, TestRevision: 1, ConstraintRevision: 1, Generation: 1,
		Constraints: []labs.Constraint{{ID: "C1", Text: "Do not call `std::sort`.", Deduction: 300}},
		Tests:       []labs.Test{{ID: "01", Input: "1\n", ExpectedOutput: "1\n", Sample: true}},
	}
}
