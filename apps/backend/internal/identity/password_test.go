package identity

import (
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	encoded, err := HashPassword("synthetic-ta-password-1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if strings.Contains(encoded, "synthetic-ta-password-1") {
		t.Fatal("encoded hash must not contain the password")
	}
	ok, err := VerifyPassword(encoded, "synthetic-ta-password-1")
	if err != nil || !ok {
		t.Fatalf("correct password must verify: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword(encoded, "wrong")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("wrong password must not verify")
	}
}

func TestHashIsSalted(t *testing.T) {
	a, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	b, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if a == b {
		t.Fatal("two hashes of the same password must differ due to random salt")
	}
}

func TestVerifyMalformedEncoding(t *testing.T) {
	for _, encoded := range []string{
		"",
		"plaintext",
		"$argon2id$v=19$too-few-parts",
		"$argon2id$v=19$m=65536,t=1,p=2$not-base64!!$AAAA",
		"$argon2id$v=19$m=65536,t=1,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAA",
	} {
		if ok, err := VerifyPassword(encoded, "x"); ok || err == nil {
			t.Fatalf("malformed encoding %q must fail without verifying", encoded)
		}
	}
}
