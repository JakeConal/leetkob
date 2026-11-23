package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/review"
)

const (
	httpLab      = "11111111-1111-4111-8111-111111111111"
	httpClass    = "22222222-2222-4222-8222-222222222222"
	httpOther    = "33333333-3333-4333-8333-333333333333"
	httpTA       = "44444444-4444-4444-8444-444444444444"
	httpStudent  = "55555555-5555-4555-8555-555555555555"
	httpOtherTA  = "66666666-6666-4666-8666-666666666666"
	httpFinding  = "77777777-7777-4777-8777-777777777777"
	httpUnsure   = "88888888-8888-4888-8888-888888888888"
	httpPass     = "99999999-9999-4999-8999-999999999999"
	httpSource   = "int main() {\n  std::sort(a, a + n); // MARKER_SOURCE_LINE\n}\n"
	httpEvidence = "  std::sort(a, a + n); // MARKER_SOURCE_LINE"
	httpOrigin   = "http://127.0.0.1:3000"
)

func TestAssignedQueueShowsFindingFieldsAndHidesThemFromOthers(t *testing.T) {
	store, auth := queueFixture()
	spy := &countingStore{Memory: store}
	handler := &ReviewsHandler{Auth: auth, Reviews: spy, WebOrigin: httpOrigin, Now: fixedNow}

	ta := getQueue(t, handler, "ta-token")
	if ta.Code != http.StatusOK {
		t.Fatalf("ta status %d body %s", ta.Code, ta.Body.String())
	}
	if cc := ta.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	var page struct {
		Items []struct {
			ConstraintID   string             `json:"constraintId"`
			SourceKind     string             `json:"sourceKind"`
			CheckerVerdict string             `json:"checkerVerdict"`
			UnsureCause    string             `json:"unsureCause"`
			Evidence       []review.Evidence  `json:"evidence"`
			Deduction      json.Number        `json:"deduction"`
			RequiredReview bool               `json:"requiredReview"`
			Revision       int                `json:"revision"`
			Source         string             `json:"source"`
			Decision       *review.TADecision `json:"decision"`
		} `json:"items"`
		NextCursor        *string `json:"nextCursor"`
		ProcessingPending bool    `json:"processingPending"`
		ReviewPending     int     `json:"reviewPending"`
	}
	if err := json.Unmarshal(ta.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if !page.ProcessingPending || page.ReviewPending != 2 || page.NextCursor != nil || len(page.Items) != 3 {
		t.Fatalf("page = %+v", page)
	}
	byID := map[string]int{}
	for i, item := range page.Items {
		byID[item.ConstraintID] = i
		if item.Decision != nil || item.Revision != 1 || item.Source != httpSource {
			t.Fatalf("item = %+v", item)
		}
	}
	violation := page.Items[byID["ban-sort"]]
	if violation.CheckerVerdict != "violation" || violation.SourceKind != "hard-rule" || violation.Deduction.String() != "3.00" || !violation.RequiredReview {
		t.Fatalf("violation = %+v", violation)
	}
	if len(violation.Evidence) != 1 || violation.Evidence[0].Text != httpEvidence {
		t.Fatalf("evidence = %+v", violation.Evidence)
	}
	unsure := page.Items[byID["timeout"]]
	if unsure.CheckerVerdict != "unsure" || unsure.UnsureCause != "provider-timeout" || unsure.SourceKind != "semantic" || unsure.Deduction.String() != "1.50" || !unsure.RequiredReview {
		t.Fatalf("unsure = %+v", unsure)
	}
	pass := page.Items[byID["style"]]
	if pass.CheckerVerdict != "pass" || pass.RequiredReview || pass.Deduction.String() != "0.50" {
		t.Fatalf("pass sample was redrawn: %+v", pass)
	}
	again := getQueue(t, handler, "ta-token")
	if again.Body.String() != ta.Body.String() {
		t.Fatalf("refresh changed the queue\n%s\n%s", ta.Body.String(), again.Body.String())
	}

	student := getQueue(t, handler, "student-token")
	assertConcealed(t, student)
	other := getQueue(t, handler, "other-token")
	assertConcealed(t, other)
	if spy.lists != 2 {
		t.Fatalf("list calls = %d, want 2 TA reads and no concealed reads", spy.lists)
	}
	missing := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v2/labs/"+httpLab+"/reviews", nil)
	req.Header.Set("X-Role", "ta")
	handler.ServeHTTP(missing, req)
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status %d", missing.Code)
	}
}

