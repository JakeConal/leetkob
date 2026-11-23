package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
	"github.com/JakeConal/leetkob/backend/internal/auth/authtest"
	"github.com/JakeConal/leetkob/backend/internal/httpapi"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

const (
	readClassAID     = "11111111-1111-4111-8111-aaaaaaaaaaaa"
	readClassBID     = "22222222-2222-4222-8222-bbbbbbbbbbbb"
	readTAOneID      = "33333333-3333-4333-8333-aaaaaaaaaaaa"
	readTATwoID      = "44444444-4444-4444-8444-bbbbbbbbbbbb"
	readStudentOneID = "55555555-5555-4555-8555-aaaaaaaaaaaa"
	readStudentTwoID = "66666666-6666-4666-8666-aaaaaaaaaaaa"
	readAdminID      = "77777777-7777-4777-8777-aaaaaaaaaaaa"
	readLabAID       = "88888888-8888-4888-8888-aaaaaaaaaaaa"
	readLabBID       = "99999999-9999-4999-8999-bbbbbbbbbbbb"
	readSubmission1  = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"
	readSubmission2  = "bbbbbbbb-2222-4222-8222-aaaaaaaaaaaa"
	readSubmission3  = "cccccccc-3333-4333-8333-bbbbbbbbbbbb"
)

// memorySubmissionReader is an in-memory httpapi.SubmissionReader. It keeps
// the HTTP authorization behavior testable without a database.
type memorySubmissionReader struct {
	views map[string]postgres.SubmissionView
	labs  map[string]string
}

func newMemorySubmissionReader() *memorySubmissionReader {
	return &memorySubmissionReader{
		views: map[string]postgres.SubmissionView{},
		labs:  map[string]string{readLabAID: readClassAID, readLabBID: readClassBID},
	}
}

func (m *memorySubmissionReader) add(view postgres.SubmissionView) {
	m.views[view.ID] = view
}

func (m *memorySubmissionReader) GetSubmissionForRead(_ context.Context, submissionID string) (postgres.SubmissionView, error) {
	view, ok := m.views[submissionID]
	if !ok {
		return postgres.SubmissionView{}, postgres.ErrSubmissionNotFound
	}
	return view, nil
}

func (m *memorySubmissionReader) LabClassID(_ context.Context, labID string) (string, error) {
	classID, ok := m.labs[labID]
	if !ok {
		return "", postgres.ErrLabNotFound
	}
	return classID, nil
}

func (m *memorySubmissionReader) ListSubmissionsForLab(_ context.Context, labID string, limit, offset int) ([]postgres.SubmissionView, error) {
	return m.list(func(view postgres.SubmissionView) bool { return view.LabID == labID }, limit, offset), nil
}

func (m *memorySubmissionReader) ListSubmissionsForStudent(_ context.Context, labID, studentID string, limit, offset int) ([]postgres.SubmissionView, error) {
	return m.list(func(view postgres.SubmissionView) bool {
		return view.LabID == labID && view.StudentID == studentID
	}, limit, offset), nil
}

func (m *memorySubmissionReader) list(keep func(postgres.SubmissionView) bool, limit, offset int) []postgres.SubmissionView {
	matching := make([]postgres.SubmissionView, 0, len(m.views))
	for _, view := range m.views {
		if keep(view) {
			matching = append(matching, view)
		}
	}
	sort.Slice(matching, func(i, j int) bool {
		if !matching[i].ReceivedAt.Equal(matching[j].ReceivedAt) {
			return matching[i].ReceivedAt.After(matching[j].ReceivedAt)
		}
		return matching[i].ID < matching[j].ID
	})
	if offset >= len(matching) {
		return nil
	}
	end := offset + limit
	if end > len(matching) {
		end = len(matching)
	}
	return matching[offset:end]
}

type submissionReadFixture struct {
	server *httptest.Server
	reads  *memorySubmissionReader
}

