package sandbox

import (
	"bytes"
	"testing"
)

func TestLimitedBufferCapsAndDrains(t *testing.T) {
	buffer := newLimitedBuffer(10)

	n, err := buffer.Write([]byte("0123456789ABCDEF"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 16 {
		t.Fatalf("Write accepted %d bytes, want 16 (the writer must drain)", n)
	}
	if got := buffer.String(); got != "0123456789" {
		t.Fatalf("collected %q, want first 10 bytes", got)
	}
	if !buffer.Truncated() {
		t.Fatal("overflow was not recorded")
	}
	if buffer.Len() != 10 {
		t.Fatalf("collected length = %d, want 10", buffer.Len())
	}

	// A second large write must keep draining and never exceed the cap.
	if _, err := buffer.Write(bytes.Repeat([]byte("x"), 1<<20)); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if buffer.Len() != 10 {
		t.Fatalf("collected length after flood = %d, want 10", buffer.Len())
	}
}

func TestLimitedBufferUnderCapIsNotTruncated(t *testing.T) {
	buffer := newLimitedBuffer(32)
	if _, err := buffer.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buffer.Truncated() {
		t.Fatal("under-cap write reported truncation")
	}
	if got := buffer.String(); got != "hello" {
		t.Fatalf("collected %q", got)
	}
}
