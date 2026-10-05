package site

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/auth/bunker"
	"github.com/buildtall-systems/buildtall/btk/auth/challenge"
	"github.com/buildtall-systems/buildtall/btk/auth/identity"
	"github.com/buildtall-systems/buildtall/btk/auth/session"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
	btkauth "github.com/buildtall-systems/buildtall/btk/views/auth"
	"github.com/buildtall-systems/buildtall/btk/views/login"
)

const (
	maxRequestBody  = 1 << 20
	eventFormField  = "event"
	contentTypeHTML = "text/html; charset=utf-8"
	defaultRedirect = "/"
)

// Options are one site's part in its authentication surface.
type Options struct {
	// Sessions issues and clears the session cookie.
	Sessions *session.Manager
	// Pool, when set, carries the NIP-46 transport. Leave it nil to give the
	// bunker engine its own pool, so the transport relays never receive the
	// NIP-42 auth that a service's home-relay pool sends.
	Pool   *nostr.SimplePool
	Logger *slog.Logger
	// Resolve builds the signed-in user's nav identity. It must not return
	// nil; the mount sets BunkerBacked on a copy.
	Resolve   func(ctx context.Context, npub string) *identity.UserInfo
	MenuItems func(user *identity.UserInfo) []identity.MenuItem
	// Admit, when set, refuses an npub before any cookie exists. Its error
	// text is what the visitor sees.
	Admit func(ctx context.Context, npub string) error
	// OnLogin, when set, runs in the background after every successful
	// login of either kind, on a context that outlives the request.
	OnLogin func(ctx context.Context, npub string)
	// MeExtras, when set, adds headers and out-of-band markup to /me. What
	// it writes follows the nav fragment.
	MeExtras func(w http.ResponseWriter, r *http.Request, user *identity.UserInfo)
	// LoginPage renders the login page in the site's own layout.
	LoginPage func(paths login.Paths, next string) templ.Component
	Name      string
	BaseURL   string
	// Redirect is where a login lands when it carries no local next path.
	Redirect string
	// Paths are the site's auth routes; the zero value means the defaults.
	Paths login.Paths
	NIP46 NIP46Config
	// SignBridge mounts the sign bridge, so Signer also serves sessions that
	// signed in with the extension. Each such signature waits for one of the
	// visitor's open pages, which carry data-btk-sign-bridge on the body.
	SignBridge bool
}

// Site is a mounted authentication surface.
type Site struct {
	bunkers    *bunker.Manager
	challenges *challenge.Store
	bridge     *bridge
	log        *slog.Logger
	opts       Options
}

