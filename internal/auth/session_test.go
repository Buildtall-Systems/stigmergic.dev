package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSessionManager_GeneratesSecret(t *testing.T) {
	t.Parallel()

	sm, err := NewSessionManager("", "24h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sm.secret) != 32 {
		t.Errorf("expected 32-byte generated secret, got %d", len(sm.secret))
	}
}

func TestNewSessionManager_UsesProvidedSecret(t *testing.T) {
	t.Parallel()

	sm, err := NewSessionManager("mysecret", "24h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(sm.secret) != "mysecret" {
		t.Error("expected provided secret to be used")
	}
}

func TestNewSessionManager_InvalidDuration(t *testing.T) {
	t.Parallel()

	_, err := NewSessionManager("", "notaduration")
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestSessionCreateAndValidate(t *testing.T) {
	t.Parallel()

	sm, err := NewSessionManager("testsecret", "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pubkey := testPubkey
	cookieValue, expiry := sm.CreateSession(pubkey)

	if cookieValue == "" {
		t.Fatal("expected non-empty cookie value")
	}

	if expiry.Before(time.Now()) {
		t.Error("expected expiry in the future")
	}

	result, err := sm.ValidateSession(cookieValue)
	if err != nil {
		t.Fatalf("expected valid session, got error: %v", err)
	}
	if result != pubkey {
		t.Errorf("expected pubkey %s, got %s", pubkey, result)
	}
}

func TestSessionValidate_TamperedSignature(t *testing.T) {
	t.Parallel()

	sm, err := NewSessionManager("testsecret", "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pubkey := testPubkey
	cookieValue, _ := sm.CreateSession(pubkey)

	tampered := cookieValue[:len(cookieValue)-2] + "XX"
	_, err = sm.ValidateSession(tampered)
	if err == nil {
		t.Fatal("expected error for tampered session")
	}
}

func TestSessionValidate_WrongSecret(t *testing.T) {
	t.Parallel()

	sm1, _ := NewSessionManager("secret1", "1h")
	sm2, _ := NewSessionManager("secret2", "1h")

	pubkey := testPubkey
	cookieValue, _ := sm1.CreateSession(pubkey)

	_, err := sm2.ValidateSession(cookieValue)
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestSessionValidate_Expired(t *testing.T) {
	t.Parallel()

	sm, err := NewSessionManager("testsecret", "100ms")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pubkey := testPubkey
	cookieValue, _ := sm.CreateSession(pubkey)

	time.Sleep(200 * time.Millisecond)

	_, err = sm.ValidateSession(cookieValue)
	if err == nil {
		t.Fatal("expected error for expired session")
	}
}

func TestSessionValidate_InvalidEncoding(t *testing.T) {
	t.Parallel()

	sm, _ := NewSessionManager("testsecret", "1h")

	_, err := sm.ValidateSession("!!!invalid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid encoding")
	}
}

func TestSetSessionCookie(t *testing.T) {
	t.Parallel()

	sm, _ := NewSessionManager("testsecret", "1h")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)

	pubkey := testPubkey
	sm.SetSessionCookie(recorder, req, pubkey)

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != CookieName {
		t.Errorf("expected cookie name %s, got %s", CookieName, cookie.Name)
	}
	if !cookie.HttpOnly {
		t.Error("expected HttpOnly cookie")
	}
	if cookie.Secure {
		t.Error("expected non-secure cookie for localhost")
	}

	result, err := sm.ValidateSession(cookie.Value)
	if err != nil {
		t.Fatalf("expected valid cookie, got error: %v", err)
	}
	if result != pubkey {
		t.Errorf("expected pubkey %s, got %s", pubkey, result)
	}
}

func TestSetSessionCookie_SecureForRemoteHost(t *testing.T) {
	t.Parallel()

	sm, _ := NewSessionManager("testsecret", "1h")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	req.Host = "example.com"

	sm.SetSessionCookie(recorder, req, "abc123")

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	if !cookies[0].Secure {
		t.Error("expected Secure cookie for remote host")
	}
}

func TestClearSessionCookie(t *testing.T) {
	t.Parallel()

	sm, _ := NewSessionManager("testsecret", "1h")

	recorder := httptest.NewRecorder()
	sm.ClearSessionCookie(recorder)

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != CookieName {
		t.Errorf("expected cookie name %s, got %s", CookieName, cookie.Name)
	}
	if cookie.MaxAge != -1 {
		t.Errorf("expected MaxAge -1, got %d", cookie.MaxAge)
	}
}
