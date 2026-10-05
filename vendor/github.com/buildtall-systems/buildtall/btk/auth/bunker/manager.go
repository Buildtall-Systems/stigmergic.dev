// Package bunker holds server-side NIP-46 login state: pending logins that
// are still handshaking with a remote signer, and live bunker sessions keyed
// by the per-login session id carried in the btk session cookie. The browser
// never touches NIP-46; services mount the package's handlers and route
// signing requests here.
package bunker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/keyer"
	"github.com/nbd-wtf/go-nostr/nip46"

	"github.com/buildtall-systems/buildtall/btk/views/login"
)

// DefaultTransportRelay carries nostrconnect handshake and RPC traffic. Its
// relay-manager exempts kind 24133 from AUTH and the writer list.
const DefaultTransportRelay = "wss://relay.nostr.io"

const (
	pendingExpiration  = 5 * time.Minute
	defaultSessionTTL  = 24 * time.Hour
	defaultIdleTimeout = 2 * time.Hour
	signRequestTimeout = 60 * time.Second
	sweepInterval      = time.Minute
	defaultPerms       = "get_public_key,ping,sign_event"
)

var (
	ErrUnknownPending = errors.New("unknown pending login")
	ErrNotReady       = errors.New("pending login is not ready")
	ErrNoSession      = errors.New("no live bunker session")
)

type State string

const (
	StateWaiting      State = "waiting"
	StateAuthRequired State = "auth_required"
	StateReady        State = "ready"
	StateFailed       State = "failed"
)

// Status is a point-in-time view of a pending login for polling handlers.
// URI is set for the nostrconnect flow only, and is what the QR handler
// renders.
type Status struct {
	State   State
	AuthURL string
	Npub    string
	Reason  string
	URI     string
}

type Manager struct {
	pool            *nostr.SimplePool
	name            string
	url             string
	perms           string
	log             *slog.Logger
	pendings        map[string]*pendingLogin
	sessions        map[string]*Session
	done            chan struct{}
	paths           login.Paths
	transportRelays []string
	sessionTTL      time.Duration
	idleTimeout     time.Duration
	mu              sync.Mutex
	closeOnce       sync.Once
	ownsPool        bool
}

type Option func(*Manager)

func WithPool(pool *nostr.SimplePool) Option {
	return func(m *Manager) { m.pool = pool }
}

func WithTransportRelays(relays ...string) Option {
	return func(m *Manager) { m.transportRelays = relays }
}

func WithClientMetadata(name, url string) Option {
	return func(m *Manager) {
		m.name = name
		m.url = url
	}
}

// WithLoginPaths names the site's auth routes, which the login fragments
// link to.
func WithLoginPaths(paths login.Paths) Option {
	return func(m *Manager) { m.paths = paths }
}

func WithPerms(perms string) Option {
	return func(m *Manager) { m.perms = perms }
}

func WithSessionTTL(d time.Duration) Option {
	return func(m *Manager) { m.sessionTTL = d }
}

func WithIdleTimeout(d time.Duration) Option {
	return func(m *Manager) { m.idleTimeout = d }
}

func WithLogger(log *slog.Logger) Option {
	return func(m *Manager) { m.log = log }
}

