// Package postgres holds the durable store adapter shared by the API and
// worker commands. Initialization is additive. Transactions commit only
// when the caller says so, so an interrupted write is not visible after
// restart. Health still reports 503 when the configured database is
// unreachable, per the PA#3 contract's unavailable problem type.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pinger reports whether the durable store currently accepts connections.
type Pinger interface {
	Ping(ctx context.Context) error
}

// PingFunc adapts a function to Pinger so tests inject failures without a
// database.
type PingFunc func(ctx context.Context) error

// Ping implements Pinger.
func (f PingFunc) Ping(ctx context.Context) error { return f(ctx) }

// Store wraps the connection pool and applies the additive schema.
type Store struct {
	pool *pgxpool.Pool
}

// New connects a bounded pool to the given URL. The pool is small because
// the API and worker each keep only a handful of connections open.
func New(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse url: %w", err)
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Ping verifies the store accepts a connection within the context.
func (s *Store) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.pool.Ping(pingCtx)
}

// Close releases all pool connections. Safe to call more than once.
// Transactions must be committed or rolled back first; Close waits until
// checked-out connections are returned.
func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}
