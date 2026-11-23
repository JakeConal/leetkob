package access

import "fmt"

// Fixed synthetic identities. Provisioning writes these rows and nothing
// else: no lab, no submission, no finding. IDs stay stable so a repeat
// call conflicts with the existing primary key instead of inserting a
// second account.
const (
	RoleAdmin   = "admin"
	RoleTA      = "ta"
	RoleStudent = "student"

	AdminID     = "6f0c1a10-7b21-4a11-8c11-111111111111"
	TAAID       = "6f0c1a10-7b21-4a11-8c11-222222222222"
	TABID       = "6f0c1a10-7b21-4a11-8c11-333333333333"
	StudentA1ID = "6f0c1a10-7b21-4a11-8c11-444444444444"
	StudentA2ID = "6f0c1a10-7b21-4a11-8c11-555555555555"
	ClassAID    = "6f0c1a10-7b21-4a11-8c11-aaaaaaaaaaaa"
	ClassBID    = "6f0c1a10-7b21-4a11-8c11-bbbbbbbbbbbb"

	EmailAdmin     = "admin@assessment.leetkob.test"
	EmailTAA       = "ta-a@assessment.leetkob.test"
	EmailTAB       = "ta-b@assessment.leetkob.test"
	EmailStudentA1 = "student-a1@assessment.leetkob.test"
	EmailStudentA2 = "student-a2@assessment.leetkob.test"

	NameAdmin     = "PA3 Admin"
	NameTAA       = "PA3 TA A"
	NameTAB       = "PA3 TA B"
	NameStudentA1 = "PA3 Student A1"
	NameStudentA2 = "PA3 Student A2"
	ClassAName    = "PA3 Class A"
	ClassBName    = "PA3 Class B"
)

// Passwords are the operator-supplied secrets for one provisioning run.
// They are hashed before storage. A later run may pass different values;
// existing password hashes are left untouched.
type Passwords struct {
	Admin              string
	TeachingAssistantA string
	TeachingAssistantB string
	StudentA1          string
	StudentA2          string
}

// Class is a current class the caller belongs to.
type Class struct {
	ID   string
	Name string
}

// UserPublic is the safe account projection. It has no password and no
// session secret.
type UserPublic struct {
	ID          string
	Email       string
	DisplayName string
	Role        string
}

// Principal is the server-owned identity for one request. Role and
// Classes come from the database at request time. Caller headers are
// not a field on this type and cannot be copied onto it.
type Principal struct {
	User    UserPublic
	Classes []Class
}

// MemberOf reports current membership. A removed membership is absent
// from Classes, so it returns false.
func (p Principal) MemberOf(classID string) bool {
	for _, class := range p.Classes {
		if class.ID == classID {
			return true
		}
	}
	return false
}

// CanCreateLab is true only for a teaching assistant. Admin is not an
// implicit TA, and a student header cannot change this answer because
// the role is the stored one.
func (p Principal) CanCreateLab() bool {
	return p.User.Role == RoleTA
}

// CanReadSubmission is the ownership rule for a submission of classID
// written by authorID. Students read only their own. TAs read submissions
// in a class they currently belong to. Admin reads none.
func CanReadSubmission(p Principal, classID, authorID string) bool {
	switch p.User.Role {
	case RoleTA:
		return p.MemberOf(classID)
	case RoleStudent:
		return p.User.ID == authorID && p.MemberOf(classID)
	default:
		return false
	}
}

// Account is one synthetic user before hashing.
type Account struct {
	ID          string
	Email       string
	DisplayName string
	Role        string
	Password    string
}

// Membership is one class assignment. A removed row is not a Membership
// in the plan; provisioning will not recreate it.
type Membership struct {
	UserID  string
	ClassID string
}

// Plan is the complete synthetic directory. It has no lab.
type Plan struct {
	Classes     []Class
	Accounts    []Account
	Memberships []Membership
}

// BuildPlan returns the five accounts and two classes. The first TA and
// both students are in class A. The second TA is in class B. The admin
// has no class.
func BuildPlan(passwords Passwords) (Plan, error) {
	accounts := []Account{
		{ID: AdminID, Email: EmailAdmin, DisplayName: NameAdmin, Role: RoleAdmin, Password: passwords.Admin},
		{ID: TAAID, Email: EmailTAA, DisplayName: NameTAA, Role: RoleTA, Password: passwords.TeachingAssistantA},
		{ID: TABID, Email: EmailTAB, DisplayName: NameTAB, Role: RoleTA, Password: passwords.TeachingAssistantB},
		{ID: StudentA1ID, Email: EmailStudentA1, DisplayName: NameStudentA1, Role: RoleStudent, Password: passwords.StudentA1},
		{ID: StudentA2ID, Email: EmailStudentA2, DisplayName: NameStudentA2, Role: RoleStudent, Password: passwords.StudentA2},
	}
	for _, account := range accounts {
		if n := len(account.Password); n < 1 || n > 128 {
			return Plan{}, fmt.Errorf("%w: password length", ErrInvalid)
		}
	}
	return Plan{
		Classes: []Class{
			{ID: ClassAID, Name: ClassAName},
			{ID: ClassBID, Name: ClassBName},
		},
		Accounts: accounts,
		Memberships: []Membership{
			{UserID: TAAID, ClassID: ClassAID},
			{UserID: TABID, ClassID: ClassBID},
			{UserID: StudentA1ID, ClassID: ClassAID},
			{UserID: StudentA2ID, ClassID: ClassAID},
		},
	}, nil
}
