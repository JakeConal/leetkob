// Package compatpub is the Go compatibility composition for the PA#2
// publication contract (docs/contract/publish-lab.md). It exists so the
// shared seeded/fixed-clock scenarios
// (docs/contract/fixtures/publication-scenarios.json) can run unchanged
// against both the NestJS PA#2 application and this Go composition,
// proving the migration preserves the already-assessed contract behavior.
//
// This composition is test/parity scope only: the production Go routes use
// /v2 and MUST NOT register the legacy publication handler against
// production storage (see design decision 1 of implement-pa3-go-prototype).
package compatpub

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
)

// Role mirrors the PA#2 claim roles.
type Role string

const (
	RoleAdmin   Role = "admin"
	RoleStudent Role = "student"
	RoleTA      Role = "ta"
)

// Identity is the verified caller claim.
type Identity struct {
	Subject  string   `json:"subject"`
	Role     Role     `json:"role"`
	ClassIDs []string `json:"classIds"`
}

// secretMinimumBytes is the PA#2 minimum AUTH_TOKEN_SECRET length.
const secretMinimumBytes = 32

// maxAuthorizationBytes bounds the header before any parsing.
const maxAuthorizationBytes = 8192

var bearerPattern = regexp.MustCompile(`(?i)^Bearer ([A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)$`)

func hasStrongEnoughSecret(secret string) bool {
	return len(secret) >= secretMinimumBytes
}

func validClaims(claims map[string]any, nowSeconds int64) (*Identity, bool) {
	sub, ok := claims["sub"].(string)
	if !ok || len(sub) < 1 || len(sub) > 128 {
		return nil, false
	}
	roleValue, ok := claims["role"].(string)
	if !ok {
		return nil, false
	}
	role := Role(roleValue)
	if role != RoleAdmin && role != RoleStudent && role != RoleTA {
		return nil, false
	}
	rawClassIDs, ok := claims["classIds"].([]any)
	if !ok || len(rawClassIDs) > 100 {
		return nil, false
	}
	classIDs := make([]string, 0, len(rawClassIDs))
	seen := make(map[string]bool, len(rawClassIDs))
	for _, raw := range rawClassIDs {
		classID, ok := raw.(string)
		if !ok || len(classID) < 1 || len(classID) > 128 {
			return nil, false
		}
		if seen[classID] {
			return nil, false
		}
		seen[classID] = true
		classIDs = append(classIDs, classID)
	}
	expValue, ok := claims["exp"].(float64)
	if !ok || expValue != float64(int64(expValue)) {
		return nil, false
	}
	exp := int64(expValue)
	if exp <= nowSeconds {
		return nil, false
	}
	return &Identity{Subject: sub, Role: role, ClassIDs: classIDs}, true
}

func signatureFor(payloadSegment, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadSegment))
	return mac.Sum(nil)
}

// IssueToken signs the identity claims exactly like the PA#2 Node issuer:
// base64url(payload JSON) + "." + base64url(HMAC-SHA256 over the payload
// segment). Scenarios issue tokens per runner; the two runners never need
// to accept each other's tokens.
func IssueToken(identity Identity, secret string, expiresAtEpochSeconds int64) (string, error) {
	if !hasStrongEnoughSecret(secret) {
		return "", fmt.Errorf("compatpub: AUTH_TOKEN_SECRET must contain at least %d UTF-8 bytes", secretMinimumBytes)
	}
	claims := tokenClaims(identity, expiresAtEpochSeconds)
	if _, ok := validClaims(claims, 0); !ok {
		return "", fmt.Errorf("compatpub: cannot issue a token for invalid claims")
	}
	payloadSegment, err := encodeSegment(claims)
	if err != nil {
		return "", err
	}
	signatureSegment := base64.RawURLEncoding.EncodeToString(signatureFor(payloadSegment, secret))
	return payloadSegment + "." + signatureSegment, nil
}

// tokenClaims builds the signed claim set with JSON-compatible types.
func tokenClaims(identity Identity, expiresAtEpochSeconds int64) map[string]any {
	classIDs := make([]any, 0, len(identity.ClassIDs))
	for _, classID := range identity.ClassIDs {
		classIDs = append(classIDs, classID)
	}
	return map[string]any{
		"sub":      identity.Subject,
		"role":     string(identity.Role),
		"classIds": classIDs,
		"exp":      float64(expiresAtEpochSeconds),
	}
}

// encodeSegment marshals the claims to the base64url payload segment.
func encodeSegment(claims map[string]any) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("compatpub: marshal claims: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

// VerifyAuthorizationHeader mirrors the PA#2 Node verifier. Any malformed,
// tampered, structurally invalid or expired header yields no identity.
func VerifyAuthorizationHeader(authorization, secret string, nowSeconds int64) (*Identity, bool) {
	if authorization == "" || len(authorization) > maxAuthorizationBytes || !hasStrongEnoughSecret(secret) {
		return nil, false
	}
	match := bearerPattern.FindStringSubmatch(authorization)
	if match == nil {
		return nil, false
	}
	token := match[1]
	for separator := 0; separator < len(token); separator++ {
		if token[separator] != '.' {
			continue
		}
		if separator < 1 {
			return nil, false
		}
		payloadSegment := token[:separator]
		signatureSegment := token[separator+1:]
		supplied, err := base64.RawURLEncoding.DecodeString(signatureSegment)
		if err != nil {
			return nil, false
		}
		expected := signatureFor(payloadSegment, secret)
		if len(supplied) != len(expected) || subtle.ConstantTimeCompare(supplied, expected) != 1 {
			return nil, false
		}
		payload, err := base64.RawURLEncoding.DecodeString(payloadSegment)
		if err != nil {
			return nil, false
		}
		var claims map[string]any
		if err := json.Unmarshal(payload, &claims); err != nil {
			return nil, false
		}
		return validClaims(claims, nowSeconds)
	}
	return nil, false
}
