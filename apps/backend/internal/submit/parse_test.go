package submit

import (
	"strings"
	"testing"
	"time"
)

func TestParseSourceBounds(t *testing.T) {
	got, err := ParseSource([]byte(`{"source":"int main(){return 0;}\n"}`))
	if err != nil || got != "int main(){return 0;}\n" {
		t.Fatalf("parse = %q, %v", got, err)
	}
	spaced, err := ParseSource([]byte("{\n  \"source\": \"int main(){return 0;}\\n\"\n}\n"))
	if err != nil || spaced != got {
		t.Fatalf("spaced = %q, %v", spaced, err)
	}
	if _, err := ParseSource([]byte(`{"source":"abc","extra":1}`)); err != ErrInvalid {
		t.Fatalf("unknown field err = %v", err)
	}
	if _, err := ParseSource([]byte(`{"source":"abc"}{"source":"def"}`)); err != ErrInvalid {
		t.Fatalf("trailing err = %v", err)
	}
	if _, err := ParseSource([]byte(`{"source":""}`)); err != ErrInvalid {
		t.Fatalf("empty err = %v", err)
	}
	if _, err := ParseSource([]byte(`{"source":"a\u0000b"}`)); err != ErrInvalid {
		t.Fatalf("nul err = %v", err)
	}
	exact := `{"source":"` + strings.Repeat("a", MaxSourceBytes) + `"}`
	if _, err := ParseSource([]byte(exact)); err != nil {
		t.Fatalf("exact ceiling: %v", err)
	}
	over := `{"source":"` + strings.Repeat("a", MaxSourceBytes+1) + `"}`
	if _, err := ParseSource([]byte(over)); err != ErrInvalid {
		t.Fatalf("over ceiling err = %v", err)
	}
}

func TestFormatTimeUsesMilliseconds(t *testing.T) {
	got := FormatTime(time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC))
	if got != "2026-11-25T15:00:00.000Z" {
		t.Fatalf("time = %s", got)
	}
}

func TestIdempotencyKeyAndUUID(t *testing.T) {
	if ValidIdempotencyKey("short") || !ValidIdempotencyKey("submit-0001") {
		t.Fatal("idempotency key rule changed")
	}
	if ValidIdempotencyKey("has space!") || ValidIdempotencyKey("new\nline1") {
		t.Fatal("idempotency key accepted non-visible characters")
	}
	if !ValidUUID("2c1b7b37-1a0f-4e4e-b3f2-6d8f8f0f2f12") || ValidUUID("not-a-uuid") {
		t.Fatal("uuid rule changed")
	}
}
