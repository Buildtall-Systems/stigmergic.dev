package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_PublicPaths(t *testing.T) {
	t.Parallel()

	sm, _ := NewSessionManager("testsecret", "1h")
	mw := Middleware(sm)

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(inner)

	paths := []string{LoginPath, "/auth/verify", "/auth/logout", "/static/js/htmx.min.js"}
	for _, path := range paths {
		called = false
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !called {
			t.Errorf("expected inner handler called for public path %s", path)
		}
	}
}

func TestMiddleware_RedirectsWithoutSession(t *testing.T) {
	t.Parallel()

	sm, _ := NewSessionManager("testsecret", "1h")
	mw := Middleware(sm)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called")
	})

	handler := mw(inner)

	req := httptest.NewRequest(http.MethodGet, "/file/test.md", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/auth/login?redirect=/file/test.md" {
		t.Errorf("expected redirect with path, got %s", location)
	}
}

func TestMiddleware_RedirectsRootWithoutRedirectParam(t *testing.T) {
	t.Parallel()

	sm, _ := NewSessionManager("testsecret", "1h")
	mw := Middleware(sm)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called")
	})

	handler := mw(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != LoginPath {
		t.Errorf("expected redirect to /auth/login without redirect param, got %s", location)
	}
}

func TestMiddleware_ValidSession(t *testing.T) {
	t.Parallel()

	sm, _ := NewSessionManager("testsecret", "1h")
	mw := Middleware(sm)

	pubkey := testPubkey
	cookieValue, _ := sm.CreateSession(pubkey)

	var gotPubkey string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPubkey = PubkeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(inner)

	req := httptest.NewRequest(http.MethodGet, "/file/test.md", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: cookieValue})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if gotPubkey != pubkey {
		t.Errorf("expected pubkey %s in context, got %s", pubkey, gotPubkey)
	}
}

func TestMiddleware_InvalidSession(t *testing.T) {
	t.Parallel()

	sm, _ := NewSessionManager("testsecret", "1h")
	mw := Middleware(sm)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called")
	})

	handler := mw(inner)

	req := httptest.NewRequest(http.MethodGet, "/file/test.md", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "invalid-cookie-value"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rec.Code)
	}
}

func TestPubkeyFromContext_Empty(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	pubkey := PubkeyFromContext(req.Context())
	if pubkey != "" {
		t.Errorf("expected empty pubkey from context without auth, got %s", pubkey)
	}
}
