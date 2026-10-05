package site

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/auth/session"
	btkhttp "github.com/buildtall-systems/buildtall/btk/http"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

const (
	// bridgeSignEvent names the SSE event that carries one unsigned event to
	// the page.
	bridgeSignEvent = "sign"
	// bridgeHeartbeat keeps an idle bridge stream alive through proxies.
	bridgeHeartbeat = 25 * time.Second
)

var (
	errBridgeKind   = fmt.Errorf("site: the sign bridge signs only kind %d", nostr.KindClientAuthentication)
	errBridgeClosed = errors.New("site: the sign bridge is closed")
)

// bridge carries signing requests for extension-backed sessions to the
// visitor's open pages and their answers back. The extension lives only in
// the browser, so a relay AUTH that a service makes as the visitor waits for
// a page to sign it.
type bridge struct {
	log       *slog.Logger
	done      chan struct{}
	sids      map[string]*bridgeSession
	heartbeat time.Duration
	mu        sync.Mutex
	closed    bool
}

// bridgeSession is the bridge state of one session id: its open page
// streams and its requests that wait for a signature.
type bridgeSession struct {
	streams map[chan struct{}]struct{}
	pending map[string]*bridgeRequest
	order   []string
}

// bridgeRequest is one unsigned event and every signer that waits on it.
// Two signers that ask for the same event share one request.
type bridgeRequest struct {
	done    chan struct{}
	event   nostr.Event
	sig     string
	waiters int
}

func newBridge(log *slog.Logger) *bridge {
	return &bridge{
		log:       log,
		done:      make(chan struct{}),
		sids:      make(map[string]*bridgeSession),
		heartbeat: bridgeHeartbeat,
	}
}

// close ends every stream and fails every waiting signer.
func (b *bridge) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	close(b.done)
}

// sessionLocked returns the state of sid, created on first use. The caller
// holds b.mu.
func (b *bridge) sessionLocked(sid string) *bridgeSession {
	bs, ok := b.sids[sid]
	if !ok {
		bs = &bridgeSession{
			streams: make(map[chan struct{}]struct{}),
			pending: make(map[string]*bridgeRequest),
		}
		b.sids[sid] = bs
	}
	return bs
}

// forgetIfIdleLocked removes the state of sid once no stream and no request
// remain. The caller holds b.mu.
func (b *bridge) forgetIfIdleLocked(sid string) {
	bs, ok := b.sids[sid]
	if ok && len(bs.streams) == 0 && len(bs.pending) == 0 {
		delete(b.sids, sid)
	}
}

// enqueue adds evt, whose id is set, to the requests of sid and wakes every
// open stream of sid. It returns the request to wait on.
func (b *bridge) enqueue(sid string, evt nostr.Event) (*bridgeRequest, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errBridgeClosed
	}
	bs := b.sessionLocked(sid)
	req, ok := bs.pending[evt.ID]
	if !ok {
		req = &bridgeRequest{event: evt, done: make(chan struct{})}
		bs.pending[evt.ID] = req
		bs.order = append(bs.order, evt.ID)
		for wake := range bs.streams {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
	}
	req.waiters++
	return req, nil
}

// release drops one waiter of the request for id. The last waiter removes
// an unanswered request, so a page never signs an event nobody waits for.
func (b *bridge) release(sid, id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	bs, ok := b.sids[sid]
	if !ok {
		return
	}
	req, ok := bs.pending[id]
	if !ok {
		return
	}
	req.waiters--
	if req.waiters <= 0 {
		bs.removeLocked(id)
		b.forgetIfIdleLocked(sid)
	}
}

func (bs *bridgeSession) removeLocked(id string) {
	delete(bs.pending, id)
	for i, pending := range bs.order {
		if pending == id {
			bs.order = append(bs.order[:i], bs.order[i+1:]...)
			return
		}
	}
}

// unsent returns, in queue order, the requests of sid that the stream has
// not yet sent, and marks them sent.
func (b *bridge) unsent(sid string, sent map[string]bool) []nostr.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	bs, ok := b.sids[sid]
	if !ok {
		return nil
	}
	var out []nostr.Event
	for _, id := range bs.order {
		if !sent[id] {
			sent[id] = true
			out = append(out, bs.pending[id].event)
		}
	}
	return out
}