func NewManager(opts ...Option) *Manager {
	m := &Manager{
		transportRelays: []string{DefaultTransportRelay},
		paths:           login.DefaultPaths(),
		perms:           defaultPerms,
		sessionTTL:      defaultSessionTTL,
		idleTimeout:     defaultIdleTimeout,
		log:             slog.Default(),
		pendings:        make(map[string]*pendingLogin),
		sessions:        make(map[string]*Session),
		done:            make(chan struct{}),
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.pool == nil {
		m.pool = nostr.NewSimplePool(context.Background())
		m.ownsPool = true
	}
	go m.janitor()
	return m
}

// Status reports the state of a pending login. The second return is false
// when the id is unknown, expired, or already consumed.
func (m *Manager) Status(id string) (Status, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pendings[id]
	if !ok {
		return Status{}, false
	}
	return Status{
		State:   p.state,
		AuthURL: p.authURL,
		Npub:    p.userNpub,
		Reason:  p.failure,
		URI:     p.uri,
	}, true
}

// Bind consumes a ready pending login and installs its bunker client as a
// live session under sid. Pending ids are single-use.
func (m *Manager) Bind(id, sid string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pendings[id]
	if !ok {
		return "", ErrUnknownPending
	}
	if p.state != StateReady {
		return "", ErrNotReady
	}
	delete(m.pendings, id)
	now := time.Now()
	m.sessions[sid] = &Session{
		signer:    keyer.NewBunkerSignerFromBunkerClient(p.client).WithTimeout(signRequestTimeout),
		cancel:    p.cancel,
		userNpub:  p.userNpub,
		userHex:   p.userHex,
		lastUsed:  now,
		expiresAt: now.Add(m.sessionTTL),
	}
	return p.userNpub, nil
}

// Sign routes an event through the live session for sid. ErrNoSession maps
// to 410 at the HTTP layer so the browser facade can prompt a re-login.
func (m *Manager) Sign(ctx context.Context, sid string, evt *nostr.Event) error {
	m.mu.Lock()
	s, ok := m.sessions[sid]
	if ok {
		s.touch()
	}
	m.mu.Unlock()
	if !ok {
		return ErrNoSession
	}
	return s.SignEvent(ctx, evt)
}

// Session returns the live session for sid, if any.
func (m *Manager) Session(sid string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sid]
	return s, ok
}

// Discard ends a pending login without binding it, which is how a site
// refuses an npub the signer has already confirmed. Unknown ids are a no-op.
func (m *Manager) Discard(id string) {
	m.mu.Lock()
	p, ok := m.pendings[id]
	delete(m.pendings, id)
	m.mu.Unlock()
	if ok {
		p.cancel()
	}
}

// Teardown ends the session for sid. Unknown sids are a no-op.
func (m *Manager) Teardown(sid string) {
	m.mu.Lock()
	s, ok := m.sessions[sid]
	delete(m.sessions, sid)
	m.mu.Unlock()
	if ok {
		s.Close()
	}
}

// Close stops the janitor, tears down every pending login and live session,
// and, when the manager owns its pool, closes the pool's relays. The fork's
// Relay.Close is not idempotent; per-relay failures are logged, never fatal.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.done)
	})

	m.mu.Lock()
	pendings := m.pendings
	sessions := m.sessions
	m.pendings = make(map[string]*pendingLogin)
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, p := range pendings {
		p.cancel()
	}
	for _, s := range sessions {
		s.Close()
	}

	if m.ownsPool {
		m.pool.Close("bunker manager closed")
		m.pool.Relays.Range(func(addr string, relay *nostr.Relay) bool {
			if err := relay.Close(); err != nil {
				m.log.Debug("closing bunker relay", "relay", addr, "error", err)
			}
			return true
		})
	}
}

func (m *Manager) janitor() {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.sweep()
		}
	}
}

func (m *Manager) sweep() {
	now := time.Now()

	m.mu.Lock()
	var expiredPendings []*pendingLogin
	for id, p := range m.pendings {
		if now.After(p.expiresAt) {
			delete(m.pendings, id)
			expiredPendings = append(expiredPendings, p)
		}
	}
	var expiredSessions []*Session
	for sid, s := range m.sessions {
		if s.expired(now, m.idleTimeout) {
			delete(m.sessions, sid)
			expiredSessions = append(expiredSessions, s)
		}
	}
	m.mu.Unlock()

	for _, p := range expiredPendings {
		p.cancel()
	}
	for _, s := range expiredSessions {
		s.Close()
	}
}

func (m *Manager) setAuthURL(id, authURL string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pendings[id]
	if !ok {
		return
	}
	if p.state == StateWaiting {
		p.state = StateAuthRequired
		p.authURL = authURL
	}
}

func (m *Manager) failPending(id string, cause error) {
	m.mu.Lock()
	p, ok := m.pendings[id]
	if ok {
		p.state = StateFailed
		p.failure = cause.Error()
	}
	m.mu.Unlock()
	if ok {
		p.cancel()
		m.log.Debug("bunker login failed", "error", cause)
	}
}

func (m *Manager) markReady(id, userHex, userNpub string, client *nip46.BunkerClient) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pendings[id]
	if !ok {
		return fmt.Errorf("pending login expired before completion")
	}
	p.state = StateReady
	p.authURL = ""
	p.userHex = userHex
	p.userNpub = userNpub
	p.client = client
	return nil
}
