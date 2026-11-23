package compatpub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// scenarioFile holds the shared seeded/fixed-clock publication scenarios
// executed unchanged against the NestJS PA#2 application and this Go
// compatibility composition.
type scenarioFile struct {
	Secret     string `json:"secret"`
	NowSeconds int64  `json:"nowSeconds"`
	Instant    string `json:"instant"`
	Identities map[string]struct {
		Subject  string   `json:"subject"`
		Role     string   `json:"role"`
		ClassIDs []string `json:"classIds"`
	} `json:"identities"`
	Labs      map[string]string `json:"labs"`
	Deadlines struct {
		Passed string `json:"passed"`
		Future string `json:"future"`
	} `json:"deadlines"`
	Scenarios []scenario `json:"scenarios"`
}

type scenario struct {
	Name        string `json:"name"`
	FailLabSave bool   `json:"failLabSave"`
	Labs        []struct {
		ID                 string `json:"id"`
		ClassID            string `json:"classId"`
		Status             string `json:"status"`
		PendingReviews     int    `json:"pendingReviews"`
		SubmissionDeadline string `json:"submissionDeadline"`
	} `json:"labs"`
	Identities map[string]struct {
		Subject  string   `json:"subject"`
		Role     string   `json:"role"`
		ClassIDs []string `json:"classIds"`
	} `json:"identities"`
	Steps []map[string]any `json:"steps"`
}

func TestPublicationScenarios(t *testing.T) {
	raw, err := os.ReadFile(scenarioPath(t))
	if err != nil {
		t.Fatalf("read scenarios: %v", err)
	}
	var file scenarioFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse scenarios: %v", err)
	}

	subs := map[string]string{
		"{instant}": file.Instant,
		"{passed}":  file.Deadlines.Passed,
		"{future}":  file.Deadlines.Future,
	}
	for name, id := range file.Labs {
		subs["{"+name+"}"] = id
	}
	instant, err := time.Parse(time.RFC3339, file.Instant)
	if err != nil {
		t.Fatalf("parse instant: %v", err)
	}

	for _, sc := range file.Scenarios {
		sc := sc
		t.Run(sc.Name, func(t *testing.T) {
			runScenario(t, file, sc, subs, instant)
		})
	}
}

func scenarioPath(t *testing.T) string {
	t.Helper()
	candidate := filepath.Join("..", "..", "..", "..", "docs", "contract", "fixtures", "publication-scenarios.json")
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("publication scenarios file not found at %s: %v", candidate, err)
	}
	return candidate
}

