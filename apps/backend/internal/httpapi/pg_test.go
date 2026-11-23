package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
)

const defaultMaintenanceURL = "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"

func openAPIStore(t *testing.T) *postgres.Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	maintenance := strings.TrimSpace(os.Getenv("POSTGRES_URL"))
	if maintenance == "" {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:5432", 250*time.Millisecond)
		if err != nil {
			t.Skip("POSTGRES_URL is unset and no PostgreSQL is reachable on 127.0.0.1:5432; integration checks were not run")
		}
		_ = conn.Close()
		maintenance = defaultMaintenanceURL
	}

	admin, err := pgx.Connect(ctx, maintenance)
	if err != nil {
		if os.Getenv("POSTGRES_URL") == "" {
			t.Skipf("PostgreSQL on 127.0.0.1:5432 rejected the default disposable credentials: %v; set POSTGRES_URL. Integration checks were not run", err)
		}
		t.Fatalf("connect maintenance database: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		_ = admin.Close(ctx)
		if os.Getenv("POSTGRES_URL") == "" {
			t.Skipf("PostgreSQL on 127.0.0.1:5432 did not accept the default disposable credentials: %v; set POSTGRES_URL. Integration checks were not run", err)
		}
		t.Fatalf("ping maintenance database: %v", err)
	}

	name := "leetkob_it_" + randomHex(t, 8)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoteIdent(name)); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create disposable database: %v", err)
	}

	// pgx ConnString returns the URL that was parsed, not a rebuild after
	// Database is overwritten. Replace the path so the pool opens the new
	// database instead of the maintenance database.
	databaseURL, err := urlWithDatabase(maintenance, name)
	if err != nil {
		t.Fatalf("disposable URL: %v", err)
	}
	store, err := postgres.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect disposable database: %v", err)
	}
	if err := store.Initialize(ctx); err != nil {
		store.Close()
		t.Fatalf("initialize: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_, _ = admin.Exec(dropCtx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, name)
		if _, err := admin.Exec(dropCtx, "DROP DATABASE IF EXISTS "+quoteIdent(name)); err != nil {
			t.Errorf("remove disposable database %s: %v", name, err)
		}
		_ = admin.Close(dropCtx)
	})
	return store
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
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

func randomHex(t *testing.T, n int) string {
	t.Helper()
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(buf)
}
