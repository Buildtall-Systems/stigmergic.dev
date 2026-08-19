package session

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const pubkeyContextKey contextKey = "session_pubkey"

func PubkeyFromContext(ctx context.Context) string {
	v, ok := ctx.Value(pubkeyContextKey).(string)
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

func ExtractSession(sm *Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sm.CookieName())
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			pubkey, err := sm.ValidateSession(cookie.Value)
			if err != nil {
				sm.ClearSessionCookie(w)
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), pubkeyContextKey, pubkey)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuth(sm *Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
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

func isPublicPath(path string) bool {
	if strings.HasPrefix(path, "/api/auth/") {
		return true
	}
	if strings.HasPrefix(path, "/static/") {
		return true
	}
	return false
}
