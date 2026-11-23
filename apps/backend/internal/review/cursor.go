package review

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// EncodeCursor hides one queue position. Clients send it back unchanged.
func EncodeCursor(seq int64, pos int, id string) string {
	raw := strconv.FormatInt(seq, 10) + "|" + strconv.Itoa(pos) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor reads a cursor from EncodeCursor. An empty cursor is the
// first page and is not passed here.
func DecodeCursor(raw string) (seq int64, pos int, id string, err error) {
	if raw == "" || len(raw) > 256 {
		return 0, 0, "", ErrBadCursor
	}
	decoded, decErr := base64.RawURLEncoding.DecodeString(raw)
	if decErr != nil {
		return 0, 0, "", ErrBadCursor
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 || parts[2] == "" {
		return 0, 0, "", ErrBadCursor
	}
	seq, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || seq < 0 {
		return 0, 0, "", ErrBadCursor
	}
	pos, err = strconv.Atoi(parts[1])
	if err != nil || pos < 0 {
		return 0, 0, "", ErrBadCursor
	}
	return seq, pos, parts[2], nil
}
