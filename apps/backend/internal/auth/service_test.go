package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/auth"
	"github.com/JakeConal/leetkob/backend/internal/auth/authtest"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time { return c.now }

func newFixture(t *testing.T) (*auth.Service, *authtest.FakeStore, *fixedClock) {
	t.Helper()
	store := authtest.NewFakeStore()
	hash, err := identity.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	store.AddUser(
		auth.UserRecord{ID: "user-ta", Email: "ta@example.test", DisplayName: "TA One", Role: auth.RoleTA, PasswordHash: hash},
		auth.ClassSummary{ID: "class-a", Name: "Class A"},
	)
	clock := &fixedClock{now: time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)}
	service := auth.NewService(store, clock, identity.VerifyPassword)
	return service, store, clock
}

func TestLoginCreatesOpaqueSessionAndAuthenticates(t *testing.T) {
	service, store, _ := newFixture(t)
	ctx := context.Background()

	result, err := service.Login(ctx, "TA@example.test", "correct-horse-battery", "127.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.User.ID != "user-ta" || result.Token == "" || result.CSRFToken == "" {
		t.Fatalf("unexpected login result: %+v", result)
	}
	if result.Token == result.CSRFToken {
		t.Fatal("session token and CSRF token must differ")
	}
	// The store holds only digests: neither raw value may appear.
	for key := range store.Sessions {
		if strings.Contains(key, result.Token) || strings.Contains(key, result.CSRFToken) {
			t.Fatal("store must not contain raw tokens")
		}
	}

	identity, err := service.Authenticate(ctx, result.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if identity.User.ID != "user-ta" {
		t.Fatalf("identity user = %q, want user-ta", identity.User.ID)
	}
	if identity.SessionID == "" || identity.SessionID == identity.User.ID {
		t.Fatalf("session id %q must be its own identifier", identity.SessionID)
	}
}

func TestLoginFailuresAreGeneric(t *testing.T) {
	service, _, _ := newFixture(t)
	ctx := context.Background()

	_, wrongPassword := service.Login(ctx, "ta@example.test", "wrong", "127.0.0.1")
	_, unknownAccount := service.Login(ctx, "nobody@example.test", "correct-horse-battery", "127.0.0.1")
	if !errors.Is(wrongPassword, auth.ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v, want ErrInvalidCredentials", wrongPassword)
	}
	if !errors.Is(unknownAccount, auth.ErrInvalidCredentials) {
		t.Fatalf("unknown account err = %v, want ErrInvalidCredentials", unknownAccount)
	}
}

func TestLoginIsRateLimitedPerIP(t *testing.T) {
	service, _, _ := newFixture(t)
	ctx := context.Background()

	for i := 0; i < auth.IPLimit; i++ {
		if _, err := service.Login(ctx, "someone@example.test", "x", "10.0.0.1"); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("attempt %d err = %v, want generic failure", i+1, err)
		}
	}
	_, err := service.Login(ctx, "someone@example.test", "x", "10.0.0.1")
	var limited *auth.RateLimitedError
	if !errors.As(err, &limited) {
		t.Fatalf("11th attempt err = %v, want RateLimitedError", err)
	}
	if limited.RetryAfter <= 0 {
		t.Fatalf("retry after = %v, want positive", limited.RetryAfter)
	}
}

func TestLoginIsRateLimitedPerAccountAcrossIPs(t *testing.T) {
	service, _, _ := newFixture(t)
	ctx := context.Background()

	for i := 0; i < auth.AccountLimit; i++ {
		_, _ = service.Login(ctx, "ta@example.test", "wrong", "10.0.0."+itoa(i))
	}
	_, err := service.Login(ctx, "ta@example.test", "correct-horse-battery", "10.9.9.9")
	var limited *auth.RateLimitedError
	if !errors.As(err, &limited) {
		t.Fatalf("err = %v, want RateLimitedError even with the right password", err)
	}
}

func TestSessionExpiryAndLogoutInvalidate(t *testing.T) {
	service, _, clock := newFixture(t)
	ctx := context.Background()

	result, err := service.Login(ctx, "ta@example.test", "correct-horse-battery", "127.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	clock.now = clock.now.Add(auth.SessionLifetime)
	if _, err := service.Authenticate(ctx, result.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("expired session err = %v, want ErrUnauthenticated", err)
	}

	clock.now = clock.now.Add(-auth.SessionLifetime / 2) // back inside the lifetime
	identity, err := service.Authenticate(ctx, result.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if err := service.Logout(ctx, identity); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := service.Authenticate(ctx, result.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("revoked session err = %v, want ErrUnauthenticated", err)
	}
}

func TestCSRFVerification(t *testing.T) {
	service, _, _ := newFixture(t)
	ctx := context.Background()

	result, err := service.Login(ctx, "ta@example.test", "correct-horse-battery", "127.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	identity, err := service.Authenticate(ctx, result.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	if ok, err := service.VerifyCSRF(ctx, identity, result.CSRFToken); err != nil || !ok {
		t.Fatalf("correct CSRF verdict ok=%v err=%v", ok, err)
	}
	if ok, _ := service.VerifyCSRF(ctx, identity, "wrong-token"); ok {
		t.Fatal("wrong CSRF token must not verify")
	}
	if ok, _ := service.VerifyCSRF(ctx, identity, ""); ok {
		t.Fatal("empty CSRF token must not verify")
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := "0123456789"
	out := ""
	for value > 0 {
		out = string(digits[value%10]) + out
		value /= 10
	}
	return out
}
