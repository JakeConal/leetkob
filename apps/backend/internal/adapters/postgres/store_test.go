package postgres

import (
	"context"
	"testing"
)

func TestNewRejectsMalformedURL(t *testing.T) {
	_, err := New(context.Background(), "not-a-postgres-url")
	if err == nil {
		t.Fatal("malformed URL must be rejected before a connection is attempted")
	}
}

func TestStoreImplementsHealthPinger(t *testing.T) {
	var _ Pinger = (*Store)(nil)
	var ping Pinger = PingFunc(func(context.Context) error { return nil })
	if err := ping.Ping(context.Background()); err != nil {
		t.Fatalf("ping func: %v", err)
	}
}