func newSubmissionReadFixture(t *testing.T) *submissionReadFixture {
	t.Helper()
	store := authtest.NewFakeStore()
	hash, err := identity.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	classA := auth.ClassSummary{ID: readClassAID, Name: "PA3 Class A"}
	classB := auth.ClassSummary{ID: readClassBID, Name: "PA3 Class B"}
	store.AddUser(auth.UserRecord{ID: readTAOneID, Email: "ta1@leetkob.local", DisplayName: "TA One", Role: auth.RoleTA, PasswordHash: hash}, classA)
	store.AddUser(auth.UserRecord{ID: readTATwoID, Email: "ta2@leetkob.local", DisplayName: "TA Two", Role: auth.RoleTA, PasswordHash: hash}, classB)
	store.AddUser(auth.UserRecord{ID: readStudentOneID, Email: "student1@leetkob.local", DisplayName: "Student One", Role: auth.RoleStudent, PasswordHash: hash}, classA)
	store.AddUser(auth.UserRecord{ID: readStudentTwoID, Email: "student2@leetkob.local", DisplayName: "Student Two", Role: auth.RoleStudent, PasswordHash: hash}, classA)
	store.AddUser(auth.UserRecord{ID: readAdminID, Email: "admin@leetkob.local", DisplayName: "Admin", Role: auth.RoleAdmin, PasswordHash: hash})
	service := auth.NewService(store, &testClock{now: time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)}, identity.VerifyPassword)
	reads := newMemorySubmissionReader()
	server := httptest.NewServer(httpapi.NewServer(httpapi.Deps{
		Auth: service, SubmissionReader: reads, WebOrigin: webOrigin,
	}))
	t.Cleanup(server.Close)
	return &submissionReadFixture{server: server, reads: reads}
}

func (f *submissionReadFixture) login(t *testing.T, email string) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	response := postJSON(t, client, f.server.URL+"/v2/auth/login", loginBody(t, email), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s login = %d", email, response.StatusCode)
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	return client, session.CSRFToken
}

func ptr(value string) *string { return &value }

func seedReadSubmissions(f *submissionReadFixture) {
	base := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	f.reads.add(postgres.SubmissionView{
		ID: readSubmission1, LabID: readLabAID, StudentID: readStudentOneID,
		ClassID: readClassAID, State: "judged", ReceivedAt: base,
		Verdict: ptr("AC"),
		Tests: []postgres.SubmissionTestResult{
			{TestID: "01", Verdict: "AC", TimeMs: 12},
			{TestID: "02", Verdict: "AC", TimeMs: 7},
		},
	})
	f.reads.add(postgres.SubmissionView{
		ID: readSubmission2, LabID: readLabAID, StudentID: readStudentTwoID,
		ClassID: readClassAID, State: "queued", ReceivedAt: base.Add(time.Minute),
	})
	f.reads.add(postgres.SubmissionView{
		ID: readSubmission3, LabID: readLabBID, StudentID: readStudentOneID,
		ClassID: readClassBID, State: "judged", ReceivedAt: base.Add(2 * time.Minute),
		Verdict: ptr("WA"),
	})
}

func doRead(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func decodeReadBody(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	return decoded
}

// An unjudged submission has no judge member at all, and a judged one exposes
// only the aggregate verdict and per-test timings.
func TestOwnSubmissionProjectionHidesJudgeUntilJudged(t *testing.T) {
	f := newSubmissionReadFixture(t)
	seedReadSubmissions(f)
	student, _ := f.login(t, "student1@leetkob.local")

	ownJudged := doRead(t, student, f.server.URL+"/v2/submissions/"+readSubmission1)
	if ownJudged.StatusCode != http.StatusOK {
		t.Fatalf("own submission = %d", ownJudged.StatusCode)
	}
	if cc := ownJudged.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	judged := decodeReadBody(t, ownJudged)
	judge, ok := judged["judge"].(map[string]any)
	if !ok {
		t.Fatalf("judged submission has no judge: %+v", judged)
	}
	if judge["verdict"] != "AC" {
		t.Fatalf("verdict = %v", judge["verdict"])
	}
	tests, ok := judge["tests"].([]any)
	if !ok || len(tests) != 2 {
		t.Fatalf("judge tests = %+v", judge["tests"])
	}
	first := tests[0].(map[string]any)
	if first["id"] != "01" || first["verdict"] != "AC" || first["timeMs"] != float64(12) {
		t.Fatalf("first test = %+v", first)
	}
	for key := range judged {
		switch key {
		case "id", "labId", "state", "receivedAt", "judge":
		default:
			t.Fatalf("submission response exposed unexpected member %q", key)
		}
	}
	for key := range judge {
		if key != "verdict" && key != "tests" {
			t.Fatalf("judge exposed unexpected member %q", key)
		}
	}
	for key := range first {
		if key != "id" && key != "verdict" && key != "timeMs" {
			t.Fatalf("test result exposed unexpected member %q", key)
		}
	}
}

func TestUnjudgedSubmissionHasNoJudgeMember(t *testing.T) {
	f := newSubmissionReadFixture(t)
	seedReadSubmissions(f)
	student, _ := f.login(t, "student2@leetkob.local")

	response := doRead(t, student, f.server.URL+"/v2/submissions/"+readSubmission2)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("own queued submission = %d", response.StatusCode)
	}
	body := decodeReadBody(t, response)
	if body["state"] != "queued" {
		t.Fatalf("state = %v", body["state"])
	}
	if _, present := body["judge"]; present {
		t.Fatalf("unjudged submission exposed a judge member: %+v", body)
	}
}