// answer completes the request that evt signs, when evt is a valid
// signature by the session's key over an event that sid waits on.
func (b *bridge) answer(sid, userHex string, evt *nostr.Event) (int, error) {
	if !evt.CheckID() {
		return http.StatusBadRequest, errors.New("event id does not match its content")
	}
	if evt.PubKey != userHex {
		return http.StatusForbidden, errors.New("event is not signed by this session's key")
	}
	if ok, err := evt.CheckSignature(); err != nil || !ok {
		return http.StatusBadRequest, errors.New("invalid signature")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	bs, ok := b.sids[sid]
	if !ok {
		return http.StatusNotFound, errors.New("no pending request")
	}
	req, ok := bs.pending[evt.ID]
	if !ok {
		return http.StatusNotFound, errors.New("no pending request")
	}
	req.sig = evt.Sig
	close(req.done)
	bs.removeLocked(evt.ID)
	b.forgetIfIdleLocked(sid)
	return http.StatusOK, nil
}

// handleRequests streams the session's signing requests to one open page:
// those already queued, then each new one.
func (b *bridge) handleRequests(w http.ResponseWriter, r *http.Request) {
	sid := session.SessionIDFromContext(r.Context())
	if sid == "" || session.PubkeyFromContext(r.Context()) == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	wake := make(chan struct{}, 1)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		http.Error(w, "sign bridge closed", http.StatusServiceUnavailable)
		return
	}
	b.sessionLocked(sid).streams[wake] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		if bs, ok := b.sids[sid]; ok {
			delete(bs.streams, wake)
			b.forgetIfIdleLocked(sid)
		}
		b.mu.Unlock()
	}()

	stream, err := btkhttp.NewStream(w)
	if err != nil {
		b.log.Warn("preparing sign bridge stream", "error", err)
		return
	}

	heartbeat := time.NewTicker(b.heartbeat)
	defer heartbeat.Stop()

	sent := make(map[string]bool)
	for {
		for _, evt := range b.unsent(sid, sent) {
			if err := stream.Send(bridgeSignEvent, evt); err != nil {
				b.log.Debug("client gone during sign bridge send", "error", err)
				return
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-b.done:
			return
		case <-wake:
		case <-heartbeat.C:
			if err := stream.Heartbeat(); err != nil {
				b.log.Debug("client gone during sign bridge heartbeat", "error", err)
				return
			}
		}
	}
}

// handleAnswer takes a page's signed event and wakes the signer that waits
// on it.
func (b *bridge) handleAnswer(w http.ResponseWriter, r *http.Request) {
	sid := session.SessionIDFromContext(r.Context())
	npub := session.PubkeyFromContext(r.Context())
	if sid == "" || npub == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	userHex, err := btknostr.NpubToHex(npub)
	if err != nil {
		http.Error(w, "invalid session npub", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var evt nostr.Event
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		b.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if status, err := b.answer(sid, userHex, &evt); err != nil {
		b.writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *bridge) writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		b.log.Debug("encoding response", "error", err)
	}
}

// bridgeSigner signs relay AUTH as an extension-backed session's user, by
// way of the user's open pages.
type bridgeSigner struct {
	bridge  *bridge
	sid     string
	userHex string
}

var _ nostr.Signer = (*bridgeSigner)(nil)

func (s *bridgeSigner) GetPublicKey(context.Context) (string, error) {
	return s.userHex, nil
}

// SignEvent signs a kind 22242 event and refuses every other kind. It sets
// the author and id, then waits for a page to answer, for the bridge to
// close, or for ctx to end.
func (s *bridgeSigner) SignEvent(ctx context.Context, evt *nostr.Event) error {
	if evt.Kind != nostr.KindClientAuthentication {
		return errBridgeKind
	}
	evt.PubKey = s.userHex
	evt.ID = evt.GetID()

	req, err := s.bridge.enqueue(s.sid, *evt)
	if err != nil {
		return err
	}
	defer s.bridge.release(s.sid, evt.ID)

	select {
	case <-req.done:
		evt.Sig = req.sig
		return nil
	case <-s.bridge.done:
		return errBridgeClosed
	case <-ctx.Done():
		return fmt.Errorf("site: waiting for the page to sign: %w", ctx.Err())
	}
}
