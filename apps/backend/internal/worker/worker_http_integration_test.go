package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/auth"
	"github.com/JakeConal/leetkob/backend/internal/httpapi"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

type workerHTTPSession struct {
	client *http.Client
	csrf   string
}

func loginWorkerHTTP(t *testing.T, baseURL, email string) workerHTTPSession {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, workerTestPassword)
	response := workerHTTPRequest(t, client, http.MethodPost, baseURL+"/v2/auth/login", body, nil)
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("login %s = %d: %s", email, response.StatusCode, raw)
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if session.CSRFToken == "" {
		t.Fatalf("login %s returned no CSRF token", email)
	}
	return workerHTTPSession{client: client, csrf: session.CSRFToken}
}

func workerHTTPRequest(t *testing.T, client *http.Client, method, url, body string, headers map[string]string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func decodeWorkerHTTP(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

// PA3-JUDGE-07 end to end: the real HTTP ingress accepts the submission while
// the worker is stopped, the real worker judges it through the sandbox, and
// the owner-safe result endpoint exposes verdicts/timings to the owner and the
// assigned TA while concealing the same ID from another student and another
// class's TA.
func TestHTTPIngressToJudgedResultStaysOwnerPrivate(t *testing.T) {
	image := workerSandboxImage(t)
	store := startWorkerTestDatabase(t)
	seedWorkerLab(t, store)
	controller := newSandboxJudge(t, image)

	service := auth.NewService(store, auth.SystemClock{}, identity.VerifyPassword)
	server := httptest.NewServer(httpapi.NewServer(httpapi.Deps{
		Pinger:           store,
		Auth:             service,
		Labs:             store,
		Idempotency:      store,
		Submissions:      store,
		SubmissionReader: store,
		WebOrigin:        "http://127.0.0.1:3000",
	}))
	t.Cleanup(server.Close)

	owner := loginWorkerHTTP(t, server.URL, "worker-student@example.test")
	otherStudent := loginWorkerHTTP(t, server.URL, "worker-student2@example.test")
	assignedTA := loginWorkerHTTP(t, server.URL, "worker-ta@example.test")
	otherTA := loginWorkerHTTP(t, server.URL, "worker-ta2@example.test")

	// The worker is deliberately not running yet.
	source, err := json.Marshal(map[string]string{"source": workerSource})
	if err != nil {
		t.Fatalf("marshal source: %v", err)
	}
	submit := workerHTTPRequest(t, owner.client, http.MethodPost,
		server.URL+"/v2/labs/"+workerLabID+"/submissions", string(source),
		map[string]string{
			"X-CSRF-Token":    owner.csrf,
			"Idempotency-Key": "http-worker-key-1",
		})
	if submit.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(submit.Body)
		t.Fatalf("submit = %d: %s", submit.StatusCode, raw)
	}
	accepted := decodeWorkerHTTP(t, submit)
	submissionID, _ := accepted["id"].(string)
	if submissionID == "" || accepted["state"] != "queued" {
		t.Fatalf("accept body = %+v", accepted)
	}

	queuedResponse := workerHTTPRequest(t, owner.client, http.MethodGet,
		server.URL+"/v2/submissions/"+submissionID, "", nil)
	if queuedResponse.StatusCode != http.StatusOK {
		t.Fatalf("queued read = %d", queuedResponse.StatusCode)
	}
	queued := decodeWorkerHTTP(t, queuedResponse)
	if queued["state"] != "queued" {
		t.Fatalf("queued state = %v", queued["state"])
	}
	if _, present := queued["judge"]; present {
		t.Fatalf("unjudged response exposed judge: %+v", queued)
	}

	for name, session := range map[string]workerHTTPSession{
		"other student":  otherStudent,
		"other-class TA": otherTA,
	} {
		concealed := workerHTTPRequest(t, session.client, http.MethodGet,
			server.URL+"/v2/submissions/"+submissionID, "", nil)
		if concealed.StatusCode != http.StatusNotFound {
			t.Fatalf("%s read = %d, want 404", name, concealed.StatusCode)
		}
		concealedBody := decodeWorkerHTTP(t, concealed)
		if concealedBody["type"] != "https://leetkob.example/problems/resource-not-found" {
			t.Fatalf("%s problem = %+v", name, concealedBody)
		}
	}

	cancel, done := startDurableWorker(t, store, store, controller, "worker-http", 2*time.Minute)
	judgedBody := waitForJudgedOverHTTP(t, owner, server.URL, submissionID)
	stopDurableWorker(t, cancel, done)

	judge, ok := judgedBody["judge"].(map[string]any)
	if !ok || judge["verdict"] != "AC" {
		t.Fatalf("judged body = %+v", judgedBody)
	}
	tests, ok := judge["tests"].([]any)
	if !ok || len(tests) != 2 {
		t.Fatalf("judge tests = %+v", judge["tests"])
	}
	for _, item := range tests {
		test := item.(map[string]any)
		if test["verdict"] != "AC" {
			t.Fatalf("per-test verdict = %+v", test)
		}
		if _, exists := test["timeMs"]; !exists {
			t.Fatalf("per-test timing missing: %+v", test)
		}
	}
	if got := countRows(t, store, `SELECT count(*) FROM submission_test_results WHERE submission_id = $1`, submissionID); got != 2 {
		t.Fatalf("stored test rows = %d, want 2", got)
	}

	assignedRead := workerHTTPRequest(t, assignedTA.client, http.MethodGet,
		server.URL+"/v2/submissions/"+submissionID, "", nil)
	if assignedRead.StatusCode != http.StatusOK {
		t.Fatalf("assigned TA read = %d", assignedRead.StatusCode)
	}
	if cc := assignedRead.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	concealedAfter := workerHTTPRequest(t, otherTA.client, http.MethodGet,
		server.URL+"/v2/submissions/"+submissionID, "", nil)
	if concealedAfter.StatusCode != http.StatusNotFound {
		t.Fatalf("other-class TA judged read = %d, want 404", concealedAfter.StatusCode)
	}

	list := workerHTTPRequest(t, owner.client, http.MethodGet,
		server.URL+"/v2/labs/"+workerLabID+"/submissions", "", nil)
	if list.StatusCode != http.StatusOK {
		t.Fatalf("owner list = %d", list.StatusCode)
	}
	listed := decodeWorkerHTTP(t, list)
	items, ok := listed["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("owner submission list = %+v", listed["items"])
	}
	if items[0].(map[string]any)["id"] != submissionID {
		t.Fatalf("owner list exposed another submission: %+v", items[0])
	}
}

func waitForJudgedOverHTTP(t *testing.T, session workerHTTPSession, baseURL, submissionID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(workerJudgeDeadline)
	for time.Now().Before(deadline) {
		response := workerHTTPRequest(t, session.client, http.MethodGet,
			baseURL+"/v2/submissions/"+submissionID, "", nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status read = %d", response.StatusCode)
		}
		body := decodeWorkerHTTP(t, response)
		if body["state"] == "judged" {
			return body
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("submission %s did not reach judged over HTTP", submissionID)
	return nil
}
