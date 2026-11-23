package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JakeConal/leetkob/backend/internal/access"
	"github.com/JakeConal/leetkob/backend/internal/labs"
)

// ApplyAssessment inserts the synthetic directory. Conflicts on the
// primary key do nothing, so a repeat call does not rewrite password_hash
// or any other column. A membership is inserted only when that user and
// class have no row yet, including a row that was later removed.
func (s *Store) ApplyAssessment(ctx context.Context, rows access.AssessmentRows) error {
	return s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		for _, class := range rows.Classes {
			if err := tx.Exec(ctx, `
				INSERT INTO classes (id, name)
				VALUES ($1::uuid, $2)
				ON CONFLICT (id) DO NOTHING`, class.ID, class.Name); err != nil {
				return err
			}
		}
		for _, user := range rows.Users {
			if err := tx.Exec(ctx, `
				INSERT INTO users (id, email, display_name, role, password_hash)
				VALUES ($1::uuid, $2, $3, $4, $5)
				ON CONFLICT (id) DO NOTHING`,
				user.ID, user.Email, user.DisplayName, user.Role, user.PasswordHash); err != nil {
				return err
			}
		}
		for _, membership := range rows.Memberships {
			if err := tx.Exec(ctx, `
				INSERT INTO memberships (user_id, class_id)
				SELECT $1::uuid, $2::uuid
				WHERE NOT EXISTS (
					SELECT 1 FROM memberships
					WHERE user_id = $1::uuid AND class_id = $2::uuid
				)`, membership.UserID, membership.ClassID); err != nil {
				return err
			}
		}
		return nil
	})
}

// UserByEmail looks up one account by case-insensitive email.
func (s *Store) UserByEmail(ctx context.Context, email string) (access.UserSecret, bool, error) {
	var user access.UserSecret
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, display_name, role, password_hash
		FROM users
		WHERE lower(email) = lower($1)`, email).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.UserSecret{}, false, nil
	}
	if err != nil {
		return access.UserSecret{}, false, err
	}
	return user, true, nil
}

// InsertSession stores the hashes of a new opaque session.
func (s *Store) InsertSession(ctx context.Context, session access.SessionInsert) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, csrf_token_hash, expires_at)
		VALUES ($1::uuid, $2, $3, $4)`,
		session.UserID, session.TokenHash, session.CSRFHash, session.ExpiresAt.UTC())
	return err
}

// SessionByTokenHash loads the session and its account. The raw token is
// not a column and cannot be selected.
func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash []byte) (access.SessionRow, bool, error) {
	var row access.SessionRow
	err := s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.role,
		       s.expires_at, s.revoked_at, s.csrf_token_hash
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1`, tokenHash).Scan(
		&row.UserID, &row.Email, &row.DisplayName, &row.Role,
		&row.ExpiresAt, &row.RevokedAt, &row.CSRFHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.SessionRow{}, false, nil
	}
	if err != nil {
		return access.SessionRow{}, false, err
	}
	return row, true, nil
}

// RevokeSession marks one session revoked. A second call changes nothing.
func (s *Store) RevokeSession(ctx context.Context, tokenHash []byte, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = $2
		WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash, at.UTC())
	return err
}

// Memberships returns classes the user currently belongs to, in name order.
func (s *Store) Memberships(ctx context.Context, userID string) ([]access.Class, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id::text, c.name
		FROM memberships m
		JOIN classes c ON c.id = m.class_id
		WHERE m.user_id = $1::uuid AND m.removed_at IS NULL
		ORDER BY c.name, c.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	classes := []access.Class{}
	for rows.Next() {
		var class access.Class
		if err := rows.Scan(&class.ID, &class.Name); err != nil {
			return nil, err
		}
		classes = append(classes, class)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return classes, nil
}

// RemoveMembership ends one current membership. The row stays, so a later
// provisioning run does not insert a replacement.
func (s *Store) RemoveMembership(ctx context.Context, userID, classID string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE memberships
		SET removed_at = $3
		WHERE user_id = $1::uuid AND class_id = $2::uuid AND removed_at IS NULL`,
		userID, classID, at.UTC())
	return err
}

// FailureCount is the recorded failures in one durable window. A missing
// row is zero.
func (s *Store) FailureCount(ctx context.Context, bucket, subject string, windowStart time.Time) (int, error) {
	var attempts int
	err := s.pool.QueryRow(ctx, `
		SELECT attempts
		FROM auth_rate_limits
		WHERE bucket = $1 AND subject = $2 AND window_started_at = $3`,
		bucket, subject, windowStart.UTC()).Scan(&attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return attempts, nil
}

// AddFailure increments the durable counter for one window and returns
// the new total.
func (s *Store) AddFailure(ctx context.Context, bucket, subject string, windowStart time.Time) (int, error) {
	var attempts int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO auth_rate_limits (bucket, subject, window_started_at, attempts)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (bucket, subject, window_started_at)
		DO UPDATE SET attempts = auth_rate_limits.attempts + 1
		RETURNING attempts`,
		bucket, subject, windowStart.UTC()).Scan(&attempts)
	if err != nil {
		return 0, err
	}
	return attempts, nil
}

var (
	_ access.Repository = (*Store)(nil)
	_ labs.Repository   = (*Store)(nil)
)
