package httpapi

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 100
	cursorPrefix     = "v1:"
)

// pageRequest is one validated limit/cursor pair. The cursor is opaque to
// clients: it encodes only the offset of the next page.
type pageRequest struct {
	limit  int
	offset int
}

// parsePage reads the documented limit (1-100, default 50) and cursor query
// parameters. Invalid values are a 400 invalid-request problem.
func parsePage(w http.ResponseWriter, r *http.Request) (pageRequest, bool) {
	limit := defaultPageLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxPageLimit {
			writeInvalidRequestProblem(w, r)
			return pageRequest{}, false
		}
		limit = parsed
	}

	offset := 0
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || !strings.HasPrefix(string(decoded), cursorPrefix) {
			writeInvalidRequestProblem(w, r)
			return pageRequest{}, false
		}
		parsed, err := strconv.Atoi(strings.TrimPrefix(string(decoded), cursorPrefix))
		if err != nil || parsed < 0 {
			writeInvalidRequestProblem(w, r)
			return pageRequest{}, false
		}
		offset = parsed
	}
	return pageRequest{limit: limit, offset: offset}, true
}

// nextCursor encodes the next offset, or returns nil when the listing is
// complete.
func nextCursor(offset int) *string {
	if offset <= 0 {
		return nil
	}
	value := base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + strconv.Itoa(offset)))
	return &value
}
