package sandbox

import (
	"bytes"
	"strings"
	"testing"
)

func TestCollectLimitedStopsAtTheCap(t *testing.T) {
	out, errOut, truncated := collectLimited(
		strings.NewReader(strings.Repeat("A", 5000)+"OUTPUT_DONE\n"),
		strings.NewReader("ERR"),
		100,
	)
	if !truncated {
		t.Fatal("cap was not enforced")
	}
	if len(out)+len(errOut) > 100 {
		t.Fatalf("retained %d bytes", len(out)+len(errOut))
	}
	if bytes.Contains(out, []byte("OUTPUT_DONE")) || bytes.Contains(errOut, []byte("OUTPUT_DONE")) {
		t.Fatal("marker survived a cap that should have excluded it")
	}
}

func TestCollectLimitedKeepsShortOutput(t *testing.T) {
	out, errOut, truncated := collectLimited(
		strings.NewReader("OUTPUT_DONE\n"),
		strings.NewReader(""),
		1000,
	)
	if truncated {
		t.Fatal("short output was truncated")
	}
	if !bytes.Contains(out, []byte("OUTPUT_DONE")) {
		t.Fatalf("stdout = %q, stderr = %q", out, errOut)
	}
}
