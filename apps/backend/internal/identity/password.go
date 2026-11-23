// Package identity provides bounded Argon2id password hashing for the
// synthetic assessment accounts. Sessions store only a digest of a random
// opaque token (task 2.2); this package never logs or returns raw
// credentials.
package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Bounded Argon2id settings for synthetic assessment accounts.
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 2
	argonKeyLen  = 32
	saltLen      = 16
)

// ErrMalformedHash is returned when a stored encoded hash cannot be parsed.
var ErrMalformedHash = errors.New("identity: malformed encoded hash")

// HashPassword derives the Argon2id encoding for a password with a fresh
// random salt. The returned string is the only value stored; the caller
// must never store or log the password itself.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("identity: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return encode(salt, key), nil
}

// VerifyPassword reports whether password matches the stored encoded
// hash. Comparison is constant time with respect to the derived key.
func VerifyPassword(encoded, password string) (bool, error) {
	salt, key, err := decode(encoded)
	if err != nil {
		return false, err
	}
	derived := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, uint32(len(key)))
	if subtle.ConstantTimeCompare(derived, key) != 1 {
		return false, nil
	}
	return true, nil
}

func encode(salt, key []byte) string {
	parts := []string{
		"$argon2id$v=19",
		fmt.Sprintf("m=%d,t=%d,p=%d", argonMemory, argonTime, argonThreads),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}
	return strings.Join(parts, "$")
}

func decode(encoded string) (salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	// "$argon2id$v=19$m=...,t=...,p=...$salt$key" splits into six parts,
	// the first empty because of the leading '$'.
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return nil, nil, ErrMalformedHash
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return nil, nil, ErrMalformedHash
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return nil, nil, ErrMalformedHash
	}
	if len(salt) != saltLen || len(key) != argonKeyLen {
		return nil, nil, ErrMalformedHash
	}
	return salt, key, nil
}
