package access

import (
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/identity"
)

func TestSessionMaterialIsHashedAndDistinct(t *testing.T) {
	material, err := NewMaterial()
	if err != nil {
		t.Fatalf("material: %v", err)
	}
	if material.Token == "" || material.CSRF == "" || material.Token == material.CSRF {
		t.Fatal("token and csrf must both exist and differ")
	}
	if len(material.Token) < 40 || string(material.TokenHash) == material.Token {
		t.Fatal("stored form must be a digest, not the cookie value")
	}
	if !ProofMatches(material.CSRF, material.CSRFHash) {
		t.Fatal("csrf proof must match its digest")
	}
	if ProofMatches(material.Token, material.CSRFHash) || ProofMatches("", material.CSRFHash) {
		t.Fatal("a different or empty proof must not match")
	}
}

func TestExpiryBoundary(t *testing.T) {
	expires := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	if Expired(expires.Add(-time.Nanosecond), expires) {
		t.Fatal("a session is live before expiresAt")
	}
	if !Expired(expires, expires) || !Expired(expires.Add(time.Second), expires) {
		t.Fatal("a session is expired at and after expiresAt")
	}
	if expires.Add(SessionTTL).Sub(expires) != 8*time.Hour {
		t.Fatal("session lifetime must be eight hours")
	}
}

func TestOriginRule(t *testing.T) {
	const origin = "http://127.0.0.1:3000"
	if !OriginAllowed("", origin) {
		t.Fatal("a missing origin is not a different origin")
	}
	if !OriginAllowed(origin, origin) {
		t.Fatal("the configured origin must be accepted")
	}
	if OriginAllowed("http://evil.example", origin) || OriginAllowed("http://127.0.0.1:3000.evil.example", origin) {
		t.Fatal("a different origin must be rejected")
	}
	if OriginAllowed("http://evil.example", "") {
		t.Fatal("a present origin with no configured origin must be rejected")
	}
}

func TestDummyHashIsARealArgon2Encoding(t *testing.T) {
	ok, err := identity.VerifyPassword(dummyPasswordHash, "not-a-real-account")
	if err != nil || !ok {
		t.Fatalf("dummy encoding must verify its own input: ok=%v err=%v", ok, err)
	}
	ok, err = identity.VerifyPassword(dummyPasswordHash, "synthetic-admin-password")
	if err != nil || ok {
		t.Fatal("dummy encoding must not verify an account password")
	}
}
