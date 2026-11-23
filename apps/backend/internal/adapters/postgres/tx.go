package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TxFunc is one unit of domain work executed inside a single database
// transaction. Returning an error rolls the whole unit back.
type TxFunc func(ctx context.Context, tx pgx.Tx) error

// InTx runs fn inside one serializable-by-default transaction. Domain
// ports receive the transaction and never open nested ones; the adapter
// owns commit/rollback so HTTP handlers and model adapters cannot leak
// partial writes.
func (s *Store) InTx(ctx context.Context, fn TxFunc) error {
	if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(ctx, tx)
	}); err != nil {
		return fmt.Errorf("postgres: transaction: %w", err)
	}
	return nil
}