// Mount registers a site's whole authentication surface on mux: the
// extension login routes, the remote-signer routes, and the login page. The
// routes read the session from the request context, so the handler that
// serves mux must sit behind session.ExtractSession for opts.Sessions. A site
// that also uses session.RequireAuth with a non-default base passes
// session.WithPublicPrefix for that base. Close the returned Site on
// shutdown.
func Mount(mux *http.ServeMux, opts Options) (*Site, error) {
	switch {
	case opts.Sessions == nil:
		return nil, errors.New("site: Options.Sessions is required")
	case opts.Resolve == nil:
		return nil, errors.New("site: Options.Resolve is required")
	case opts.LoginPage == nil:
		return nil, errors.New("site: Options.LoginPage is required")
	}

	if opts.Paths.Base == "" {
		opts.Paths = login.DefaultPaths()
	}
	if opts.Redirect == "" {
		opts.Redirect = defaultRedirect
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	opts.NIP46.Defaults()

	bunkerOpts := []bunker.Option{
		bunker.WithClientMetadata(opts.Name, opts.BaseURL),
		bunker.WithTransportRelays(opts.NIP46.TransportRelays...),
		bunker.WithLoginPaths(opts.Paths),
		bunker.WithLogger(opts.Logger),
	}
	if opts.Pool != nil {
		bunkerOpts = append(bunkerOpts, bunker.WithPool(opts.Pool))
	}

	s := &Site{
		opts:       opts,
		bunkers:    bunker.NewManager(bunkerOpts...),
		challenges: challenge.NewStore(),
		log:        opts.Logger,
	}
	if opts.SignBridge {
		s.bridge = newBridge(opts.Logger)
	}
	s.register(mux)
	return s, nil
}

func (s *Site) register(mux *http.ServeMux) {
	p := s.opts.Paths
	mux.HandleFunc("POST "+p.Challenge, s.handleChallenge)
	mux.HandleFunc("POST "+p.Verify, s.handleVerify)
	mux.HandleFunc("POST "+p.Logout, s.handleLogout)
	mux.HandleFunc("GET "+p.Me, s.handleMe)

	mux.Handle("POST "+p.Start, bunker.NewStartHandler(s.bunkers))
	mux.Handle("GET "+p.Status, bunker.NewStatusHandler(s.bunkers, s.opts.Sessions, s.opts.Redirect, bunker.LoginHooks{
		Admit:   s.opts.Admit,
		OnLogin: s.opts.OnLogin,
	}))
	mux.Handle("GET "+p.QR, bunker.NewQRHandler(s.bunkers))
	mux.Handle("POST "+p.Sign, bunker.NewSignHandler(s.bunkers))
	mux.Handle("GET "+p.Pubkey, bunker.NewPubkeyHandler(s.bunkers))

	mux.HandleFunc("GET "+p.Login, s.handleLogin)

	if s.bridge != nil {
		mux.HandleFunc("GET "+p.BridgeRequests, s.bridge.handleRequests)
		mux.HandleFunc("POST "+p.BridgeAnswer, s.bridge.handleAnswer)
	}
}

// BunkerBacked reports whether the request's session signs through a live
// bunker session, so a page's nav tells btkSigner where to sign.
func (s *Site) BunkerBacked(r *http.Request) bool {
	sid := session.SessionIDFromContext(r.Context())
	if sid == "" {
		return false
	}
	_, live := s.bunkers.Session(sid)
	return live
}

// Signer returns a signer for the request's signed-in user. A bunker-backed
// session signs through its live bunker session. Any other session signs
// through the sign bridge, which signs relay AUTH only and needs
// Options.SignBridge. It reports false for an anonymous request, and for an
// extension-backed session without the bridge.
func (s *Site) Signer(r *http.Request) (nostr.Signer, bool) {
	sid := session.SessionIDFromContext(r.Context())
	npub := session.PubkeyFromContext(r.Context())
	if sid == "" || npub == "" {
		return nil, false
	}
	if live, ok := s.bunkers.Session(sid); ok {
		return live, true
	}
	if s.bridge == nil {
		return nil, false
	}
	userHex, err := btknostr.NpubToHex(npub)
	if err != nil {
		s.log.Warn("decoding session npub", "error", err)
		return nil, false
	}
	return &bridgeSigner{bridge: s.bridge, sid: sid, userHex: userHex}, true
}

// User resolves the request's signed-in user, or nil for an anonymous one.
func (s *Site) User(r *http.Request) *identity.UserInfo {
	npub := session.PubkeyFromContext(r.Context())
	if npub == "" {
		return nil
	}
	user := *s.opts.Resolve(r.Context(), npub)
	user.BunkerBacked = s.BunkerBacked(r)
	return &user
}

// Logout ends the request's bunker session, while the request still carries
// its session id, then clears the session cookie. A site's own logout route
// calls it; the mounted logout route already does.
func (s *Site) Logout(w http.ResponseWriter, r *http.Request) {
	bunker.Logout(s.bunkers, r)
	s.opts.Sessions.ClearSessionCookie(w)
}

// Close ends every pending login and bunker session, and the sign bridge.
func (s *Site) Close() {
	s.bunkers.Close()
	if s.bridge != nil {
		s.bridge.close()
	}
}

type challengeRequest struct {
	Pubkey string `json:"pubkey"`
}

func (s *Site) handleChallenge(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var req challengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Pubkey == "" {
		s.writeError(w, http.StatusBadRequest, "pubkey required")
		return
	}
	npub, err := btknostr.HexToNpub(req.Pubkey)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid pubkey")
		return
	}

	evt, err := s.challenges.NewChallenge(npub)
	if err != nil {
		s.log.Error("creating challenge", "error", err)
		s.writeError(w, http.StatusInternalServerError, "challenge creation failed")
		return
	}
	s.writeJSON(w, http.StatusOK, evt)
}

