package access

import "errors"

var (
	// ErrUnauthenticated is a generic sign-in or session failure.
	// Callers must not attach which check failed.
	ErrUnauthenticated = errors.New("access: authentication required")
	// ErrForbidden is a wrong role, a rejected origin, or a CSRF miss.
	ErrForbidden = errors.New("access: role required")
	// ErrNotFound is an absent resource and an inaccessible one.
	ErrNotFound = errors.New("access: not found")
	// ErrInvalid is a malformed provisioning or request input.
	ErrInvalid = errors.New("access: invalid request")
)

// RateLimitError is a durable login throttle. RetryAfter is whole seconds,
// at least 1.
type RateLimitError struct {
	RetryAfter int
}

func (e RateLimitError) Error() string { return "access: rate limited" }
