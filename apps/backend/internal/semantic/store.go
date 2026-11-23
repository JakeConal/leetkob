package semantic

import (
	"context"
	"fmt"
	"sync"
)

// MemoryStore is the in-memory ReservationStore. Holds are atomic under
// one mutex so concurrent reservations cannot pass the lab cap.
type MemoryStore struct {
	mu    sync.Mutex
	seq   uint64
	holds map[string]*reservationHold
	labs  map[string]*labSpend
}

type reservationHold struct {
	id     string
	labID  string
	amount int64
	open   bool
}

type labSpend struct {
	settled  int64
	reserved int64
}

// NewMemoryStore returns an empty budget with no labs charged.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		holds: map[string]*reservationHold{},
		labs:  map[string]*labSpend{},
	}
}

// Reserve holds amountMicroUSD against capMicroUSD for one lab.
// The hold is visible to Charged before the caller dispatches.
func (m *MemoryStore) Reserve(ctx context.Context, labID string, amountMicroUSD, capMicroUSD int64) (Reservation, error) {
	if err := ctx.Err(); err != nil {
		return Reservation{}, err
	}
	if labID == "" || amountMicroUSD < 0 || capMicroUSD < 0 {
		return Reservation{}, fmt.Errorf("semantic: invalid reservation")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	acct := m.labs[labID]
	if acct == nil {
		acct = &labSpend{}
		m.labs[labID] = acct
	}
	used, ok := addMicro(acct.settled, acct.reserved)
	next, okNext := addMicro(used, amountMicroUSD)
	if !ok || !okNext || next > capMicroUSD {
		return Reservation{}, ErrBudgetExceeded
	}
	m.seq++
	id := fmt.Sprintf("r%d", m.seq)
	m.holds[id] = &reservationHold{id: id, labID: labID, amount: amountMicroUSD, open: true}
	acct.reserved += amountMicroUSD
	return Reservation{ID: id, LabID: labID, AmountMicroUSD: amountMicroUSD}, nil
}

// Settle charges actualMicroUSD and releases the rest of the hold.
// actualMicroUSD must be known and no larger than the hold.
func (m *MemoryStore) Settle(ctx context.Context, reservationID string, actualMicroUSD int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hold, acct, err := m.open(reservationID)
	if err != nil {
		return err
	}
	if actualMicroUSD < 0 || actualMicroUSD > hold.amount {
		return fmt.Errorf("semantic: settled usage exceeds the reservation")
	}
	acct.reserved -= hold.amount
	acct.settled += actualMicroUSD
	hold.open = false
	return nil
}

// Keep charges the full hold. Unknown provider usage uses this so the
// lab cannot spend the same money again.
func (m *MemoryStore) Keep(ctx context.Context, reservationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hold, acct, err := m.open(reservationID)
	if err != nil {
		return err
	}
	acct.reserved -= hold.amount
	acct.settled += hold.amount
	hold.open = false
	return nil
}

// Release drops a hold that never dispatched.
func (m *MemoryStore) Release(ctx context.Context, reservationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hold, acct, err := m.open(reservationID)
	if err != nil {
		return err
	}
	acct.reserved -= hold.amount
	hold.open = false
	return nil
}

// Charged is settled usage plus still-open holds for the lab.
func (m *MemoryStore) Charged(ctx context.Context, labID string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	acct := m.labs[labID]
	if acct == nil {
		return 0, nil
	}
	total, ok := addMicro(acct.settled, acct.reserved)
	if !ok {
		return 0, fmt.Errorf("semantic: charged total overflow")
	}
	return total, nil
}

func (m *MemoryStore) open(id string) (*reservationHold, *labSpend, error) {
	hold := m.holds[id]
	if hold == nil {
		return nil, nil, ErrReservationNotFound
	}
	if !hold.open {
		return nil, nil, fmt.Errorf("semantic: reservation %s is already closed", id)
	}
	acct := m.labs[hold.labID]
	if acct == nil {
		return nil, nil, ErrReservationNotFound
	}
	return hold, acct, nil
}
