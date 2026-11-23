package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultLocalMaintenanceURL = "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"

// errPostgresUnavailable is the skip reason when no disposable server can be
// used. A skipped test is not a passing durability check.
var errPostgresUnavailable = errors.New("POSTGRES_URL is unset and no PostgreSQL is reachable on 127.0.0.1:5432; integration checks were not run")

func TestResolveMaintenanceURLUsesEnvironmentWithoutDialing(t *testing.T) {
	got, err := resolveMaintenanceURL(
		func(string) string { return "postgres://example.invalid/db" },
		func(string, string) (net.Conn, error) {
			t.Fatal("dialed even though POSTGRES_URL is set")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "postgres://example.invalid/db" {
		t.Fatalf("url = %q", got)
	}
}

func TestResolveMaintenanceURLSkipsWhenUnsetAndUnreachable(t *testing.T) {
	_, err := resolveMaintenanceURL(
		func(string) string { return "" },
		func(string, string) (net.Conn, error) {
			return nil, errors.New("connection refused")
		},
	)
	if !errors.Is(err, errPostgresUnavailable) {
		t.Fatalf("err = %v, want unavailable sentinel", err)
	}
}

func TestResolveMaintenanceURLUsesLoopbackDefaultWhenReachable(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	got, err := resolveMaintenanceURL(
		func(string) string { return "  " },
		func(string, string) (net.Conn, error) { return client, nil },
	)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != defaultLocalMaintenanceURL {
		t.Fatalf("url = %q", got)
	}
}

func TestDurableInitializationRollbackAndRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	store, databaseURL := openDisposable(t, ctx)
	if err := store.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("repeat initialize: %v", err)
	}
	assertSchema(t, ctx, store)

	keptID := newUUID(t)
	keptEmail := "disposable-" + keptID + "@example.test"
	if err := store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		return insertDisposableUser(ctx, tx, keptID, keptEmail)
	}); err != nil {
		t.Fatalf("commit disposable user: %v", err)
	}
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("initialize after write: %v", err)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM users WHERE id = $1`, keptID); got != 1 {
		t.Fatalf("repeat initialize kept %d rows, want 1", got)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM schema_migrations WHERE version = '001_init'`); got != 1 {
		t.Fatalf("schema_migrations rows = %d, want 1", got)
	}

	interruptedUser := newUUID(t)
	interruptedEmail := "interrupted-" + interruptedUser + "@example.test"
	err := store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		if err := insertDisposableUser(ctx, tx, interruptedUser, interruptedEmail); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `
			INSERT INTO sessions (user_id, token_hash, csrf_token_hash, expires_at)
			VALUES ($1, $2, $3, $4)`,
			interruptedUser, []byte("disposable-token-hash"), []byte("disposable-csrf-hash"), time.Now().Add(time.Hour).UTC(),
		); err != nil {
			return err
		}
		// Unique email collision aborts the transaction after both inserts.
		return insertDisposableUser(ctx, tx, newUUID(t), interruptedEmail)
	})
	if err == nil {
		t.Fatal("failed write returned nil")
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM users WHERE id = $1`, interruptedUser); got != 0 {
		t.Fatalf("rolled-back user still visible, count %d", got)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM sessions WHERE user_id = $1`, interruptedUser); got != 0 {
		t.Fatalf("rolled-back session still visible, count %d", got)
	}

	// A dropped connection is the process-exit case: the server aborts the
	// open transaction. Closing the pool while it still has the transaction
	// checked out would wait forever, so this uses a single connection.
	abandoned, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open abandoned connection: %v", err)
	}
	t.Cleanup(func() { _ = abandoned.Close(context.Background()) })
	abandonedUser := newUUID(t)
	abandonedTx, err := abandoned.Begin(ctx)
	if err != nil {
		t.Fatalf("begin abandoned: %v", err)
	}
	if _, err := abandonedTx.Exec(ctx, `
		INSERT INTO users (id, email, display_name, role, password_hash)
		VALUES ($1, $2, 'Disposable', 'student', 'disposable-not-a-credential')`,
		abandonedUser, "abandoned-"+abandonedUser+"@example.test"); err != nil {
		t.Fatalf("abandoned insert: %v", err)
	}
	if err := abandoned.Close(ctx); err != nil {
		t.Fatalf("close abandoned connection: %v", err)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM users WHERE id = $1`, abandonedUser); got != 0 {
		t.Fatalf("uncommitted row survived connection close, count %d", got)
	}

	store.Close()
	restarted, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(restarted.Close)
	if err := restarted.Ping(ctx); err != nil {
		t.Fatalf("ping after restart: %v", err)
	}
	if got := countRows(t, ctx, restarted, `SELECT count(*) FROM users WHERE email = $1`, keptEmail); got != 1 {
		t.Fatalf("committed row after new connection = %d, want 1", got)
	}
	if err := restarted.Initialize(ctx); err != nil {
		t.Fatalf("initialize on restarted store: %v", err)
	}
	if got := countRows(t, ctx, restarted, `SELECT count(*) FROM users WHERE email = $1`, keptEmail); got != 1 {
		t.Fatalf("committed row after restarted initialize = %d, want 1", got)
	}
}

func openDisposable(t *testing.T, ctx context.Context) (*Store, string) {
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

	admin, err := New(ctx, maintenance)
	if err != nil {
		if os.Getenv("POSTGRES_URL") == "" {
			t.Skipf("PostgreSQL on 127.0.0.1:5432 rejected the default disposable credentials: %v; set POSTGRES_URL. Integration checks were not run", err)
		}
		t.Fatalf("connect maintenance database: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		if os.Getenv("POSTGRES_URL") == "" {
			t.Skipf("PostgreSQL on 127.0.0.1:5432 did not accept the default disposable credentials: %v; set POSTGRES_URL. Integration checks were not run", err)
		}
		t.Fatalf("ping maintenance database: %v", err)
	}

	name := "leetkob_it_" + randomHex(t, 8)
	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.pool.Exec(dropCtx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, name)
		conn, acquireErr := admin.pool.Acquire(dropCtx)
		if acquireErr == nil {
			dropErr := execSimple(dropCtx, conn.Conn().PgConn(), "DROP DATABASE IF EXISTS "+quoteIdent(name))
			conn.Release()
			if dropErr != nil {
				t.Errorf("remove disposable database %s: %v", name, dropErr)
			}
		}
		admin.Close()
	})

	conn, err := admin.pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire maintenance connection: %v", err)
	}
	err = execSimple(ctx, conn.Conn().PgConn(), "CREATE DATABASE "+quoteIdent(name))
	conn.Release()
	if err != nil {
		t.Fatalf("create disposable database: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(maintenance)
	if err != nil {
		t.Fatalf("parse maintenance URL: %v", err)
	}
	cfg.ConnConfig.Database = name
	databaseURL := cfg.ConnString()
	store, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect disposable database: %v", err)
	}
	t.Cleanup(store.Close)
	return store, databaseURL
}

func assertSchema(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	rows, err := store.pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		got[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	for _, name := range requiredSchemaTables {
		if !got[name] {
			t.Fatalf("missing table %s", name)
		}
		delete(got, name)
	}
	for extra := range got {
		t.Fatalf("unexpected table %s", extra)
	}

	var triggers int
	if err := store.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_trigger g
		JOIN pg_class c ON c.oid = g.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND NOT g.tgisinternal`).Scan(&triggers); err != nil {
		t.Fatalf("count triggers: %v", err)
	}
	if triggers != 0 {
		t.Fatalf("public triggers = %d, want 0", triggers)
	}
}

