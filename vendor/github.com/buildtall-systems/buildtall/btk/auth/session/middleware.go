package session

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const pubkeyContextKey contextKey = "session_pubkey"

const sessionIDContextKey contextKey = "session_id"

func PubkeyFromContext(ctx context.Context) string {
	v, ok := ctx.Value(pubkeyContextKey).(string)
	if !ok {
		return ""
	}
	return v
}

// SessionIDFromContext returns the per-login session id, or "" for requests
// authenticated by a legacy three-part cookie or not authenticated at all.
func SessionIDFromContext(ctx context.Context) string {
	v, ok := ctx.Value(sessionIDContextKey).(string)
	if !ok {
		return ""
	}
	return v
}

// ContextWithPubkey returns a context carrying the session npub exactly as
// ExtractSession would set it. It lets handler tests and non-HTTP callers
// establish the session identity without a cookie round-trip.
func ContextWithPubkey(ctx context.Context, npub string) context.Context {
	return context.WithValue(ctx, pubkeyContextKey, npub)
}

// ContextWithSessionID is the session id counterpart of ContextWithPubkey.
func ContextWithSessionID(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, sessionIDContextKey, sid)
}

func ExtractSession(sm *Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sm.CookieName())
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			pubkey, sid, err := sm.ValidateSessionWithID(cookie.Value)
			if err != nil {
				sm.ClearSessionCookie(w)
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), pubkeyContextKey, pubkey)
			ctx = context.WithValue(ctx, sessionIDContextKey, sid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuthOption adjusts which paths RequireAuth leaves open.
type RequireAuthOption func(*requireAuthConfig)

type requireAuthConfig struct {
	publicPrefixes []string
}

// WithPublicPrefix leaves every path under prefix open, for a site whose
// auth routes live somewhere other than /api/auth.
func WithPublicPrefix(prefix string) RequireAuthOption {
	return func(c *requireAuthConfig) {
		c.publicPrefixes = append(c.publicPrefixes, prefix)
	}
}

func RequireAuth(sm *Manager, opts ...RequireAuthOption) func(http.Handler) http.Handler {
	cfg := requireAuthConfig{publicPrefixes: []string{"/api/auth/", "/static/"}}
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.isPublic(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			pubkey := PubkeyFromContext(r.Context())
			if pubkey == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (c requireAuthConfig) isPublic(path string) bool {
	for _, prefix := range c.publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
