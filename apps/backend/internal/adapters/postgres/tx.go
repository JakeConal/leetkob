package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/domain"
)

// Tx is one database transaction. Commit makes its writes durable.
// Rollback discards them. Closing the process before Commit leaves no
// partial writes for a later connection to read.
type Tx struct {
	pg   pgx.Tx
	done bool
}

var (
	_ domain.Transaction = (*Tx)(nil)
	_ domain.Transactor  = (*Store)(nil)
)

// Begin starts a transaction. The caller decides Commit or Rollback.
func (s *Store) Begin(ctx context.Context) (domain.Transaction, error) {
	return s.begin(ctx)
}

func (s *Store) begin(ctx context.Context) (*Tx, error) {
	pg, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: begin: %w", err)
	}
	return &Tx{pg: pg}, nil
}

// Commit persists the transaction. A second call returns an error.
func (t *Tx) Commit(ctx context.Context) error {
	if t.done {
		return errors.New("postgres: transaction already finished")
	}
	t.done = true
	if err := t.pg.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	return nil
}

// Rollback discards the transaction. It is safe after Commit.
func (t *Tx) Rollback(ctx context.Context) error {
	if t.done {
		return nil
	}
	t.done = true
	if err := t.pg.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("postgres: rollback: %w", err)
	}
	return nil
}

// Exec runs one statement inside the transaction. Later repositories use
// it; task 2.1 tests use it for disposable rows.
func (t *Tx) Exec(ctx context.Context, sql string, args ...any) error {
	if _, err := t.pg.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("postgres: exec: %w", err)
	}
	return nil
}

// QueryRow reads one row inside the transaction.
func (t *Tx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return t.pg.QueryRow(ctx, sql, args...)
}

// Within runs fn in a transaction. An error from fn, or a failure before
// Commit, rolls the writes back. Commit runs only when fn returns nil.
func (s *Store) Within(ctx context.Context, fn func(context.Context, *Tx) error) (err error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && err == nil {
			err = rbErr
		}
	}()
	if err = fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
