package budget_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/budget"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

// fakeStore models the cap arithmetic with a mutex, the way the durable
// adapter serializes per lab.
type fakeStore struct {
	mu    sync.Mutex
	spent map[string]int
	held  map[string]heldReservation
	seq   int
}

type heldReservation struct {
	labID string
	cents int
}

func newFakeStore() *fakeStore {
	return &fakeStore{spent: map[string]int{}, held: map[string]heldReservation{}}
}

func (f *fakeStore) Reserve(_ context.Context, labID string, amount, cap int, now time.Time) (budget.Reservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.spent[labID]+amount > cap {
		return budget.Reservation{}, budget.ErrCapExceeded
	}
	f.spent[labID] += amount
	f.seq++
	id := "res-" + string(rune('a'+f.seq))
	f.held[id] = heldReservation{labID: labID, cents: amount}
	return budget.Reservation{ID: id, LabID: labID, ReservedCents: amount, CreatedAt: now}, nil
}

func (f *fakeStore) Settle(_ context.Context, id string, settled int, _ string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	reservation, ok := f.held[id]
	if !ok {
		return budget.ErrReservationNotFound
	}
	f.spent[reservation.labID] += settled - reservation.cents
	delete(f.held, id)
	return nil
}

func (f *fakeStore) Release(_ context.Context, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	reservation, ok := f.held[id]
	if !ok {
		return budget.ErrReservationNotFound
	}
	f.spent[reservation.labID] -= reservation.cents
	delete(f.held, id)
	return nil
}

func (f *fakeStore) CommittedCents(_ context.Context, labID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.spent[labID], nil
}

func TestMaxCostRoundsUpAndRefusesUnknownPrice(t *testing.T) {
	service := budget.NewService(nil, fixedClock{}, budget.PriceSchedule{InputCentsPer1K: 3, OutputCentsPer1K: 15})
	cost, err := service.MaxCost(1000, 1000)
	if err != nil || cost != 18 {
		t.Fatalf("MaxCost(1000,1000) = %d, %v, want 18", cost, err)
	}
	// Any partial unit rounds up: a partial 1K unit costs one cent per
	// component, so a one-token call is charged 2 cents here.
	partial, err := service.MaxCost(1, 1)
	if err != nil || partial != 2 {
		t.Fatalf("MaxCost(1,1) = %d, %v, want 2 (rounded up)", partial, err)
	}
	if _, err := budget.NewService(nil, fixedClock{}, budget.PriceSchedule{}).MaxCost(100, 100); !errors.Is(err, budget.ErrUnknownPrice) {
		t.Fatalf("unknown price err = %v, want ErrUnknownPrice", err)
	}
}

func TestReserveAndSettleReleasesDifference(t *testing.T) {
	store := newFakeStore()
	service := budget.NewService(store, fixedClock{}, budget.PriceSchedule{InputCentsPer1K: 1, OutputCentsPer1K: 1})
	ctx := context.Background()
	service.CapCents = 10

	reservation, err := service.ReserveForCall(ctx, "lab", 2000, 2000) // max 4 cents
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if reservation.ReservedCents != 4 {
		t.Fatalf("reserved = %d, want 4", reservation.ReservedCents)
	}
	if err := service.Settle(ctx, reservation, budget.Usage{InputTokens: 500, OutputTokens: 500, Known: true}, "req-1"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	committed, err := service.CommittedCents(ctx, "lab")
	if err != nil {
		t.Fatalf("committed: %v", err)
	}
	if committed != 2 {
		t.Fatalf("committed = %d, want 2 after settling half the reservation", committed)
	}
}

func TestUnknownUsageStaysChargedAtReservedAmount(t *testing.T) {
	store := newFakeStore()
	service := budget.NewService(store, fixedClock{}, budget.PriceSchedule{InputCentsPer1K: 1, OutputCentsPer1K: 1})
	ctx := context.Background()
	service.CapCents = 10

	reservation, err := service.ReserveForCall(ctx, "lab", 1000, 1000) // 2 cents
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := service.Settle(ctx, reservation, budget.Usage{Known: false}, "req-2"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	committed, _ := service.CommittedCents(ctx, "lab")
	if committed != 2 {
		t.Fatalf("committed = %d, want the full reservation when usage is unknown", committed)
	}
}

func TestConcurrentReservationsCannotExceedCap(t *testing.T) {
	store := newFakeStore()
	service := budget.NewService(store, fixedClock{}, budget.PriceSchedule{InputCentsPer1K: 1, OutputCentsPer1K: 1})
	ctx := context.Background()
	service.CapCents = 10

	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.ReserveForCall(ctx, "lab", 1000, 1000) // 2 cents each
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, budget.ErrCapExceeded) {
			t.Fatalf("unexpected reserve error: %v", err)
		}
	}
	if successes != 5 {
		t.Fatalf("successful reservations = %d, want exactly 5 within the 10-cent cap", successes)
	}
	committed, _ := service.CommittedCents(ctx, "lab")
	if committed > 10 {
		t.Fatalf("committed = %d exceeds the cap", committed)
	}
}

func TestReleaseReturnsUnusedBudget(t *testing.T) {
	store := newFakeStore()
	service := budget.NewService(store, fixedClock{}, budget.PriceSchedule{InputCentsPer1K: 1, OutputCentsPer1K: 1})
	ctx := context.Background()
	service.CapCents = 10

	reservation, err := service.ReserveForCall(ctx, "lab", 1000, 1000)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := service.Release(ctx, reservation); err != nil {
		t.Fatalf("release: %v", err)
	}
	committed, _ := service.CommittedCents(ctx, "lab")
	if committed != 0 {
		t.Fatalf("committed after release = %d, want 0", committed)
	}
}
