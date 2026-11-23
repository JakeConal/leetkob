package postgres

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// openIsolated creates an empty database and returns a store connected to
// it. pgx ConnString returns the URL that was parsed, so changing
// Config.Database and calling ConnString still opens the maintenance
// database. The name is written into the URL path instead.
func openIsolated(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	maintenance, err := resolveMaintenanceURL(os.Getenv, func(network, addr string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 250 * time.Millisecond}).Dial(network, addr)
	})
	if errors.Is(err, errPostgresUnavailable) {
		t.Skip(err.Error())
	}
	if err != nil {
		t.Fatalf("resolve maintenance URL: %v", err)
	}

	admin, err := pgx.Connect(ctx, maintenance)
	if err != nil {
		if os.Getenv("POSTGRES_URL") == "" {
			t.Skipf("PostgreSQL on 127.0.0.1:5432 rejected the default disposable credentials: %v; set POSTGRES_URL. Integration checks were not run", err)
		}
		t.Fatalf("connect maintenance database: %v", err)
	}
	name := "leetkob_it_" + randomHex(t, 8)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoteIdent(name)); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create disposable database: %v", err)
	}
	databaseURL, err := urlWithDatabase(maintenance, name)
	if err != nil {
		t.Fatalf("disposable URL: %v", err)
	}
	store, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect disposable database: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(dropCtx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, name)
		if _, err := admin.Exec(dropCtx, "DROP DATABASE IF EXISTS "+quoteIdent(name)); err != nil {
			t.Errorf("remove disposable database %s: %v", name, err)
		}
		_ = admin.Close(dropCtx)
	})
	return store
}

func urlWithDatabase(raw, name string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("maintenance target must be a postgres URL")
	}
	parsed.Path = "/" + name
	return parsed.String(), nil
}
