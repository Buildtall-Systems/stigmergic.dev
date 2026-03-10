package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/Buildtall-Systems/btk/auth/session"
)

const (
	testPrivateKey = "9a9787e3e31a4b0e7e483ed97b1ab0a45534675b07003a51c0840d6a681ad53a"
	kindHTTPAuth   = 27235
)

func testSessionManager(t *testing.T) *session.Manager {
	t.Helper()
	sm, err := session.NewManager("stigmergic_session", "testsecret", "1h")
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	return sm
}

func TestLoginHandler_GET(t *testing.T) {
	t.Parallel()

	handler := LoginHandler("http://localhost:8080")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, session.LoginPath, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestLoginHandler_POST_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	handler := LoginHandler("http://localhost:8080")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, session.LoginPath, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestVerifyHandler_ValidEvent(t *testing.T) {
	t.Parallel()

	sm := testSessionManager(t)
	serverURL := "http://localhost:8080"
	verifyURL := serverURL + "/auth/verify"

	pub, _ := nostr.GetPublicKey(testPrivateKey)
	allowedPubkeys := []string{pub}

	event := &nostr.Event{
		Kind:      kindHTTPAuth,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Tags: nostr.Tags{
			{"u", verifyURL},
			{"method", "POST"},
		},
		Content: "",
	}
	_ = event.Sign(testPrivateKey)

	body, _ := json.Marshal(verifyRequest{Event: event})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler := VerifyHandler(sm, allowedPubkeys, serverURL)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp verifyResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.OK {
		t.Errorf("expected ok=true, got error: %s", resp.Error)
	}

	if resp.Redirect != "/" {
		t.Errorf("expected redirect /, got %s", resp.Redirect)
	}

	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == sm.CookieName() {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected session cookie to be set")
	}
}

func TestVerifyHandler_UnauthorizedPubkey(t *testing.T) {
	t.Parallel()

	sm := testSessionManager(t)
	serverURL := "http://localhost:8080"
	verifyURL := serverURL + "/auth/verify"

	allowedPubkeys := []string{"0000000000000000000000000000000000000000000000000000000000000000"}

	event := &nostr.Event{
		Kind:      kindHTTPAuth,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Tags: nostr.Tags{
			{"u", verifyURL},
			{"method", "POST"},
		},
		Content: "",
	}
	_ = event.Sign(testPrivateKey)

	body, _ := json.Marshal(verifyRequest{Event: event})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler := VerifyHandler(sm, allowedPubkeys, serverURL)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestVerifyHandler_InvalidBody(t *testing.T) {
	t.Parallel()

	sm := testSessionManager(t)
	handler := VerifyHandler(sm, nil, "http://localhost:8080")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/verify", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestVerifyHandler_MissingEvent(t *testing.T) {
	t.Parallel()

	sm := testSessionManager(t)
	handler := VerifyHandler(sm, nil, "http://localhost:8080")

	body, _ := json.Marshal(verifyRequest{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/verify", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestLogoutHandler(t *testing.T) {
	t.Parallel()

	sm := testSessionManager(t)
	handler := LogoutHandler(sm)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != session.LoginPath {
		t.Errorf("expected redirect to /auth/login, got %s", location)
	}

	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == sm.CookieName() && c.MaxAge == -1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected session cookie to be cleared")
	}
}
