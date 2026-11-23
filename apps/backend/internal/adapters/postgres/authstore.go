package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/auth"
)

// ErrAccountNotFound is returned for unknown sign-in accounts; callers
// must surface it as the same generic failure as a wrong password.
var ErrAccountNotFound = errors.New("postgres: account not found")

// FindUserByEmail returns one account by normalized email.
func (s *Store) FindUserByEmail(ctx context.Context, email string) (auth.UserRecord, error) {
	var user auth.UserRecord
	if err := s.pool.QueryRow(ctx, `
		SELECT id, email, display_name, role, password_hash
		FROM users WHERE email = $1`, email,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserRecord{}, ErrAccountNotFound
		}
		return auth.UserRecord{}, fmt.Errorf("postgres: find user: %w", err)
	}
	return user, nil
}

// ClassesForUser lists current memberships as safe class summaries.
func (s *Store) ClassesForUser(ctx context.Context, userID string) ([]auth.ClassSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT classes.id, classes.name
		FROM memberships
		JOIN classes ON classes.id = memberships.class_id
		WHERE memberships.user_id = $1
		ORDER BY classes.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list classes: %w", err)
	}
	defer rows.Close()
	classes := make([]auth.ClassSummary, 0, 4)
	for rows.Next() {
		var class auth.ClassSummary
		if err := rows.Scan(&class.ID, &class.Name); err != nil {
			return nil, fmt.Errorf("postgres: scan class: %w", err)
		}
		classes = append(classes, class)
	}
	return classes, rows.Err()
}

// RecordAttempt records one login attempt and reports whether both
// throttle windows allow it, plus how long to wait when they do not.
func (s *Store) RecordAttempt(ctx context.Context, clientIP, accountKey string, now time.Time) (bool, time.Duration, error) {
	var allowed bool
	var retryAfter time.Duration
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO login_attempts (attempted_at, client_ip, account_key) VALUES ($1, $2, $3)`,
			now, clientIP, accountKey,
		); err != nil {
			return err
		}

		var ipCount int
		var ipOldest time.Time
		if err := tx.QueryRow(ctx, `
			SELECT count(*), COALESCE(min(attempted_at), $2)
			FROM login_attempts
			WHERE client_ip = $1 AND attempted_at > $2`,
			clientIP, now.Add(-auth.IPWindow),
		).Scan(&ipCount, &ipOldest); err != nil {
			return err
		}

		var accountCount int
		var accountOldest time.Time
		if err := tx.QueryRow(ctx, `
			SELECT count(*), COALESCE(min(attempted_at), $2)
			FROM login_attempts
			WHERE account_key = $1 AND attempted_at > $2`,
			accountKey, now.Add(-auth.AccountWindow),
		).Scan(&accountCount, &accountOldest); err != nil {
			return err
		}

		allowed = ipCount <= auth.IPLimit && accountCount <= auth.AccountLimit
		if ipCount > auth.IPLimit {
			retryAfter = maxDuration(retryAfter, ipOldest.Add(auth.IPWindow).Sub(now))
		}
		if accountCount > auth.AccountLimit {
			retryAfter = maxDuration(retryAfter, accountOldest.Add(auth.AccountWindow).Sub(now))
		}
		// Opportunistic additive cleanup of rows older than the widest
		// window keeps the table small without affecting live counts.
		if _, err := tx.Exec(ctx,
			`DELETE FROM login_attempts WHERE attempted_at < $1`, now.Add(-auth.AccountWindow-time.Minute),
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return false, 0, fmt.Errorf("postgres: record login attempt: %w", err)
	}
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return allowed, retryAfter, nil
}

// CreateSession persists one session (token digests only).
func (s *Store) CreateSession(ctx context.Context, session auth.SessionRecord) error {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, csrf_token_hash, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		session.ID, session.UserID, session.TokenHash, session.CSRFTokenHash,
		session.CreatedAt, session.ExpiresAt,
	); err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	return nil
}

// SessionByTokenHash returns the unrevoked session matching a token
// digest together with its account. Expiry is enforced by the service
// clock, not by database time.
func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash []byte) (auth.SessionWithUser, error) {
	var result auth.SessionWithUser
	if err := s.pool.QueryRow(ctx, `
		SELECT sessions.id, sessions.user_id, sessions.token_hash, sessions.csrf_token_hash,
			sessions.created_at, sessions.expires_at,
			users.id, users.email, users.display_name, users.role, users.password_hash
		FROM sessions
		JOIN users ON users.id = sessions.user_id
		WHERE sessions.token_hash = $1 AND sessions.revoked_at IS NULL`, tokenHash,
	).Scan(
		&result.Session.ID, &result.Session.UserID, &result.Session.TokenHash,
		&result.Session.CSRFTokenHash, &result.Session.CreatedAt, &result.Session.ExpiresAt,
		&result.User.ID, &result.User.Email, &result.User.DisplayName, &result.User.Role,
		&result.User.PasswordHash,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.SessionWithUser{}, auth.ErrUnauthenticated
		}
		return auth.SessionWithUser{}, fmt.Errorf("postgres: session lookup: %w", err)
	}
	return result, nil
}

// SessionByID returns one unrevoked session record.
func (s *Store) SessionByID(ctx context.Context, sessionID string) (auth.SessionRecord, error) {
	var session auth.SessionRecord
	if err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, csrf_token_hash, created_at, expires_at
		FROM sessions WHERE id = $1 AND revoked_at IS NULL`, sessionID,
	).Scan(&session.ID, &session.UserID, &session.TokenHash, &session.CSRFTokenHash,
		&session.CreatedAt, &session.ExpiresAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.SessionRecord{}, auth.ErrUnauthenticated
		}
		return auth.SessionRecord{}, fmt.Errorf("postgres: read session: %w", err)
	}
	return session, nil
}

