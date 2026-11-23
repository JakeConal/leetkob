package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/access"
)

func TestSessionCookieIsHttpOnlyAndHostOnly(t *testing.T) {
	rec := httptest.NewRecorder()
	expires := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	setSessionCookie(rec, "opaque-token", expires, false)
	response := rec.Result()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != access.SessionCookieName || cookie.Value != "opaque-token" {
		t.Fatalf("cookie = %#v", cookie)
	}
	if !cookie.HttpOnly || cookie.Secure || cookie.Domain != "" || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("attributes = HttpOnly:%v Secure:%v Domain:%q Path:%q SameSite:%v", cookie.HttpOnly, cookie.Secure, cookie.Domain, cookie.Path, cookie.SameSite)
	}
	if strings.Contains(cookie.String(), "Domain=") {
		t.Fatalf("cookie must be host-only: %s", cookie.String())
	}

	secure := httptest.NewRecorder()
	setSessionCookie(secure, "opaque-token", expires, true)
	if !secure.Result().Cookies()[0].Secure {
		t.Fatal("deployed HTTPS must set Secure")
	}
}

func TestConcealedNotFoundMatchesThePublishedProblem(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v2/labs/2c1b7b37-1a0f-4e4e-b3f2-6d8f8f0f2f12", nil)
	writeNotFound(rec, req)
	assertProblem(t, rec, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)
}

func TestUnsupportedRevisionRouteIsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Dependencies{})
	for _, target := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/v2/labs/6f0c1a10-7b21-4a11-8c11-aaaaaaaaaaaa"},
		{http.MethodPatch, "/v2/labs/6f0c1a10-7b21-4a11-8c11-aaaaaaaaaaaa"},
		{http.MethodPost, "/v2/labs/6f0c1a10-7b21-4a11-8c11-aaaaaaaaaaaa/revisions"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(target.method, target.path, strings.NewReader(`{"statement":"replaced"}`))
		mux.ServeHTTP(rec, req)
		assertProblem(t, rec, http.StatusNotFound, "resource-not-found", "Resource not found", detailNotFound)
	}
}

func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, suffix, title, detail string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content-type = %q", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control = %q", rec.Header().Get("Cache-Control"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["type"] != ProblemNamespace+suffix || body["title"] != title || body["detail"] != detail {
		t.Fatalf("problem = %#v", body)
	}
	if int(body["status"].(float64)) != status {
		t.Fatalf("problem status = %v", body["status"])
	}
	if _, ok := body["instance"].(string); !ok || body["instance"] == "" {
		t.Fatal("instance must be the request path")
	}
}
