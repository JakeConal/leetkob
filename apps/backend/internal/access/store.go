package access

import (
	"context"
	"time"
)

// UserSecret is the credential row used only inside Login. The HTTP
// boundary must not copy PasswordHash into a response.
type UserSecret struct {
	ID           string
	Email        string
	DisplayName  string
	Role         string
	PasswordHash string
}

// UserInsert is one account write. PasswordHash is already encoded.
type UserInsert struct {
	ID           string
	Email        string
	DisplayName  string
	Role         string
	PasswordHash string
}

// AssessmentRows is the provisioning write set. There is no lab field.
type AssessmentRows struct {
	Classes     []Class
	Users       []UserInsert
	Memberships []Membership
}

// SessionInsert is a new opaque session. Only hashes are stored.
type SessionInsert struct {
	UserID    string
	TokenHash []byte
	CSRFHash  []byte
	ExpiresAt time.Time
}

// SessionRow is a stored session plus the account it names. RevokedAt is
// set after logout and after the first request that observes expiry.
type SessionRow struct {
	UserID      string
	Email       string
	DisplayName string
	Role        string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CSRFHash    []byte
}

// Repository is the durable identity store. Implementations must make
// ApplyAssessment idempotent: existing users keep their password hash,
// and an existing membership row, including a removed one, is not
// inserted again.
type Repository interface {
	ApplyAssessment(ctx context.Context, rows AssessmentRows) error
	UserByEmail(ctx context.Context, email string) (UserSecret, bool, error)
	InsertSession(ctx context.Context, session SessionInsert) error
	SessionByTokenHash(ctx context.Context, tokenHash []byte) (SessionRow, bool, error)
	RevokeSession(ctx context.Context, tokenHash []byte, at time.Time) error
	Memberships(ctx context.Context, userID string) ([]Class, error)
	RemoveMembership(ctx context.Context, userID, classID string, at time.Time) error
	FailureCount(ctx context.Context, bucket, subject string, windowStart time.Time) (int, error)
	AddFailure(ctx context.Context, bucket, subject string, windowStart time.Time) (int, error)
}
