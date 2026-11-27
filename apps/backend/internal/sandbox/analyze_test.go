package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAnalyzeRejectsInvalidSourceBeforeDocker(t *testing.T) {
	c := &Controller{Docker: "/not-a-docker-command"}
	for _, source := range []string{"", strings.Repeat("x", MaxSourceBytes+1)} {
		if _, err := c.Analyze(context.Background(), source); !errors.Is(err, ErrPolicy) {
			t.Fatalf("source size %d: expected policy refusal, got %v", len(source), err)
		}
	}
}
