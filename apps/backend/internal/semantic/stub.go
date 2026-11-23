package semantic

import (
	"context"
	"errors"
	"net/http"
)

var errNotImplemented = errors.New("semantic: not implemented")

// ConservativeMicroUSD reports the maximum priced cost of one call.
func (p Price) ConservativeMicroUSD() (int64, bool) {
	return 0, false
}

// Sanitize removes comments and known identities. A non-nil error means
// the input must not be sent.
func Sanitize(in SanitizeInput) (Sanitized, error) {
	return Sanitized{}, errNotImplemented
}

// Check applies the semantic guards. The stub exists so the limit tests
// compile before the behavior lands.
func (c *Checker) Check(ctx context.Context, in Input) (Result, error) {
	return Result{}, errNotImplemented
}

// MemoryStore is the in-memory ReservationStore.
type MemoryStore struct{}

// NewMemoryStore returns an empty budget.
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

// Reserve holds amount against the lab cap.
func (m *MemoryStore) Reserve(ctx context.Context, labID string, amountMicroUSD, capMicroUSD int64) (Reservation, error) {
	return Reservation{}, errNotImplemented
}

// Settle records known usage and releases the unused hold.
func (m *MemoryStore) Settle(ctx context.Context, reservationID string, actualMicroUSD int64) error {
	return errNotImplemented
}

// Keep charges the full hold when usage is unknown.
func (m *MemoryStore) Keep(ctx context.Context, reservationID string) error {
	return errNotImplemented
}

// Release drops a hold that was never dispatched.
func (m *MemoryStore) Release(ctx context.Context, reservationID string) error {
	return errNotImplemented
}

// Charged is settled usage plus open holds.
func (m *MemoryStore) Charged(ctx context.Context, labID string) (int64, error) {
	return 0, errNotImplemented
}

// HTTPAdapter talks to a model with net/http. Tests inject Client.
type HTTPAdapter struct {
	URL    string
	Model  string
	Token  string
	Client *http.Client
}

// Complete performs one provider request.
func (h *HTTPAdapter) Complete(ctx context.Context, req CompletionRequest) (ModelResponse, error) {
	return ModelResponse{}, errNotImplemented
}
