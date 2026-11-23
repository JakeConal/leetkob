package labs

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// EncodeCursor builds an opaque page token from the last row's creation
// time and id. The instant is stored in microseconds so the next page
// compares equal to the PostgreSQL timestamp and does not repeat the row.
func EncodeCursor(createdAt time.Time, id string) string {
	payload := strconv.FormatInt(createdAt.UTC().UnixMicro(), 10) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// DecodeCursor reverses EncodeCursor. A token that was not produced by
// EncodeCursor is ErrInvalid.
func DecodeCursor(token string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, "", ErrInvalid
	}
	micros, id, ok := strings.Cut(string(raw), "|")
	if !ok || id == "" || micros == "" {
		return time.Time{}, "", ErrInvalid
	}
	value, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return time.Time{}, "", ErrInvalid
	}
	if !uuidPattern.MatchString(id) {
		return time.Time{}, "", ErrInvalid
	}
	return time.UnixMicro(value).UTC(), id, nil
}
