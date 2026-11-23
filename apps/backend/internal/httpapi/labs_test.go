package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
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
)

const (
	classAID = "11111111-1111-4111-8111-111111111111"
	classBID = "22222222-2222-4222-8222-222222222222"
	classCID = "33333333-3333-4333-8333-333333333333"
	classDID = "44444444-4444-4444-8444-444444444444"

	taOneID      = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	taTwoID      = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	studentOneID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	studentTwoID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	adminID      = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	multiID      = "ffffffff-ffff-4fff-8fff-ffffffffffff"

	hiddenSentinel = "HIDDEN-TEST-SENTINEL-DO-NOT-SERIALIZE"
	hiddenAnswer   = "HIDDEN-ANSWER-SENTINEL-DO-NOT-SERIALIZE"
)

// memoryLabStore implements httpapi.LabStore without a database.
type memoryLabStore struct {
	mu        sync.Mutex
	labs      map[string]postgres.LabRecord
	createErr error
}

func newMemoryLabStore() *memoryLabStore {
	return &memoryLabStore{labs: map[string]postgres.LabRecord{}}
}

func (m *memoryLabStore) CreateLab(_ context.Context, lab postgres.LabRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.labs[lab.ID]; exists {
		return fmt.Errorf("memory lab store: duplicate lab %s", lab.ID)
	}
	m.labs[lab.ID] = lab
	return nil
}

func (m *memoryLabStore) GetLabForClass(_ context.Context, labID, classID string) (postgres.LabRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lab, ok := m.labs[labID]
	if !ok || lab.ClassID != classID {
		return postgres.LabRecord{}, postgres.ErrLabNotFound
	}
	return lab, nil
}

func (m *memoryLabStore) ListLabsByClass(_ context.Context, classID string, limit, offset int) ([]postgres.LabRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	matching := make([]postgres.LabRecord, 0, len(m.labs))
	for _, lab := range m.labs {
		if lab.ClassID == classID {
			matching = append(matching, lab)
		}
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].ID < matching[j].ID })
	if offset >= len(matching) {
		return nil, nil
	}
	end := offset + limit
	if end > len(matching) {
		end = len(matching)
	}
	return matching[offset:end], nil
}

func (m *memoryLabStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.labs)
}

// memoryIdempotencyStore implements httpapi.IdempotencyStore in memory.
type memoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]postgres.IdempotencyRecord
}

func newMemoryIdempotencyStore() *memoryIdempotencyStore {
	return &memoryIdempotencyStore{records: map[string]postgres.IdempotencyRecord{}}
}

func idempotencyMapKey(actorID, operation, resourceID, key string) string {
	return actorID + "\x00" + operation + "\x00" + resourceID + "\x00" + key
}

func (m *memoryIdempotencyStore) FindIdempotency(_ context.Context, actorID, operation, resourceID, key string) (postgres.IdempotencyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[idempotencyMapKey(actorID, operation, resourceID, key)]
	if !ok {
		return postgres.IdempotencyRecord{}, postgres.ErrIdempotencyNotFound
	}
	return record, nil
}

func (m *memoryIdempotencyStore) SaveIdempotency(_ context.Context, record postgres.IdempotencyRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := idempotencyMapKey(record.ActorID, record.Operation, record.ResourceID, record.IdempotencyKey)
	if _, exists := m.records[key]; exists {
		return fmt.Errorf("memory idempotency store: duplicate key")
	}
	m.records[key] = record
	return nil
}

type labFixture struct {
	server    *httptest.Server
	authStore *authtest.FakeStore
	labs      *memoryLabStore
	idem      *memoryIdempotencyStore
	clock     *testClock
}

func newLabFixture(t *testing.T) *labFixture {
	t.Helper()
	store := authtest.NewFakeStore()
	hash, err := identity.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	classA := auth.ClassSummary{ID: classAID, Name: "PA3 Class A"}
	classB := auth.ClassSummary{ID: classBID, Name: "PA3 Class B"}
	classC := auth.ClassSummary{ID: classCID, Name: "PA3 Class C"}
	classD := auth.ClassSummary{ID: classDID, Name: "PA3 Class D"}
	store.AddUser(auth.UserRecord{ID: taOneID, Email: "ta1@leetkob.local", DisplayName: "TA One", Role: auth.RoleTA, PasswordHash: hash}, classA)
	store.AddUser(auth.UserRecord{ID: taTwoID, Email: "ta2@leetkob.local", DisplayName: "TA Two", Role: auth.RoleTA, PasswordHash: hash}, classB)
	store.AddUser(auth.UserRecord{ID: studentOneID, Email: "student1@leetkob.local", DisplayName: "Student One", Role: auth.RoleStudent, PasswordHash: hash}, classA)
	store.AddUser(auth.UserRecord{ID: studentTwoID, Email: "student2@leetkob.local", DisplayName: "Student Two", Role: auth.RoleStudent, PasswordHash: hash}, classA)
	store.AddUser(auth.UserRecord{ID: adminID, Email: "admin@leetkob.local", DisplayName: "Admin", Role: auth.RoleAdmin, PasswordHash: hash})
	store.AddUser(auth.UserRecord{ID: multiID, Email: "multi@leetkob.local", DisplayName: "Multi Class", Role: auth.RoleStudent, PasswordHash: hash}, classA, classC, classD)

	clock := &testClock{now: time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)}
	service := auth.NewService(store, clock, identity.VerifyPassword)
	labs := newMemoryLabStore()
	idem := newMemoryIdempotencyStore()
	server := httptest.NewServer(httpapi.NewServer(httpapi.Deps{
		Auth:        service,
		Labs:        labs,
		Idempotency: idem,
		WebOrigin:   webOrigin,
	}))
	t.Cleanup(server.Close)
	return &labFixture{server: server, authStore: store, labs: labs, idem: idem, clock: clock}
}

