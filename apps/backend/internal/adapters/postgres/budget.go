package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

// Budget is the PostgreSQL ReservationStore. A reservation is committed
// before the caller dispatches, and the charged total cannot pass the
// smaller of the caller cap and the lab's budget_cap_micros. New labs
// default that column to USD 0.25 (250000 micro-dollars).
type Budget struct {
	Store *Store
	// Optional deployment limits also bound free-tier calls. They count every
	// reservation, including retries and failures, and survive worker restarts.
	MaxLabCalls   int64
	MaxDailyCalls int64
}

var _ semantic.ReservationStore = (*Budget)(nil)

const chargedSQL = `
SELECT COALESCE((
    SELECT SUM(
        CASE state
            WHEN 'open' THEN reserved_micros
            WHEN 'settled' THEN settled_micros
            WHEN 'kept' THEN reserved_micros
            ELSE 0
        END
    )::bigint
    FROM budget_reservations
    WHERE lab_id = $1::uuid
), 0)`

// Reserve holds amountMicroUSD against the lab. The hold is visible to
// Charged before this method returns.
func (b *Budget) Reserve(ctx context.Context, labID string, amountMicroUSD, capMicroUSD int64) (semantic.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return semantic.Reservation{}, err
	}
	if b == nil || b.Store == nil {
		return semantic.Reservation{}, errors.New("postgres: budget store is not configured")
	}
	if labID == "" || amountMicroUSD < 0 || capMicroUSD < 0 {
		return semantic.Reservation{}, fmt.Errorf("postgres: invalid reservation")
	}
	var reserved semantic.Reservation
	err := b.Store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		if b.MaxDailyCalls > 0 {
			// Serialize the global quota before taking a lab lock, so different
			// labs cannot both consume the final daily request allowance.
			if err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(73402183)`); err != nil {
				return err
			}
		}
		var labCap int64
		err := tx.QueryRow(ctx, `SELECT budget_cap_micros FROM labs WHERE id = $1::uuid FOR UPDATE`, labID).Scan(&labCap)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("postgres: lab not found")
		}
		if err != nil {
			return err
		}
		if b.MaxLabCalls > 0 || b.MaxDailyCalls > 0 {
			var labCalls, dailyCalls int64
			if err := tx.QueryRow(ctx, `
				SELECT count(*) FILTER (WHERE lab_id=$1::uuid),
				       count(*) FILTER (WHERE created_at >= date_trunc('day', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')
				FROM budget_reservations`, labID).Scan(&labCalls, &dailyCalls); err != nil {
				return err
			}
			if (b.MaxLabCalls > 0 && labCalls >= b.MaxLabCalls) || (b.MaxDailyCalls > 0 && dailyCalls >= b.MaxDailyCalls) {
				return semantic.ErrBudgetExceeded
			}
		}
		var charged int64
		if err := tx.QueryRow(ctx, chargedSQL, labID).Scan(&charged); err != nil {
			return err
		}
		limit := capMicroUSD
		if labCap < limit {
			limit = labCap
		}
		if limit < 0 || amountMicroUSD > limit || charged > limit-amountMicroUSD {
			return semantic.ErrBudgetExceeded
		}
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO budget_reservations (lab_id, reserved_micros, state)
			VALUES ($1::uuid, $2, 'open')
			RETURNING id::text`, labID, amountMicroUSD).Scan(&id); err != nil {
			return err
		}
		reserved = semantic.Reservation{ID: id, LabID: labID, AmountMicroUSD: amountMicroUSD}
		return nil
	})
	if err != nil {
		return semantic.Reservation{}, err
	}
	return reserved, nil
}

// Settle charges actualMicroUSD and releases the rest of an open hold.
func (b *Budget) Settle(ctx context.Context, reservationID string, actualMicroUSD int64) error {
	return b.close(ctx, reservationID, func(reserved int64) (string, any, error) {
		if actualMicroUSD < 0 || actualMicroUSD > reserved {
			return "", nil, fmt.Errorf("postgres: settled usage exceeds the reservation")
		}
		return "settled", actualMicroUSD, nil
	})
}

// Keep charges the full hold. Unknown provider usage uses this.
func (b *Budget) Keep(ctx context.Context, reservationID string) error {
	return b.close(ctx, reservationID, func(reserved int64) (string, any, error) {
		return "kept", reserved, nil
	})
}

// Release drops a hold that never dispatched.
func (b *Budget) Release(ctx context.Context, reservationID string) error {
	return b.close(ctx, reservationID, func(int64) (string, any, error) {
		return "released", nil, nil
	})
}

// Charged is settled usage plus still-open holds for the lab.
func (b *Budget) Charged(ctx context.Context, labID string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if b == nil || b.Store == nil {
		return 0, errors.New("postgres: budget store is not configured")
	}
	var total int64
	if err := b.Store.pool.QueryRow(ctx, chargedSQL, labID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (b *Budget) close(ctx context.Context, reservationID string, next func(reserved int64) (string, any, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b == nil || b.Store == nil {
		return errors.New("postgres: budget store is not configured")
	}
	if reservationID == "" {
		return semantic.ErrReservationNotFound
	}
	return b.Store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		var reserved int64
		var state string
		err := tx.QueryRow(ctx, `
			SELECT reserved_micros, state
			FROM budget_reservations
			WHERE id = $1::uuid
			FOR UPDATE`, reservationID).Scan(&reserved, &state)
		if errors.Is(err, pgx.ErrNoRows) {
			return semantic.ErrReservationNotFound
		}
		if err != nil {
			return err
		}
		if state != "open" {
			return fmt.Errorf("postgres: reservation %s is already closed", reservationID)
		}
		state, settled, err := next(reserved)
		if err != nil {
			return err
		}
		tag, err := tx.pg.Exec(ctx, `
			UPDATE budget_reservations
			SET state = $2, settled_micros = $3
			WHERE id = $1::uuid AND state = 'open'`, reservationID, state, settled)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return semantic.ErrReservationNotFound
		}
		return nil
	})
}
