package submit

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

var (
	uuidPattern        = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	idempotencyPattern = regexp.MustCompile(`^[!-~]{8,128}$`)
)

// ValidUUID reports whether id is a UUID version 1 through 5.
func ValidUUID(id string) bool {
	return uuidPattern.MatchString(id)
}

// ValidIdempotencyKey reports whether key is 8-128 visible ASCII characters.
func ValidIdempotencyKey(key string) bool {
	return idempotencyPattern.MatchString(key)
}

// ParseSource decodes a submission body. The fingerprint is the decoded
// source, so insignificant JSON spacing is the same content.
func ParseSource(body []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var payload struct {
		Source string `json:"source"`
	}
	if err := dec.Decode(&payload); err != nil {
		return "", ErrInvalid
	}
	if dec.More() {
		return "", ErrInvalid
	}
	if strings.ContainsRune(payload.Source, 0) {
		return "", ErrInvalid
	}
	if len(payload.Source) < 1 || len(payload.Source) > MaxSourceBytes {
		return "", ErrInvalid
	}
	return payload.Source, nil
}

// FormatTime renders an instant as RFC 3339 UTC with milliseconds.
func FormatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// MarshalAccept builds the 202 body stored with the replay row.
func MarshalAccept(id, labID string, received time.Time) ([]byte, error) {
	return json.Marshal(struct {
		ID         string `json:"id"`
		LabID      string `json:"labId"`
		State      string `json:"state"`
		ReceivedAt string `json:"receivedAt"`
	}{
		ID:         id,
		LabID:      labID,
		State:      "queued",
		ReceivedAt: FormatTime(received),
	})
}
