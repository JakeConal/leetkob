package postgres

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestJudgeInputAndSafeReads covers the worker and API store additions end
// to end: the trusted judge input matches the accepted source, and only the
// owner-scoped safe projection is returned.
func TestJudgeInputAndSafeReads(t *testing.T) {
	labID, store := seedAcceptanceLab(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	result, err := store.AcceptSubmission(ctx, acceptCommand(labID, "read-key-0001", "int main(){}", now))
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	input, err := store.GetJudgeInput(ctx, result.SubmissionID)
	if err != nil {
		t.Fatalf("judge input: %v", err)
	}
	if input.LabID != labID || input.Source != "int main(){}" || input.Generation != 1 {
		t.Fatalf("judge input = %+v", input)
	}
	if input.CPUSeconds != 1 || input.MemoryMiB != 256 {
		t.Fatalf("judge limits = %+v", input)
	}
	if len(input.Tests) != 2 || input.Tests[0].ID != "01" ||
		input.Tests[0].ExpectedOutput != "1 2 3\n" || input.Tests[1].ID != "02" {
		t.Fatalf("judge tests = %+v", input.Tests)
	}

	leased, err := store.LeaseJobs(ctx, "read-worker", 1, now, time.Minute)
	if err != nil || len(leased) != 1 {
		t.Fatalf("lease: %v %+v", err, leased)
	}
	if err := store.CommitJudgeResult(ctx, leased[0].JobID, leased[0].Attempt, outcome(), now.Add(time.Second)); err != nil {
		t.Fatalf("commit: %v", err)
	}

	view, err := store.GetSubmissionForRead(ctx, result.SubmissionID)
	if err != nil {
		t.Fatalf("read submission: %v", err)
	}
	if view.State != "judged" || view.Verdict == nil || *view.Verdict != "AC" {
		t.Fatalf("view = %+v", view)
	}
	if view.StudentID != studentID || view.ClassID != classID {
		t.Fatalf("ownership fields = %s/%s", view.StudentID, view.ClassID)
	}
	if len(view.Tests) != 2 || view.Tests[0].TestID != "01" ||
		view.Tests[0].Verdict != "AC" || view.Tests[0].TimeMs != 12 {
		t.Fatalf("stored tests = %+v", view.Tests)
	}

	ownerList, err := store.ListSubmissionsForStudent(ctx, labID, studentID, 50, 0)
	if err != nil {
		t.Fatalf("owner list: %v", err)
	}
	if len(ownerList) != 1 || ownerList[0].ID != result.SubmissionID || len(ownerList[0].Tests) != 2 {
		t.Fatalf("owner list = %+v", ownerList)
	}

	// A second enrolled student sees no one else's submissions.
	otherID := "77777777-7777-4777-8777-777777777777"
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO users (id, email, display_name, role, password_hash)
		VALUES ($1, 'other@example.test', 'Other Student', 'student', 'argon2id$placeholder')`,
		otherID); err != nil {
		t.Fatalf("seed other student: %v", err)
	}
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2)`, classID, otherID); err != nil {
		t.Fatalf("enroll other student: %v", err)
	}
	otherList, err := store.ListSubmissionsForStudent(ctx, labID, otherID, 50, 0)
	if err != nil {
		t.Fatalf("other list: %v", err)
	}
	if len(otherList) != 0 {
		t.Fatalf("another student saw %d submissions", len(otherList))
	}

	labList, err := store.ListSubmissionsForLab(ctx, labID, 50, 0)
	if err != nil {
		t.Fatalf("lab list: %v", err)
	}
	if len(labList) != 1 || labList[0].ID != result.SubmissionID {
		t.Fatalf("lab list = %+v", labList)
	}

	class, err := store.LabClassID(ctx, labID)
	if err != nil || class != classID {
		t.Fatalf("lab class = %q err=%v", class, err)
	}
	if _, err := store.LabClassID(ctx, "99999999-9999-4999-8999-999999999999"); !errors.Is(err, ErrLabNotFound) {
		t.Fatalf("absent lab class err = %v, want ErrLabNotFound", err)
	}
	if _, err := store.GetSubmissionForRead(ctx, "99999999-9999-4999-8999-999999999999"); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("absent submission err = %v, want ErrSubmissionNotFound", err)
	}
	if _, err := store.GetJudgeInput(ctx, "99999999-9999-4999-8999-999999999999"); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("absent judge input err = %v, want ErrSubmissionNotFound", err)
	}
}
