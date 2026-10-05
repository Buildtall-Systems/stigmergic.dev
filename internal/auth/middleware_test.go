package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buildtall-systems/buildtall/btk/auth/session"
	"github.com/buildtall-systems/buildtall/btk/views/login"
)

// testNpub is alice from the nostr skill's test keypair table.
const testNpub = "npub1paydacp4r4njzewfxr0xjjs9g9gyw5jtr04qxv8ranpdrqk25ves7sp2vp"

func testSessionManager(t *testing.T) *session.Manager {
	t.Helper()
	sm, err := session.NewManager("stigmergic_session", "testsecret", "1h")
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	return sm
}

func requestWithSession(t *testing.T, sm *session.Manager, path string) *http.Request {
	t.Helper()
	value, _, _ := sm.CreateSession(testNpub)
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

// gateThrough runs one request through ExtractSession and the gate, as the
// server composes them, and reports what the inner handler saw.
func gateThrough(sm *session.Manager, req *http.Request) (rec *httptest.ResponseRecorder, called bool, npub string) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		npub = session.PubkeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	rec = httptest.NewRecorder()
	session.ExtractSession(sm)(Gate(inner)).ServeHTTP(rec, req)
	return rec, called, npub
}

func TestGatePassesAnAuthenticatedRequest(t *testing.T) {
	t.Parallel()
	sm := testSessionManager(t)
	rec, called, npub := gateThrough(sm, requestWithSession(t, sm, "/file/notes.md"))
	if !called {
		t.Fatal("the handler did not run for an authenticated request")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if npub != testNpub {
		t.Errorf("context identity = %q, want the session's %q", npub, testNpub)
	}
}

func TestGateRedirectsAnUnauthenticatedRequest(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/file/notes.md", nil)
	rec, called, _ := gateThrough(testSessionManager(t), req)
	if called {
		t.Fatal("the handler ran for an unauthenticated request")
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc, want := rec.Header().Get("Location"), login.LoginPath+"?next=%2Ffile%2Fnotes.md"; loc != want {
		t.Errorf("location = %q, want %q", loc, want)
	}
}

func TestGateRedirectsTheRootWithoutAQuery(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec, called, _ := gateThrough(testSessionManager(t), req)
	if called {
		t.Fatal("the handler ran for an unauthenticated request")
	}
	if loc := rec.Header().Get("Location"); loc != login.LoginPath {
		t.Errorf("location = %q, want the bare %q", loc, login.LoginPath)
	}
}

func TestGateExemptsPublicPaths(t *testing.T) {
	t.Parallel()
	paths := []string{
		login.LoginPath,
		login.DefaultPaths().Challenge,
		login.DefaultPaths().BridgeRequests,
		"/static/btk/js/nostr.js",
		"/events",
	}
	for _, path := range paths {
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

func TestGateHoldsTheOldLoginRoutes(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/auth/login", "/auth/verify", "/api/authx"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		_, called, _ := gateThrough(testSessionManager(t), req)
		if called {
			t.Errorf("the handler ran for the non-public path %q", path)
		}
	}
}