func runScenario(t *testing.T, file scenarioFile, sc scenario, subs map[string]string, instant time.Time) {
	t.Helper()

	var labs []Lab
	for _, seeded := range sc.Labs {
		labs = append(labs, Lab{
			ID:                 resolve(subs, seeded.ID),
			ClassID:            seeded.ClassID,
			Status:             LabStatus(seeded.Status),
			PendingReviews:     seeded.PendingReviews,
			SubmissionDeadline: resolve(subs, seeded.SubmissionDeadline),
		})
	}
	labRepo := NewInMemoryLabRepository(labs)
	var repo LabRepository = labRepo
	if sc.FailLabSave {
		repo = failingSaveRepository{inner: labRepo}
	}
	mux := NewMux(HandlerDeps{
		Secret:      file.Secret,
		NowSeconds:  func() int64 { return file.NowSeconds },
		Labs:        repo,
		Idempotency: NewInMemoryIdempotencyRepository(),
		Clock:       FixedClock{Instant: instant},
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	savedBodies := map[string]any{}
	for i, step := range sc.Steps {
		kind, _ := step["type"].(string)
		switch kind {
		case "request":
			response := doRequest(t, server.URL, file, sc, step, subs)
			assertExpected(t, step, response, savedBodies, subs)
			drainAndClose(t, response)
		case "saveLab":
			encoded, err := json.Marshal(step["lab"])
			if err != nil {
				t.Fatalf("step %d: marshal lab: %v", i, err)
			}
			var seeded struct {
				ID                 string `json:"id"`
				ClassID            string `json:"classId"`
				Status             string `json:"status"`
				PendingReviews     int    `json:"pendingReviews"`
				SubmissionDeadline string `json:"submissionDeadline"`
			}
			if err := json.Unmarshal(encoded, &seeded); err != nil {
				t.Fatalf("step %d: parse lab: %v", i, err)
			}
			if err := labRepo.Save(Lab{
				ID:                 resolve(subs, seeded.ID),
				ClassID:            seeded.ClassID,
				Status:             LabStatus(seeded.Status),
				PendingReviews:     seeded.PendingReviews,
				SubmissionDeadline: resolve(subs, seeded.SubmissionDeadline),
			}); err != nil {
				t.Fatalf("step %d: save lab: %v", i, err)
			}
		case "expectLab":
			labID := resolve(subs, fmt.Sprint(step["labId"]))
			lab, ok := labRepo.FindByID(labID)
			if !ok {
				t.Fatalf("step %d: lab %s not found", i, labID)
			}
			want, _ := step["status"].(string)
			if string(lab.Status) != want {
				t.Fatalf("step %d: lab %s status = %q, want %q", i, labID, lab.Status, want)
			}
		case "concurrent":
			requests, _ := step["requests"].([]any)
			statuses := make([]int, len(requests))
			var wg sync.WaitGroup
			for j, raw := range requests {
				requestStep, _ := raw.(map[string]any)
				wg.Add(1)
				go func(j int, requestStep map[string]any) {
					defer wg.Done()
					response := doRequest(t, server.URL, file, sc, requestStep, subs)
					statuses[j] = response.StatusCode
					drainAndClose(t, response)
				}(j, requestStep)
			}
			wg.Wait()
			sort.Ints(statuses)
			expected, _ := step["expectedStatuses"].([]any)
			want := make([]int, 0, len(expected))
			for _, status := range expected {
				want = append(want, int(status.(float64)))
			}
			if !reflect.DeepEqual(statuses, want) {
				t.Fatalf("step %d: concurrent statuses = %v, want %v", i, statuses, want)
			}
		case "assertNotContains":
			text := marshalCompact(t, savedBodies[nameOf(step["from"])])
			for _, forbidden := range stringSlice(step["forbidden"]) {
				if strings.Contains(text, forbidden) {
					t.Fatalf("step %d: body contains forbidden %q", i, forbidden)
				}
			}
		case "assertNotMatches":
			text := marshalCompact(t, savedBodies[nameOf(step["from"])])
			pattern, _ := step["pattern"].(string)
			if regexp.MustCompile(`(?i)` + pattern).MatchString(text) {
				t.Fatalf("step %d: body matches forbidden pattern %q", i, pattern)
			}
		default:
			t.Fatalf("step %d: unknown step type %q", i, kind)
		}
	}
}

// doRequest performs one scenario request and returns the open response.
func doRequest(t *testing.T, baseURL string, file scenarioFile, sc scenario, step map[string]any, subs map[string]string) *http.Response {
	t.Helper()
	method, _ := step["method"].(string)
	if method == "" {
		method = http.MethodPost
	}
	path := resolve(subs, fmt.Sprint(step["path"]))
	var body io.Reader
	if raw, ok := step["body"]; ok {
		body = strings.NewReader(fmt.Sprint(raw))
	}
	request, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	applyToken(t, request, file, sc, step)
	if raw, ok := step["idempotencyKey"]; ok {
		request.Header.Set("Idempotency-Key", fmt.Sprint(raw))
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, path, err)
	}
	return response
}

func drainAndClose(t *testing.T, response *http.Response) {
	t.Helper()
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}

func applyToken(t *testing.T, request *http.Request, file scenarioFile, sc scenario, step map[string]any) {
	t.Helper()
	kind, _ := step["tokenKind"].(string)
	if kind == "absent" {
		return
	}
	if kind == "malformed" {
		request.Header.Set("Authorization", "Bearer not-a-token")
		return
	}
	identity, ok := identityFor(file, sc, step)
	if !ok {
		t.Fatalf("missing identity for token kind %q", kind)
	}
	switch kind {
	case "tampered":
		valid, err := IssueToken(identity, file.Secret, file.NowSeconds+60)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+valid[:len(valid)-1]+"x")
	case "expired":
		token, err := IssueToken(identity, file.Secret, file.NowSeconds)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
	default:
		token, err := IssueToken(identity, file.Secret, file.NowSeconds+60)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
}

func identityFor(file scenarioFile, sc scenario, step map[string]any) (Identity, bool) {
	name := ""
	if raw, ok := step["identity"]; ok {
		name, _ = raw.(string)
	} else if raw, ok := step["identityRef"]; ok {
		name, _ = raw.(string)
	}
	if local, ok := sc.Identities[name]; ok {
		return Identity{Subject: local.Subject, Role: Role(local.Role), ClassIDs: local.ClassIDs}, true
	}
	if global, ok := file.Identities[name]; ok {
		return Identity{Subject: global.Subject, Role: Role(global.Role), ClassIDs: global.ClassIDs}, true
	}
	return Identity{}, false
}

func assertExpected(t *testing.T, step map[string]any, response *http.Response, saved map[string]any, subs map[string]string) {
	t.Helper()
	var body any
	read := false
	readOnce := func() any {
		if !read {
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			read = true
		}
		return body
	}

	if expected, ok := step["expected"].(map[string]any); ok {
		wantStatus := int(expected["status"].(float64))
		if response.StatusCode != wantStatus {
			t.Fatalf("status = %d, want %d", response.StatusCode, wantStatus)
		}
		wantContentType, _ := expected["contentType"].(string)
		if wantContentType == "" {
			wantContentType = "application/problem+json"
		}
		if !strings.HasPrefix(response.Header.Get("Content-Type"), wantContentType) {
			t.Fatalf("content-type = %q, want prefix %q", response.Header.Get("Content-Type"), wantContentType)
		}
		if cc := response.Header.Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("cache-control = %q, want no-store", cc)
		}
		if rawBody, ok := expected["body"]; ok {
			assertJSONEqual(t, readOnce(), resolveAny(subs, rawBody))
		}
	}
	if equalTo, ok := step["expectedBodyEqualTo"]; ok {
		assertJSONEqual(t, readOnce(), saved[nameOf(equalTo)])
	}
	if saveAs, ok := step["saveBodyAs"]; ok {
		saved[nameOf(saveAs)] = readOnce()
	}
}

