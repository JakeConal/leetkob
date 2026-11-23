package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/budget"
)

func TestBudgetReservationsEnforceTheCapAtomically(t *testing.T) {
	pool := migratedPool(t)
	seedIdentity(t, pool)
	lab := testLab("dddddddd-dddd-4ddd-8ddd-dddddddddddd", classID, taID)
	if err := (&Store{pool: pool}).CreateLab(context.Background(), lab); err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	store := &Store{pool: pool}
	service := budget.NewService(store, budget.SystemClock{}, budget.PriceSchedule{InputCentsPer1K: 1, OutputCentsPer1K: 1})
	service.CapCents = 10
	ctx := context.Background()

	first, err := service.ReserveForCall(ctx, lab.ID, 2000, 2000) // 4 cents
	if err != nil {
		t.Fatalf("first reservation: %v", err)
	}
	second, err := service.ReserveForCall(ctx, lab.ID, 2000, 2000)
	if err != nil {
		t.Fatalf("second reservation: %v", err)
	}
	third, err := service.ReserveForCall(ctx, lab.ID, 3000, 3000) // 6 cents: 4+4+6 > 10
	if !errors.Is(err, budget.ErrCapExceeded) {
		t.Fatalf("third reservation err = %v, want ErrCapExceeded", err)
	}
	_ = third

	if err := service.Settle(ctx, first, budget.Usage{InputTokens: 500, OutputTokens: 500, Known: true}, "req-1"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := service.Release(ctx, second); err != nil {
		t.Fatalf("release: %v", err)
	}
	committed, err := store.CommittedCents(ctx, lab.ID)
	if err != nil {
		t.Fatalf("committed: %v", err)
	}
	// first settled at 2 cents; second was released before dispatch.
	if committed != 2 {
		t.Fatalf("committed = %d, want 2", committed)
	}
}

func TestConcurrentReservationsNeverOvershootTheCap(t *testing.T) {
	pool := migratedPool(t)
	seedIdentity(t, pool)
	lab := testLab("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", classID, taID)
	if err := (&Store{pool: pool}).CreateLab(context.Background(), lab); err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	store := &Store{pool: pool}
	ctx := context.Background()

	const capCents = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	var unexpected []error
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Reserve(ctx, lab.ID, 2, capCents, time.Now())
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				successes++
			case errors.Is(err, budget.ErrCapExceeded):
			default:
				unexpected = append(unexpected, err)
			}
		}()
	}
	wg.Wait()
	if len(unexpected) > 0 {
		t.Fatalf("unexpected errors: %v", unexpected)
	}
	if successes != 5 {
		t.Fatalf("successful reservations = %d, want exactly 5 within a 10-cent cap at 2 cents each", successes)
	}
	committed, err := store.CommittedCents(ctx, lab.ID)
	if err != nil {
		t.Fatalf("committed: %v", err)
	}
	if committed > capCents {
		t.Fatalf("committed %d exceeds the %d-cent cap", committed, capCents)
	}
}

func TestUnknownUsageKeepsTheReservedCharge(t *testing.T) {
	pool := migratedPool(t)
	seedIdentity(t, pool)
	lab := testLab("ffffffff-ffff-4fff-8fff-ffffffffffff", classID, taID)
	if err := (&Store{pool: pool}).CreateLab(context.Background(), lab); err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	store := &Store{pool: pool}
	service := budget.NewService(store, budget.SystemClock{}, budget.PriceSchedule{InputCentsPer1K: 1, OutputCentsPer1K: 1})
	ctx := context.Background()

	reservation, err := service.ReserveForCall(ctx, lab.ID, 1000, 1000)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := service.Settle(ctx, reservation, budget.Usage{Known: false}, "req-unknown"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	committed, err := store.CommittedCents(ctx, lab.ID)
	if err != nil {
		t.Fatalf("committed: %v", err)
	}
	if committed != reservation.ReservedCents {
		t.Fatalf("committed = %d, want the full reserved %d when usage is unknown", committed, reservation.ReservedCents)
	}
}
