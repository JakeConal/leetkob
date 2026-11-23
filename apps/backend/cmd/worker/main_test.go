package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestRunRequiresDatabaseURL(t *testing.T) {
	err := run(context.Background(), slog.New(slog.DiscardHandler), func(string) string { return "  " })
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v", err)
	}
}
