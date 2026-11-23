// Package budget enforces the PA#3 per-lab model spending cap. A
// conservative, rounded-up maximum is reserved atomically before every
// provider dispatch (first call, repair retry and rechecks); settlements
// reconcile the difference, unknown usage stays charged, and an unknown
// price refuses dispatch. The repository unchanged cap is USD 0.25 per
// lab, stored and compared in integer cents.
package budget

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultLabCapCents is the repository's documented USD 0.25 per-lab cap.
const DefaultLabCapCents = 25

// Errors surfaced to the caller.
var (
	// ErrCapExceeded means the lab's remaining budget cannot cover the
	// conservative maximum for this dispatch.
	ErrCapExceeded = errors.New("budget: lab cap exceeded")
	// ErrUnknownPrice means no verified price schedule is configured;
	// dispatch must be refused rather than charged at zero.
	ErrUnknownPrice = errors.New("budget: price schedule unknown")
	// ErrReservationNotFound means the reservation id is unknown.
	ErrReservationNotFound = errors.New("budget: reservation not found")
)

// PriceSchedule is a verified per-1K-token price in integer cents.
type PriceSchedule struct {
	InputCentsPer1K  int
	OutputCentsPer1K int
}

// Known reports whether the schedule carries verified prices.
func (s PriceSchedule) Known() bool {
	return s.InputCentsPer1K > 0 && s.OutputCentsPer1K > 0
}

// Usage is measured provider usage. Known=false means the provider gave no
// usage; the reservation is then settled at its reserved maximum.
type Usage struct {
	InputTokens  int
	OutputTokens int
	Known        bool
}

// Reservation is one held amount against a lab.
type Reservation struct {
	ID            string
	LabID         string
	ReservedCents int
	CreatedAt     time.Time
}

// Store is the durable port. Reserve must be atomic per lab: it computes
// committed spend and inserts the reservation in one transaction, so
// concurrent reservations can never overshoot the cap.
type Store interface {
	Reserve(ctx context.Context, labID string, amountCents, capCents int, now time.Time) (Reservation, error)
	Settle(ctx context.Context, reservationID string, settledCents int, providerRequestID string, now time.Time) error
	Release(ctx context.Context, reservationID string, now time.Time) error
	CommittedCents(ctx context.Context, labID string) (int, error)
}

// Clock supplies server time for reservation records.
type Clock interface {
	Now() time.Time
}

// SystemClock returns real UTC time.
type SystemClock struct{}

// Now implements Clock.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// Service reserves and reconciles spend for one lab at a time.
type Service struct {
	Store    Store
	Clock    Clock
	Prices   PriceSchedule
	CapCents int
}

// NewService wires the service with the documented default cap.
func NewService(store Store, clock Clock, prices PriceSchedule) *Service {
	return &Service{Store: store, Clock: clock, Prices: prices, CapCents: DefaultLabCapCents}
}

// MaxCost is the conservative, rounded-up integer-cent maximum for one
// dispatch. Prices are per 1K tokens, so any partial unit rounds up.
func (s *Service) MaxCost(maxInputTokens, maxOutputTokens int) (int, error) {
	if !s.Prices.Known() || maxInputTokens < 0 || maxOutputTokens < 0 {
		return 0, ErrUnknownPrice
	}
	inputCents := ceilDiv(maxInputTokens*s.Prices.InputCentsPer1K, 1000)
	outputCents := ceilDiv(maxOutputTokens*s.Prices.OutputCentsPer1K, 1000)
	cost := inputCents + outputCents
	if cost <= 0 {
		// A priced call with a positive token bound never costs zero;
		// refuse rather than dispatch for free.
		return 0, ErrUnknownPrice
	}
	return cost, nil
}

// ReserveForCall holds the conservative maximum for one dispatch before
// any provider request is made. Repair retries and rechecks call this
// again, so they are included in the cap.
func (s *Service) ReserveForCall(ctx context.Context, labID string, maxInputTokens, maxOutputTokens int) (Reservation, error) {
	amount, err := s.MaxCost(maxInputTokens, maxOutputTokens)
	if err != nil {
		return Reservation{}, err
	}
	capCents := s.CapCents
	if capCents <= 0 {
		capCents = DefaultLabCapCents
	}
	reservation, err := s.Store.Reserve(ctx, labID, amount, capCents, s.Clock.Now())
	if err != nil {
		return Reservation{}, fmt.Errorf("budget: reserve: %w", err)
	}
	return reservation, nil
}

// Settle reconciles a reservation with measured usage. Unknown usage
// remains charged at the reserved amount; known usage settles the actual
// rounded-up cost, releasing the difference.
func (s *Service) Settle(ctx context.Context, reservation Reservation, usage Usage, providerRequestID string) error {
	settled := reservation.ReservedCents
	if usage.Known {
		actual, err := s.actualCost(usage.InputTokens, usage.OutputTokens)
		if err != nil {
			// Unknown price at settlement keeps the reservation charged.
			return nil
		}
		if actual < 0 {
			actual = 0
		}
		// Never settle above the amount that was reserved: the cap was
		// checked against that maximum.
		if actual > reservation.ReservedCents {
			actual = reservation.ReservedCents
		}
		settled = actual
	}
	return s.Store.Settle(ctx, reservation.ID, settled, providerRequestID, s.Clock.Now())
}

// Release returns a reservation that never dispatched. It is only for
// pre-dispatch failures; unknown usage must use Settle.
func (s *Service) Release(ctx context.Context, reservation Reservation) error {
	return s.Store.Release(ctx, reservation.ID, s.Clock.Now())
}

// CommittedCents reports settled plus held spend for one lab.
func (s *Service) CommittedCents(ctx context.Context, labID string) (int, error) {
	return s.Store.CommittedCents(ctx, labID)
}

func (s *Service) actualCost(inputTokens, outputTokens int) (int, error) {
	if !s.Prices.Known() || inputTokens < 0 || outputTokens < 0 {
		return 0, ErrUnknownPrice
	}
	inputCents := ceilDiv(inputTokens*s.Prices.InputCentsPer1K, 1000)
	outputCents := ceilDiv(outputTokens*s.Prices.OutputCentsPer1K, 1000)
	return inputCents + outputCents, nil
}

func ceilDiv(value, divisor int) int {
	if value <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}
