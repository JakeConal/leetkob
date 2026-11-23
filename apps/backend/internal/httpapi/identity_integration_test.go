package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/access"
	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
)

const webOrigin = "http://127.0.0.1:3000"

func TestIdentitySessions(t *testing.T) {
	h := newHarness(t)
	passwords := assessmentPasswords()
	if err := access.Provision(t.Context(), h.store, passwords); err != nil {
		t.Fatalf("provision: %v", err)
	}
	rotated := passwords
	rotated.Admin = "rotated-admin-password-not-stored"
	rotated.TeachingAssistantA = "rotated-ta-a-password-not-stored"
	rotated.TeachingAssistantB = "rotated-ta-b-password-not-stored"
	rotated.StudentA1 = "rotated-student-a1-not-stored"
	rotated.StudentA2 = "rotated-student-a2-not-stored"
	if err := access.Provision(t.Context(), h.store, rotated); err != nil {
		t.Fatalf("repeat provision: %v", err)
	}

	admin := h.signIn(access.EmailAdmin, passwords.Admin)
	if admin.user.Role != access.RoleAdmin || len(admin.classes) != 0 {
		t.Fatalf("admin = %#v classes %#v", admin.user, admin.classes)
	}
	taA := h.signIn(access.EmailTAA, passwords.TeachingAssistantA)
	if taA.user.Role != access.RoleTA || len(taA.classes) != 1 || taA.classes[0].ID != access.ClassAID {
		t.Fatalf("ta A = %#v", taA)
	}
	taB := h.signIn(access.EmailTAB, passwords.TeachingAssistantB)
	if taB.user.Role != access.RoleTA || len(taB.classes) != 1 || taB.classes[0].ID != access.ClassBID {
		t.Fatalf("ta B = %#v", taB)
	}
	student := h.signIn(access.EmailStudentA1, passwords.StudentA1)
	other := h.signIn(access.EmailStudentA2, passwords.StudentA2)
	if student.user.Role != access.RoleStudent || other.user.Role != access.RoleStudent {
		t.Fatal("students must sign in as students")
	}
	if len(student.classes) != 1 || student.classes[0].ID != access.ClassAID || other.classes[0].ID != access.ClassAID {
		t.Fatalf("student classes = %#v %#v", student.classes, other.classes)
	}
	wrong := h.request(http.MethodPost, "/v2/auth/login", "", "", "", "192.0.2.10:9", []byte(`{"email":"`+access.EmailAdmin+`","password":"not-the-password"}`), nil)
	missing := h.request(http.MethodPost, "/v2/auth/login", "", "", "", "192.0.2.11:9", []byte(`{"email":"nobody@assessment.leetkob.test","password":"not-the-password"}`), nil)
	assertProblem(t, wrong, http.StatusUnauthorized, "authentication-required", "Authentication required", detailAuth)
	assertProblem(t, missing, http.StatusUnauthorized, "authentication-required", "Authentication required", detailAuth)
	if wrong.Body.String() != missing.Body.String() {
		t.Fatalf("auth failures must be identical\n%q\n%q", wrong.Body.String(), missing.Body.String())
	}
	if strings.Contains(wrong.Body.String(), "password") || strings.Contains(missing.Body.String(), access.EmailAdmin) {
		t.Fatal("auth failure disclosed which check failed")
	}

	// A mutation without the CSRF proof, and one from another origin, changes nothing.
	denied := h.request(http.MethodPost, "/v2/auth/logout", student.cookie, "", webOrigin, "192.0.2.10:9", nil, nil)
	assertProblem(t, denied, http.StatusForbidden, "role-required", "Role required", detailRole)
	cross := h.request(http.MethodPost, "/v2/auth/logout", student.cookie, student.csrf, "http://evil.example", "192.0.2.10:9", nil, nil)
	assertProblem(t, cross, http.StatusForbidden, "role-required", "Role required", detailRole)
	if h.me(student).Code != http.StatusOK {
		t.Fatal("rejected mutations must leave the session usable")
	}

	loggedOut := h.request(http.MethodPost, "/v2/auth/logout", student.cookie, student.csrf, webOrigin, "192.0.2.10:9", nil, nil)
	if loggedOut.Code != http.StatusNoContent {
		t.Fatalf("logout = %d %s", loggedOut.Code, loggedOut.Body.String())
	}
	cleared := loggedOut.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 || !cleared[0].HttpOnly {
		t.Fatalf("clear cookie = %#v", cleared)
	}
	if h.me(student).Code != http.StatusUnauthorized {
		t.Fatal("logout must revoke the session")
	}

	fresh := h.signIn(access.EmailStudentA2, passwords.StudentA2)
	h.now = fresh.expires.Add(time.Nanosecond)
	if h.me(fresh).Code != http.StatusUnauthorized {
		t.Fatal("an expired session must be rejected")
	}
	h.now = fresh.expires.Add(-time.Hour)
	if h.me(fresh).Code != http.StatusUnauthorized {
		t.Fatal("expiry must revoke the session, not merely compare the clock")
	}

	// The original password still works after the rotated provisioning run.
	h.now = time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC)
	if rec := h.login(access.EmailAdmin, rotated.Admin, "192.0.2.80:9"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("rotated password was accepted: %d", rec.Code)
	}
	h.signIn(access.EmailAdmin, passwords.Admin)
}