// PA3-JUDGE-07 privacy: another student, another class's TA and an admin
// receive the same concealed not-found body as an absent identifier.
func TestSubmissionReadPrivacyIsConcealed(t *testing.T) {
	f := newSubmissionReadFixture(t)
	seedReadSubmissions(f)
	otherStudent, _ := f.login(t, "student2@leetkob.local")
	otherTA, _ := f.login(t, "ta2@leetkob.local")
	admin, _ := f.login(t, "admin@leetkob.local")

	absent := doRead(t, otherTA, f.server.URL+"/v2/submissions/11111111-1111-4111-8111-999999999999")
	absentBody := decodeReadBody(t, absent)
	if absent.StatusCode != http.StatusNotFound {
		t.Fatalf("absent = %d, want 404", absent.StatusCode)
	}

	cases := []struct {
		name   string
		client *http.Client
		id     string
	}{
		{"other student in the same class", otherStudent, readSubmission1},
		{"TA of another class", otherTA, readSubmission1},
		{"admin is not an implicit TA", admin, readSubmission1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := doRead(t, testCase.client, f.server.URL+"/v2/submissions/"+testCase.id)
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want concealed 404", response.StatusCode)
			}
			body := decodeReadBody(t, response)
			if body["type"] != absentBody["type"] || body["detail"] != absentBody["detail"] {
				t.Fatalf("concealed body %+v differs from absent %+v", body, absentBody)
			}
			raw, _ := json.Marshal(body)
			if strings.Contains(string(raw), "AC") || strings.Contains(string(raw), "queued") {
				t.Fatalf("concealed response leaked judge data: %s", raw)
			}
		})
	}
}

func TestAssignedTAReadsClassSubmission(t *testing.T) {
	f := newSubmissionReadFixture(t)
	seedReadSubmissions(f)
	assignedTA, _ := f.login(t, "ta1@leetkob.local")

	response := doRead(t, assignedTA, f.server.URL+"/v2/submissions/"+readSubmission1)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("assigned TA read = %d", response.StatusCode)
	}
	body := decodeReadBody(t, response)
	if body["id"] != readSubmission1 {
		t.Fatalf("body = %+v", body)
	}

	// The assigned TA still cannot read another class's submission.
	crossClass := doRead(t, assignedTA, f.server.URL+"/v2/submissions/"+readSubmission3)
	if crossClass.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-class TA read = %d, want 404", crossClass.StatusCode)
	}
}

