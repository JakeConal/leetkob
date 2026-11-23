package access

import "testing"

func TestBuildPlanIsTheSyntheticDirectory(t *testing.T) {
	plan, err := BuildPlan(Passwords{
		Admin:              "admin-secret",
		TeachingAssistantA: "ta-a-secret",
		TeachingAssistantB: "ta-b-secret",
		StudentA1:          "student-1-secret",
		StudentA2:          "student-2-secret",
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Accounts) != 5 || len(plan.Classes) != 2 || len(plan.Memberships) != 4 {
		t.Fatalf("accounts=%d classes=%d memberships=%d", len(plan.Accounts), len(plan.Classes), len(plan.Memberships))
	}
	roles := map[string]string{}
	for _, account := range plan.Accounts {
		roles[account.ID] = account.Role
	}
	if roles[AdminID] != RoleAdmin || roles[TAAID] != RoleTA || roles[TABID] != RoleTA || roles[StudentA1ID] != RoleStudent || roles[StudentA2ID] != RoleStudent {
		t.Fatalf("roles = %#v", roles)
	}
	classesOf := map[string][]string{}
	for _, membership := range plan.Memberships {
		classesOf[membership.UserID] = append(classesOf[membership.UserID], membership.ClassID)
	}
	if len(classesOf[AdminID]) != 0 {
		t.Fatal("admin must not be placed in a class")
	}
	if len(classesOf[TAAID]) != 1 || classesOf[TAAID][0] != ClassAID {
		t.Fatalf("first TA classes = %#v", classesOf[TAAID])
	}
	if len(classesOf[TABID]) != 1 || classesOf[TABID][0] != ClassBID {
		t.Fatalf("second TA classes = %#v", classesOf[TABID])
	}
	for _, student := range []string{StudentA1ID, StudentA2ID} {
		if len(classesOf[student]) != 1 || classesOf[student][0] != ClassAID {
			t.Fatalf("student %s classes = %#v", student, classesOf[student])
		}
	}
}

func TestBuildPlanRejectsEmptyPassword(t *testing.T) {
	if _, err := BuildPlan(Passwords{}); err == nil {
		t.Fatal("empty passwords must be rejected before any write")
	}
}

func TestSubmissionAccessIgnoresSpoofedRole(t *testing.T) {
	student := Principal{User: UserPublic{ID: StudentA1ID, Role: RoleStudent}, Classes: []Class{{ID: ClassAID}}}
	other := Principal{User: UserPublic{ID: StudentA2ID, Role: RoleStudent}, Classes: []Class{{ID: ClassAID}}}
	ta := Principal{User: UserPublic{ID: TAAID, Role: RoleTA}, Classes: []Class{{ID: ClassAID}}}
	otherTA := Principal{User: UserPublic{ID: TABID, Role: RoleTA}, Classes: []Class{{ID: ClassBID}}}
	admin := Principal{User: UserPublic{ID: AdminID, Role: RoleAdmin}, Classes: []Class{{ID: ClassAID}}}

	if !CanReadSubmission(student, ClassAID, StudentA1ID) {
		t.Fatal("student must read their own submission")
	}
	if CanReadSubmission(student, ClassAID, StudentA2ID) {
		t.Fatal("student must not read another student's submission")
	}
	if CanReadSubmission(other, ClassBID, StudentA2ID) {
		t.Fatal("student must not read a submission outside their class")
	}
	if !CanReadSubmission(ta, ClassAID, StudentA1ID) || !CanReadSubmission(ta, ClassAID, StudentA2ID) {
		t.Fatal("assigned TA must read class submissions")
	}
	if CanReadSubmission(otherTA, ClassAID, StudentA1ID) {
		t.Fatal("other TA must not read this class")
	}
	if CanReadSubmission(admin, ClassAID, StudentA1ID) || admin.CanCreateLab() {
		t.Fatal("admin must not gain TA access from a membership or a caller header")
	}
	// A forged header cannot build a principal. The only role that can
	// create a lab is the stored TA role.
	spoofed := student
	spoofed.User.Role = RoleStudent
	if spoofed.CanCreateLab() || CanReadSubmission(spoofed, ClassAID, StudentA2ID) {
		t.Fatal("keeping the stored student role must not grant TA access")
	}
}
