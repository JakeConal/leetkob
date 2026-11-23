package submit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/domain"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type fakeRepo struct {
	beginCalls  int
	acceptCalls int
	abortCalls  int
	acceptErr   error
	acceptBody  []byte
}

func (f *fakeRepo) BeginAdmission(context.Context, string, string, time.Time, time.Time) (Admission, error) {
	f.beginCalls++
	return Admission{ID: "6d5e1e70-4d3c-4a71-a6c5-9a1c1c3c5c45", ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func (f *fakeRepo) AbortAdmission(context.Context, string, time.Time) error {
	f.abortCalls++
	return nil
}

func (f *fakeRepo) Accept(context.Context, AcceptIn) (AcceptOut, error) {
	f.acceptCalls++
	if f.acceptErr != nil {
		return AcceptOut{}, f.acceptErr
	}
	return AcceptOut{Status: http.StatusAccepted, Body: f.acceptBody}, nil
}

func (f *fakeRepo) GetSubmission(context.Context, string, string, time.Time) (View, error) {
	return View{}, ErrNotFound
}

func (f *fakeRepo) ListSubmissions(context.Context, ListQuery) (Page, error) {
	return Page{}, ErrNotFound
}

type tokenAuth struct{}

func (tokenAuth) Authenticate(context.Context, string) (Principal, error) {
	return Principal{UserID: "57000000-0000-4000-8000-000000000001", CSRFToken: "csrf-token-0123456789abcdef"}, nil
}

func TestHandlerDoesNotAcknowledgeAFailedCommit(t *testing.T) {
	repo := &fakeRepo{acceptErr: errors.New("commit failed")}
	handler := &Handler{
		Service:   &Service{Repo: repo, Clock: fixedClock{time.Date(2026, 11, 24, 1, 0, 0, 0, time.UTC)}},
		Auth:      tokenAuth{},
		WebOrigin: "http://127.0.0.1:3000",
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2/labs/1ab00000-0000-4000-8000-000000000001/submissions", strings.NewReader(`{"source":"int main(){return 0;}"}`))
	req.SetPathValue("labId", "1ab00000-0000-4000-8000-000000000001")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "submit-0001")
	req.Header.Set("X-CSRF-Token", "csrf-token-0123456789abcdef")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session"})
	handler.ServeLabSubmissions(rec, req)
	if rec.Code == http.StatusAccepted {
		t.Fatal("acknowledged a failed commit")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if strings.Contains(string(body), "commit failed") || strings.Contains(string(body), `"state"`) {
		t.Fatalf("body leaked the failure or a submission: %s", body)
	}
	if repo.acceptCalls != 1 {
		t.Fatalf("accept calls = %d", repo.acceptCalls)
	}
}

func TestHandlerRejectsMissingSessionAndBadCSRF(t *testing.T) {
	handler := &Handler{
		Service:   &Service{Repo: &fakeRepo{}, Clock: domain.SystemClock{}},
		Auth:      tokenAuth{},
		WebOrigin: "http://127.0.0.1:3000",
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2/labs/1ab00000-0000-4000-8000-000000000001/submissions", strings.NewReader(`{"source":"a"}`))
	req.SetPathValue("labId", "1ab00000-0000-4000-8000-000000000001")
	handler.ServeLabSubmissions(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v2/labs/1ab00000-0000-4000-8000-000000000001/submissions", strings.NewReader(`{"source":"a"}`))
	req.SetPathValue("labId", "1ab00000-0000-4000-8000-000000000001")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "submit-0001")
	req.Header.Set("X-CSRF-Token", "wrong-csrf-token-value")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session"})
	handler.ServeLabSubmissions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("csrf status = %d", rec.Code)
	}
}

func TestHandlerRejectsOversizedBodyBeforeAccept(t *testing.T) {
	repo := &fakeRepo{}
	handler := &Handler{
		Service:   &Service{Repo: repo, Clock: domain.SystemClock{}},
		Auth:      tokenAuth{},
		WebOrigin: "http://127.0.0.1:3000",
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2/labs/1ab00000-0000-4000-8000-000000000001/submissions", strings.NewReader(strings.Repeat("a", maxBodyBytes+1)))
	req.SetPathValue("labId", "1ab00000-0000-4000-8000-000000000001")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "submit-0001")
	req.Header.Set("X-CSRF-Token", "csrf-token-0123456789abcdef")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session"})
	handler.ServeLabSubmissions(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if repo.acceptCalls != 0 || repo.abortCalls != 1 || repo.beginCalls != 1 {
		t.Fatalf("begin=%d accept=%d abort=%d", repo.beginCalls, repo.acceptCalls, repo.abortCalls)
	}
}