var requiredSchemaTables = []string{
	"schema_migrations",
	"users",
	"sessions",
	"classes",
	"memberships",
	"labs",
	"lab_revisions",
	"lab_tests",
	"lab_constraints",
	"submissions",
	"counted_selections",
	"judge_verdicts",
	"judge_result_tests",
	"jobs",
	"findings",
	"finding_evidence",
	"decisions",
	"replays",
	"budget_reservations",
	"admission_leases",
	"auth_rate_limits",
}

func insertDisposableUser(ctx context.Context, tx *Tx, id, email string) error {
	return tx.Exec(ctx, `
		INSERT INTO users (id, email, display_name, role, password_hash)
		VALUES ($1, $2, 'Disposable', 'student', 'disposable-not-a-credential')`,
		id, email)
}

func countRows(t *testing.T, ctx context.Context, store *Store, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := store.pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	return n
}

func resolveMaintenanceURL(getenv func(string) string, dial func(string, string) (net.Conn, error)) (string, error) {
	if url := strings.TrimSpace(getenv("POSTGRES_URL")); url != "" {
		return url, nil
	}
	conn, err := dial("tcp", "127.0.0.1:5432")
	if err != nil {
		return "", errPostgresUnavailable
	}
	_ = conn.Close()
	return defaultLocalMaintenanceURL, nil
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func newUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("uuid: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random: %v", err)
	}
	return hex.EncodeToString(buf)
}
