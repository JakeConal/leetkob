package httpapi

import (
	"encoding/json"
	"net/http"
)

// writeJSONResponse marshals one JSON response with no-store caching. It
// returns the encoded bytes so callers can persist an idempotent replay.
func writeJSONResponse(w http.ResponseWriter, r *http.Request, status int, value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		writeInternalErrorProblem(w, r)
		return nil
	}
	writeRawResponse(w, status, "application/json; charset=utf-8", payload)
	return payload
}

// writeRawResponse emits a pre-encoded body, preserving the status and
// content type recorded for an idempotent replay.
func writeRawResponse(w http.ResponseWriter, status int, contentType string, payload []byte) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