func TestLoginRateLimit(t *testing.T) {
	h := newHarness(t)
	if err := access.Provision(t.Context(), h.store, assessmentPasswords()); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"email":"` + access.EmailStudentA1 + `","password":"wrong-password"}`)
	for i := 0; i < 10; i++ {
		rec := h.request(http.MethodPost, "/v2/auth/login", "", "", "", "203.0.113.8:9", body, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	blocked := h.request(http.MethodPost, "/v2/auth/login", "", "", "", "203.0.113.8:9", body, nil)
	assertProblem(t, blocked, http.StatusTooManyRequests, "rate-limited", "Rate limited", detailRate)
	if blocked.Header().Get("Retry-After") == "" {
		t.Fatal("429 must send Retry-After")
	}
	// A different account on a different address is not blocked by the first account's counter.
	other := h.login(access.EmailStudentA2, assessmentPasswords().StudentA2, "203.0.113.9:9")
	if other.Code != http.StatusOK {
		t.Fatalf("unrelated account = %d %s", other.Code, other.Body.String())
	}
	// The locked account stays locked from a new address until its window passes.
	still := h.request(http.MethodPost, "/v2/auth/login", "", "", "", "203.0.113.10:9", []byte(`{"email":"`+access.EmailStudentA1+`","password":"`+assessmentPasswords().StudentA1+`"}`), nil)
	if still.Code != http.StatusTooManyRequests {
		t.Fatalf("account limit = %d", still.Code)
	}
	h.now = h.now.Add(15 * time.Minute)
	again := h.login(access.EmailStudentA1, assessmentPasswords().StudentA1, "203.0.113.10:9")
	if again.Code != http.StatusOK {
		t.Fatalf("after the window = %d %s", again.Code, again.Body.String())
	}
}

type harness struct {
	t     *testing.T
	mux   *http.ServeMux
	store *postgres.Store
	now   time.Time
}

type signedIn struct {
	cookie  string
	csrf    string
	expires time.Time
	user    userBody
	classes []classBody
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t:     t,
		mux:   http.NewServeMux(),
		store: openAPIStore(t),
		now:   time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC),
	}
	Register(h.mux, Dependencies{
		Accounts:  h.store,
		Labs:      h.store,
		Now:       func() time.Time { return h.now },
		WebOrigin: webOrigin,
	})
	return h
}

func (h *harness) signIn(email, password string) signedIn {
	h.t.Helper()
	rec := h.login(email, password, "192.0.2.10:4242")
	if rec.Code != http.StatusOK {
		h.t.Fatalf("login %s = %d %s", email, rec.Code, rec.Body.String())
	}
	var body userSessionBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		h.t.Fatal(err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Domain != "" || cookies[0].SameSite != http.SameSiteLaxMode {
		h.t.Fatalf("cookie = %#v", cookies)
	}
	if strings.Contains(rec.Body.String(), cookies[0].Value) || strings.Contains(rec.Body.String(), password) {
		h.t.Fatal("login body contained the cookie or the password")
	}
	expires, err := time.Parse(time.RFC3339Nano, body.ExpiresAt)
	if err != nil {
		h.t.Fatal(err)
	}
	if !expires.Equal(h.now.Add(8 * time.Hour)) {
		h.t.Fatalf("expiresAt = %s", body.ExpiresAt)
	}
	me := h.request(http.MethodGet, "/v2/auth/me", cookies[0].Value, "", "", "192.0.2.10:4242", nil, nil)
	if me.Code != http.StatusOK || me.Header().Get("Cache-Control") != "no-store" {
		h.t.Fatalf("me = %d %s", me.Code, me.Body.String())
	}
	var current currentUserBody
	if err := json.Unmarshal(me.Body.Bytes(), &current); err != nil {
		h.t.Fatal(err)
	}
	if current.User != body.User {
		h.t.Fatalf("me user = %#v, login user %#v", current.User, body.User)
	}
	return signedIn{cookie: cookies[0].Value, csrf: body.CSRFToken, expires: expires, user: body.User, classes: current.Classes}
}

func (h *harness) login(email, password, remote string) *httptest.ResponseRecorder {
	h.t.Helper()
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	return h.request(http.MethodPost, "/v2/auth/login", "", "", "", remote, payload, nil)
}

func (h *harness) me(session signedIn) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.request(http.MethodGet, "/v2/auth/me", session.cookie, "", "", "192.0.2.10:4242", nil, nil)
}

func (h *harness) request(method, path, cookie, csrf, origin, remote string, body []byte, extra map[string]string) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req := httptest.NewRequest(method, path, reader)
	if remote == "" {
		remote = "192.0.2.10:4242"
	}
	req.RemoteAddr = remote
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: access.SessionCookieName, Value: cookie})
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range extra {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

func assessmentPasswords() access.Passwords {
	return access.Passwords{
		Admin:              "synthetic-admin-password",
		TeachingAssistantA: "synthetic-ta-a-password",
		TeachingAssistantB: "synthetic-ta-b-password",
		StudentA1:          "synthetic-student-a1-password",
		StudentA2:          "synthetic-student-a2-password",
	}
}
