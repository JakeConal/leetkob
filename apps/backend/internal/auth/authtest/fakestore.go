// Package authtest provides an in-memory auth.Store for deterministic
// service and HTTP tests. It never touches the network or a database.
package authtest

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/auth"
)

type attempt struct {
	at      time.Time
	ip      string
	account string
}

// FakeStore implements auth.Store in memory.
type FakeStore struct {
	mu       sync.Mutex
	Users    map[string]auth.UserRecord // keyed by email
	Classes  map[string][]auth.ClassSummary
	Sessions map[string]auth.SessionWithUser // keyed by token hash hex
	attempts []attempt
	revoked  map[string]bool
}

// NewFakeStore builds an empty store.
func NewFakeStore() *FakeStore {
	return &FakeStore{
		Users:    map[string]auth.UserRecord{},
		Classes:  map[string][]auth.ClassSummary{},
		Sessions: map[string]auth.SessionWithUser{},
		revoked:  map[string]bool{},
	}
}

// AddUser registers an account with its classes.
func (f *FakeStore) AddUser(user auth.UserRecord, classes ...auth.ClassSummary) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Users[user.Email] = user
	f.Classes[user.ID] = classes
}

// FindUserByEmail implements auth.Store.
func (f *FakeStore) FindUserByEmail(_ context.Context, email string) (auth.UserRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user, ok := f.Users[email]
	if !ok {
		return auth.UserRecord{}, auth.ErrInvalidCredentials
	}
	return user, nil
}

// ClassesForUser implements auth.Store.
func (f *FakeStore) ClassesForUser(_ context.Context, userID string) ([]auth.ClassSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Classes[userID], nil
}

// RecordAttempt implements auth.Store with the same window semantics as
// the durable adapter.
func (f *FakeStore) RecordAttempt(_ context.Context, clientIP, accountKey string, now time.Time) (bool, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts = append(f.attempts, attempt{at: now, ip: clientIP, account: accountKey})
	ipCount, accountCount := 0, 0
	var ipOldest, accountOldest time.Time
	for _, item := range f.attempts {
		if item.ip == clientIP && item.at.After(now.Add(-auth.IPWindow)) {
			ipCount++
			if ipOldest.IsZero() || item.at.Before(ipOldest) {
				ipOldest = item.at
			}
		}
		if item.account == accountKey && item.at.After(now.Add(-auth.AccountWindow)) {
			accountCount++
			if accountOldest.IsZero() || item.at.Before(accountOldest) {
				accountOldest = item.at
			}
		}
	}
	allowed := ipCount <= auth.IPLimit && accountCount <= auth.AccountLimit
	retryAfter := time.Duration(0)
	if ipCount > auth.IPLimit {
		retryAfter = ipOldest.Add(auth.IPWindow).Sub(now)
	}
	if accountCount > auth.AccountLimit {
		candidate := accountOldest.Add(auth.AccountWindow).Sub(now)
		if candidate > retryAfter {
			retryAfter = candidate
		}
	}
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return allowed, retryAfter, nil
}

// CreateSession implements auth.Store.
func (f *FakeStore) CreateSession(_ context.Context, session auth.SessionRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var user auth.UserRecord
	for _, candidate := range f.Users {
		if candidate.ID == session.UserID {
			user = candidate
			break
		}
	}
	if user.ID == "" {
		return fmt.Errorf("authtest: session user %s not found", session.UserID)
	}
	f.Sessions[hex.EncodeToString(session.TokenHash)] = auth.SessionWithUser{Session: session, User: user}
	return nil
}

// SessionByTokenHash implements auth.Store.
func (f *FakeStore) SessionByTokenHash(_ context.Context, tokenHash []byte) (auth.SessionWithUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	session, ok := f.Sessions[hex.EncodeToString(tokenHash)]
	if !ok || f.revoked[session.Session.ID] {
		return auth.SessionWithUser{}, auth.ErrUnauthenticated
	}
	return session, nil
}

// SessionByID implements auth.Store.
func (f *FakeStore) SessionByID(_ context.Context, sessionID string) (auth.SessionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, session := range f.Sessions {
		if session.Session.ID == sessionID && !f.revoked[sessionID] {
			return session.Session, nil
		}
	}
	return auth.SessionRecord{}, auth.ErrUnauthenticated
}

// RevokeSession implements auth.Store.
func (f *FakeStore) RevokeSession(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked[sessionID] = true
	return nil
}

// SessionCount reports how many sessions are stored (including revoked).
func (f *FakeStore) SessionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Sessions)
}
