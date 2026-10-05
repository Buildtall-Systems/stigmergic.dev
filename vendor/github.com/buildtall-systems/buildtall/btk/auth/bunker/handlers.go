package bunker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/buildtall-systems/buildtall/btk/auth/session"
	"github.com/buildtall-systems/buildtall/btk/views/login"
)

const (
	qrCodeSize    = 256
	expiredReason = "This login expired. Start again."
	// signReplyMargin is the time left to write the answer after the signer
	// call ends. A reverse proxy in front must wait longer than
	// signRequestTimeout plus this margin.
	signReplyMargin = 10 * time.Second
)

// allowSignerWait moves the write deadline past the signer call, which can
// wait on the visitor for up to signRequestTimeout. A service's server-wide
// WriteTimeout is usually shorter. A writer that cannot move its deadline
// keeps the server's.
func allowSignerWait(w http.ResponseWriter) {
	deadline := time.Now().Add(signRequestTimeout + signReplyMargin)
	if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil {
		return
	}
}

type startRequest struct {
	BunkerURI string `json:"bunker_uri"`
	Flow      string `json:"flow"`
	Next      string `json:"next"`
}

// readStartRequest accepts either a form post from the login panel or a JSON
// body, so the panel and any programmatic caller share one endpoint.
func readStartRequest(r *http.Request) (startRequest, bool) {
	var req startRequest

	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return req, false
		}
		return req, true
	}

	if err := r.ParseForm(); err != nil {
		return req, false
	}
	req.BunkerURI = r.PostForm.Get("bunker_uri")
	req.Flow = r.PostForm.Get("flow")
	req.Next = r.PostForm.Get(login.NextParam)
	return req, true
}

// localNext keeps a requested return path only when it stays on this site.
func localNext(next string) string {
	if !login.IsLocalPath(next) {
		return ""
	}
	return next
}

// renderPending writes the polling fragment. A render failure has already
// written a partial body, so it is logged by the caller's server, not
// converted into a status code here.
func renderPending(w http.ResponseWriter, r *http.Request, m *Manager, p login.Pending) {
	p.Paths = m.paths
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := login.PendingFragment(p).Render(r.Context(), w); err != nil {
		return
	}
}

// NewStartHandler begins a login and answers with the polling fragment. It
// accepts a bunker URI for the paste flow, or flow=nostrconnect for the QR
// flow, from either a form post or a JSON body.
func NewStartHandler(m *Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		req, ok := readStartRequest(r)
		if !ok {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		next := localNext(req.Next)

		switch {
		case req.Flow == login.FlowNostrConnect:
			id, err := m.StartNostrConnectLogin()
			if err != nil {
				http.Error(w, "starting login failed", http.StatusInternalServerError)
				return
			}
			renderPending(w, r, m, login.Pending{ID: id, State: login.StateWaiting, ShowQR: true, Next: next})
		case req.BunkerURI != "":
			id, err := m.StartBunkerLogin(req.BunkerURI)
			if err != nil {
				http.Error(w, "invalid bunker URI", http.StatusBadRequest)
				return
			}
			renderPending(w, r, m, login.Pending{ID: id, State: login.StateWaiting, Next: next})
		default:
			http.Error(w, "provide bunker_uri or flow", http.StatusBadRequest)
		}
	})
}

// LoginHooks are a site's say in a remote-signer login. Both are optional.
// Admit runs before any cookie exists; its error refuses the login, and its
// text is what the visitor sees. OnLogin runs in the background after a
// login succeeds, on a context that outlives the request.
type LoginHooks struct {
	Admit   func(ctx context.Context, npub string) error
	OnLogin func(ctx context.Context, npub string)
}

