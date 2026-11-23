package httpapi

import (
	"net/http"
)

type classListResponse struct {
	Items      []classView `json:"items"`
	NextCursor *string     `json:"nextCursor"`
}

// listClasses handles GET /v2/classes: only the caller's current
// memberships are returned, filtered before pagination.
func (h *sessionHandlers) listClasses(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	page, ok := parsePage(w, r)
	if !ok {
		return
	}
	classes, err := h.auth.Classes(r.Context(), identity.User.ID)
	if err != nil {
		writeInternalErrorProblem(w, r)
		return
	}

	start := page.offset
	if start > len(classes) {
		start = len(classes)
	}
	end := start + page.limit
	if end > len(classes) {
		end = len(classes)
	}
	items := make([]classView, 0, end-start)
	for _, class := range classes[start:end] {
		items = append(items, classView{ID: class.ID, Name: class.Name})
	}
	var cursor *string
	if end < len(classes) {
		cursor = nextCursor(end)
	}
	writeJSONResponse(w, r, http.StatusOK, classListResponse{Items: items, NextCursor: cursor})
}
