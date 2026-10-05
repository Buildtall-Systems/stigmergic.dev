// Package login renders the service-agnostic login surface: a panel offering
// the extension, paste, and QR paths, and the fragment the panel swaps itself
// for while a remote signer completes the handshake.
package login

import (
	"encoding/json"
	"net/url"
	"strings"
)

// DefaultBase is where a site's auth routes live unless it names another.
const DefaultBase = "/api/auth"

// LoginPath is the login page. It stays at the site root whatever the auth
// base, because it is a page a visitor sees, not an API route.
const LoginPath = "/login"

// Paths is every route of one site's auth surface.
type Paths struct {
	Base      string
	Challenge string
	Verify    string
	Logout    string
	Me        string
	Start     string
	Status    string
	QR        string
	Sign      string
	Pubkey    string
	Login     string
	// BridgeRequests and BridgeAnswer carry the sign bridge, which a site
	// mounts only when it opts in.
	BridgeRequests string
	BridgeAnswer   string
}

// PathsAt derives a site's auth routes from its base.
func PathsAt(base string) Paths {
	return Paths{
		Base:      base,
		Challenge: base + "/challenge",
		Verify:    base + "/verify",
		Logout:    base + "/logout",
		Me:        base + "/me",
		Start:     base + "/nip46/start",
		Status:    base + "/nip46/status",
		QR:        base + "/nip46/qr",
		Sign:      base + "/nip46/sign",
		Pubkey:    base + "/nip46/pubkey",
		Login:     LoginPath,

		BridgeRequests: base + "/bridge/requests",
		BridgeAnswer:   base + "/bridge/answer",
	}
}

// DefaultPaths are the routes of a site that keeps DefaultBase.
func DefaultPaths() Paths {
	return PathsAt(DefaultBase)
}

// orDefault lets a zero Paths stand for the default routes.
func (p Paths) orDefault() Paths {
	if p.Base == "" {
		return DefaultPaths()
	}
	return p
}

// The default routes as constants, for callers that keep DefaultBase.
const (
	StartPath  = DefaultBase + "/nip46/start"
	StatusPath = DefaultBase + "/nip46/status"
	QRPath     = DefaultBase + "/nip46/qr"
	SignPath   = DefaultBase + "/nip46/sign"
	PubkeyPath = DefaultBase + "/nip46/pubkey"
)

// State values mirror the bunker engine's. They are plain strings here
// because the bunker handlers render these fragments, so this package must
// not import bunker.
const (
	StateWaiting      = "waiting"
	StateAuthRequired = "auth_required"
	StateFailed       = "failed"
	// StateRefused is the site's own answer, not the engine's: the signer
	// completed the login, and the site does not admit the npub.
	StateRefused = "refused"
)

// FlowNostrConnect is the value the QR button posts and the start handler
// switches on. The panel and the handler read it from here so the wire value
// has one source.
const FlowNostrConnect = "nostrconnect"

// NextParam carries the page a visitor returns to after a remote-signer
// login, from the panel through the start and status requests.
const NextParam = "next"

// nostrConnectVals is the QR button's request body. It carries next, because
// the button posts no form.
func nostrConnectVals(next string) string {
	vals, err := json.Marshal(map[string]string{"flow": FlowNostrConnect, NextParam: next})
	if err != nil {
		return `{"flow":"` + FlowNostrConnect + `"}`
	}
	return string(vals)
}

// IsLocalPath reports whether p is a path on this site. A redirect target
// that comes from a request must pass this check, so a login link can never
// send a visitor to another host. Browsers read a leading `/\` as "//".
func IsLocalPath(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, `/\`) {
		return false
	}
	u, err := url.Parse(p)
	return err == nil && u.Scheme == "" && u.Host == ""
}

// PollTrigger is how often a pending login asks for its status.
const PollTrigger = "every 2s"

// Pending is the polling fragment's view model.
type Pending struct {
	ID      string
	State   string
	AuthURL string
	Reason  string
	// Next is where a completed login lands. Empty means the service default.
	Next string
	// Paths are the site's auth routes. The zero value means DefaultPaths.
	Paths  Paths
	ShowQR bool
}

// Polling reports whether the fragment should keep asking for status. A
// terminal state renders without the polling attributes, which is what stops
// the browser from asking again.
func (p Pending) Polling() bool {
	return p.State == StateWaiting || p.State == StateAuthRequired
}

func (p Pending) StatusURL() string {
	q := url.Values{"id": {p.ID}}
	if p.Next != "" {
		q.Set(NextParam, p.Next)
	}
	return p.Paths.orDefault().Status + "?" + q.Encode()
}

// RestartURL is the login page, keeping the page the visitor returns to.
func (p Pending) RestartURL() string {
	loginPath := p.Paths.orDefault().Login
	if p.Next == "" {
		return loginPath
	}
	return loginPath + "?" + url.Values{NextParam: {p.Next}}.Encode()
}

func (p Pending) QRURL() string {
	return p.Paths.orDefault().QR + "?id=" + url.QueryEscape(p.ID)
}
