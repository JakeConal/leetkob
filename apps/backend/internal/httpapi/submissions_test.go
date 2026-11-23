package httpapi_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
	"github.com/JakeConal/leetkob/backend/internal/auth/authtest"
	"github.com/JakeConal/leetkob/backend/internal/httpapi"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

type fakeAcceptor struct {
	mu       sync.Mutex
	commands []postgres.AcceptCommand
	result   postgres.AcceptResult
	err      error
}

func (f *fakeAcceptor) AcceptSubmission(_ context.Context, command postgres.AcceptCommand) (postgres.AcceptResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, command)
	if f.err != nil {
		return postgres.AcceptResult{}, f.err
	}
	result := f.result
	if result.SubmissionID == "" {
		result = postgres.AcceptResult{
			SubmissionID: "12121212-1212-4212-8212-121212121212",
			LabID:        command.LabID,
			State:        "queued",
			ReceivedAt:   command.ReceivedAt,
		}
	}
	return result, nil
}

type submissionFixture struct {
	server   *httptest.Server
	acceptor *fakeAcceptor
}

func newSubmissionFixture(t *testing.T, acceptor *fakeAcceptor) submissionFixture {
	t.Helper()
	store := authtest.NewFakeStore()
	hash, err := identity.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	store.AddUser(auth.UserRecord{ID: "student-1", Email: "student1@leetkob.local", DisplayName: "Student One", Role: auth.RoleStudent, PasswordHash: hash},
		auth.ClassSummary{ID: "class-a", Name: "PA3 Class A"})
	store.AddUser(auth.UserRecord{ID: "ta-1", Email: "ta1@leetkob.local", DisplayName: "TA One", Role: auth.RoleTA, PasswordHash: hash},
		auth.ClassSummary{ID: "class-a", Name: "PA3 Class A"})
	service := auth.NewService(store, &testClock{now: time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)}, identity.VerifyPassword)
	server := httptest.NewServer(httpapi.NewServer(httpapi.Deps{
		Auth: service, WebOrigin: webOrigin, Submissions: acceptor,
	}))
	t.Cleanup(server.Close)
	return submissionFixture{server: server, acceptor: acceptor}
}

type sessionState struct {
	cookie    *http.Cookie
	csrfToken string
}

