// Package auth holds the PA#3 session rules: password sign-in for the
// synthetic assessment accounts, opaque revocable sessions, durable login
// throttling and the current-authorization lookups. Storage is an injected
// port; the domain never depends on PostgreSQL or HTTP.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"
)

// SessionLifetime is the documented eight-hour session length.
const SessionLifetime = 8 * time.Hour

// Rate-limit windows from the approved contract: ten attempts per minute
// per client address and ten per fifteen minutes per account identifier.
const (
	IPWindow      = time.Minute
	IPLimit       = 10
	AccountWindow = 15 * time.Minute
	AccountLimit  = 10
)

// Role mirrors the persisted account roles.
type Role string

// Account roles.
const (
	RoleAdmin   Role = "admin"
	RoleTA      Role = "ta"
	RoleStudent Role = "student"
)

// UserRecord is one provisioned account.
type UserRecord struct {
	ID           string
	Email        string
	DisplayName  string
	Role         Role
	PasswordHash string
}

// ClassSummary is one authorized class membership.
type ClassSummary struct {
	ID   string
	Name string
}

// SessionRecord is one persisted session; only token digests are stored.
type SessionRecord struct {
	ID            string
	UserID        string
	TokenHash     []byte
	CSRFTokenHash []byte
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

// SessionWithUser joins a live session with its account.
type SessionWithUser struct {
	Session SessionRecord
	User    UserRecord
}

// Identity is the authenticated caller context resolved on every request.
type Identity struct {
	SessionID string
	User      UserRecord
}

// Store is the durable port implemented by the PostgreSQL adapter.
type Store interface {
	FindUserByEmail(ctx context.Context, email string) (UserRecord, error)
	ClassesForUser(ctx context.Context, userID string) ([]ClassSummary, error)
	// RecordAttempt records one login attempt and reports whether the
	// caller stays within both windows.
	RecordAttempt(ctx context.Context, clientIP, accountKey string, now time.Time) (bool, time.Duration, error)
	CreateSession(ctx context.Context, session SessionRecord) error
	// SessionByTokenHash returns a live (unexpired, unrevoked) session.
	SessionByTokenHash(ctx context.Context, tokenHash []byte) (SessionWithUser, error)
	// SessionByID returns one session record by identifier.
	SessionByID(ctx context.Context, sessionID string) (SessionRecord, error)
	RevokeSession(ctx context.Context, sessionID string) error
}

// Clock supplies server time for expiry and throttle windows.
type Clock interface {
	Now() time.Time
}

// SystemClock returns real UTC time.
type SystemClock struct{}

// Now implements Clock.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// PasswordVerifier compares a stored encoded hash with a password.
type PasswordVerifier func(encoded, password string) (bool, error)

// Errors surfaced to the HTTP boundary.
var (
	// ErrInvalidCredentials is deliberately generic: it never discloses
	// whether the account exists, and unknown accounts and wrong
	// passwords are indistinguishable.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrUnauthenticated means no live session matches the presented token.
	ErrUnauthenticated = errors.New("auth: unauthenticated")
)

// RateLimitedError reports a throttled login attempt.
type RateLimitedError struct {
	RetryAfter time.Duration
}

// Error implements error.
func (e *RateLimitedError) Error() string { return "auth: rate limited" }

// Service applies the session rules over the injected store.
type Service struct {
	Store    Store
	Clock    Clock
	Verify   PasswordVerifier
	randRead func([]byte) (int, error)
}

// NewService wires the service with production defaults.
func NewService(store Store, clock Clock, verify PasswordVerifier) *Service {
	return &Service{Store: store, Clock: clock, Verify: verify, randRead: rand.Read}
}

// SessionResult is the safe login outcome returned to the caller.
type SessionResult struct {
	User      UserRecord
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

// Login verifies a password, throttles attempts and creates one opaque
// session. On success the raw token and CSRF value are returned exactly
// once; only their digests are stored.
func (s *Service) Login(ctx context.Context, email, password, clientIP string) (SessionResult, error) {
	accountKey := normalizeAccount(email)
	allowed, retryAfter, err := s.Store.RecordAttempt(ctx, clientIP, accountKey, s.Clock.Now())
	if err != nil {
		return SessionResult{}, err
	}
	if !allowed {
		return SessionResult{}, &RateLimitedError{RetryAfter: retryAfter}
	}

	user, err := s.Store.FindUserByEmail(ctx, accountKey)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return SessionResult{}, ErrInvalidCredentials
		}
		return SessionResult{}, err
	}
	ok, err := s.Verify(user.PasswordHash, password)
	if err != nil || !ok {
		return SessionResult{}, ErrInvalidCredentials
	}

	token, err := randomToken(s.randRead)
	if err != nil {
		return SessionResult{}, err
	}
	csrfToken, err := randomToken(s.randRead)
	if err != nil {
		return SessionResult{}, err
	}
	now := s.Clock.Now()
	tokenDigest := sha256.Sum256([]byte(token))
	csrfDigest := sha256.Sum256([]byte(csrfToken))
	session := SessionRecord{
		ID:            mustUUID(s.randRead),
		UserID:        user.ID,
		TokenHash:     tokenDigest[:],
		CSRFTokenHash: csrfDigest[:],
		CreatedAt:     now,
		ExpiresAt:     now.Add(SessionLifetime),
	}
	if err := s.Store.CreateSession(ctx, session); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{User: user, Token: token, CSRFToken: csrfToken, ExpiresAt: session.ExpiresAt}, nil
}