// RevokeSession marks one session revoked; already-revoked sessions stay
// revoked without error.
func (s *Store) RevokeSession(ctx context.Context, sessionID string) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, sessionID,
	); err != nil {
		return fmt.Errorf("postgres: revoke session: %w", err)
	}
	return nil
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// ProvisionedAccount is one synthetic assessment account outcome.
type ProvisionedAccount struct {
	Email       string
	DisplayName string
	Role        auth.Role
	ClassName   string
	// Password is set only when the account was created by this call.
	Password string
	Created  bool
}

// ProvisionAccountSeed describes one synthetic account and its class.
type ProvisionAccountSeed struct {
	Email       string
	DisplayName string
	Role        auth.Role
	ClassName   string
	ClassID     string
}

// HashPasswordFunc derives a stored password hash from plaintext.
type HashPasswordFunc func(password string) (string, error)

// ProvisionAccounts creates the synthetic accounts, classes and
// memberships idempotently. Existing accounts keep their IDs and
// credentials: a repeated run never resets a password silently.
func (s *Store) ProvisionAccounts(ctx context.Context, seeds []ProvisionAccountSeed, hashPassword HashPasswordFunc) ([]ProvisionedAccount, error) {
	results := make([]ProvisionedAccount, 0, len(seeds))
	for _, seed := range seeds {
		outcome := ProvisionedAccount{
			Email: seed.Email, DisplayName: seed.DisplayName, Role: seed.Role, ClassName: seed.ClassName,
		}
		if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			if seed.ClassID != "" {
				if _, err := tx.Exec(ctx,
					`INSERT INTO classes (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
					seed.ClassID, seed.ClassName,
				); err != nil {
					return err
				}
			}
			password, err := generatePassword()
			if err != nil {
				return err
			}
			encoded, err := hashPassword(password)
			if err != nil {
				return err
			}
			var userID string
			err = tx.QueryRow(ctx, `
				INSERT INTO users (id, email, display_name, role, password_hash)
				VALUES (gen_random_uuid(), $1, $2, $3, $4)
				ON CONFLICT (email) DO NOTHING
				RETURNING id`, seed.Email, seed.DisplayName, seed.Role, encoded,
			).Scan(&userID)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				// Account already exists: keep its credentials unchanged.
				if err := tx.QueryRow(ctx,
					`SELECT id FROM users WHERE email = $1`, seed.Email,
				).Scan(&userID); err != nil {
					return err
				}
			case err != nil:
				return err
			default:
				outcome.Created = true
				outcome.Password = password
			}
			if seed.ClassID != "" {
				if _, err := tx.Exec(ctx,
					`INSERT INTO memberships (class_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
					seed.ClassID, userID,
				); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("postgres: provision %s: %w", seed.Email, err)
		}
		results = append(results, outcome)
	}
	return results, nil
}

func generatePassword() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("postgres: generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
