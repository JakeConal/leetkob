package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/access"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

func TestProvisionDoesNotResetOrSeedALab(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	store := openIsolated(t, ctx)
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	first := access.Passwords{
		Admin: "first-admin-password", TeachingAssistantA: "first-ta-a-password",
		TeachingAssistantB: "first-ta-b-password", StudentA1: "first-student-a1-password",
		StudentA2: "first-student-a2-password",
	}
	second := access.Passwords{
		Admin: "second-admin-password", TeachingAssistantA: "second-ta-a-password",
		TeachingAssistantB: "second-ta-b-password", StudentA1: "second-student-a1-password",
		StudentA2: "second-student-a2-password",
	}
	if err := access.Provision(ctx, store, first); err != nil {
		t.Fatalf("provision: %v", err)
	}
	var hash string
	if err := store.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, access.AdminID).Scan(&hash); err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := access.Provision(ctx, store, second); err != nil {
		t.Fatalf("repeat provision: %v", err)
	}
	var again string
	if err := store.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, access.AdminID).Scan(&again); err != nil {
		t.Fatalf("hash again: %v", err)
	}
	if hash != again {
		t.Fatal("repeat provisioning reset the stored password hash")
	}
	ok, err := identity.VerifyPassword(again, first.Admin)
	if err != nil || !ok {
		t.Fatal("original password must still verify")
	}
	ok, err = identity.VerifyPassword(again, second.Admin)
	if err != nil || ok {
		t.Fatal("the password from the second run must not replace the first")
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM users`); got != 5 {
		t.Fatalf("users = %d, want 5", got)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM classes`); got != 2 {
		t.Fatalf("classes = %d, want 2", got)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM memberships`); got != 4 {
		t.Fatalf("memberships = %d, want 4", got)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM labs`); got != 0 {
		t.Fatalf("labs = %d, want 0", got)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM submissions`); got != 0 {
		t.Fatalf("submissions = %d, want 0", got)
	}

	now := time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC)
	result, err := access.Login(ctx, store, access.LoginInput{
		Email: access.EmailAdmin, Password: first.Admin, RemoteHost: "192.0.2.40", Now: now,
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	var tokenHash, csrfHash []byte
	if err := store.pool.QueryRow(ctx, `
		SELECT token_hash, csrf_token_hash FROM sessions WHERE user_id = $1`, access.AdminID).Scan(&tokenHash, &csrfHash); err != nil {
		t.Fatalf("session: %v", err)
	}
	sum := sha256.Sum256([]byte(result.CookieToken))
	if !bytes.Equal(tokenHash, sum[:]) || bytes.Contains(tokenHash, []byte(result.CookieToken)) {
		t.Fatal("session row must store the token digest and not the cookie value")
	}
	csrfSum := sha256.Sum256([]byte(result.CSRF))
	if !bytes.Equal(csrfHash, csrfSum[:]) {
		t.Fatal("session row must store the csrf digest")
	}
	if result.ExpiresAt.Sub(now) != access.SessionTTL {
		t.Fatalf("expiresAt delta = %s", result.ExpiresAt.Sub(now))
	}

	if err := store.RemoveMembership(ctx, access.StudentA1ID, access.ClassAID, now); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := access.Provision(ctx, store, first); err != nil {
		t.Fatalf("provision after removal: %v", err)
	}
	var removed bool
	if err := store.pool.QueryRow(ctx, `
		SELECT removed_at IS NOT NULL FROM memberships
		WHERE user_id = $1 AND class_id = $2`, access.StudentA1ID, access.ClassAID).Scan(&removed); err != nil {
		t.Fatalf("removed flag: %v", err)
	}
	if !removed {
		t.Fatal("repeat provisioning resurrected a removed membership")
	}
	classes, err := store.Memberships(ctx, access.StudentA1ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 0 {
		t.Fatalf("current classes after removal = %d", len(classes))
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM memberships WHERE user_id = $1 AND class_id = $2`, access.StudentA1ID, access.ClassAID); got != 1 {
		t.Fatalf("membership rows = %d, want the original removed row only", got)
	}
}
