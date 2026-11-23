package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/auth"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

const (
	provisionClassA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	provisionClassB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func provisionSeeds() []ProvisionAccountSeed {
	return []ProvisionAccountSeed{
		{Email: "admin@leetkob.local", DisplayName: "PA3 Admin", Role: auth.RoleAdmin},
		{Email: "ta1@leetkob.local", DisplayName: "PA3 TA One", Role: auth.RoleTA, ClassID: provisionClassA, ClassName: "PA3 Class A"},
		{Email: "ta2@leetkob.local", DisplayName: "PA3 TA Two", Role: auth.RoleTA, ClassID: provisionClassB, ClassName: "PA3 Class B"},
		{Email: "student1@leetkob.local", DisplayName: "PA3 Student One", Role: auth.RoleStudent, ClassID: provisionClassA, ClassName: "PA3 Class A"},
		{Email: "student2@leetkob.local", DisplayName: "PA3 Student Two", Role: auth.RoleStudent, ClassID: provisionClassA, ClassName: "PA3 Class A"},
	}
}

func TestProvisioningIsIdempotentAndNeverResetsCredentials(t *testing.T) {
	pool := migratedPool(t)
	store := &Store{pool: pool}
	ctx := context.Background()

	first, err := store.ProvisionAccounts(ctx, provisionSeeds(), identity.HashPassword)
	if err != nil {
		t.Fatalf("first provisioning: %v", err)
	}
	if len(first) != 5 {
		t.Fatalf("provisioned %d accounts, want 5", len(first))
	}
	passwords := map[string]string{}
	for _, account := range first {
		if !account.Created || account.Password == "" {
			t.Fatalf("first run must create %s with a password: %+v", account.Email, account)
		}
		passwords[account.Email] = account.Password
	}

	second, err := store.ProvisionAccounts(ctx, provisionSeeds(), identity.HashPassword)
	if err != nil {
		t.Fatalf("second provisioning: %v", err)
	}
	for _, account := range second {
		if account.Created || account.Password != "" {
			t.Fatalf("second run must not recreate or reset %s: %+v", account.Email, account)
		}
	}

	var users, classes, memberships int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM users),
		       (SELECT count(*) FROM classes),
		       (SELECT count(*) FROM memberships)`).Scan(&users, &classes, &memberships); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if users != 5 || classes != 2 || memberships != 4 {
		t.Fatalf("row counts = users:%d classes:%d memberships:%d, want 5/2/4", users, classes, memberships)
	}

	// The original passwords still verify: nothing was silently reset.
	for email, password := range passwords {
		user, err := store.FindUserByEmail(ctx, email)
		if err != nil {
			t.Fatalf("find %s: %v", email, err)
		}
		ok, err := identity.VerifyPassword(user.PasswordHash, password)
		if err != nil || !ok {
			t.Fatalf("stored password for %s no longer verifies (ok=%v err=%v)", email, ok, err)
		}
	}

	// Memberships match the documented layout: TA/students in class A,
	// TA Two in class B, admin in neither.
	taOne, _ := store.FindUserByEmail(ctx, "ta1@leetkob.local")
	classesA, err := store.ClassesForUser(ctx, taOne.ID)
	if err != nil || len(classesA) != 1 || classesA[0].ID != provisionClassA {
		t.Fatalf("ta1 classes = %+v err=%v", classesA, err)
	}
	admin, _ := store.FindUserByEmail(ctx, "admin@leetkob.local")
	adminClasses, err := store.ClassesForUser(ctx, admin.ID)
	if err != nil || len(adminClasses) != 0 {
		t.Fatalf("admin classes = %+v err=%v", adminClasses, err)
	}
}

func TestSessionRoundTripAndRevocation(t *testing.T) {
	pool := migratedPool(t)
	store := &Store{pool: pool}
	ctx := context.Background()
	if _, err := store.ProvisionAccounts(ctx, provisionSeeds()[:2], identity.HashPassword); err != nil {
		t.Fatalf("provision: %v", err)
	}
	user, err := store.FindUserByEmail(ctx, "ta1@leetkob.local")
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	now := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	session := auth.SessionRecord{
		ID:            "99999999-9999-4999-8999-999999999999",
		UserID:        user.ID,
		TokenHash:     []byte("token-digest-32-bytes-long-value!"),
		CSRFTokenHash: []byte("csrf-digest-32-bytes-long-value!!"),
		CreatedAt:     now,
		ExpiresAt:     now.Add(8 * time.Hour),
	}
	if err := store.CreateSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}

	found, err := store.SessionByTokenHash(ctx, session.TokenHash)
	if err != nil {
		t.Fatalf("lookup session: %v", err)
	}
	if found.User.ID != user.ID || found.Session.ID != session.ID {
		t.Fatalf("session join mismatch: %+v", found)
	}
	byID, err := store.SessionByID(ctx, session.ID)
	if err != nil || string(byID.CSRFTokenHash) != string(session.CSRFTokenHash) {
		t.Fatalf("session by id mismatch: %+v err=%v", byID, err)
	}

	if err := store.RevokeSession(ctx, session.ID); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	if _, err := store.SessionByTokenHash(ctx, session.TokenHash); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("revoked lookup err = %v, want ErrUnauthenticated", err)
	}
	if _, err := store.SessionByID(ctx, session.ID); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("revoked by-id err = %v, want ErrUnauthenticated", err)
	}
}

func TestRecordAttemptEnforcesBothWindows(t *testing.T) {
	pool := migratedPool(t)
	store := &Store{pool: pool}
	ctx := context.Background()
	now := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)

	for i := 0; i < auth.IPLimit; i++ {
		allowed, _, err := store.RecordAttempt(ctx, "198.51.100.7", "ta@example.test", now)
		if err != nil {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("attempt %d blocked too early", i+1)
		}
	}
	allowed, retryAfter, err := store.RecordAttempt(ctx, "198.51.100.7", "ta@example.test", now)
	if err != nil {
		t.Fatalf("over-limit attempt: %v", err)
	}
	if allowed {
		t.Fatal("11th attempt in one minute must be blocked")
	}
	if retryAfter <= 0 || retryAfter > auth.AccountWindow {
		t.Fatalf("retry after = %v, want within the account window", retryAfter)
	}

	// One minute later the address window clears but the account window
	// does not.
	later := now.Add(auth.IPWindow + time.Second)
	allowed, _, err = store.RecordAttempt(ctx, "198.51.100.7", "ta@example.test", later)
	if err != nil {
		t.Fatalf("attempt after ip window: %v", err)
	}
	if allowed {
		t.Fatal("account window must still block after the ip window clears")
	}
	allowed, _, err = store.RecordAttempt(ctx, "203.0.113.9", "other@example.test", later)
	if err != nil {
		t.Fatalf("attempt for another account: %v", err)
	}
	if !allowed {
		t.Fatal("a different address and account must not be blocked")
	}
}
