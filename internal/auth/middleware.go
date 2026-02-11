package auth

import (
	"context"
	"net/http"
	"strings"
)

const LoginPath = "/auth/login"

type contextKey string

const pubkeyContextKey contextKey = "auth_pubkey"

func Middleware(sm *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(CookieName)
			if err != nil {
				redirectToLogin(w, r)
				return
			}

			pubkey, err := sm.ValidateSession(cookie.Value)
			if err != nil {
				sm.ClearSessionCookie(w)
				redirectToLogin(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), pubkeyContextKey, pubkey)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func PubkeyFromContext(ctx context.Context) string {
	v, _ := ctx.Value(pubkeyContextKey).(string)
	return v
}

func isPublicPath(path string) bool {
	if strings.HasPrefix(path, "/auth/") {
		return true
	}
	if strings.HasPrefix(path, "/static/") {
		return true
	}
	return false
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	target := LoginPath
	if r.URL.Path != "/" && r.URL.Path != "" {
		target += "?redirect=" + r.URL.Path
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