func (f *labFixture) client(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func (f *labFixture) login(t *testing.T, email string) (*http.Client, string) {
	t.Helper()
	client := f.client(t)
	response := postJSON(t, client, f.server.URL+"/v2/auth/login", loginBody(t, email), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s login = %d, want 200", email, response.StatusCode)
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatalf("%s decode login: %v", email, err)
	}
	if session.CSRFToken == "" {
		t.Fatalf("%s login returned no CSRF token", email)
	}
	return client, session.CSRFToken
}

func doRequest(t *testing.T, client *http.Client, method, url, body string, headers map[string]string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build %s request: %v", method, err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("do %s request: %v", method, err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func readBody(t *testing.T, response *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return body
}

type apiProblem struct {
	Type         string `json:"type"`
	Title        string `json:"title"`
	Status       int    `json:"status"`
	Detail       string `json:"detail"`
	Instance     string `json:"instance"`
	ConflictKind string `json:"conflictKind"`
}

func decodeProblem(t *testing.T, response *http.Response) apiProblem {
	t.Helper()
	var body apiProblem
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if body.Status != response.StatusCode {
		t.Fatalf("problem status %d != HTTP status %d", body.Status, response.StatusCode)
	}
	return body
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(encoded)
}

// sortMergeRequest returns a fresh request map using the published
// sort-merge statement and constraint texts from the contract fixtures. The
// evaluation corpus itself is not vendored, so the test vectors below are
// representative sort-merge cases rather than copied hidden test files.
func sortMergeRequest() map[string]any {
	return map[string]any{
		"title":     "Sorting lab",
		"statement": "# Merge sort\n\nSort N integers.",
		"opensAt":   "2026-11-20T04:00:00Z",
		"deadline":  "2026-11-25T15:00:00Z",
		"constraints": []any{
			map[string]any{
				"id":        "C1",
				"text":      "Do not call a library sort function: `std::sort`, `std::stable_sort`.",
				"deduction": 3,
			},
			map[string]any{
				"id":        "C2",
				"text":      "Sort with a merge sort you wrote yourself.",
				"deduction": 4,
			},
		},
		"tests": []any{
			map[string]any{"id": "01", "input": "5\n5 4 3 2 1\n", "expectedOutput": "1 2 3 4 5\n", "sample": true},
			map[string]any{"id": "02", "input": "1\n42\n", "expectedOutput": "42\n", "sample": true},
			map[string]any{"id": "03", "input": "3\n3 1 2\n", "expectedOutput": "1 2 3\n", "sample": true},
			map[string]any{"id": "04", "input": hiddenSentinel, "expectedOutput": hiddenAnswer, "sample": false},
			map[string]any{"id": "05", "input": hiddenSentinel + "-2", "expectedOutput": hiddenAnswer + "-2", "sample": false},
		},
		"limits": map[string]any{"cpuSeconds": 1, "memoryMiB": 256},
	}
}

func labIDFrom(t *testing.T, response *http.Response) string {
	t.Helper()
	var body struct {
		ID       string `json:"id"`
		ClassID  string `json:"classId"`
		Revision int    `json:"revision"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode created lab: %v", err)
	}
	if body.ID == "" {
		t.Fatal("created lab has no id")
	}
	return body.ID
}

func (f *labFixture) createLab(t *testing.T, client *http.Client, csrf, classID, key string, payload map[string]any) *http.Response {
	t.Helper()
	return postJSON(t, client, f.server.URL+"/v2/classes/"+classID+"/labs", mustJSON(t, payload), map[string]string{
		"X-CSRF-Token":    csrf,
		"Idempotency-Key": key,
	})
}

func (f *labFixture) createLabOK(t *testing.T, client *http.Client, csrf, classID, key string, payload map[string]any) string {
	t.Helper()
	response := f.createLab(t, client, csrf, classID, key, payload)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create lab = %d, want 201: %s", response.StatusCode, readBody(t, response))
	}
	return labIDFrom(t, response)
}

// PA3-ID-03: another TA guessing the first TA's lab or class returns the
// concealed not-found problem, with no protected data in the body.
func TestPA3ID03CrossClassLabIsConcealed(t *testing.T) {
	f := newLabFixture(t)
	ta1, csrf1 := f.login(t, "ta1@leetkob.local")
	labID := f.createLabOK(t, ta1, csrf1, classAID, "create-key-0001", sortMergeRequest())

	ta2, _ := f.login(t, "ta2@leetkob.local")

	crossClass := doRequest(t, ta2, http.MethodGet, f.server.URL+"/v2/labs/"+labID, "", nil)
	if crossClass.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-class lab read = %d, want 404", crossClass.StatusCode)
	}
	if cc := crossClass.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cross-class Cache-Control = %q", cc)
	}
	crossBody := readBody(t, crossClass)
	var crossProblem apiProblem
	if err := json.Unmarshal(crossBody, &crossProblem); err != nil {
		t.Fatalf("decode cross-class problem: %v", err)
	}
	if crossProblem.Type != httpapi.ProblemNamespace+"resource-not-found" ||
		crossProblem.Detail != "The requested resource does not exist." {
		t.Fatalf("cross-class problem = %+v", crossProblem)
	}
	if strings.Contains(string(crossBody), "Sorting lab") || strings.Contains(string(crossBody), "Merge sort") {
		t.Fatalf("cross-class response leaked protected data: %s", crossBody)
	}

	absent := doRequest(t, ta2, http.MethodGet,
		f.server.URL+"/v2/labs/99999999-9999-4999-8999-999999999999", "", nil)
	if absent.StatusCode != http.StatusNotFound {
		t.Fatalf("absent lab read = %d, want 404", absent.StatusCode)
	}
	var absentProblem apiProblem
	if err := json.NewDecoder(absent.Body).Decode(&absentProblem); err != nil {
		t.Fatalf("decode absent problem: %v", err)
	}
	if absentProblem.Type != crossProblem.Type || absentProblem.Detail != crossProblem.Detail {
		t.Fatalf("cross-class %+v and absent %+v must be indistinguishable", crossProblem, absentProblem)
	}

	crossList := doRequest(t, ta2, http.MethodGet, f.server.URL+"/v2/classes/"+classAID+"/labs", "", nil)
	if crossList.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-class lab list = %d, want 404", crossList.StatusCode)
	}
	if problem := decodeProblem(t, crossList); problem.Type != httpapi.ProblemNamespace+"resource-not-found" {
		t.Fatalf("cross-class list problem = %+v", problem)
	}

	ownList := doRequest(t, ta2, http.MethodGet, f.server.URL+"/v2/classes/"+classBID+"/labs", "", nil)
	if ownList.StatusCode != http.StatusOK {
		t.Fatalf("own class lab list = %d, want 200", ownList.StatusCode)
	}
	var listing struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(ownList.Body).Decode(&listing)
	if len(listing.Items) != 0 {
		t.Fatalf("second TA sees %d labs in its own class, want 0", len(listing.Items))
	}
}

// PA3-ID-04: forged role/class headers and an admin account never grant TA
// authority, and a student cannot read another class's lab.
func TestPA3ID04ForgedHeadersAndWrongRole(t *testing.T) {
	f := newLabFixture(t)
	ta1, csrf1 := f.login(t, "ta1@leetkob.local")
	labA := f.createLabOK(t, ta1, csrf1, classAID, "create-key-0101", sortMergeRequest())
	ta2, csrf2 := f.login(t, "ta2@leetkob.local")
	labB := f.createLabOK(t, ta2, csrf2, classBID, "create-key-0102", sortMergeRequest())

	student, studentCSRF := f.login(t, "student1@leetkob.local")
	forged := map[string]string{
		"X-CSRF-Token":    studentCSRF,
		"Idempotency-Key": "forged-student-key",
		"X-Role":          "ta",
		"X-User-Role":     "ta",
		"X-Class-Id":      classAID,
		"X-User-Id":       taOneID,
	}
	response := postJSON(t, student, f.server.URL+"/v2/classes/"+classAID+"/labs",
		mustJSON(t, sortMergeRequest()), forged)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("student with TA headers = %d, want 403", response.StatusCode)
	}
	if problem := decodeProblem(t, response); problem.Type != httpapi.ProblemNamespace+"role-required" {
		t.Fatalf("student forged-header problem = %+v", problem)
	}
	if f.labs.count() != 2 {
		t.Fatalf("student forged headers created a lab; count = %d", f.labs.count())
	}

	admin, adminCSRF := f.login(t, "admin@leetkob.local")
	adminCreate := postJSON(t, admin, f.server.URL+"/v2/classes/"+classAID+"/labs",
		mustJSON(t, sortMergeRequest()), map[string]string{
			"X-CSRF-Token":    adminCSRF,
			"Idempotency-Key": "admin-create-key",
		})
	if adminCreate.StatusCode != http.StatusForbidden {
		t.Fatalf("admin create = %d, want 403 (admin is not an implicit TA)", adminCreate.StatusCode)
	}
	if problem := decodeProblem(t, adminCreate); problem.Type != httpapi.ProblemNamespace+"role-required" {
		t.Fatalf("admin create problem = %+v", problem)
	}

	adminRead := doRequest(t, admin, http.MethodGet, f.server.URL+"/v2/labs/"+labA, "", nil)
	if adminRead.StatusCode != http.StatusNotFound {
		t.Fatalf("admin lab read = %d, want concealed 404", adminRead.StatusCode)
	}

	studentCross := doRequest(t, student, http.MethodGet, f.server.URL+"/v2/labs/"+labB, "", map[string]string{
		"X-Role": "ta", "X-Class-Id": classBID, "X-TA": "true",
	})
	if studentCross.StatusCode != http.StatusNotFound {
		t.Fatalf("student cross-class lab read = %d, want 404", studentCross.StatusCode)
	}
	studentOwn := doRequest(t, student, http.MethodGet, f.server.URL+"/v2/labs/"+labA, "", nil)
	if studentOwn.StatusCode != http.StatusOK {
		t.Fatalf("enrolled student lab read = %d, want 200", studentOwn.StatusCode)
	}
}

// PA3-ID-05: a still-valid session loses access as soon as its current
// membership is removed.
func TestPA3ID05RemovedMembershipDeniesExistingSession(t *testing.T) {
	f := newLabFixture(t)
	ta1, csrf1 := f.login(t, "ta1@leetkob.local")
	labA := f.createLabOK(t, ta1, csrf1, classAID, "create-key-0201", sortMergeRequest())

	student, _ := f.login(t, "student1@leetkob.local")
	if response := doRequest(t, student, http.MethodGet, f.server.URL+"/v2/labs/"+labA, "", nil); response.StatusCode != http.StatusOK {
		t.Fatalf("student lab read before removal = %d, want 200", response.StatusCode)
	}
	f.authStore.RemoveClass(studentOneID, classAID)

	if response := doRequest(t, student, http.MethodGet, f.server.URL+"/v2/labs/"+labA, "", nil); response.StatusCode != http.StatusNotFound {
		t.Fatalf("student lab read after removal = %d, want 404", response.StatusCode)
	}
	classes := doRequest(t, student, http.MethodGet, f.server.URL+"/v2/classes", "", nil)
	if classes.StatusCode != http.StatusOK {
		t.Fatalf("class list after removal = %d, want 200", classes.StatusCode)
	}
	var listing struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(classes.Body).Decode(&listing)
	if len(listing.Items) != 0 {
		t.Fatalf("removed membership still listed: %+v", listing.Items)
	}

	// A TA whose membership is removed cannot create or read in that class,
	// even though the same session and CSRF proof remain valid.
	f.authStore.RemoveClass(taOneID, classAID)
	create := f.createLab(t, ta1, csrf1, classAID, "create-key-0202", sortMergeRequest())
	if create.StatusCode != http.StatusNotFound {
		t.Fatalf("create after TA membership removal = %d, want 404", create.StatusCode)
	}
	if response := doRequest(t, ta1, http.MethodGet, f.server.URL+"/v2/labs/"+labA, "", nil); response.StatusCode != http.StatusNotFound {
		t.Fatalf("TA lab read after removal = %d, want 404", response.StatusCode)
	}
	if f.labs.count() != 1 {
		t.Fatalf("removed TA created a partial lab; count = %d", f.labs.count())
	}
}

// PA3-LAB-01: the full sort-merge lab is created through HTTP only, and a
// student read exposes samples and constraints but never hidden tests.
func TestPA3LAB01CreateLabThroughHTTP(t *testing.T) {
	f := newLabFixture(t)
	if f.labs.count() != 0 {
		t.Fatal("fixture must start without a seeded lab")
	}
	ta1, csrf := f.login(t, "ta1@leetkob.local")
	payload := sortMergeRequest()
	response := f.createLab(t, ta1, csrf, classAID, "lab-create-key-01", payload)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %s", response.StatusCode, readBody(t, response))
	}
	if cc := response.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("create Cache-Control = %q, want no-store", cc)
	}
	rawCreate := readBody(t, response)
	var created struct {
		ID          string `json:"id"`
		ClassID     string `json:"classId"`
		Title       string `json:"title"`
		Statement   string `json:"statement"`
		OpensAt     string `json:"opensAt"`
		Deadline    string `json:"deadline"`
		Revision    int    `json:"revision"`
		ModelNotice string `json:"modelNotice"`
		Constraints []struct {
			ID        string  `json:"id"`
			Text      string  `json:"text"`
			Deduction float64 `json:"deduction"`
		} `json:"constraints"`
		Limits struct {
			CPUSeconds int `json:"cpuSeconds"`
			MemoryMiB  int `json:"memoryMiB"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(rawCreate, &created); err != nil {
		t.Fatalf("decode created lab: %v", err)
	}
	if created.ID == "" || created.ClassID != classAID || created.Revision != 1 {
		t.Fatalf("created lab identity = %+v", created)
	}
	if created.OpensAt != "2026-11-20T04:00:00.000Z" || created.Deadline != "2026-11-25T15:00:00.000Z" {
		t.Fatalf("created lab schedule = %q -> %q", created.OpensAt, created.Deadline)
	}
	if len(created.Constraints) != 2 ||
		created.Constraints[0].ID != "C1" || created.Constraints[0].Deduction != 3 ||
		created.Constraints[1].ID != "C2" || created.Constraints[1].Deduction != 4 {
		t.Fatalf("created constraints = %+v", created.Constraints)
	}
	if created.Limits.CPUSeconds != 1 || created.Limits.MemoryMiB != 256 {
		t.Fatalf("created limits = %+v", created.Limits)
	}
	if created.ModelNotice == "" || created.Statement == "" {
		t.Fatalf("created lab missing statement/model notice: %+v", created)
	}
	if f.labs.count() != 1 {
		t.Fatalf("lab store count = %d, want 1", f.labs.count())
	}

	// The class listing is a safe projection: no statement or tests.
	student, _ := f.login(t, "student1@leetkob.local")
	list := doRequest(t, student, http.MethodGet, f.server.URL+"/v2/classes/"+classAID+"/labs", "", nil)
	if list.StatusCode != http.StatusOK {
		t.Fatalf("student lab list = %d, want 200", list.StatusCode)
	}
	if cc := list.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("list Cache-Control = %q", cc)
	}
	rawList := readBody(t, list)
	if strings.Contains(string(rawList), "Merge sort") || strings.Contains(string(rawList), hiddenSentinel) {
		t.Fatalf("lab summary leaked statement/tests: %s", rawList)
	}
	var summary struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rawList, &summary); err != nil {
		t.Fatalf("decode lab list: %v", err)
	}
	if len(summary.Items) != 1 || summary.Items[0]["id"] != created.ID || summary.Items[0]["revision"] != float64(1) {
		t.Fatalf("lab summary = %+v", summary.Items)
	}
	if _, present := summary.Items[0]["statement"]; present {
		t.Fatal("lab summary must not include the statement")
	}

	// A student read exposes sample tests only.
	detail := doRequest(t, student, http.MethodGet, f.server.URL+"/v2/labs/"+created.ID, "", nil)
	if detail.StatusCode != http.StatusOK {
		t.Fatalf("student lab detail = %d, want 200", detail.StatusCode)
	}
	rawDetail := readBody(t, detail)
	if strings.Contains(string(rawDetail), hiddenSentinel) || strings.Contains(string(rawDetail), hiddenAnswer) {
		t.Fatalf("student lab detail leaked hidden tests: %s", rawDetail)
	}
	var read struct {
		ID          string `json:"id"`
		Statement   string `json:"statement"`
		Revision    int    `json:"revision"`
		ModelNotice string `json:"modelNotice"`
		Constraints []struct {
			ID        string  `json:"id"`
			Deduction float64 `json:"deduction"`
		} `json:"constraints"`
		Limits      map[string]int `json:"limits"`
		SampleTests []struct {
			ID     string `json:"id"`
			Sample bool   `json:"sample"`
		} `json:"sampleTests"`
	}
	if err := json.Unmarshal(rawDetail, &read); err != nil {
		t.Fatalf("decode lab detail: %v", err)
	}
	if read.ID != created.ID || read.Revision != 1 || read.Statement == "" || read.ModelNotice == "" {
		t.Fatalf("lab detail = %+v", read)
	}
	if len(read.Constraints) != 2 || read.Constraints[0].Deduction != 3 || read.Constraints[1].Deduction != 4 {
		t.Fatalf("student constraints = %+v", read.Constraints)
	}
	if len(read.SampleTests) != 3 {
		t.Fatalf("sample tests = %+v, want 3", read.SampleTests)
	}
	for _, test := range read.SampleTests {
		if !test.Sample {
			t.Fatalf("non-sample test exposed: %+v", test)
		}
	}

	// Idempotent replay returns the same lab and creates nothing new.
	replay := f.createLab(t, ta1, csrf, classAID, "lab-create-key-01", payload)
	if replay.StatusCode != http.StatusCreated {
		t.Fatalf("idempotent replay = %d, want 201", replay.StatusCode)
	}
	replayBody := readBody(t, replay)
	if string(replayBody) != string(rawCreate) {
		t.Fatalf("replay body differs from original:\n%s\n%s", replayBody, rawCreate)
	}
	if f.labs.count() != 1 {
		t.Fatalf("replay created another lab; count = %d", f.labs.count())
	}

	// Same key with different content is an explicit conflict.
	changed := sortMergeRequest()
	changed["title"] = "Different lab"
	conflict := f.createLab(t, ta1, csrf, classAID, "lab-create-key-01", changed)
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("content mismatch = %d, want 409", conflict.StatusCode)
	}
	if problem := decodeProblem(t, conflict); problem.Type != httpapi.ProblemNamespace+"conflict" ||
		problem.ConflictKind != "idempotency-content-mismatch" {
		t.Fatalf("content mismatch problem = %+v", problem)
	}
	if f.labs.count() != 1 {
		t.Fatalf("content mismatch created a lab; count = %d", f.labs.count())
	}
}