// Authenticate resolves the presented opaque token to a current identity.
// Unknown, expired and revoked sessions are indistinguishable.
func (s *Service) Authenticate(ctx context.Context, token string) (Identity, error) {
	if token == "" || len(token) > 512 {
		return Identity{}, ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(token))
	session, err := s.Store.SessionByTokenHash(ctx, digest[:])
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			return Identity{}, ErrUnauthenticated
		}
		return Identity{}, err
	}
	if !s.Clock.Now().Before(session.Session.ExpiresAt) {
		return Identity{}, ErrUnauthenticated
	}
	return Identity{SessionID: session.Session.ID, User: session.User}, nil
}

// VerifyCSRF compares the presented CSRF proof with the stored digest of
// the authenticated session.
func (s *Service) VerifyCSRF(ctx context.Context, identity Identity, presented string) (bool, error) {
	if presented == "" {
		return false, nil
	}
	stored, err := s.Store.SessionByID(ctx, identity.SessionID)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			return false, nil
		}
		return false, err
	}
	digest := sha256.Sum256([]byte(presented))
	return equalDigest(stored.CSRFTokenHash, digest[:]), nil
}

// Logout revokes the current session.
func (s *Service) Logout(ctx context.Context, identity Identity) error {
	return s.Store.RevokeSession(ctx, identity.SessionID)
}

// Classes returns the caller's current class memberships.
func (s *Service) Classes(ctx context.Context, userID string) ([]ClassSummary, error) {
	return s.Store.ClassesForUser(ctx, userID)
}

func normalizeAccount(email string) string {
	normalized := make([]byte, 0, len(email))
	for i := 0; i < len(email); i++ {
		c := email[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		normalized = append(normalized, c)
	}
	return string(normalized)
}

func randomToken(read func([]byte) (int, error)) (string, error) {
	buffer := make([]byte, 32)
	if _, err := read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// mustUUID builds a random RFC 4122 version 4 identifier.
func mustUUID(read func([]byte) (int, error)) string {
	buffer := make([]byte, 16)
	if _, err := read(buffer); err != nil {
		return ""
	}
	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 36)
	positions := []int{8, 13, 18, 23}
	offset := 0
	for i := 0; i < 36; i++ {
		if offset < len(positions) && i == positions[offset] {
			out[i] = '-'
			offset++
			continue
		}
		out[i] = hexDigits[buffer[(i-offset)/2]>>4]
		if (i-offset)%2 == 1 {
			out[i] = hexDigits[buffer[(i-offset)/2]&0x0f]
		}
	}
	return string(out)
}

func equalDigest(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
