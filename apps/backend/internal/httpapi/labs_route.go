package httpapi

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/JakeConal/leetkob/backend/internal/labs"
)

var pathUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var idempotencyKeyPattern = regexp.MustCompile(`^[\x21-\x7e]{8,128}$`)

func (h *api) listLabs(w http.ResponseWriter, r *http.Request) {
	authn, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	classID, ok := pathID(w, r, "classId")
	if !ok {
		return
	}
	limit, cursor, err := parseListQuery(r.URL.Query())
	if err != nil {
		writeInvalid(w, r)
		return
	}
	items, next, err := h.deps.Labs.ListLabs(r.Context(), authn.Principal.User.ID, authn.Principal.User.Role, classID, h.now(), limit, cursor)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	views := make([]labs.SummaryView, 0, len(items))
	for _, item := range items {
		views = append(views, labs.PresentSummary(item))
	}
	var nextCursor *string
	if next != "" {
		nextCursor = &next
	}
	writeJSON(w, http.StatusOK, labListBody{Items: views, NextCursor: nextCursor})
}

func (h *api) createLab(w http.ResponseWriter, r *http.Request) {
	authn, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.allowMutation(w, r, authn.CSRFHash) {
		return
	}
	classID, ok := pathID(w, r, "classId")
	if !ok {
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	if !jsonMedia(r.Header.Get("Content-Type")) {
		writeInvalid(w, r)
		return
	}
	body, tooBig, err := readBounded(w, r, maxLabBody)
	if tooBig {
		writeTooLarge(w, r)
		return
	}
	if err != nil {
		writeInvalid(w, r)
		return
	}
	view, err := labs.Create(r.Context(), h.deps.Labs, authn.Principal, classID, key, body)
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (h *api) getLab(w http.ResponseWriter, r *http.Request) {
	authn, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	labID, ok := pathID(w, r, "labId")
	if !ok {
		return
	}
	lab, found, err := h.deps.Labs.ReadLab(r.Context(), authn.Principal.User.ID, authn.Principal.User.Role, labID, h.now())
	if err != nil {
		writeAccessError(w, r, err)
		return
	}
	if !found {
		writeNotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, labs.PresentDetail(lab))
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	value := strings.ToLower(r.PathValue(name))
	if !pathUUID.MatchString(value) {
		writeInvalid(w, r)
		return "", false
	}
	return value, true
}

func idempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	values := r.Header.Values("Idempotency-Key")
	if len(values) != 1 || !idempotencyKeyPattern.MatchString(values[0]) {
		writeInvalid(w, r)
		return "", false
	}
	return values[0], true
}

type labListBody struct {
	Items      []labs.SummaryView `json:"items"`
	NextCursor *string            `json:"nextCursor"`
}