// Authorization uses the current server-owned role, not the role captured at
// login time and never a caller-supplied header.
func TestCurrentRoleIsResolvedPerRequest(t *testing.T) {
	f := newLabFixture(t)
	student, csrf := f.login(t, "student1@leetkob.local")
	response := postJSON(t, student, f.server.URL+"/v2/classes/"+classAID+"/labs",
		mustJSON(t, sortMergeRequest()), map[string]string{
			"X-CSRF-Token": csrf, "Idempotency-Key": "role-change-key-01",
			"X-Role": "ta",
		})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("student create = %d, want 403", response.StatusCode)
	}

	f.authStore.SetRole(studentOneID, auth.RoleTA)
	promoted := postJSON(t, student, f.server.URL+"/v2/classes/"+classAID+"/labs",
		mustJSON(t, sortMergeRequest()), map[string]string{
			"X-CSRF-Token": csrf, "Idempotency-Key": "role-change-key-02",
		})
	if promoted.StatusCode != http.StatusCreated {
		t.Fatalf("create after server-side role change = %d, want 201: %s",
			promoted.StatusCode, readBody(t, promoted))
	}
	if f.labs.count() != 1 {
		t.Fatalf("lab store count = %d, want 1", f.labs.count())
	}
}

// PA3-LAB-02: invalid or unauthorized creation leaves no partial lab.
func TestPA3LAB02InvalidAndUnauthorizedCreation(t *testing.T) {
	f := newLabFixture(t)
	ta1, csrf := f.login(t, "ta1@leetkob.local")

	longTitle := strings.Repeat("t", 201)
	longStatement := strings.Repeat("s", 65537)
	longConstraintText := strings.Repeat("c", 4097)
	longTestID := strings.Repeat("i", 65)
	twentyOneConstraints := make([]any, 0, 21)
	duplicateConstraints := make([]any, 0, 2)
	for i := 0; i < 21; i++ {
		twentyOneConstraints = append(twentyOneConstraints, map[string]any{
			"id": fmt.Sprintf("C%d", i), "text": "Valid text.", "deduction": 1,
		})
	}
	duplicateConstraints = append(duplicateConstraints,
		map[string]any{"id": "C1", "text": "First.", "deduction": 1},
		map[string]any{"id": "C1", "text": "Second.", "deduction": 2})

	fiveHundredOneTests := make([]any, 0, 501)
	for i := 0; i < 501; i++ {
		fiveHundredOneTests = append(fiveHundredOneTests, map[string]any{
			"id": fmt.Sprintf("%03d", i), "input": "1\n1\n", "expectedOutput": "1\n", "sample": false,
		})
	}
	duplicateTests := []any{
		map[string]any{"id": "01", "input": "1\n1\n", "expectedOutput": "1\n", "sample": true},
		map[string]any{"id": "01", "input": "2\n2\n", "expectedOutput": "2\n", "sample": false},
	}

	mutate := func(change func(map[string]any)) string {
		payload := sortMergeRequest()
		change(payload)
		return mustJSON(t, payload)
	}
	cases := []struct {
		name string
		body string
	}{
		{"empty title", mutate(func(p map[string]any) { p["title"] = "" })},
		{"long title", mutate(func(p map[string]any) { p["title"] = longTitle })},
		{"empty statement", mutate(func(p map[string]any) { p["statement"] = "" })},
		{"long statement", mutate(func(p map[string]any) { p["statement"] = longStatement })},
		{"statement over 64 KiB bytes", mutate(func(p map[string]any) {
			p["statement"] = strings.Repeat("é", 32769)
		})},
		{"equal schedule", mutate(func(p map[string]any) { p["deadline"] = p["opensAt"] })},
		{"reversed schedule", mutate(func(p map[string]any) { p["opensAt"] = "2026-11-26T00:00:00Z" })},
		{"missing opensAt", mutate(func(p map[string]any) { delete(p, "opensAt") })},
		{"zero constraints", mutate(func(p map[string]any) { p["constraints"] = []any{} })},
		{"too many constraints", mutate(func(p map[string]any) { p["constraints"] = twentyOneConstraints })},
		{"duplicate constraint ids", mutate(func(p map[string]any) { p["constraints"] = duplicateConstraints })},
		{"invalid constraint id start", mutate(func(p map[string]any) {
			p["constraints"].([]any)[0].(map[string]any)["id"] = "1C"
		})},
		{"long constraint id", mutate(func(p map[string]any) {
			p["constraints"].([]any)[0].(map[string]any)["id"] = "C" + strings.Repeat("x", 32)
		})},
		{"empty constraint text", mutate(func(p map[string]any) {
			p["constraints"].([]any)[0].(map[string]any)["text"] = ""
		})},
		{"long constraint text", mutate(func(p map[string]any) {
			p["constraints"].([]any)[0].(map[string]any)["text"] = longConstraintText
		})},
		{"deduction too large", mutate(func(p map[string]any) {
			p["constraints"].([]any)[0].(map[string]any)["deduction"] = 10.01
		})},
		{"deduction three decimals", mutate(func(p map[string]any) {
			p["constraints"].([]any)[0].(map[string]any)["deduction"] = 3.333
		})},
		{"negative deduction", mutate(func(p map[string]any) {
			p["constraints"].([]any)[0].(map[string]any)["deduction"] = -1
		})},
		{"zero tests", mutate(func(p map[string]any) { p["tests"] = []any{} })},
		{"too many tests", mutate(func(p map[string]any) { p["tests"] = fiveHundredOneTests })},
		{"duplicate test ids", mutate(func(p map[string]any) { p["tests"] = duplicateTests })},
		{"empty test id", mutate(func(p map[string]any) {
			p["tests"].([]any)[0].(map[string]any)["id"] = ""
		})},
		{"long test id", mutate(func(p map[string]any) {
			p["tests"].([]any)[0].(map[string]any)["id"] = longTestID
		})},
		{"missing test sample flag", mutate(func(p map[string]any) {
			delete(p["tests"].([]any)[0].(map[string]any), "sample")
		})},
		{"cpu too low", mutate(func(p map[string]any) { p["limits"].(map[string]any)["cpuSeconds"] = 0 })},
		{"cpu too high", mutate(func(p map[string]any) { p["limits"].(map[string]any)["cpuSeconds"] = 6 })},
		{"memory too low", mutate(func(p map[string]any) { p["limits"].(map[string]any)["memoryMiB"] = 63 })},
		{"memory too high", mutate(func(p map[string]any) { p["limits"].(map[string]any)["memoryMiB"] = 513 })},
		{"unknown top-level field", mutate(func(p map[string]any) { p["compiler"] = "g++ -O2" })},
		{"unknown limits field", mutate(func(p map[string]any) {
			p["limits"].(map[string]any)["compiler"] = "g++"
		})},
		{"trailing json", mutate(func(map[string]any) {}) + " {}"},
	}
	for i, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := postJSON(t, ta1, f.server.URL+"/v2/classes/"+classAID+"/labs", testCase.body,
				map[string]string{
					"X-CSRF-Token":    csrf,
					"Idempotency-Key": fmt.Sprintf("invalid-lab-key-%03d", i),
				})
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", response.StatusCode, readBody(t, response))
			}
			if contentType := response.Header.Get("Content-Type"); contentType != "application/problem+json" {
				t.Fatalf("Content-Type = %q", contentType)
			}
			if problem := decodeProblem(t, response); problem.Type != httpapi.ProblemNamespace+"invalid-request" {
				t.Fatalf("problem = %+v", problem)
			}
		})
	}
	if f.labs.count() != 0 {
		t.Fatalf("invalid requests created %d labs", f.labs.count())
	}

	// Combined test payload above the 10 MiB cap is rejected without a lab.
	oversizedPayload := sortMergeRequest()
	oversizedPayload["tests"] = []any{
		map[string]any{
			"id": "01", "input": strings.Repeat("a", 10<<20), "expectedOutput": "x", "sample": true,
		},
	}
	oversized := postJSON(t, ta1, f.server.URL+"/v2/classes/"+classAID+"/labs",
		mustJSON(t, oversizedPayload), map[string]string{
			"X-CSRF-Token": csrf, "Idempotency-Key": "oversized-payload-key",
		})
	if oversized.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized test payload = %d, want 400", oversized.StatusCode)
	}

	// Another TA's class is concealed, not a role or validation error.
	crossClass := f.createLab(t, ta1, csrf, classBID, "cross-class-create-key", sortMergeRequest())
	if crossClass.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-class create = %d, want 404", crossClass.StatusCode)
	}
	if problem := decodeProblem(t, crossClass); problem.Type != httpapi.ProblemNamespace+"resource-not-found" {
		t.Fatalf("cross-class create problem = %+v", problem)
	}

	// CSRF and idempotency key requirements.
	missingCSRF := postJSON(t, ta1, f.server.URL+"/v2/classes/"+classAID+"/labs",
		mustJSON(t, sortMergeRequest()), map[string]string{"Idempotency-Key": "missing-csrf-key"})
	if missingCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("missing CSRF = %d, want 403", missingCSRF.StatusCode)
	}
	wrongCSRF := postJSON(t, ta1, f.server.URL+"/v2/classes/"+classAID+"/labs",
		mustJSON(t, sortMergeRequest()), map[string]string{
			"X-CSRF-Token": "wrong-proof", "Idempotency-Key": "wrong-csrf-key",
		})
	if wrongCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong CSRF = %d, want 403", wrongCSRF.StatusCode)
	}
	for _, key := range []string{"", "short", strings.Repeat("a", 129), "has space here", "éééééééé"} {
		response := postJSON(t, ta1, f.server.URL+"/v2/classes/"+classAID+"/labs",
			mustJSON(t, sortMergeRequest()), map[string]string{
				"X-CSRF-Token": csrf, "Idempotency-Key": key,
			})
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("idempotency key %q = %d, want 400", key, response.StatusCode)
		}
	}

	// Wrong roles never grant creation, even with an otherwise valid request.
	student, studentCSRF := f.login(t, "student1@leetkob.local")
	studentCreate := postJSON(t, student, f.server.URL+"/v2/classes/"+classAID+"/labs",
		mustJSON(t, sortMergeRequest()), map[string]string{
			"X-CSRF-Token": studentCSRF, "Idempotency-Key": "student-create-key",
		})
	if studentCreate.StatusCode != http.StatusForbidden {
		t.Fatalf("student create = %d, want 403", studentCreate.StatusCode)
	}
	admin, adminCSRF := f.login(t, "admin@leetkob.local")
	adminCreate := postJSON(t, admin, f.server.URL+"/v2/classes/"+classAID+"/labs",
		mustJSON(t, sortMergeRequest()), map[string]string{
			"X-CSRF-Token": adminCSRF, "Idempotency-Key": "admin-create-key",
		})
	if adminCreate.StatusCode != http.StatusForbidden {
		t.Fatalf("admin create = %d, want 403", adminCreate.StatusCode)
	}
	if f.labs.count() != 0 {
		t.Fatalf("unauthorized requests created %d labs", f.labs.count())
	}
}