func TestDecisionIsExplicitRevisionBoundAndDoesNotPublish(t *testing.T) {
	store, auth := queueFixture()
	handler := &ReviewsHandler{Auth: auth, Reviews: store, WebOrigin: httpOrigin, Now: fixedNow}
	path := "/v2/labs/" + httpLab + "/reviews/" + httpUnsure + "/decision"

	missing := postDecision(t, handler, "ta-token", "ta-csrf", "decide-unsure-01", path, `{"expectedRevision":1,"reason":"no verdict"}`)
	if missing.Code != http.StatusBadRequest || store.DecisionCount(httpUnsure) != 0 {
		t.Fatalf("missing verdict status %d count %d", missing.Code, store.DecisionCount(httpUnsure))
	}
	unsureVerdict := postDecision(t, handler, "ta-token", "ta-csrf", "decide-unsure-02", path, `{"expectedRevision":1,"verdict":"unsure","reason":"leave it"}`)
	if unsureVerdict.Code != http.StatusBadRequest || store.DecisionCount(httpUnsure) != 0 {
		t.Fatalf("unsure verdict status %d", unsureVerdict.Code)
	}
	noEvidence := postDecision(t, handler, "ta-token", "ta-csrf", "decide-ban-01", "/v2/labs/"+httpLab+"/reviews/"+httpFinding+"/decision",
		`{"expectedRevision":1,"verdict":"violation","reason":"It calls sort."}`)
	if noEvidence.Code != http.StatusBadRequest || store.DecisionCount(httpFinding) != 0 {
		t.Fatalf("violation without evidence status %d", noEvidence.Code)
	}

	ok := postDecision(t, handler, "ta-token", "ta-csrf", "decide-unsure-03", path,
		`{"expectedRevision":1,"verdict":"pass","reason":"The timeout is not a violation."}`)
	if ok.Code != http.StatusOK {
		t.Fatalf("decision status %d body %s", ok.Code, ok.Body.String())
	}
	var decided struct {
		CheckerVerdict string             `json:"checkerVerdict"`
		UnsureCause    string             `json:"unsureCause"`
		SourceKind     string             `json:"sourceKind"`
		Deduction      json.Number        `json:"deduction"`
		Revision       int                `json:"revision"`
		Decision       *review.TADecision `json:"decision"`
	}
	if err := json.Unmarshal(ok.Body.Bytes(), &decided); err != nil {
		t.Fatal(err)
	}
	if decided.CheckerVerdict != "unsure" || decided.UnsureCause != "provider-timeout" || decided.SourceKind != "semantic" || decided.Deduction.String() != "1.50" || decided.Revision != 2 {
		t.Fatalf("provenance changed: %+v", decided)
	}
	if decided.Decision == nil || decided.Decision.Verdict != "pass" || decided.Decision.Revision != 2 || decided.Decision.DecidedBy != httpTA || decided.Decision.DecidedAt != "2026-11-24T01:02:03.000Z" {
		t.Fatalf("decision = %+v", decided.Decision)
	}
	if strings.Contains(ok.Body.String(), `"grade"`) || strings.Contains(ok.Body.String(), `"published"`) {
		t.Fatalf("decision published a mark: %s", ok.Body.String())
	}

	replay := postDecision(t, handler, "ta-token", "ta-csrf", "decide-unsure-03", path,
		`{"expectedRevision":1,"verdict":"pass","reason":"The timeout is not a violation."}`)
	if replay.Code != http.StatusOK || store.DecisionCount(httpUnsure) != 1 {
		t.Fatalf("replay status %d count %d body %s", replay.Code, store.DecisionCount(httpUnsure), replay.Body.String())
	}
	stale := postDecision(t, handler, "ta-token", "ta-csrf", "decide-unsure-04", path,
		`{"expectedRevision":1,"verdict":"violation","reason":"It is a real call.","evidence":{"line":2,"text":"`+httpEvidence+`"}}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale status %d body %s", stale.Code, stale.Body.String())
	}
	var problem struct {
		Type             string `json:"type"`
		ConflictKind     string `json:"conflictKind"`
		ExpectedRevision int    `json:"expectedRevision"`
		CurrentRevision  int    `json:"currentRevision"`
		Detail           string `json:"detail"`
	}
	if err := json.Unmarshal(stale.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Type != ProblemNamespace+"conflict" || problem.ConflictKind != "stale-revision" || problem.ExpectedRevision != 1 || problem.CurrentRevision != 2 {
		t.Fatalf("problem = %+v", problem)
	}
	if problem.Detail != "The item changed before this request was applied." || store.DecisionCount(httpUnsure) != 1 {
		t.Fatalf("detail %q count %d", problem.Detail, store.DecisionCount(httpUnsure))
	}

	student := postDecision(t, handler, "student-token", "student-csrf", "decide-student-1", path, `{"expectedRevision":2,"verdict":"pass","reason":"I disagree."}`)
	assertConcealed(t, student)
	if store.DecisionCount(httpUnsure) != 1 {
		t.Fatal("student decision was stored")
	}

	publish := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2/labs/"+httpLab+"/publish", strings.NewReader(`{}`))
	req.AddCookie(&http.Cookie{Name: "leetkob_session", Value: "ta-token"})
	req.Header.Set("X-CSRF-Token", "ta-csrf")
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(publish, req)
	if publish.Code != http.StatusNotFound || strings.Contains(publish.Body.String(), "MARKER_SOURCE_LINE") {
		t.Fatalf("publish status %d body %s", publish.Code, publish.Body.String())
	}
}

func TestDecisionConflictAndInternalErrorHideInternals(t *testing.T) {
	auth := mapAuth{sessions: map[string]Identity{"ta-token": {UserID: httpTA, CSRF: "ta-csrf"}}}
	handler := &ReviewsHandler{Auth: auth, Reviews: boomStore{}, WebOrigin: httpOrigin}
	rec := getQueue(t, handler, "ta-token")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "super-secret") || strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func queueFixture() (*review.Memory, mapAuth) {
	store := review.NewMemory()
	store.PutUser(httpTA, "ta")
	store.PutUser(httpStudent, "student")
	store.PutUser(httpOtherTA, "ta")
	store.PutLab(httpLab, httpClass, true)
	store.PutLab(httpOther, httpOther, false)
	store.PutMember(httpClass, httpTA)
	store.PutMember(httpClass, httpStudent)
	store.PutMember(httpOther, httpOtherTA)
	store.PutFinding(review.Finding{
		ID: httpFinding, SubmissionID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", LabID: httpLab,
		ConstraintID: "ban-sort", ConstraintRevision: 1, Revision: 1, SourceKind: "hard-rule",
		CheckerVerdict: "violation", Evidence: []review.Evidence{{Line: 2, Text: httpEvidence}},
		Deduction: 300, RequiredReview: true, Source: httpSource,
	}, 1, 1, true)
	store.PutFinding(review.Finding{
		ID: httpUnsure, SubmissionID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", LabID: httpLab,
		ConstraintID: "timeout", ConstraintRevision: 1, Revision: 1, SourceKind: "semantic",
		CheckerVerdict: "unsure", UnsureCause: "provider-timeout", Evidence: []review.Evidence{},
		Deduction: 150, RequiredReview: true, Source: httpSource,
	}, 1, 2, true)
	store.PutFinding(review.Finding{
		ID: httpPass, SubmissionID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", LabID: httpLab,
		ConstraintID: "style", ConstraintRevision: 1, Revision: 1, SourceKind: "semantic",
		CheckerVerdict: "pass", Evidence: []review.Evidence{}, Deduction: 50, RequiredReview: false, Source: httpSource,
	}, 1, 3, true)
	return store, mapAuth{sessions: map[string]Identity{
		"ta-token":      {UserID: httpTA, CSRF: "ta-csrf"},
		"student-token": {UserID: httpStudent, CSRF: "student-csrf"},
		"other-token":   {UserID: httpOtherTA, CSRF: "other-csrf"},
	}}
}

func fixedNow() time.Time {
	return time.Date(2026, 11, 24, 1, 2, 3, 0, time.UTC)
}

func getQueue(t *testing.T, handler http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v2/labs/"+httpLab+"/reviews", nil)
	req.Header.Set("X-Role", "ta")
	req.Header.Set("X-User-Id", httpTA)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "leetkob_session", Value: token})
	}
	handler.ServeHTTP(rec, req)
	return rec
}

func postDecision(t *testing.T, handler http.Handler, token, csrf, key, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "leetkob_session", Value: token})
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Role", "ta")
	handler.ServeHTTP(rec, req)
	return rec
}

func assertConcealed(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content type %q", ct)
	}
	body := rec.Body.String()
	for _, secret := range []string{"MARKER_SOURCE_LINE", "provider-timeout", "ban-sort", "checkerVerdict", "items"} {
		if strings.Contains(body, secret) {
			t.Fatalf("concealed response contains %q: %s", secret, body)
		}
	}
	var problem struct {
		Type   string `json:"type"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Type != ProblemNamespace+"resource-not-found" || problem.Status != http.StatusNotFound || problem.Detail != "The requested resource does not exist." {
		t.Fatalf("problem = %+v", problem)
	}
}

type mapAuth struct {
	sessions map[string]Identity
}

func (m mapAuth) Identify(r *http.Request) (Identity, bool) {
	cookie, err := r.Cookie("leetkob_session")
	if err != nil {
		return Identity{}, false
	}
	identity, ok := m.sessions[cookie.Value]
	return identity, ok
}

type countingStore struct {
	*review.Memory
	lists int
}

func (s *countingStore) List(ctx context.Context, labID string, limit int, cursor string) (review.Page, error) {
	s.lists++
	return s.Memory.List(ctx, labID, limit, cursor)
}

type boomStore struct{}

func (boomStore) Access(context.Context, string, string) (review.Access, error) {
	return review.Access{}, errors.New("password=super-secret")
}

func (boomStore) List(context.Context, string, int, string) (review.Page, error) {
	return review.Page{}, errors.New("password=super-secret")
}

func (boomStore) Decide(context.Context, review.DecisionRequest) (review.DecisionResult, error) {
	return review.DecisionResult{}, errors.New("password=super-secret")
}
