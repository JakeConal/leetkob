package httpapi_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
	"github.com/JakeConal/leetkob/backend/internal/auth/authtest"
	"github.com/JakeConal/leetkob/backend/internal/httpapi"
	"github.com/JakeConal/leetkob/backend/internal/identity"
	"github.com/JakeConal/leetkob/backend/internal/review"
)

const (
	reviewLabID     = "77777777-7777-4777-8777-777777777777"
	reviewFindingID = "88888888-8888-4888-8888-888888888888"
)

type fakeReviewStore struct {
	mu             sync.Mutex
	queue          postgres.ReviewQueue
	queueErr       error
	queueQueries   []postgres.ReviewQueueQuery
	decisionResult review.Finding
	decisionErr    error
	decisions      []postgres.DecisionCommand
}

func (f *fakeReviewStore) ListReviewQueue(_ context.Context, query postgres.ReviewQueueQuery) (postgres.ReviewQueue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queueQueries = append(f.queueQueries, query)
	if f.queueErr != nil {
		return postgres.ReviewQueue{}, f.queueErr
	}
	return f.queue, nil
}

func (f *fakeReviewStore) RecordReviewDecision(_ context.Context, command postgres.DecisionCommand) (review.Finding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.decisions = append(f.decisions, command)
	if f.decisionErr != nil {
		return review.Finding{}, f.decisionErr
	}
	return f.decisionResult, nil
}

type fakeIdempotency struct {
	mu      sync.Mutex
	records map[string]postgres.IdempotencyRecord
}

func newFakeIdempotency() *fakeIdempotency {
	return &fakeIdempotency{records: map[string]postgres.IdempotencyRecord{}}
}

func idempotencyKeyFor(actorID, operation, resourceID, key string) string {
	return actorID + "\x00" + operation + "\x00" + resourceID + "\x00" + key
}

func (f *fakeIdempotency) FindIdempotency(_ context.Context, actorID, operation, resourceID, key string) (postgres.IdempotencyRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.records[idempotencyKeyFor(actorID, operation, resourceID, key)]
	if !ok {
		return postgres.IdempotencyRecord{}, postgres.ErrIdempotencyNotFound
	}
	return record, nil
}

func (f *fakeIdempotency) SaveIdempotency(_ context.Context, record postgres.IdempotencyRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[idempotencyKeyFor(record.ActorID, record.Operation, record.ResourceID, record.IdempotencyKey)] = record
	return nil
}

type reviewFixture struct {
	server  *httptest.Server
	reviews *fakeReviewStore
	idem    *fakeIdempotency
}

func newReviewFixture(t *testing.T, reviews *fakeReviewStore, idem *fakeIdempotency) reviewFixture {
	t.Helper()
	store := authtest.NewFakeStore()
	hash, err := identity.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	store.AddUser(auth.UserRecord{ID: "ta-1", Email: "ta1@leetkob.local", DisplayName: "TA One", Role: auth.RoleTA, PasswordHash: hash},
		auth.ClassSummary{ID: "class-a", Name: "PA3 Class A"})
	store.AddUser(auth.UserRecord{ID: "ta-2", Email: "ta2@leetkob.local", DisplayName: "TA Two", Role: auth.RoleTA, PasswordHash: hash},
		auth.ClassSummary{ID: "class-b", Name: "PA3 Class B"})
	store.AddUser(auth.UserRecord{ID: "student-1", Email: "student1@leetkob.local", DisplayName: "Student One", Role: auth.RoleStudent, PasswordHash: hash},
		auth.ClassSummary{ID: "class-a", Name: "PA3 Class A"})
	service := auth.NewService(store, &testClock{now: time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)}, identity.VerifyPassword)
	server := httptest.NewServer(httpapi.NewServer(httpapi.Deps{
		Auth: service, Reviews: reviews, Idempotency: idem, WebOrigin: webOrigin,
	}))
	t.Cleanup(server.Close)
	return reviewFixture{server: server, reviews: reviews, idem: idem}
}