// The whole request body is bounded at 12 MiB with the documented 413.
func TestPA3LAB02BodyTooLarge(t *testing.T) {
	f := newLabFixture(t)
	ta1, csrf := f.login(t, "ta1@leetkob.local")
	body := strings.Repeat("a", 12<<20+1)
	response := postJSON(t, ta1, f.server.URL+"/v2/classes/"+classAID+"/labs", body,
		map[string]string{
			"X-CSRF-Token": csrf, "Idempotency-Key": "too-large-body-key",
		})
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body = %d, want 413", response.StatusCode)
	}
	if problem := decodeProblem(t, response); problem.Type != httpapi.ProblemNamespace+"payload-too-large" {
		t.Fatalf("oversized body problem = %+v", problem)
	}
	if f.labs.count() != 0 {
		t.Fatalf("oversized body created %d labs", f.labs.count())
	}
}

// PA3-LAB-04: revision/edit routes are not registered and cannot replace the
// immutable requirements.
func TestPA3LAB04RevisionRoutesAreAbsent(t *testing.T) {
	f := newLabFixture(t)
	ta1, csrf := f.login(t, "ta1@leetkob.local")
	labID := f.createLabOK(t, ta1, csrf, classAID, "revision-probe-key", sortMergeRequest())

	probes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v2/labs/" + labID},
		{http.MethodPut, "/v2/labs/" + labID},
		{http.MethodPatch, "/v2/labs/" + labID},
		{http.MethodDelete, "/v2/labs/" + labID},
		{http.MethodPost, "/v2/labs/" + labID + "/revisions"},
		{http.MethodPut, "/v2/labs/" + labID + "/revisions"},
		{http.MethodGet, "/v2/labs/" + labID + "/revisions"},
	}
	for _, probe := range probes {
		t.Run(probe.method+" "+probe.path, func(t *testing.T) {
			response := doRequest(t, ta1, probe.method, f.server.URL+probe.path,
				`{"expectedRevision":1,"title":"replaced"}`, map[string]string{
					"X-CSRF-Token": csrf, "Idempotency-Key": "revision-probe-request",
				})
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("%s %s = %d, want 404", probe.method, probe.path, response.StatusCode)
			}
			if problem := decodeProblem(t, response); problem.Type != httpapi.ProblemNamespace+"route-not-found" {
				t.Fatalf("probe problem = %+v", problem)
			}
		})
	}

	read := doRequest(t, ta1, http.MethodGet, f.server.URL+"/v2/labs/"+labID, "", nil)
	if read.StatusCode != http.StatusOK {
		t.Fatalf("lab read after probes = %d, want 200", read.StatusCode)
	}
	var lab struct {
		Revision    int `json:"revision"`
		Constraints []struct {
			ID string `json:"id"`
		} `json:"constraints"`
	}
	if err := json.NewDecoder(read.Body).Decode(&lab); err != nil {
		t.Fatalf("decode lab: %v", err)
	}
	if lab.Revision != 1 || len(lab.Constraints) != 2 {
		t.Fatalf("requirements changed after revision probes: %+v", lab)
	}
}

