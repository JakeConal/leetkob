package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JakeConal/leetkob/backend/db"
)

// initLockID serializes additive initialization across connections. It is
// not a domain rule; it only keeps concurrent startups from creating the
// same object at the same moment.
const initLockID int64 = 38021001

// Initialize applies the additive schema. A second call is a no-op for
// objects that already exist and does not remove or rewrite rows.
func (s *Store) Initialize(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("postgres: acquire: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", initLockID); err != nil {
		return fmt.Errorf("postgres: initialize lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", initLockID)
	}()

	if err := execSimple(ctx, conn.Conn().PgConn(), db.InitSQL); err != nil {
		return fmt.Errorf("postgres: initialize: %w", err)
	}
	return nil
}

func execSimple(ctx context.Context, pg *pgconn.PgConn, sql string) error {
	results, err := pg.Exec(ctx, sql).ReadAll()
	if err != nil {
		return err
	}
	for _, result := range results {
		if err := result.Err; err != nil {
			return err
		}
	}
	return nil
}
