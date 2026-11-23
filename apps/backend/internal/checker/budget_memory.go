package checker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/budget"
)

// memoryBudgetStore is the run-scoped budget.Store implementation used by the
// marking command. It is deliberately not durable: one command run is one
// spend bucket, while the worker supplies the PostgreSQL store. Reserve is
// atomic under a mutex, so concurrent calls cannot overshoot the lab cap.
type memoryBudgetStore struct {
	mu           sync.Mutex
	committed    int
	reservations map[string]int
}

func newMemoryBudgetStore() *memoryBudgetStore {
	return &memoryBudgetStore{reservations: make(map[string]int)}
}

func (s *memoryBudgetStore) Reserve(_ context.Context, labID string, amountCents, capCents int, now time.Time) (budget.Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if amountCents < 0 {
		return budget.Reservation{}, fmt.Errorf("budget: negative reservation")
	}
	if capCents <= 0 {
		capCents = budget.DefaultLabCapCents
	}
	if s.committed+amountCents > capCents {
		return budget.Reservation{}, budget.ErrCapExceeded
	}
	id := fmt.Sprintf("mem-%s-%d", labID, now.UnixNano())
	s.reservations[id] = amountCents
	return budget.Reservation{
		ID:            id,
		LabID:         labID,
		ReservedCents: amountCents,
		CreatedAt:     now,
	}, nil
}

func (s *memoryBudgetStore) Settle(_ context.Context, reservationID string, settledCents int, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	reserved, ok := s.reservations[reservationID]
	if !ok {
		return budget.ErrReservationNotFound
	}
	delete(s.reservations, reservationID)
	if settledCents < 0 {
		settledCents = 0
	}
	if settledCents > reserved {
		settledCents = reserved
	}
	s.committed += settledCents
	return nil
}

func (s *memoryBudgetStore) Release(_ context.Context, reservationID string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.reservations[reservationID]; !ok {
		return budget.ErrReservationNotFound
	}
	delete(s.reservations, reservationID)
	return nil
}

func (s *memoryBudgetStore) CommittedCents(_ context.Context, _ string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.committed, nil
}

var _ budget.Store = (*memoryBudgetStore)(nil)