// NewStatusHandler reports a pending login's state. On ready it asks Admit,
// issues the session cookie, binds the sid to the bunker session, fires
// OnLogin, and redirects to the poll's next path, or to redirectTo when next
// is absent or not local. A refused login is discarded and issues no cookie.
// Pending ids are single-use: once bound, refused, or expired, further polls
// answer failed. The browser learns that its session is bunker-backed from
// the nav fragment, which the service renders from Manager.Session.
func NewStatusHandler(m *Manager, sessions *session.Manager, redirectTo string, hooks LoginHooks) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		next := localNext(r.URL.Query().Get(login.NextParam))

		st, ok := m.Status(id)
		if !ok {
			renderPending(w, r, m, login.Pending{ID: id, State: login.StateFailed, Reason: expiredReason, Next: next})
			return
		}

		switch st.State {
		case StateReady:
			if hooks.Admit != nil {
				if err := hooks.Admit(r.Context(), st.Npub); err != nil {
					m.Discard(id)
					renderPending(w, r, m, login.Pending{ID: id, State: login.StateRefused, Reason: err.Error(), Next: next})
					return
				}
			}
			sid := sessions.SetSessionCookie(w, st.Npub)
			if _, err := m.Bind(id, sid); err != nil {
				sessions.ClearSessionCookie(w)
				renderPending(w, r, m, login.Pending{ID: id, State: login.StateFailed, Reason: expiredReason, Next: next})
				return
			}
			if hooks.OnLogin != nil {
				go hooks.OnLogin(context.WithoutCancel(r.Context()), st.Npub)
			}
			if next == "" {
				next = redirectTo
			}
			w.Header().Set("HX-Redirect", next)
			w.WriteHeader(http.StatusOK)
		case StateFailed:
			renderPending(w, r, m, login.Pending{ID: id, State: login.StateFailed, Reason: st.Reason, Next: next})
		default:
			renderPending(w, r, m, login.Pending{
				ID:      id,
				State:   string(st.State),
				AuthURL: st.AuthURL,
				ShowQR:  st.URI != "",
				Next:    next,
			})
		}
	})
}

// NewQRHandler renders a pending nostrconnect login's URI as a PNG. Only the
// QR flow has a URI, so a paste-flow id answers 404.
func NewQRHandler(m *Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		st, ok := m.Status(r.URL.Query().Get("id"))
		if !ok || st.URI == "" {
			http.Error(w, "no QR code for this login", http.StatusNotFound)
			return
		}

		png, err := qrcode.Encode(st.URI, qrcode.Medium, qrCodeSize)
		if err != nil {
			http.Error(w, "rendering the QR code failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := w.Write(png); err != nil {
			return
		}
	})
}

// NewPubkeyHandler answers with the caller's own public key in hex, the form
// the browser signer facade needs for events and filters. Same 401 and 410
// split as the sign endpoint.
func NewPubkeyHandler(m *Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sid := session.SessionIDFromContext(r.Context())
		if sid == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		s, ok := m.Session(sid)
		if !ok {
			http.Error(w, "session gone", http.StatusGone)
			return
		}

		writeJSON(w, map[string]string{"pubkey": s.UserPubkeyHex()})
	})
}

// NewSignHandler signs the posted event through the caller's bunker session.
// 401 when the request carries no session id, 410 when the sid maps to no
// live session so the facade can distinguish dead-session from logged-out.
func NewSignHandler(m *Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sid := session.SessionIDFromContext(r.Context())
		if sid == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var evt nostr.Event
		if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}

		allowSignerWait(w)

		if err := m.Sign(r.Context(), sid, &evt); err != nil {
			if errors.Is(err, ErrNoSession) {
				http.Error(w, "session gone", http.StatusGone)
				return
			}
			http.Error(w, "signing failed", http.StatusInternalServerError)
			return
		}

		writeJSON(w, evt)
	})
}

// Logout tears down the caller's bunker session, if any. Services call this
// from their logout handlers. The session cookie itself stays the caller's
// business.
func Logout(m *Manager, r *http.Request) {
	if sid := session.SessionIDFromContext(r.Context()); sid != "" {
		m.Teardown(sid)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return
	}
}