func signIn(t *testing.T, f submissionFixture, email string) sessionState {
	t.Helper()
	client := &http.Client{}
	response := postJSON(t, client, f.server.URL+"/v2/auth/login", loginBody(t, email), nil)
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

func postSubmission(t *testing.T, f submissionFixture, session sessionState, labID, key, source string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"source": source})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, f.server.URL+"/v2/labs/"+labID+"/submissions", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if session.cookie != nil {
		request.AddCookie(session.cookie)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if session.csrfToken != "" {
		request.Header.Set("X-CSRF-Token", session.csrfToken)
	}
	if mutate != nil {
		mutate(request)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

const testLabID = "34343434-3434-4434-8434-343434343434"

func TestSubmissionAcceptedWithDurableIdentity(t *testing.T) {
	f := newSubmissionFixture(t, &fakeAcceptor{})
	student := signIn(t, f, "student1@leetkob.local")

	response := postSubmission(t, f, student, testLabID, "submit-key-0001", "int main(){return 0;}", nil)
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body %s", response.StatusCode, body)
	}
	if cc := response.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	wait := time.Now()
	var body struct {
		ID         string `json:"id"`
		LabID      string `json:"labId"`
		State      string `json:"state"`
		ReceivedAt string `json:"receivedAt"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.LabID != testLabID || body.State != "queued" || body.ID == "" || body.ReceivedAt == "" {
		t.Fatalf("body = %+v", body)
	}
	if _, err := time.Parse(time.RFC3339Nano, body.ReceivedAt); err != nil {
		t.Fatalf("receivedAt %q: %v", body.ReceivedAt, err)
	}

	command := f.acceptor.commands[0]
	wantDigest := sha256.Sum256([]byte("int main(){return 0;}"))
	if command.ActorID != "student-1" || command.LabID != testLabID ||
		command.IdempotencyKey != "submit-key-0001" || command.Fingerprint != hex.EncodeToString(wantDigest[:]) ||
		command.Source != "int main(){return 0;}" {
		t.Fatalf("command = %+v", command)
	}
	if command.ReceivedAt.Before(wait.Add(-time.Minute)) || command.ReceivedAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("receivedAt %v is not server time", command.ReceivedAt)
	}
}

func TestSubmissionAuthorizationAndRequestProblems(t *testing.T) {
	f := newSubmissionFixture(t, &fakeAcceptor{})
	student := signIn(t, f, "student1@leetkob.local")
	ta := signIn(t, f, "ta1@leetkob.local")

	cases := []struct {
		name    string
		session sessionState
		labID   string
		key     string
		mutate  func(*http.Request)
		status  int
		kind    string
	}{
		{name: "ta cannot submit", session: ta, labID: testLabID, key: "submit-key-0002", status: 403, kind: "role-required"},
		{name: "missing csrf", session: sessionState{cookie: student.cookie}, labID: testLabID, key: "submit-key-0003", status: 403, kind: "role-required"},
		{name: "short key", session: student, labID: testLabID, key: "short", status: 400, kind: "invalid-request"},
		{name: "missing key", session: student, labID: testLabID, key: "", status: 400, kind: "invalid-request"},
		{name: "bad lab uuid", session: student, labID: "not-a-uuid", key: "submit-key-0004", status: 400, kind: "invalid-request"},
		{name: "unknown field", session: student, labID: testLabID, key: "submit-key-0005",
			mutate: func(r *http.Request) {
				payload := `{"source":"int main(){}","extra":true}`
				r.Body = io.NopCloser(strings.NewReader(payload))
				r.ContentLength = int64(len(payload))
			}, status: 400, kind: "invalid-request"},
		{name: "oversized source", session: student, labID: testLabID, key: "submit-key-0006",
			mutate: func(r *http.Request) {
				big := strings.Repeat("a", httpapi.MaxSubmissionSourceBytes+1024)
				payload := `{"source":"` + big + `"}`
				r.Body = io.NopCloser(strings.NewReader(payload))
				r.ContentLength = int64(len(payload))
			}, status: 400, kind: "invalid-request"},
		{name: "oversized body", session: student, labID: testLabID, key: "submit-key-0007",
			mutate: func(r *http.Request) {
				big := strings.Repeat("a", httpapi.MaxSubmissionSourceBytes+8192)
				payload := `{"source":"` + big + `"}`
				r.Body = io.NopCloser(strings.NewReader(payload))
				r.ContentLength = int64(len(payload))
			}, status: 413, kind: "payload-too-large"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := postSubmission(t, f, test.session, test.labID, test.key, "int main(){}", test.mutate)
			if response.StatusCode != test.status {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("status = %d, want %d (%s)", response.StatusCode, test.status, body)
			}
			if got := problemType(t, response); got != "https://leetkob.example/problems/"+test.kind {
				t.Fatalf("problem type = %q", got)
			}
		})
	}
}

func TestSubmissionFailureMappings(t *testing.T) {
	deadline := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		err        error
		status     int
		kind       string
		extensions map[string]any
	}{
		{name: "window closed", err: &postgres.SubmissionWindowClosedError{Deadline: deadline}, status: 409, kind: "conflict",
			extensions: map[string]any{"conflictKind": "submission-window-closed"}},
		{name: "content mismatch", err: postgres.ErrIdempotencyContentConflict, status: 409, kind: "conflict",
			extensions: map[string]any{"conflictKind": "idempotency-content-mismatch"}},
		{name: "concealed not found", err: postgres.ErrLabNotAccepting, status: 404, kind: "resource-not-found"},
		{name: "internal failure", err: errors.New("boom"), status: 500, kind: "internal-error"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newSubmissionFixture(t, &fakeAcceptor{err: test.err})
			student := signIn(t, f, "student1@leetkob.local")
			response := postSubmission(t, f, student, testLabID, "submit-key-0100", "int main(){}", nil)
			if response.StatusCode != test.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.status)
			}
			var problem map[string]any
			if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if problem["type"] != "https://leetkob.example/problems/"+test.kind {
				t.Fatalf("type = %v", problem["type"])
			}
			for key, want := range test.extensions {
				if fmt.Sprint(problem[key]) != fmt.Sprint(want) {
					t.Fatalf("extension %s = %v, want %v", key, problem[key], want)
				}
			}
			if test.name == "window closed" {
				if _, ok := problem["deadline"].(string); !ok {
					t.Fatalf("deadline extension missing: %+v", problem)
				}
			}
		})
	}
}

func TestSubmissionReplayReturnsStoredResult(t *testing.T) {
	stored := postgres.AcceptResult{
		SubmissionID: "56565656-5656-4656-8656-565656565656",
		LabID:        testLabID,
		State:        "queued",
		ReceivedAt:   time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC),
		Replayed:     true,
	}
	f := newSubmissionFixture(t, &fakeAcceptor{result: stored})
	student := signIn(t, f, "student1@leetkob.local")

	response := postSubmission(t, f, student, testLabID, "submit-key-0200", "int main(){}", nil)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var body struct {
		ID         string `json:"id"`
		ReceivedAt string `json:"receivedAt"`
	}
	_ = json.NewDecoder(response.Body).Decode(&body)
	if body.ID != stored.SubmissionID || !strings.HasPrefix(body.ReceivedAt, "2026-11-23T09:00:00") {
		t.Fatalf("replay body = %+v", body)
	}
}