func assertJSONEqual(t *testing.T, got, want any) {
	t.Helper()
	gotText, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	wantText, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(gotText, &gotValue); err != nil {
		t.Fatalf("normalize got: %v", err)
	}
	if err := json.Unmarshal(wantText, &wantValue); err != nil {
		t.Fatalf("normalize want: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("bodies differ:\n got: %s\nwant: %s", gotText, wantText)
	}
}

func resolve(subs map[string]string, value string) string {
	for placeholder, replacement := range subs {
		value = strings.ReplaceAll(value, placeholder, replacement)
	}
	return value
}

func resolveAny(subs map[string]string, value any) any {
	switch typed := value.(type) {
	case string:
		return resolve(subs, typed)
	case map[string]any:
		resolved := map[string]any{}
		for key, member := range typed {
			resolved[key] = resolveAny(subs, member)
		}
		return resolved
	case []any:
		resolved := make([]any, 0, len(typed))
		for _, member := range typed {
			resolved = append(resolved, resolveAny(subs, member))
		}
		return resolved
	default:
		return value
	}
}

func marshalCompact(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(encoded)
}

func nameOf(value any) string {
	return fmt.Sprint(value)
}

func stringSlice(value any) []string {
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

// failingSaveRepository simulates persistence failure: Save errors and
// the stored lab is never mutated.
type failingSaveRepository struct {
	inner *InMemoryLabRepository
}

func (r failingSaveRepository) FindByID(id string) (Lab, bool) {
	return r.inner.FindByID(id)
}

func (r failingSaveRepository) Save(lab Lab) error {
	return fmt.Errorf("database-password and internal table must not leak")
}