func reviewSignIn(t *testing.T, f reviewFixture, email string) sessionState {
	t.Helper()
	response := postJSON(t, &http.Client{}, f.server.URL+"/v2/auth/login", loginBody(t, email), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login %s status = %d", email, response.StatusCode)
	}
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	_ = json.NewDecoder(response.Body).Decode(&body)
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies = %d", len(cookies))
	}
	return sessionState{cookie: cookies[0], csrfToken: body.CSRFToken}
}

func getReviewQueue(t *testing.T, f reviewFixture, session sessionState, labID, query string) *http.Response {
	t.Helper()
	target := f.server.URL + "/v2/labs/" + labID + "/reviews"
	if query != "" {
		target += "?" + query
	}
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if session.cookie != nil {
		request.AddCookie(session.cookie)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func postDecision(t *testing.T, f reviewFixture, session sessionState, labID, findingID, body string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost,
		f.server.URL+"/v2/labs/"+labID+"/reviews/"+findingID+"/decision", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "decision-key-0001")
	if session.cookie != nil {
		request.AddCookie(session.cookie)
	}
	if session.csrfToken != "" {
		request.Header.Set("X-CSRF-Token", session.csrfToken)
	}
	if mutate != nil {
		mutate(request)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func decisionBody(expectedRevision int, verdict, reason string, evidence map[string]any) string {
	payload := map[string]any{"expectedRevision": expectedRevision, "verdict": verdict, "reason": reason}
	if evidence != nil {
		payload["evidence"] = evidence
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func findingFixture(id, constraintID, sourceKind, verdict, source string, deduction int) review.Finding {
	finding := review.Finding{
		ID:                 id,
		SubmissionID:       "99999999-9999-4999-8999-999999999999",
		LabID:              reviewLabID,
		ConstraintID:       constraintID,
		ConstraintRevision: 1,
		SourceKind:         sourceKind,
		Source:             source,
		CheckerVerdict:     verdict,
		Evidence:           []review.Evidence{{Line: 2, Text: "    stable_sort(a, a + n);"}},
		DeductionCents:     deduction,
		Revision:           1,
		CreatedAt:          time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC),
	}
	return finding
}

func assertProblem(t *testing.T, response *http.Response, status int, suffix string) map[string]any {
	t.Helper()
	if response.StatusCode != status {
		body := readBody(t, response)
		t.Fatalf("status = %d, want %d (%s)", response.StatusCode, status, body)
	}
	var problem map[string]any
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["type"] != "https://leetkob.example/problems/"+suffix {
		t.Fatalf("problem type = %v", problem["type"])
	}
	return problem
}

func findingKeys(t *testing.T, finding map[string]any) []string {
	t.Helper()
	keys := make([]string, 0, len(finding))
	for key := range finding {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// PA3-REVIEW-01/02: the assigned TA sees per-constraint checker evidence and
// configured deductions; another class's TA gets a concealed 404 and a
// student gets no private data. Refreshing returns the same stable sample.
func TestReviewQueuePrivateAndStable(t *testing.T) {
	pass := findingFixture("a0000000-0000-4000-8000-000000000001", "C1", "model", review.VerdictPass, "int main() { return 0; }", 100)
	pass.Sampled = true
	violation := findingFixture("a0000000-0000-4000-8000-000000000002", "C2", "hard-rule", review.VerdictViolation, "int main() { stable_sort(a, a + n); }", 300)
	timeout := "timeout"
	unsure := findingFixture("a0000000-0000-4000-8000-000000000003", "C3", "model", review.VerdictUnsure, "int main() { return 0; }", 400)
	unsure.UnsureCause = &timeout

	reviews := &fakeReviewStore{queue: postgres.ReviewQueue{
		Items:         []review.Finding{violation, pass, unsure},
		ReviewPending: 3,
	}}
	f := newReviewFixture(t, reviews, newFakeIdempotency())
	ta := reviewSignIn(t, f, "ta1@leetkob.local")

	first := getReviewQueue(t, f, ta, reviewLabID, "")
	if first.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", first.StatusCode)
	}
	if cc := first.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	if ct := first.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	firstBody := readBody(t, first)
	var queue struct {
		Items []struct {
			ID                 string  `json:"id"`
			SubmissionID       string  `json:"submissionId"`
			LabID              string  `json:"labId"`
			ConstraintID       string  `json:"constraintId"`
			ConstraintRevision int     `json:"constraintRevision"`
			Source             string  `json:"source"`
			CheckerVerdict     string  `json:"checkerVerdict"`
			UnsureCause        *string `json:"unsureCause"`
			Evidence           []struct {
				Line int    `json:"line"`
				Text string `json:"text"`
			} `json:"evidence"`
			Deduction      float64 `json:"deduction"`
			RequiredReview bool    `json:"requiredReview"`
			Decision       *struct {
				Verdict string `json:"verdict"`
			} `json:"decision"`
		} `json:"items"`
		NextCursor        *string `json:"nextCursor"`
		ProcessingPending bool    `json:"processingPending"`
		ReviewPending     int     `json:"reviewPending"`
	}
	if err := json.Unmarshal(firstBody, &queue); err != nil {
		t.Fatalf("decode queue: %v", err)
	}
	if len(queue.Items) != 3 || queue.NextCursor != nil || queue.ProcessingPending || queue.ReviewPending != 3 {
		t.Fatalf("queue envelope = %+v", queue)
	}
	for _, item := range queue.Items {
		if item.SubmissionID == "" || item.LabID != reviewLabID || item.Source == "" ||
			len(item.Evidence) != 1 || item.Evidence[0].Line != 2 || item.Decision != nil {
			t.Fatalf("private finding projection = %+v", item)
		}
	}
	if !queue.Items[0].RequiredReview || !queue.Items[1].RequiredReview || !queue.Items[2].RequiredReview {
		t.Fatalf("requiredReview flags = %v/%v/%v",
			queue.Items[0].RequiredReview, queue.Items[1].RequiredReview, queue.Items[2].RequiredReview)
	}
	if queue.Items[0].Deduction != 3 || queue.Items[1].Deduction != 1 || queue.Items[2].Deduction != 4 {
		t.Fatalf("deductions = %v/%v/%v",
			queue.Items[0].Deduction, queue.Items[1].Deduction, queue.Items[2].Deduction)
	}
	if queue.Items[2].UnsureCause == nil || *queue.Items[2].UnsureCause != "timeout" {
		t.Fatalf("unsureCause = %v", queue.Items[2].UnsureCause)
	}

	// Refresh returns the identical membership, sample and evidence.
	second := getReviewQueue(t, f, ta, reviewLabID, "")
	if string(readBody(t, second)) != string(firstBody) {
		t.Fatal("queue refresh changed the stable response")
	}
	queries := reviews.queueQueries
	if len(queries) != 2 || queries[0].ActorID != "ta-1" || queries[0].LabID != reviewLabID ||
		queries[0].Limit != 51 || queries[0].Offset != 0 {
		t.Fatalf("store query = %+v", queries)
	}

	// Another class's TA is concealed; a student never reaches the handler.
	otherReviews := &fakeReviewStore{queueErr: postgres.ErrReviewNotFound}
	other := newReviewFixture(t, otherReviews, newFakeIdempotency())
	otherTA := reviewSignIn(t, other, "ta2@leetkob.local")
	assertProblem(t, getReviewQueue(t, other, otherTA, reviewLabID, ""), http.StatusNotFound, "resource-not-found")

	student := reviewSignIn(t, f, "student1@leetkob.local")
	assertProblem(t, getReviewQueue(t, f, student, reviewLabID, ""), http.StatusForbidden, "role-required")
	assertProblem(t, getReviewQueue(t, f, ta, "not-a-uuid", ""), http.StatusBadRequest, "invalid-request")
}

func TestReviewQueuePagination(t *testing.T) {
	items := make([]review.Finding, 0, 51)
	for i := 0; i < 51; i++ {
		items = append(items, findingFixture(
			fmt.Sprintf("b0000000-0000-4000-8000-%012d", i), fmt.Sprintf("C%02d", i),
			"model", review.VerdictPass, "int main() { return 0; }", 100))
	}
	reviews := &fakeReviewStore{queue: postgres.ReviewQueue{Items: items}}
	f := newReviewFixture(t, reviews, newFakeIdempotency())
	ta := reviewSignIn(t, f, "ta1@leetkob.local")

	response := getReviewQueue(t, f, ta, reviewLabID, "limit=50")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var page struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	if len(page.Items) != 50 || page.NextCursor == nil {
		t.Fatalf("page = %d items, cursor %v", len(page.Items), page.NextCursor)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*page.NextCursor)
	if err != nil || string(decoded) != "v1:50" {
		t.Fatalf("cursor %q decodes to %q (%v)", *page.NextCursor, decoded, err)
	}
	secondPage := getReviewQueue(t, f, ta, reviewLabID, "limit=50&cursor="+*page.NextCursor)
	if secondPage.StatusCode != http.StatusOK {
		t.Fatalf("second page status = %d", secondPage.StatusCode)
	}
	if got := reviews.queueQueries[len(reviews.queueQueries)-1].Offset; got != 50 {
		t.Fatalf("second page offset = %d", got)
	}
	assertProblem(t, getReviewQueue(t, f, ta, reviewLabID, "limit=0"), http.StatusBadRequest, "invalid-request")
}

// PA3-REVIEW-03: an unsure timeout finding exposes its cause and has no
// default decision; the TA must explicitly pass or fail it with a current
// revision and a reason.
func TestReviewDecisionExplicitChoiceOnUnsure(t *testing.T) {
	original := findingFixture(reviewFindingID, "C3", "model", review.VerdictUnsure, "int main() { return 0; }", 400)
	timeout := "timeout"
	original.UnsureCause = &timeout
	original.Evidence = []review.Evidence{}

	decidedAt := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	decided := original
	decided.Revision = 2
	decided.Decision = &review.Decision{
		Verdict:   review.VerdictPass,
		Reason:    "The model timed out; the source satisfies the statement.",
		Revision:  2,
		DecidedBy: "ta-1",
		DecidedAt: decidedAt,
	}

	reviews := &fakeReviewStore{
		queue:          postgres.ReviewQueue{Items: []review.Finding{original}, ReviewPending: 1},
		decisionResult: decided,
	}
	f := newReviewFixture(t, reviews, newFakeIdempotency())
	ta := reviewSignIn(t, f, "ta1@leetkob.local")

	queueResponse := getReviewQueue(t, f, ta, reviewLabID, "")
	var queue struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(queueResponse.Body).Decode(&queue); err != nil {
		t.Fatalf("decode queue: %v", err)
	}
	if len(queue.Items) != 1 || queue.Items[0]["decision"] != nil {
		t.Fatalf("unsure finding has a default decision: %+v", queue.Items)
	}
	if queue.Items[0]["unsureCause"] != "timeout" || queue.Items[0]["requiredReview"] != true {
		t.Fatalf("unsure projection = %+v", queue.Items[0])
	}

	body := decisionBody(1, "pass", "The model timed out; the source satisfies the statement.", nil)
	response := postDecision(t, f, ta, reviewLabID, reviewFindingID, body, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d (%s)", response.StatusCode, readBody(t, response))
	}
	if cc := response.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	var finding map[string]any
	if err := json.NewDecoder(response.Body).Decode(&finding); err != nil {
		t.Fatalf("decode decision response: %v", err)
	}
	wantKeys := []string{
		"checkerVerdict", "constraintId", "constraintRevision", "decision", "deduction",
		"evidence", "id", "labId", "requiredReview", "source", "submissionId", "unsureCause",
	}
	if got := findingKeys(t, finding); strings.Join(got, ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("finding keys = %v, want %v", got, wantKeys)
	}
	decision, ok := finding["decision"].(map[string]any)
	if !ok {
		t.Fatalf("decision = %v", finding["decision"])
	}
	if decision["verdict"] != "pass" || decision["revision"] != float64(2) ||
		decision["decidedBy"] != "ta-1" || decision["decidedAt"] != "2026-11-24T09:00:00Z" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision["reason"] != "The model timed out; the source satisfies the statement." {
		t.Fatalf("decision reason = %v", decision["reason"])
	}

	if len(reviews.decisions) != 1 {
		t.Fatalf("store decisions = %d", len(reviews.decisions))
	}
	command := reviews.decisions[0]
	if command.ActorID != "ta-1" || command.LabID != reviewLabID || command.FindingID != reviewFindingID ||
		command.ExpectedRevision != 1 || command.Verdict != "pass" || command.Evidence != nil {
		t.Fatalf("decision command = %+v", command)
	}
	if command.DecidedAt.Before(time.Now().Add(-time.Minute)) || command.DecidedAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("decidedAt %v is not server time", command.DecidedAt)
	}

	// Replaying the same key and content returns the stored decision without
	// a second store write; changed content conflicts.
	replay := postDecision(t, f, ta, reviewLabID, reviewFindingID, body, nil)
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("replay status = %d", replay.StatusCode)
	}
	var replayed map[string]any
	if err := json.NewDecoder(replay.Body).Decode(&replayed); err != nil {
		t.Fatalf("decode replay: %v", err)
	}
	if replayed["decision"].(map[string]any)["verdict"] != "pass" || len(reviews.decisions) != 1 {
		t.Fatalf("replay wrote again: decisions=%d", len(reviews.decisions))
	}
	changed := postDecision(t, f, ta, reviewLabID, reviewFindingID,
		decisionBody(1, "pass", "a different reason", nil), nil)
	problem := assertProblem(t, changed, http.StatusConflict, "conflict")
	if problem["conflictKind"] != "idempotency-content-mismatch" {
		t.Fatalf("conflictKind = %v", problem["conflictKind"])
	}
}

// PA3-REVIEW-04: a second TA submitting against the same previous revision
// receives the documented stale-revision conflict and the request is not
// applied.
func TestReviewDecisionStaleRevisionConflict(t *testing.T) {
	reviews := &fakeReviewStore{decisionErr: &postgres.StaleRevisionError{Expected: 1, Current: 2}}
	f := newReviewFixture(t, reviews, newFakeIdempotency())
	ta := reviewSignIn(t, f, "ta2@leetkob.local")

	response := postDecision(t, f, ta, reviewLabID, reviewFindingID,
		decisionBody(1, "pass", "I read revision one.", nil), nil)
	problem := assertProblem(t, response, http.StatusConflict, "conflict")
	if problem["conflictKind"] != "stale-revision" ||
		problem["expectedRevision"] != float64(1) || problem["currentRevision"] != float64(2) {
		t.Fatalf("stale problem = %+v", problem)
	}
	if len(reviews.decisions) != 1 {
		t.Fatalf("decision command not attempted: %d", len(reviews.decisions))
	}
}

func TestReviewDecisionRequestProblems(t *testing.T) {
	longReason := strings.Repeat("r", review.MaxReasonChars+1)
	violationEvidence := map[string]any{"line": 2, "text": "    stable_sort(a, a + n);"}

	cases := []struct {
		name       string
		session    string
		labID      string
		findingID  string
		body       string
		mutate     func(*http.Request)
		storeErr   error
		wantStatus int
		wantKind   string
	}{
		{name: "unsure is not a decision", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body: decisionBody(1, "unsure", "maybe", nil), wantStatus: 400, wantKind: "invalid-request"},
		{name: "missing verdict", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body: decisionBody(1, "", "reason", nil), wantStatus: 400, wantKind: "invalid-request"},
		{name: "blank reason", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body: decisionBody(1, "pass", "   ", nil), wantStatus: 400, wantKind: "invalid-request"},
		{name: "oversized reason", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body: decisionBody(1, "pass", longReason, nil), wantStatus: 400, wantKind: "invalid-request"},
		{name: "missing expected revision", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body: decisionBody(0, "pass", "reason", nil), wantStatus: 400, wantKind: "invalid-request"},
		{name: "violation needs evidence", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body: decisionBody(1, "violation", "banned call", nil), wantStatus: 400, wantKind: "invalid-request"},
		{name: "unknown member", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body:       `{"expectedRevision":1,"verdict":"pass","reason":"ok","extra":true}`,
			wantStatus: 400, wantKind: "invalid-request"},
		{name: "trailing json", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body:       `{"expectedRevision":1,"verdict":"pass","reason":"ok"} {}`,
			wantStatus: 400, wantKind: "invalid-request"},
		{name: "bad lab uuid", session: "ta1@leetkob.local", labID: "not-a-uuid", findingID: reviewFindingID,
			body: decisionBody(1, "pass", "ok", nil), wantStatus: 400, wantKind: "invalid-request"},
		{name: "bad finding uuid", session: "ta1@leetkob.local", labID: reviewLabID, findingID: "nope",
			body: decisionBody(1, "pass", "ok", nil), wantStatus: 400, wantKind: "invalid-request"},
		{name: "missing idempotency key", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body:       decisionBody(1, "pass", "ok", nil),
			mutate:     func(r *http.Request) { r.Header.Del("Idempotency-Key") },
			wantStatus: 400, wantKind: "invalid-request"},
		{name: "missing csrf", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body:       decisionBody(1, "pass", "ok", nil),
			mutate:     func(r *http.Request) { r.Header.Del("X-CSRF-Token") },
			wantStatus: 403, wantKind: "role-required"},
		{name: "student cannot decide", session: "student1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body: decisionBody(1, "pass", "ok", nil), wantStatus: 403, wantKind: "role-required"},
		{name: "other class is concealed", session: "ta2@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body: decisionBody(1, "pass", "ok", nil), storeErr: postgres.ErrReviewNotFound,
			wantStatus: 404, wantKind: "resource-not-found"},
		{name: "evidence mismatch", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body:       decisionBody(1, "violation", "banned call", map[string]any{"line": 2, "text": "wrong"}),
			storeErr:   &review.ValidationError{Field: "evidence.text", Detail: "does not match"},
			wantStatus: 400, wantKind: "invalid-request"},
		{name: "oversized body", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body:       decisionBody(1, "pass", strings.Repeat("r", 20<<10), nil),
			wantStatus: 413, wantKind: "payload-too-large"},
		{name: "internal failure", session: "ta1@leetkob.local", labID: reviewLabID, findingID: reviewFindingID,
			body: decisionBody(1, "pass", "ok", nil), storeErr: fmt.Errorf("boom"),
			wantStatus: 500, wantKind: "internal-error"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			reviews := &fakeReviewStore{decisionErr: test.storeErr}
			f := newReviewFixture(t, reviews, newFakeIdempotency())
			session := reviewSignIn(t, f, test.session)
			response := postDecision(t, f, session, test.labID, test.findingID, test.body, test.mutate)
			assertProblem(t, response, test.wantStatus, test.wantKind)
		})
	}

	// A valid violation with matching evidence is accepted and forwarded.
	decided := findingFixture(reviewFindingID, "C1", "model", review.VerdictViolation, reviewSourceForTest, 300)
	decided.Revision = 2
	decided.Decision = &review.Decision{
		Verdict: review.VerdictViolation, Reason: "Calls stable_sort.",
		Evidence: &review.Evidence{Line: 2, Text: "    stable_sort(a, a + n);"},
		Revision: 2, DecidedBy: "ta-1", DecidedAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	}
	reviews := &fakeReviewStore{decisionResult: decided}
	f := newReviewFixture(t, reviews, newFakeIdempotency())
	ta := reviewSignIn(t, f, "ta1@leetkob.local")
	response := postDecision(t, f, ta, reviewLabID, reviewFindingID,
		decisionBody(1, "violation", "Calls stable_sort.", violationEvidence), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid violation status = %d (%s)", response.StatusCode, readBody(t, response))
	}
	if len(reviews.decisions) != 1 {
		t.Fatalf("store decisions = %d", len(reviews.decisions))
	}
	forwarded := reviews.decisions[0].Evidence
	if forwarded == nil || forwarded.Line != 2 || forwarded.Text != "    stable_sort(a, a + n);" {
		t.Fatalf("forwarded evidence = %+v", forwarded)
	}
}

const reviewSourceForTest = "int main() {\n    stable_sort(a, a + n);\n    return 0;\n}\n"
