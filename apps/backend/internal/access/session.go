package access

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"time"
)

// SessionTTL is the opaque session lifetime from the moment of sign-in.
const SessionTTL = 8 * time.Hour

// SessionCookieName is the host-only cookie that carries the opaque token.
const SessionCookieName = "leetkob_session"

const tokenBytes = 32

// Material is one new session secret. Token and CSRF are returned to the
// caller once. TokenHash and CSRFHash are what the store keeps.
type Material struct {
	Token     string
	CSRF      string
	TokenHash []byte
	CSRFHash  []byte
}

// NewMaterial draws two random tokens and their SHA-256 digests.
func NewMaterial() (Material, error) {
	token, tokenHash, err := randomToken()
	if err != nil {
		return Material{}, err
	}
	csrf, csrfHash, err := randomToken()
	if err != nil {
		return Material{}, err
	}
	return Material{Token: token, CSRF: csrf, TokenHash: tokenHash, CSRFHash: csrfHash}, nil
}

func randomToken() (string, []byte, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("access: session token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	hash := make([]byte, sha256.Size)
	copy(hash, sum[:])
	return raw, hash, nil
}

// HashToken is the digest stored for a presented cookie or CSRF proof.
func HashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	hash := make([]byte, sha256.Size)
	copy(hash, sum[:])
	return hash
}

// ProofMatches reports whether the presented CSRF proof digests to the
// stored hash. The comparison does not stop at the first differing byte.
func ProofMatches(presented string, stored []byte) bool {
	if presented == "" || len(stored) != sha256.Size {
		return false
	}
	sum := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(sum[:], stored) == 1
}

// OriginAllowed accepts a missing Origin. A present Origin must equal the
// configured web origin exactly; a different origin is rejected.
func OriginAllowed(origin, configured string) bool {
	if origin == "" {
		return true
	}
	return configured != "" && origin == configured
}

// Expired reports whether now is at or after the session's expiry instant.
func Expired(now, expiresAt time.Time) bool {
	return !now.Before(expiresAt)
}
