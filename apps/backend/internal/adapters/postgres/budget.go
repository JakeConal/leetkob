package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/budget"
)

// Reserve inserts a reservation when the lab's settled plus held spend
// still leaves room for the conservative maximum. The lab row is locked,
// so concurrent reservations serialize per lab and can never overshoot.
func (s *Store) Reserve(ctx context.Context, labID string, amountCents, capCents int, now time.Time) (budget.Reservation, error) {
	if amountCents < 0 || capCents < 0 {
		return budget.Reservation{}, fmt.Errorf("postgres: invalid reservation amount")
	}
	var reservation budget.Reservation
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var lockedID string
		if err := tx.QueryRow(ctx, `SELECT id FROM labs WHERE id = $1 FOR UPDATE`, labID).Scan(&lockedID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return budget.ErrReservationNotFound
			}
			return err
		}
		var committed int
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(sum(CASE state WHEN 'settled' THEN settled_cents
			                            WHEN 'reserved' THEN reserved_cents ELSE 0 END), 0)
			FROM budget_reservations WHERE lab_id = $1`, labID).Scan(&committed); err != nil {
			return err
		}
		if committed+amountCents > capCents {
			return budget.ErrCapExceeded
		}
		reservation.LabID = labID
		reservation.ReservedCents = amountCents
		if err := tx.QueryRow(ctx, `
			INSERT INTO budget_reservations (id, lab_id, reserved_cents, state, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, 'reserved', $3, $3)
			RETURNING id, created_at`, labID, amountCents, now,
		).Scan(&reservation.ID, &reservation.CreatedAt); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return budget.Reservation{}, err
	}
	return reservation, nil
}

// Settle reconciles one reservation with its measured cost.
func (s *Store) Settle(ctx context.Context, reservationID string, settledCents int, providerRequestID string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE budget_reservations
		SET state = 'settled', settled_cents = $2, provider_request_id = $3, updated_at = $4
		WHERE id = $1 AND state = 'reserved'`,
		reservationID, settledCents, providerRequestID, now)
	if err != nil {
		return fmt.Errorf("postgres: settle reservation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return budget.ErrReservationNotFound
	}
	return nil
}

// Release returns a reservation that never dispatched.
func (s *Store) Release(ctx context.Context, reservationID string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE budget_reservations
		SET state = 'released', settled_cents = 0, updated_at = $2
		WHERE id = $1 AND state = 'reserved'`, reservationID, now)
	if err != nil {
		return fmt.Errorf("postgres: release reservation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return budget.ErrReservationNotFound
	}
	return nil
}

// CommittedCents reports settled plus currently held spend for one lab.
func (s *Store) CommittedCents(ctx context.Context, labID string) (int, error) {
	var committed int
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(CASE state WHEN 'settled' THEN settled_cents
		                            WHEN 'reserved' THEN reserved_cents ELSE 0 END), 0)
		FROM budget_reservations WHERE lab_id = $1`, labID).Scan(&committed); err != nil {
		return 0, fmt.Errorf("postgres: committed cents: %w", err)
	}
	return committed, nil
}

// compile-time assertion that Store satisfies the budget port.
var _ budget.Store = (*Store)(nil)