func (s *Site) handleVerify(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var signed nostr.Event
	if err := json.Unmarshal([]byte(r.FormValue(eventFormField)), &signed); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	npub, err := btknostr.HexToNpub(signed.PubKey)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid pubkey")
		return
	}

	verified, err := s.challenges.Verify(&signed, npub)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if s.opts.Admit != nil {
		if err := s.opts.Admit(r.Context(), verified); err != nil {
			s.writeError(w, http.StatusForbidden, err.Error())
			return
		}
	}

	user := &identity.UserInfo{Npub: verified}
	var buf bytes.Buffer
	if err := btkauth.NavAuthLoadingAt(s.opts.Paths.Me, user, s.menuItems(user)).Render(r.Context(), &buf); err != nil {
		s.log.Error("rendering nav auth loading", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}

	s.opts.Sessions.SetSessionCookie(w, verified)
	if s.opts.OnLogin != nil {
		go s.opts.OnLogin(context.WithoutCancel(r.Context()), verified)
	}
	s.writeHTML(w, buf.Bytes())
}

func (s *Site) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.Logout(w, r)
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// extrasWriter collects MeExtras output behind the nav fragment while its
// headers still reach the response.
type extrasWriter struct {
	http.ResponseWriter
	buf *bytes.Buffer
}

func (e extrasWriter) Write(p []byte) (int, error) {
	return e.buf.Write(p)
}

func (s *Site) handleMe(w http.ResponseWriter, r *http.Request) {
	user := s.User(r)
	if user == nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	var buf bytes.Buffer
	if err := btkauth.NavAuth(user, s.menuItems(user)).Render(r.Context(), &buf); err != nil {
		s.log.Error("rendering nav auth", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	if s.opts.MeExtras != nil {
		s.opts.MeExtras(extrasWriter{ResponseWriter: w, buf: &buf}, r, user)
	}
	s.writeHTML(w, buf.Bytes())
}

// handleLogin renders the login page. Only a path on this site survives as
// next; a visitor who already holds a session goes straight there.
func (s *Site) handleLogin(w http.ResponseWriter, r *http.Request) {
	next := s.localNext(r.URL.Query().Get(login.NextParam))
	if session.PubkeyFromContext(r.Context()) != "" {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}

	var buf bytes.Buffer
	if err := s.opts.LoginPage(s.opts.Paths, next).Render(r.Context(), &buf); err != nil {
		s.log.Error("rendering login page", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.writeHTML(w, buf.Bytes())
}

// localNext keeps a requested return path only when it stays on this site,
// and rebuilds it from escaped segments so nothing but a local path and its
// query can reach a redirect. Anything else falls back to the site's
// Redirect.
func (s *Site) localNext(next string) string {
	if !login.IsLocalPath(next) {
		return s.opts.Redirect
	}
	u, err := url.Parse(next)
	if err != nil {
		return s.opts.Redirect
	}

	// The escaped form keeps an encoded slash inside its segment, so
	// /%2Fhost stays a path and never decodes into //host.
	raw := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	segments := make([]string, 0, len(raw))
	for _, seg := range raw {
		decoded, err := url.PathUnescape(seg)
		if err != nil {
			return s.opts.Redirect
		}
		segments = append(segments, url.PathEscape(decoded))
	}
	target := "/" + strings.Join(segments, "/")

	query := u.Query()
	if len(query) == 0 {
		return target
	}
	keys := slices.Sorted(maps.Keys(query))
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		for _, v := range query[k] {
			pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return target + "?" + strings.Join(pairs, "&")
}

func (s *Site) menuItems(user *identity.UserInfo) []identity.MenuItem {
	if s.opts.MenuItems == nil {
		return nil
	}
	return s.opts.MenuItems(user)
}

func (s *Site) writeHTML(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", contentTypeHTML)
	if _, err := w.Write(body); err != nil {
		s.log.Debug("writing response", "error", err)
	}
}

func (s *Site) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Debug("encoding response", "error", err)
	}
}

func (s *Site) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}
