package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JakeConal/leetkob/backend/internal/access"
	"github.com/JakeConal/leetkob/backend/internal/labs"
)

func TestLabAuthoringAndClassIsolation(t *testing.T) {
	h := newHarness(t)
	passwords := assessmentPasswords()
	if err := access.Provision(t.Context(), h.store, passwords); err != nil {
		t.Fatal(err)
	}
	ta := h.signIn(access.EmailTAA, passwords.TeachingAssistantA)
	other := h.signIn(access.EmailTAB, passwords.TeachingAssistantB)
	student := h.signIn(access.EmailStudentA1, passwords.StudentA1)
	admin := h.signIn(access.EmailAdmin, passwords.Admin)

	const hidden = "HIDDEN-INPUT-9c2e"
	body := labBody(t, "sort-merge", "Sort the integers.\n", "2026-11-20T04:00:00Z", "2026-11-25T15:00:00Z", hidden, false)
	classLabs := "/v2/classes/" + access.ClassAID + "/labs"

	spoof := h.request(http.MethodPost, classLabs, student.cookie, student.csrf, webOrigin, "192.0.2.10:4242", body, map[string]string{
		"Idempotency-Key": "student-spoof-01",
		"X-Role":          "ta",
		"X-Class-Ids":     access.ClassAID,
	})
	assertProblem(t, spoof, http.StatusForbidden, "role-required", "Role required", detailRole)
	adminTry := h.request(http.MethodPost, classLabs, admin.cookie, admin.csrf, webOrigin, "192.0.2.10:4242", body, map[string]string{
		"Idempotency-Key": "admin-spoof-0001",
		"X-Role":          "ta",
	})
	assertProblem(t, adminTry, http.StatusForbidden, "role-required", "Role required", detailRole)
	otherClass := h.request(http.MethodPost, classLabs, other.cookie, other.csrf, webOrigin, "192.0.2.10:4242", body, map[string]string{
		"Idempotency-Key": "other-ta-key-01",
	})
	assertProblem(t, otherClass, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)

	invalid := []struct {
		name string
		body []byte
		key  string
	}{
		{"schedule", labBody(t, "sort-merge", "Sort the integers.\n", "2026-11-25T15:00:00Z", "2026-11-20T04:00:00Z", hidden, false), "bad-schedule-01"},
		{"duplicate constraints", duplicateConstraintBody(t), "bad-constraint-1"},
		{"cpu limit", labBody(t, "sort-merge", "Sort the integers.\n", "2026-11-20T04:00:00Z", "2026-11-25T15:00:00Z", hidden, true), "bad-cpu-limit-01"},
	}
	for _, tc := range invalid {
		rec := h.request(http.MethodPost, classLabs, ta.cookie, ta.csrf, webOrigin, "192.0.2.10:4242", tc.body, map[string]string{
			"Idempotency-Key": tc.key,
		})
		assertProblem(t, rec, http.StatusBadRequest, "invalid-request", "Invalid request", detailInvalid)
	}
	noCSRF := h.request(http.MethodPost, classLabs, ta.cookie, "", webOrigin, "192.0.2.10:4242", body, map[string]string{
		"Idempotency-Key": "missing-csrf-001",
	})
	assertProblem(t, noCSRF, http.StatusForbidden, "role-required", "Role required", detailRole)
	badOrigin := h.request(http.MethodPost, classLabs, ta.cookie, ta.csrf, "http://evil.example", "192.0.2.10:4242", body, map[string]string{
		"Idempotency-Key": "bad-origin-00001",
	})
	assertProblem(t, badOrigin, http.StatusForbidden, "role-required", "Role required", detailRole)
	if items := h.labIDs(ta, access.ClassAID); len(items) != 0 {
		t.Fatalf("failed creates left %d labs", len(items))
	}

	created := h.request(http.MethodPost, classLabs, ta.cookie, ta.csrf, webOrigin, "192.0.2.10:4242", body, map[string]string{
		"Idempotency-Key": "sort-merge-key1",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var createdMap map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &createdMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := createdMap["sampleTests"]; ok || strings.Contains(created.Body.String(), hidden) {
		t.Fatal("creation response included tests")
	}
	if createdMap["modelNotice"] != labs.ModelNotice || createdMap["revision"] != float64(1) {
		t.Fatalf("create body = %#v", createdMap)
	}
	labID, _ := createdMap["id"].(string)
	replay := h.request(http.MethodPost, classLabs, ta.cookie, ta.csrf, webOrigin, "192.0.2.10:4242", body, map[string]string{
		"Idempotency-Key": "sort-merge-key1",
	})
	var replayed map[string]any
	if err := json.Unmarshal(replay.Body.Bytes(), &replayed); err != nil {
		t.Fatal(err)
	}
	if replay.Code != http.StatusCreated || replayed["id"] != labID {
		t.Fatalf("replay = %d %#v", replay.Code, replayed["id"])
	}
	changed := labBody(t, "replaced title", "Sort the integers.\n", "2026-11-20T04:00:00Z", "2026-11-25T15:00:00Z", hidden, false)
	conflict := h.request(http.MethodPost, classLabs, ta.cookie, ta.csrf, webOrigin, "192.0.2.10:4242", changed, map[string]string{
		"Idempotency-Key": "sort-merge-key1",
	})
	assertProblem(t, conflict, http.StatusConflict, "conflict", "Conflict", detailConflict)
	if ids := h.labIDs(ta, access.ClassAID); len(ids) != 1 || ids[0] != labID {
		t.Fatalf("labs after conflict = %#v", ids)
	}

	got := h.request(http.MethodGet, "/v2/labs/"+labID, student.cookie, "", "", "192.0.2.10:4242", nil, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("student read = %d %s", got.Code, got.Body.String())
	}
	if !strings.Contains(got.Body.String(), "1\\n42\\n") && !strings.Contains(got.Body.String(), "1\n42\n") {
		t.Fatalf("sample input missing: %s", got.Body.String())
	}
	if strings.Contains(got.Body.String(), hidden) {
		t.Fatal("student response included the hidden test")
	}
	var detail map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["modelNotice"] != labs.ModelNotice {
		t.Fatalf("notice = %#v", detail["modelNotice"])
	}
	constraints, _ := detail["constraints"].([]any)
	if len(constraints) != 2 {
		t.Fatalf("constraints = %#v", detail["constraints"])
	}
	samples, _ := detail["sampleTests"].([]any)
	if len(samples) != 1 {
		t.Fatalf("sample tests = %#v", detail["sampleTests"])
	}

	cross := h.request(http.MethodGet, "/v2/labs/"+labID, other.cookie, "", "", "192.0.2.10:4242", nil, map[string]string{"X-Role": "ta"})
	unknown := h.request(http.MethodGet, "/v2/labs/6f0c1a10-7b21-4a11-8c11-cccccccccccc", other.cookie, "", "", "192.0.2.10:4242", nil, nil)
	assertSameConcealment(t, cross, unknown)
	if strings.Contains(cross.Body.String(), "sort-merge") || strings.Contains(cross.Body.String(), hidden) {
		t.Fatal("cross-class response revealed lab data")
	}
	adminRead := h.request(http.MethodGet, "/v2/labs/"+labID, admin.cookie, "", "", "192.0.2.10:4242", nil, map[string]string{"X-Role": "ta"})
	assertProblem(t, adminRead, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)

	future := labBody(t, "not yet open", "Later.\n", "2026-11-26T00:00:00Z", "2026-11-27T00:00:00Z", "FUTURE-HIDDEN", false)
	futureRec := h.request(http.MethodPost, classLabs, ta.cookie, ta.csrf, webOrigin, "192.0.2.10:4242", future, map[string]string{
		"Idempotency-Key": "future-lab-key1",
	})
	if futureRec.Code != http.StatusCreated {
		t.Fatalf("future create = %d %s", futureRec.Code, futureRec.Body.String())
	}
	var futureBody map[string]any
	_ = json.Unmarshal(futureRec.Body.Bytes(), &futureBody)
	futureID, _ := futureBody["id"].(string)
	concealed := h.request(http.MethodGet, "/v2/labs/"+futureID, student.cookie, "", "", "192.0.2.10:4242", nil, nil)
	assertProblem(t, concealed, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)
	if h.request(http.MethodGet, "/v2/labs/"+futureID, ta.cookie, "", "", "192.0.2.10:4242", nil, nil).Code != http.StatusOK {
		t.Fatal("TA must read a lab before it opens")
	}
	if len(h.labIDs(student, access.ClassAID)) != 1 {
		t.Fatal("student list must hide a lab that has not opened")
	}

	if err := h.store.RemoveMembership(t.Context(), access.StudentA1ID, access.ClassAID, h.now); err != nil {
		t.Fatal(err)
	}
	lost := h.request(http.MethodGet, "/v2/labs/"+labID, student.cookie, "", "", "192.0.2.10:4242", nil, nil)
	assertProblem(t, lost, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)
	me := h.me(student)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"classes":[]`) {
		t.Fatalf("me after removal = %d %s", me.Code, me.Body.String())
	}
	if h.request(http.MethodGet, "/v2/labs/"+labID, ta.cookie, "", "", "192.0.2.10:4242", nil, nil).Code != http.StatusOK {
		t.Fatal("TA membership must be unchanged")
	}

	replace := h.request(http.MethodPut, "/v2/labs/"+labID, ta.cookie, ta.csrf, webOrigin, "192.0.2.10:4242", []byte(`{"statement":"replaced","constraints":[{"id":"C9","text":"new","deduction":1}]}`), map[string]string{
		"Idempotency-Key": "revise-lab-key1",
	})
	assertProblem(t, replace, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)
	revise := h.request(http.MethodPost, "/v2/labs/"+labID+"/revisions", ta.cookie, ta.csrf, webOrigin, "192.0.2.10:4242", []byte(`{"statement":"replaced"}`), map[string]string{
		"Idempotency-Key": "revise-lab-key2",
	})
	assertProblem(t, revise, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)
	after := h.request(http.MethodGet, "/v2/labs/"+labID, ta.cookie, "", "", "192.0.2.10:4242", nil, nil)
	if !strings.Contains(after.Body.String(), "Do not call") || strings.Contains(after.Body.String(), "C9") {
		t.Fatalf("revision attempt changed the lab: %s", after.Body.String())
	}

	t.Run("official sort-merge", func(t *testing.T) {
		payload, sample, hiddenSnippet := officialSortMerge(t)
		rec := h.request(http.MethodPost, classLabs, ta.cookie, ta.csrf, webOrigin, "192.0.2.10:4242", payload, map[string]string{
			"Idempotency-Key": "official-sort-merge",
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("official create = %d %s", rec.Code, rec.Body.String())
		}
		var created map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		id, _ := created["id"].(string)
		enrolled := h.signIn(access.EmailStudentA2, passwords.StudentA2)
		read := h.request(http.MethodGet, "/v2/labs/"+id, enrolled.cookie, "", "", "192.0.2.10:4242", nil, nil)
		if read.Code != http.StatusOK {
			t.Fatalf("official read = %d %s", read.Code, read.Body.String())
		}
		var detail struct {
			Statement   string `json:"statement"`
			SampleTests []struct {
				ID     string `json:"id"`
				Input  string `json:"input"`
				Sample bool   `json:"sample"`
			} `json:"sampleTests"`
		}
		if err := json.Unmarshal(read.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		if detail.Statement == "" || len(detail.SampleTests) != 1 || detail.SampleTests[0].ID != "02" || detail.SampleTests[0].Input != sample || !detail.SampleTests[0].Sample {
			t.Fatalf("official sample projection = %#v", detail.SampleTests)
		}
		if strings.Contains(read.Body.String(), hiddenSnippet) {
			t.Fatal("official response included a hidden test")
		}
		if !strings.Contains(read.Body.String(), "Do not call a library sort") || !strings.Contains(read.Body.String(), "merge sort you wrote") {
			t.Fatal("official constraints were not stored")
		}
		if created["modelNotice"] != labs.ModelNotice {
			t.Fatal("official lab omitted the model notice")
		}
	})
}

func (h *harness) labIDs(session signedIn, classID string) []string {
	h.t.Helper()
	rec := h.request(http.MethodGet, "/v2/classes/"+classID+"/labs", session.cookie, "", "", "192.0.2.10:4242", nil, nil)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		h.t.Fatal(err)
	}
	ids := make([]string, len(body.Items))
	for i, item := range body.Items {
		ids[i] = item.ID
	}
	return ids
}

func assertSameConcealment(t *testing.T, cross, unknown *httptest.ResponseRecorder) {
	t.Helper()
	assertProblem(t, cross, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)
	assertProblem(t, unknown, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)
	var left, right map[string]any
	if err := json.Unmarshal(cross.Body.Bytes(), &left); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(unknown.Body.Bytes(), &right); err != nil {
		t.Fatal(err)
	}
	if len(left) != 5 || len(right) != 5 {
		t.Fatalf("concealed problems must have the five base members only: %#v %#v", left, right)
	}
	for _, key := range []string{"type", "title", "status", "detail"} {
		if left[key] != right[key] {
			t.Fatalf("cross-class %s = %#v, unknown id %#v", key, left[key], right[key])
		}
	}
}

func labBody(t *testing.T, title, statement, opensAt, deadline, hidden string, badCPU bool) []byte {
	t.Helper()
	cpu := 2
	if badCPU {
		cpu = 9
	}
	body := map[string]any{
		"title": title, "statement": statement, "opensAt": opensAt, "deadline": deadline,
		"constraints": []map[string]any{
			{"id": "C1", "text": "Do not call `std::sort`.", "deduction": 3},
			{"id": "C2", "text": "Implement merge sort.", "deduction": 4},
		},
		"tests": []map[string]any{
			{"id": "01", "input": "1\n42\n", "expectedOutput": "42\n", "sample": true},
			{"id": "02", "input": hidden + "\n", "expectedOutput": "hidden-output\n", "sample": false},
		},
		"limits": map[string]any{"cpuSeconds": cpu, "memoryMiB": 256},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func duplicateConstraintBody(t *testing.T) []byte {
	t.Helper()
	body := map[string]any{
		"title": "sort-merge", "statement": "Sort the integers.\n",
		"opensAt": "2026-11-20T04:00:00Z", "deadline": "2026-11-25T15:00:00Z",
		"constraints": []map[string]any{
			{"id": "C1", "text": "Do not call `std::sort`.", "deduction": 3},
			{"id": "C1", "text": "Implement merge sort.", "deduction": 4},
		},
		"tests": []map[string]any{
			{"id": "01", "input": "1\n", "expectedOutput": "1\n", "sample": true},
		},
		"limits": map[string]any{"cpuSeconds": 2, "memoryMiB": 256},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func officialSortMerge(t *testing.T) (payload []byte, sample, hiddenSnippet string) {
	t.Helper()
	dir := os.Getenv("LEETKOB_OFFICIAL_DIR")
	if dir == "" {
		candidate := "/tmp/leetkob-official"
		if _, err := os.Stat(filepath.Join(candidate, "problems/sort-merge/statement.md")); err == nil {
			dir = candidate
		}
	}
	if dir == "" {
		t.Skip("LEETKOB_OFFICIAL_DIR is unset and /tmp/leetkob-official has no sort-merge statement; official sort-merge bytes were not supplied")
	}
	root := repoRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "evaluation/official-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	read := func(rel string) []byte {
		t.Helper()
		buf, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(buf)
		if hex.EncodeToString(sum[:]) != manifest.SHA256[rel] {
			t.Fatalf("hash mismatch for %s", rel)
		}
		return buf
	}
	statement := read("problems/sort-merge/statement.md")
	var constraints []struct {
		ID        string `json:"id"`
		Text      string `json:"text"`
		Deduction int    `json:"deduction"`
	}
	if err := json.Unmarshal(read("problems/sort-merge/constraints.json"), &constraints); err != nil {
		t.Fatal(err)
	}
	if len(constraints) != 2 {
		t.Fatalf("official constraints = %d", len(constraints))
	}
	tests := make([]map[string]any, 0, 7)
	for _, id := range []string{"01", "02", "03", "04", "05", "06", "07"} {
		input := read("problems/sort-merge/tests/" + id + ".in")
		output := read("problems/sort-merge/tests/" + id + ".out")
		// The statement's worked example is tests/02. The TA marks that pair
		// as the sample and the other official pairs as hidden.
		tests = append(tests, map[string]any{
			"id": id, "input": string(input), "expectedOutput": string(output), "sample": id == "02",
		})
	}
	hidden := read("problems/sort-merge/tests/05.in")
	const hiddenMarker = "-638801160"
	if !strings.Contains(string(hidden), hiddenMarker) {
		t.Fatal("official hidden test no longer contains the expected marker")
	}
	sampleInput := string(read("problems/sort-merge/tests/02.in"))
	body := map[string]any{
		"title": "sort-merge", "statement": string(statement),
		"opensAt": "2026-11-20T04:00:00Z", "deadline": "2026-11-25T15:00:00Z",
		"constraints": []map[string]any{
			{"id": constraints[0].ID, "text": constraints[0].Text, "deduction": constraints[0].Deduction},
			{"id": constraints[1].ID, "text": constraints[1].Text, "deduction": constraints[1].Deduction},
		},
		"tests":  tests,
		"limits": map[string]any{"cpuSeconds": 2, "memoryMiB": 256},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, sampleInput, hiddenMarker
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(dir, "..", "..", "..", ".."))
}
