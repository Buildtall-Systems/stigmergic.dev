package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/auth/session"
)

func requestWithSession(t *testing.T, sm *session.Manager, path string) *http.Request {
	t.Helper()
	pubkey, err := nostr.GetPublicKey(testPrivateKey)
	if err != nil {
		t.Fatalf("deriving the test pubkey: %v", err)
	}
	value, _ := sm.CreateSession(pubkey)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{
		Name:     sm.CookieName(),
		Value:    value,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return req
}

// gateThrough runs one request through the composed middleware and reports
// what the inner handler saw.
func gateThrough(sm *session.Manager, req *http.Request) (rec *httptest.ResponseRecorder, called bool, pubkey string) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		pubkey = session.PubkeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	rec = httptest.NewRecorder()
	Middleware(sm)(inner).ServeHTTP(rec, req)
	return rec, called, pubkey
}

func TestMiddlewarePassesAnAuthenticatedRequest(t *testing.T) {
	t.Parallel()
	sm := testSessionManager(t)
	rec, called, pubkey := gateThrough(sm, requestWithSession(t, sm, "/file/notes.md"))
	if !called {
		t.Fatal("the handler did not run for an authenticated request")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	want, err := nostr.GetPublicKey(testPrivateKey)
	if err != nil {
		t.Fatalf("deriving the test pubkey: %v", err)
	}
	if pubkey != want {
		t.Errorf("context identity = %q, want the session's %q", pubkey, want)
	}
}

func TestMiddlewareRedirectsAnUnauthenticatedRequest(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/file/notes.md", nil)
	rec, called, _ := gateThrough(testSessionManager(t), req)
	if called {
		t.Fatal("the handler ran for an unauthenticated request")
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc, want := rec.Header().Get("Location"), LoginPath+"?redirect=%2Ffile%2Fnotes.md"; loc != want {
		t.Errorf("location = %q, want %q", loc, want)
	}
}

func TestMiddlewareRedirectsTheRootWithoutAQuery(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec, called, _ := gateThrough(testSessionManager(t), req)
	if called {
		t.Fatal("the handler ran for an unauthenticated request")
	}
	if loc := rec.Header().Get("Location"); loc != LoginPath {
		t.Errorf("location = %q, want the bare %q", loc, LoginPath)
	}
}

func TestMiddlewareExemptsPublicPaths(t *testing.T) {
	t.Parallel()
	for _, path := range []string{LoginPath, "/auth/verify", "/static/app.css", "/events"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		rec, called, _ := gateThrough(testSessionManager(t), req)
		if !called {
			t.Errorf("the handler did not run for the public path %q", path)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status for %q = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}