// Class and lab listings are filtered by ownership before pagination and
// use an opaque cursor.
func TestPaginationAndCacheHeaders(t *testing.T) {
	f := newLabFixture(t)
	multi, _ := f.login(t, "multi@leetkob.local")

	first := doRequest(t, multi, http.MethodGet, f.server.URL+"/v2/classes?limit=2", "", nil)
	if first.StatusCode != http.StatusOK {
		t.Fatalf("class page = %d, want 200", first.StatusCode)
	}
	if cc := first.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("class list Cache-Control = %q", cc)
	}
	var pageOne struct {
		Items      []map[string]string `json:"items"`
		NextCursor *string             `json:"nextCursor"`
	}
	if err := json.NewDecoder(first.Body).Decode(&pageOne); err != nil {
		t.Fatalf("decode class page: %v", err)
	}
	if len(pageOne.Items) != 2 || pageOne.NextCursor == nil {
		t.Fatalf("first class page = %+v", pageOne)
	}
	second := doRequest(t, multi, http.MethodGet,
		f.server.URL+"/v2/classes?limit=2&cursor="+*pageOne.NextCursor, "", nil)
	var pageTwo struct {
		Items      []map[string]string `json:"items"`
		NextCursor *string             `json:"nextCursor"`
	}
	if err := json.NewDecoder(second.Body).Decode(&pageTwo); err != nil {
		t.Fatalf("decode second class page: %v", err)
	}
	if len(pageTwo.Items) != 1 || pageTwo.NextCursor != nil {
		t.Fatalf("second class page = %+v", pageTwo)
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?cursor=not-a-cursor"} {
		response := doRequest(t, multi, http.MethodGet, f.server.URL+"/v2/classes"+query, "", nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("class list %s = %d, want 400", query, response.StatusCode)
		}
	}

	ta1, csrf := f.login(t, "ta1@leetkob.local")
	for i := 0; i < 3; i++ {
		request := sortMergeRequest()
		request["title"] = fmt.Sprintf("Lab %d", i)
		f.createLabOK(t, ta1, csrf, classAID, fmt.Sprintf("page-lab-key-%d", i), request)
	}
	labsFirst := doRequest(t, ta1, http.MethodGet, f.server.URL+"/v2/classes/"+classAID+"/labs?limit=2", "", nil)
	if labsFirst.StatusCode != http.StatusOK {
		t.Fatalf("lab page = %d, want 200", labsFirst.StatusCode)
	}
	var labsOne struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	if err := json.NewDecoder(labsFirst.Body).Decode(&labsOne); err != nil {
		t.Fatalf("decode lab page: %v", err)
	}
	if len(labsOne.Items) != 2 || labsOne.NextCursor == nil {
		t.Fatalf("first lab page = %+v", labsOne)
	}
	labsSecond := doRequest(t, ta1, http.MethodGet,
		f.server.URL+"/v2/classes/"+classAID+"/labs?limit=2&cursor="+*labsOne.NextCursor, "", nil)
	var labsTwo struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	if err := json.NewDecoder(labsSecond.Body).Decode(&labsTwo); err != nil {
		t.Fatalf("decode second lab page: %v", err)
	}
	if len(labsTwo.Items) != 1 || labsTwo.NextCursor != nil {
		t.Fatalf("second lab page = %+v", labsTwo)
	}
}
