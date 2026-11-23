package access

import (
	"context"
	"strings"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/identity"
)

// dummyPasswordHash is a real Argon2id encoding of a password that belongs
// to no account. Unknown emails still run VerifyPassword so the failure
// path does the same key derivation as a wrong password. The encoding is
// not a credential for any synthetic account.
const dummyPasswordHash = "$argon2id$v=19$m=65536,t=1,p=2$pOFu91vnPf3G+xW0vzwc4A$Vsc23OahMk7+HHZH4MptVpDZZk7+je250SysUf8/4Yo"

// LoginInput is one password sign-in. Origin is the raw Origin header,
// empty when the client sent none. RemoteHost is the client address with
// no port.
type LoginInput struct {
	Email      string
	Password   string
	RemoteHost string
	Origin     string
	WebOrigin  string
	Now        time.Time
}

// LoginResult is the success payload. CookieToken is set as an HttpOnly
// cookie by the HTTP boundary and is not part of the JSON body. CSRF is
// returned once in that body; only its hash is stored.
type LoginResult struct {
	User        UserPublic
	CSRF        string
	CookieToken string
	ExpiresAt   time.Time
}

// Login checks origin, the durable failure counters, and the password.
// Unknown email and wrong password both return ErrUnauthenticated and
// both increment the failure counters. A counter already at the limit
// returns RateLimitError without checking the password.
func Login(ctx context.Context, repo Repository, in LoginInput) (LoginResult, error) {
	if !OriginAllowed(in.Origin, in.WebOrigin) {
		return LoginResult{}, ErrForbidden
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	host := in.RemoteHost
	if host == "" {
		host = "unknown"
	}
	now := in.Now.UTC()
	ipStart := IPWindowStart(now)
	accountStart := AccountWindowStart(now)
	ipCount, err := repo.FailureCount(ctx, BucketLoginIP, host, ipStart)
	if err != nil {
		return LoginResult{}, err
	}
	accountCount, err := repo.FailureCount(ctx, BucketLoginAccount, email, accountStart)
	if err != nil {
		return LoginResult{}, err
	}
	if blocked, retry := rateDecision(ipCount, accountCount, now, ipStart, accountStart); blocked {
		return LoginResult{}, RateLimitError{RetryAfter: retry}
	}

	user, ok, err := repo.UserByEmail(ctx, email)
	if err != nil {
		return LoginResult{}, err
	}
	hash := dummyPasswordHash
	if ok {
		hash = user.PasswordHash
	}
	match, verifyErr := identity.VerifyPassword(hash, in.Password)
	if verifyErr != nil || !ok || !match {
		if _, err := repo.AddFailure(ctx, BucketLoginIP, host, ipStart); err != nil {
			return LoginResult{}, err
		}
		if _, err := repo.AddFailure(ctx, BucketLoginAccount, email, accountStart); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrUnauthenticated
	}

	material, err := NewMaterial()
	if err != nil {
		return LoginResult{}, err
	}
	expires := now.Add(SessionTTL)
	if err := repo.InsertSession(ctx, SessionInsert{
		UserID:    user.ID,
		TokenHash: material.TokenHash,
		CSRFHash:  material.CSRFHash,
		ExpiresAt: expires,
	}); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		User: UserPublic{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			Role:        user.Role,
		},
		CSRF:        material.CSRF,
		CookieToken: material.Token,
		ExpiresAt:   expires,
	}, nil
}

// Authenticated is a live session and the principal loaded for it.
type Authenticated struct {
	Principal Principal
	CSRFHash  []byte
	ExpiresAt time.Time
}

// Load resolves a presented session cookie. Role and memberships are read
// from the store on every call. An expired session is revoked before the
// error is returned, so a later request still fails if the clock moves
// backward. Missing, malformed, revoked, and expired cookies all return
// ErrUnauthenticated.
func Load(ctx context.Context, repo Repository, rawToken string, now time.Time) (Authenticated, error) {
	if rawToken == "" || len(rawToken) > 256 {
		return Authenticated{}, ErrUnauthenticated
	}
	tokenHash := HashToken(rawToken)
	row, ok, err := repo.SessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return Authenticated{}, err
	}
	if !ok || row.RevokedAt != nil {
		return Authenticated{}, ErrUnauthenticated
	}
	now = now.UTC()
	if Expired(now, row.ExpiresAt) {
		if err := repo.RevokeSession(ctx, tokenHash, now); err != nil {
			return Authenticated{}, err
		}
		return Authenticated{}, ErrUnauthenticated
	}
	classes, err := repo.Memberships(ctx, row.UserID)
	if err != nil {
		return Authenticated{}, err
	}
	if classes == nil {
		classes = []Class{}
	}
	return Authenticated{
		Principal: Principal{
			User: UserPublic{
				ID:          row.UserID,
				Email:       row.Email,
				DisplayName: row.DisplayName,
				Role:        row.Role,
			},
			Classes: classes,
		},
		CSRFHash:  row.CSRFHash,
		ExpiresAt: row.ExpiresAt,
	}, nil
}

// Logout revokes the session named by rawToken. The caller has already
// checked the CSRF proof. Revoke is idempotent at the store.
func Logout(ctx context.Context, repo Repository, rawToken string, now time.Time) error {
	if rawToken == "" {
		return ErrUnauthenticated
	}
	return repo.RevokeSession(ctx, HashToken(rawToken), now.UTC())
}
