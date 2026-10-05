package session

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

const DefaultCookieName = "btk_session"

type Manager struct {
	cookieName string
	secret     []byte
	maxAge     time.Duration
}

func NewManager(cookieName, secret, maxAge string) (*Manager, error) {
	if cookieName == "" {
		cookieName = DefaultCookieName
	}

	var secretBytes []byte
	if secret == "" {
		secretBytes = make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, fmt.Errorf("generating session secret: %w", err)
		}
	} else {
		secretBytes = []byte(secret)
	}

	duration, err := time.ParseDuration(maxAge)
	if err != nil {
		return nil, fmt.Errorf("invalid max age %q: %w", maxAge, err)
	}

	return &Manager{
		cookieName: cookieName,
		secret:     secretBytes,
		maxAge:     duration,
	}, nil
}

func (m *Manager) CookieName() string {
	return m.cookieName
}

func (m *Manager) CreateSession(pubkey string) (string, string, time.Time) {
	expiry := time.Now().Add(m.maxAge)
	expiryStr := strconv.FormatInt(expiry.UnixMilli(), 10)
	sid := rand.Text()

	payload := pubkey + "." + expiryStr + "." + sid
	sig := m.sign(payload)

	cookieValue := base64.RawURLEncoding.EncodeToString([]byte(payload + "." + sig))
	return cookieValue, sid, expiry
}

type claims struct {
	npub string
	sid  string
}

func (m *Manager) ValidateSession(cookieValue string) (string, error) {
	c, err := m.validate(cookieValue)
	if err != nil {
		return "", err
	}
	return c.npub, nil
}

func (m *Manager) ValidateSessionWithID(cookieValue string) (string, string, error) {
	c, err := m.validate(cookieValue)
	if err != nil {
		return "", "", err
	}
	return c.npub, c.sid, nil
}

// Cookies minted before the session id claim existed have three parts and
// must keep validating through rollout; they carry an empty sid.
func (m *Manager) validate(cookieValue string) (claims, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cookieValue)
	if err != nil {
		return claims{}, fmt.Errorf("invalid session cookie encoding")
	}

	parts := strings.Split(string(decoded), ".")

	var c claims
	var expiryStr, sig string
	switch len(parts) {
	case 3:
		c.npub = parts[0]
		expiryStr = parts[1]
		sig = parts[2]
	case 4:
		c.npub = parts[0]
		expiryStr = parts[1]
		c.sid = parts[2]
		sig = parts[3]
	default:
		return claims{}, fmt.Errorf("malformed session cookie")
	}

	payload := strings.Join(parts[:len(parts)-1], ".")
	expectedSig := m.sign(payload)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return claims{}, fmt.Errorf("invalid session signature")
	}

	expiryMillis, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return claims{}, fmt.Errorf("invalid expiry timestamp")
	}

	if time.Now().UnixMilli() > expiryMillis {
		return claims{}, fmt.Errorf("session expired")
	}

	return c, nil
}

// Secure is always set: browsers exempt localhost from the https
// requirement for Secure cookies, so local dev over http keeps working.
func (m *Manager) SetSessionCookie(w http.ResponseWriter, pubkey string) string {
	value, sid, expiry := m.CreateSession(pubkey)
	cookie := &http.Cookie{
		Name:     m.cookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}
	http.SetCookie(w, cookie)
	return sid
}

func (m *Manager) ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}
	http.SetCookie(w, cookie)
}

func (m *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
