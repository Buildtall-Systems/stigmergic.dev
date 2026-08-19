package auth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/buildtall-systems/buildtall/btk/auth/session"
)

// LoginPath is the login page route. The monorepo session package deleted its
// constant when its gate went API-shaped, answering 401 under an /api/auth/
// public prefix; stigmergic keeps the redirect login experience, so the path
// lives here.
const LoginPath = "/auth/login"

// Middleware wraps next in the monorepo's ExtractSession and stigmergic's own
// gate: a public path passes through, an authenticated request proceeds with
// its identity in context, and an unauthenticated request is redirected to
// the login page carrying the original path in the redirect query.
func Middleware(sm *session.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		gate := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if session.PubkeyFromContext(r.Context()) == "" {
				redirectToLogin(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
		return session.ExtractSession(sm)(gate)
	}
}

func isPublicPath(path string) bool {
	if strings.HasPrefix(path, "/auth/") {
		return true
	}
	if strings.HasPrefix(path, "/static/") {
		return true
	}
	return path == "/events"
}

// redirectToLogin sends the request to the login page, naming the original
// path in the redirect query. The location is built from the constant login
// path, so the request can steer only the query value, never the destination.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	u := url.URL{Path: LoginPath}
	if r.URL.Path != "/" && r.URL.Path != "" {
		u.RawQuery = url.Values{"redirect": {r.URL.Path}}.Encode()
	}
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}
