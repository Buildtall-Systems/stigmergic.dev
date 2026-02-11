package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const CookieName = "stigmergic_session"

type SessionManager struct {
	secret []byte
	maxAge time.Duration
}

func NewSessionManager(secret string, maxAge string) (*SessionManager, error) {
	var secretBytes []byte
	if secret == "" {
		secretBytes = make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, fmt.Errorf("failed to generate session secret: %w", err)
		}
	} else {
		secretBytes = []byte(secret)
	}

	duration, err := time.ParseDuration(maxAge)
	if err != nil {
		return nil, fmt.Errorf("invalid session_max_age %q: %w", maxAge, err)
	}

	return &SessionManager{
		secret: secretBytes,
		maxAge: duration,
	}, nil
}

func (sm *SessionManager) CreateSession(pubkey string) (string, time.Time) {
	expiry := time.Now().Add(sm.maxAge)
	expiryStr := strconv.FormatInt(expiry.UnixMilli(), 10)

	payload := pubkey + "." + expiryStr
	sig := sm.sign(payload)

	cookieValue := base64.RawURLEncoding.EncodeToString([]byte(payload + "." + sig))
	return cookieValue, expiry
}

func (sm *SessionManager) ValidateSession(cookieValue string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cookieValue)
	if err != nil {
		return "", fmt.Errorf("invalid session cookie encoding")
	}

	parts := strings.SplitN(string(decoded), ".", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed session cookie")
	}

	pubkey := parts[0]
	expiryStr := parts[1]
	sig := parts[2]

	payload := pubkey + "." + expiryStr
	expectedSig := sm.sign(payload)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return "", fmt.Errorf("invalid session signature")
	}

	expiryMillis, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid expiry timestamp")
	}

	if time.Now().UnixMilli() > expiryMillis {
		return "", fmt.Errorf("session expired")
	}

	return pubkey, nil
}

func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, r *http.Request, pubkey string) {
	value, expiry := sm.CreateSession(pubkey)
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureContext(r),
	}
	http.SetCookie(w, cookie)
}

func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
}

func (sm *SessionManager) sign(payload string) string {
	mac := hmac.New(sha256.New, sm.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func isSecureContext(r *http.Request) bool {
	host := r.Host
	if idx := strings.IndexByte(host, ':'); idx != -1 {
		host = host[:idx]
	}
	return host != "localhost" && host != "127.0.0.1" && host != "::1"
}
