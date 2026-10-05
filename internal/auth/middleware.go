package auth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/buildtall-systems/buildtall/btk/auth/session"
	"github.com/buildtall-systems/buildtall/btk/views/login"
)

// Gate is stigmergic's --auth gate. A public path passes through, a request
// whose session identity is in context proceeds, and any other request is
// redirected to the btk login page with its path in the next query. Gate
// reads the identity session.ExtractSession put in context, so it must run
// inside that middleware.
func Gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) || session.PubkeyFromContext(r.Context()) != "" {
			next.ServeHTTP(w, r)
			return
		}
		redirectToLogin(w, r)
	})
}

// isPublicPath names what an anonymous visitor must reach to sign in: the
// btk auth API, the login page, static assets, and the event stream.
func isPublicPath(path string) bool {
	if strings.HasPrefix(path, login.DefaultBase+"/") {
		return true
	}
	if strings.HasPrefix(path, "/static/") {
		return true
	}
	return path == login.LoginPath || path == "/events"
}

// redirectToLogin sends the request to the login page, naming the original
// path in the next query. The location is built from the constant login
// path, so the request can steer only the query value, never the destination.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	u := url.URL{Path: login.LoginPath}
	if r.URL.Path != "/" && r.URL.Path != "" {
		u.RawQuery = url.Values{login.NextParam: {r.URL.Path}}.Encode()
	}
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}
