package httpapi_test

import (
	"context"
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
	"github.com/JakeConal/leetkob/backend/internal/auth/authtest"
	"github.com/JakeConal/leetkob/backend/internal/httpapi"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

const (
	testPassword = "synthetic-password-1"
	webOrigin    = "http://127.0.0.1:3000"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

type fixture struct {
	server *httptest.Server
	clock  *testClock
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	store := authtest.NewFakeStore()
	hash, err := identity.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	classA := auth.ClassSummary{ID: "class-a", Name: "PA3 Class A"}
	classB := auth.ClassSummary{ID: "class-b", Name: "PA3 Class B"}
	store.AddUser(auth.UserRecord{ID: "ta-1", Email: "ta1@leetkob.local", DisplayName: "TA One", Role: auth.RoleTA, PasswordHash: hash}, classA)
	store.AddUser(auth.UserRecord{ID: "ta-2", Email: "ta2@leetkob.local", DisplayName: "TA Two", Role: auth.RoleTA, PasswordHash: hash}, classB)
	store.AddUser(auth.UserRecord{ID: "student-1", Email: "student1@leetkob.local", DisplayName: "Student One", Role: auth.RoleStudent, PasswordHash: hash}, classA)
	clock := &testClock{now: time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)}
	service := auth.NewService(store, clock, identity.VerifyPassword)
	server := httptest.NewServer(httpapi.NewServer(httpapi.Deps{
		Auth: service, WebOrigin: webOrigin,
	}))
	t.Cleanup(server.Close)
	return fixture{server: server, clock: clock}
}

func (f fixture) client(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func postJSON(t *testing.T, client *http.Client, url, body string, headers map[string]string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
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

func loginBody(t *testing.T, email string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{"email": email, "password": testPassword})
	if err != nil {
		t.Fatalf("marshal login: %v", err)
	}
	return string(encoded)
}

func problemType(t *testing.T, response *http.Response) string {
	t.Helper()
	var body struct {
		Type     string `json:"type"`
		Status   int    `json:"status"`
		Instance string `json:"instance"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if body.Status != response.StatusCode {
		t.Fatalf("problem status %d != http status %d", body.Status, response.StatusCode)
	}
	return body.Type
}

// PA3-ID-01: each documented account signs in, sees its current role and
// class, and logout invalidates the session.
func TestSignInEachAccountThenLogoutInvalidates(t *testing.T) {
	f := newFixture(t)
	expectations := map[string]struct {
		role    auth.Role
		classes []string
	}{
		"ta1@leetkob.local":      {role: auth.RoleTA, classes: []string{"PA3 Class A"}},
		"ta2@leetkob.local":      {role: auth.RoleTA, classes: []string{"PA3 Class B"}},
		"student1@leetkob.local": {role: auth.RoleStudent, classes: []string{"PA3 Class A"}},
	}
	for email, expected := range expectations {
		client := f.client(t)
		response := postJSON(t, client, f.server.URL+"/v2/auth/login", loginBody(t, email), nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s login status = %d, want 200", email, response.StatusCode)
		}
		var session struct {
			User struct {
				Role auth.Role `json:"role"`
			} `json:"user"`
			CSRFToken string `json:"csrfToken"`
		}
		if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
			t.Fatalf("%s decode login: %v", email, err)
		}
		if session.User.Role != expected.role || session.CSRFToken == "" {
			t.Fatalf("%s login body = %+v", email, session)
		}

		cookie := response.Cookies()[0]
		if cookie.Name != httpapi.SessionCookieName || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("%s cookie attributes = %+v", email, cookie)
		}

		me, err := client.Get(f.server.URL + "/v2/auth/me")
		if err != nil {
			t.Fatalf("%s me: %v", email, err)
		}
		var meBody struct {
			Classes []struct {
				Name string `json:"name"`
			} `json:"classes"`
		}
		_ = json.NewDecoder(me.Body).Decode(&meBody)
		_ = me.Body.Close()
		if len(meBody.Classes) != len(expected.classes) {
			t.Fatalf("%s classes = %+v", email, meBody.Classes)
		}
		for i, name := range expected.classes {
			if meBody.Classes[i].Name != name {
				t.Fatalf("%s class[%d] = %q, want %q", email, i, meBody.Classes[i].Name, name)
			}
		}

		logoutRequest, _ := http.NewRequest(http.MethodPost, f.server.URL+"/v2/auth/logout", nil)
		logoutRequest.Header.Set("X-CSRF-Token", session.CSRFToken)
		logout, err := client.Do(logoutRequest)
		if err != nil {
			t.Fatalf("%s logout: %v", email, err)
		}
		_ = logout.Body.Close()
		if logout.StatusCode != http.StatusNoContent {
			t.Fatalf("%s logout status = %d, want 204", email, logout.StatusCode)
		}

		meAfter, err := client.Get(f.server.URL + "/v2/auth/me")
		if err != nil {
			t.Fatalf("%s me after logout: %v", email, err)
		}
		_ = meAfter.Body.Close()
		if meAfter.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s me after logout = %d, want 401", email, meAfter.StatusCode)
		}
	}
}

// PA3-ID-06: cross-origin mutations and missing/wrong CSRF proofs are
// rejected without changing state.
func TestCSRFAndOriginRejection(t *testing.T) {
	f := newFixture(t)
	client := f.client(t)

	// Origin mismatch on login is rejected before credentials are read.
	response := postJSON(t, client, f.server.URL+"/v2/auth/login", loginBody(t, "ta1@leetkob.local"),
		map[string]string{"Origin": "https://evil.example"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin login = %d, want 403", response.StatusCode)
	}
	if got := problemType(t, response); got != "https://leetkob.example/problems/role-required" {
		t.Fatalf("cross-origin problem type = %q", got)
	}
	if len(response.Cookies()) != 0 {
		t.Fatal("rejected login must not set a session cookie")
	}

	login := postJSON(t, client, f.server.URL+"/v2/auth/login", loginBody(t, "ta1@leetkob.local"), nil)
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	_ = json.NewDecoder(login.Body).Decode(&session)

	// Missing CSRF proof on logout is rejected and the session survives.
	missing := postJSON(t, client, f.server.URL+"/v2/auth/logout", "", nil)
	if missing.StatusCode != http.StatusForbidden {
		t.Fatalf("logout without CSRF = %d, want 403", missing.StatusCode)
	}
	if got := problemType(t, missing); got != "https://leetkob.example/problems/role-required" {
		t.Fatalf("CSRF problem type = %q", got)
	}
	me, err := client.Get(f.server.URL + "/v2/auth/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	_ = me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("session must survive a rejected logout, me = %d", me.StatusCode)
	}

	// A wrong CSRF proof is rejected the same way.
	wrong := postJSON(t, client, f.server.URL+"/v2/auth/logout", "",
		map[string]string{"X-CSRF-Token": "not-the-proof"})
	if wrong.StatusCode != http.StatusForbidden {
		t.Fatalf("logout with wrong CSRF = %d, want 403", wrong.StatusCode)
	}
}

// PA3-ID-01: generic authentication failures and login throttling.
func TestGenericFailureAndThrottle(t *testing.T) {
	f := newFixture(t)
	client := f.client(t)

	wrong := postJSON(t, client, f.server.URL+"/v2/auth/login",
		`{"email":"ta1@leetkob.local","password":"wrong"}`, nil)
	if wrong.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", wrong.StatusCode)
	}
	if got := problemType(t, wrong); got != "https://leetkob.example/problems/authentication-required" {
		t.Fatalf("wrong-password problem type = %q", got)
	}
	if len(wrong.Cookies()) != 0 {
		t.Fatal("failed login must not set a cookie")
	}

	// Exhaust the per-address window (10/minute): the wrong-password
	// attempt above already used one of the ten slots.
	for i := 0; i < auth.IPLimit-1; i++ {
		attempt := postJSON(t, client, f.server.URL+"/v2/auth/login",
			fmt.Sprintf(`{"email":"user%d@example.test","password":"x"}`, i), nil)
		if attempt.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+2, attempt.StatusCode)
		}
	}
	throttled := postJSON(t, client, f.server.URL+"/v2/auth/login",
		`{"email":"ta1@leetkob.local","password":"`+testPassword+`"}`, nil)
	if throttled.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("11th attempt = %d, want 429", throttled.StatusCode)
	}
	if throttled.Header.Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
	if got := problemType(t, throttled); got != "https://leetkob.example/problems/rate-limited" {
		t.Fatalf("throttle problem type = %q", got)
	}
}

// Sessions are HttpOnly, host-only and cached nowhere.
func TestSessionCookieAndCacheHeaders(t *testing.T) {
	f := newFixture(t)
	client := f.client(t)
	response := postJSON(t, client, f.server.URL+"/v2/auth/login", loginBody(t, "student1@leetkob.local"), nil)
	if cc := response.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || cookie.Secure || cookie.Domain != "" || cookie.Path != "/" {
		t.Fatalf("cookie attributes = %+v", cookie)
	}

	me, err := client.Get(f.server.URL + "/v2/auth/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer me.Body.Close()
	if cc := me.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("me Cache-Control = %q, want no-store", cc)
	}
	body, _ := io.ReadAll(me.Body)
	if strings.Contains(string(body), "password") || strings.Contains(string(body), "Hash") {
		t.Fatal("me response must not expose credentials")
	}
}

// Unknown routes use the documented problem body.
func TestUnknownRouteProblem(t *testing.T) {
	f := newFixture(t)
	response, err := http.Get(f.server.URL + "/v2/nothing/here")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.StatusCode)
	}
	if got := problemType(t, response); got != "https://leetkob.example/problems/route-not-found" {
		t.Fatalf("problem type = %q", got)
	}
}

var _ = context.Background