func TestSubmissionListIsOwnerScopedForStudentsAndClassScopedForTAs(t *testing.T) {
	f := newSubmissionReadFixture(t)
	seedReadSubmissions(f)
	studentOne, _ := f.login(t, "student1@leetkob.local")
	studentTwo, _ := f.login(t, "student2@leetkob.local")
	assignedTA, _ := f.login(t, "ta1@leetkob.local")
	otherTA, _ := f.login(t, "ta2@leetkob.local")

	one := doRead(t, studentOne, f.server.URL+"/v2/labs/"+readLabAID+"/submissions")
	if one.StatusCode != http.StatusOK {
		t.Fatalf("student list = %d", one.StatusCode)
	}
	oneBody := decodeReadBody(t, one)
	oneItems := oneBody["items"].([]any)
	if len(oneItems) != 1 || oneItems[0].(map[string]any)["id"] != readSubmission1 {
		t.Fatalf("student one items = %+v", oneItems)
	}

	two := doRead(t, studentTwo, f.server.URL+"/v2/labs/"+readLabAID+"/submissions")
	twoItems := decodeReadBody(t, two)["items"].([]any)
	if len(twoItems) != 1 || twoItems[0].(map[string]any)["id"] != readSubmission2 {
		t.Fatalf("student two items = %+v", twoItems)
	}

	taList := doRead(t, assignedTA, f.server.URL+"/v2/labs/"+readLabAID+"/submissions")
	if taList.StatusCode != http.StatusOK {
		t.Fatalf("TA list = %d", taList.StatusCode)
	}
	taBody := decodeReadBody(t, taList)
	if len(taBody["items"].([]any)) != 2 {
		t.Fatalf("TA items = %+v", taBody["items"])
	}
	if _, present := taBody["items"].([]any)[0].(map[string]any)["judge"]; present {
		t.Fatalf("newest item should be the unjudged submission: %+v", taBody["items"])
	}

	crossTA := doRead(t, otherTA, f.server.URL+"/v2/labs/"+readLabAID+"/submissions")
	if crossTA.StatusCode != http.StatusNotFound {
		t.Fatalf("other-class TA list = %d, want 404", crossTA.StatusCode)
	}
	crossStudent := doRead(t, studentOne, f.server.URL+"/v2/labs/"+readLabBID+"/submissions")
	if crossStudent.StatusCode != http.StatusNotFound {
		t.Fatalf("student list for another class's lab = %d, want 404", crossStudent.StatusCode)
	}
}

func TestSubmissionListPaginationAndValidation(t *testing.T) {
	f := newSubmissionReadFixture(t)
	seedReadSubmissions(f)
	assignedTA, _ := f.login(t, "ta1@leetkob.local")

	first := doRead(t, assignedTA, f.server.URL+"/v2/labs/"+readLabAID+"/submissions?limit=1")
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first page = %d", first.StatusCode)
	}
	firstBody := decodeReadBody(t, first)
	if len(firstBody["items"].([]any)) != 1 || firstBody["nextCursor"] == nil {
		t.Fatalf("first page = %+v", firstBody)
	}
	cursor := firstBody["nextCursor"].(string)
	second := doRead(t, assignedTA,
		f.server.URL+"/v2/labs/"+readLabAID+"/submissions?limit=1&cursor="+cursor)
	secondBody := decodeReadBody(t, second)
	if len(secondBody["items"].([]any)) != 1 || secondBody["nextCursor"] != nil {
		t.Fatalf("second page = %+v", secondBody)
	}

	for _, path := range []string{
		"/v2/labs/" + readLabAID + "/submissions?limit=0",
		"/v2/labs/" + readLabAID + "/submissions?limit=101",
		"/v2/labs/" + readLabAID + "/submissions?cursor=not-a-cursor",
		"/v2/submissions/not-a-uuid",
		"/v2/labs/not-a-uuid/submissions",
	} {
		response := doRead(t, assignedTA, f.server.URL+path)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", path, response.StatusCode)
		}
		if problem := decodeProblem(t, response); problem.Type != httpapi.ProblemNamespace+"invalid-request" {
			t.Fatalf("%s problem = %+v", path, problem)
		}
	}
}

func TestSubmissionReadRequiresAuthentication(t *testing.T) {
	f := newSubmissionReadFixture(t)
	client := &http.Client{}
	response := doRead(t, client, f.server.URL+"/v2/submissions/"+readSubmission1)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous read = %d, want 401", response.StatusCode)
	}
	if problem := decodeProblem(t, response); problem.Type != httpapi.ProblemNamespace+"authentication-required" {
		t.Fatalf("anonymous problem = %+v", problem)
	}
}
