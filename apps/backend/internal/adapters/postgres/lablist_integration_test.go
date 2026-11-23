package postgres

import (
	"context"
	"errors"
	"testing"
)

const classBID = "88888888-8888-4888-8888-888888888888"

func TestListLabsByClassPaginatesAndFilters(t *testing.T) {
	pool := migratedPool(t)
	seedIdentity(t, pool)
	store := &Store{pool: pool}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO classes (id, name) VALUES ($1, 'Class B')`, classBID); err != nil {
		t.Fatalf("seed class B: %v", err)
	}

	classALabs := []string{
		"a0000000-0000-4000-8000-000000000001",
		"a0000000-0000-4000-8000-000000000002",
		"a0000000-0000-4000-8000-000000000003",
	}
	for _, id := range classALabs {
		if err := store.CreateLab(ctx, testLab(id, classID, taID)); err != nil {
			t.Fatalf("create class A lab %s: %v", id, err)
		}
	}
	classBLab := "b0000000-0000-4000-8000-000000000001"
	if err := store.CreateLab(ctx, testLab(classBLab, classBID, taID)); err != nil {
		t.Fatalf("create class B lab: %v", err)
	}

	first, err := store.ListLabsByClass(ctx, classID, 2, 0)
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first page = %d labs, want 2", len(first))
	}
	for _, lab := range first {
		if lab.ClassID != classID {
			t.Fatalf("first page leaked class %q", lab.ClassID)
		}
		if len(lab.Constraints) != 0 || len(lab.Tests) != 0 {
			t.Fatal("listing must not load constraints or tests")
		}
	}

	second, err := store.ListLabsByClass(ctx, classID, 2, 2)
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second page = %d labs, want 1", len(second))
	}

	seen := map[string]bool{}
	for _, lab := range append(first, second...) {
		if seen[lab.ID] {
			t.Fatalf("duplicate lab %s across pages", lab.ID)
		}
		seen[lab.ID] = true
	}
	for _, id := range classALabs {
		if !seen[id] {
			t.Fatalf("lab %s missing from paginated class listing", id)
		}
	}
	if seen[classBLab] {
		t.Fatal("class B lab appeared in class A listing")
	}

	bLabs, err := store.ListLabsByClass(ctx, classBID, 10, 0)
	if err != nil {
		t.Fatalf("list class B: %v", err)
	}
	if len(bLabs) != 1 || bLabs[0].ID != classBLab {
		t.Fatalf("class B listing = %+v", bLabs)
	}

	past := mustList(t, store, classID, 10, 99)
	if len(past) != 0 {
		t.Fatalf("offset past the end returned %d labs", len(past))
	}
}

func mustList(t *testing.T, store *Store, classID string, limit, offset int) []LabRecord {
	t.Helper()
	labs, err := store.ListLabsByClass(context.Background(), classID, limit, offset)
	if err != nil {
		t.Fatalf("list labs: %v", err)
	}
	return labs
}

func TestGetLabForClassScopesReads(t *testing.T) {
	pool := migratedPool(t)
	seedIdentity(t, pool)
	store := &Store{pool: pool}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO classes (id, name) VALUES ($1, 'Class B')`, classBID); err != nil {
		t.Fatalf("seed class B: %v", err)
	}
	lab := testLab("a1000000-0000-4000-8000-000000000001", classID, taID)
	if err := store.CreateLab(ctx, lab); err != nil {
		t.Fatalf("create lab: %v", err)
	}

	read, err := store.GetLabForClass(ctx, lab.ID, classID)
	if err != nil {
		t.Fatalf("read own class: %v", err)
	}
	if read.ID != lab.ID || len(read.Constraints) != 2 || len(read.Tests) != 2 {
		t.Fatalf("scoped read mismatch: %+v", read)
	}

	if _, err := store.GetLabForClass(ctx, lab.ID, classBID); !errors.Is(err, ErrLabNotFound) {
		t.Fatalf("cross-class read error = %v, want ErrLabNotFound", err)
	}
	if _, err := store.GetLabForClass(ctx, "a1000000-0000-4000-8000-000000000099", classID); !errors.Is(err, ErrLabNotFound) {
		t.Fatalf("absent read error = %v, want ErrLabNotFound", err)
	}
}
