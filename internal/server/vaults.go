package server

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/auth/session"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	vaultsrc "github.com/Buildtall-Systems/stigmergic.dev/internal/source/vault"
)

// ownerRequest is one npub waiting for discovery, with the signer that acts
// as that npub when a relay challenges the read, or nil for a configured
// owner read without one.
type ownerRequest struct {
	signer nostr.Signer
	npub   string
}

// latestSigner acts as one npub through whichever signer that npub's most
// recent request carried. A reader's signer is bound to the login it came
// from: a browser session answers through its open page, a bunker through
// its connection. A relay may challenge long after the request that queued
// the read, so the loader holds this rather than the signer of the moment,
// and each request brings it up to date. A mutex rather than an atomic
// value guards it, because the signers it holds are of different types.
type latestSigner struct {
	signer nostr.Signer
	mu     sync.Mutex
}

func (l *latestSigner) set(signer nostr.Signer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.signer = signer
}

func (l *latestSigner) current() nostr.Signer {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.signer
}

// errNoSigner is what latestSigner answers before any request gave it one.
var errNoSigner = errors.New("no signer for this npub")

func (l *latestSigner) GetPublicKey(ctx context.Context) (string, error) {
	signer := l.current()
	if signer == nil {
		return "", errNoSigner
	}
	return signer.GetPublicKey(ctx)
}

func (l *latestSigner) SignEvent(ctx context.Context, evt *nostr.Event) error {
	signer := l.current()
	if signer == nil {
		return errNoSigner
	}
	return signer.SignEvent(ctx, evt)
}

// mountOwners is the one goroutine that turns npubs into mounted vaults:
// the configured owners first, then whoever signs in. Discovery and fetch
// both talk to relays, so they run here rather than on the request path,
// and running them one at a time keeps a burst of sign-ins from opening a
// relay conversation per request.
func (s *Server) mountOwners() {
	defer s.wg.Done()

	for _, owner := range s.config.Vault.Npubs {
		if s.ctx.Err() != nil {
			return
		}
		s.mountOwner(ownerRequest{npub: owner})
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case req := <-s.owners:
			s.mountOwner(req)
		}
	}
}

// mountOwner discovers one owner's vaults and mounts each of them. A failure
// is logged and dropped: a relay that will not answer costs the reader the
// vaults it holds, never the local tree they came to read.
//
// A vault read with the owner's signer is private to that owner, unless it
// was already mounted from an earlier read without one. A relay that
// challenged and got no answer still leaves every vault the other relays
// gave; when a signer was offered, the owner is forgotten so that a later
// request, perhaps with a page now open to answer, tries again.
func (s *Server) mountOwner(req ownerRequest) {
	vaults, err := s.loadVaults(s.ctx, req.npub, req.signer)
	switch {
	case errors.Is(err, ErrAuthRequired):
		logger.Log.Info("a vault relay withheld events behind authentication", "npub", req.npub, "signer", req.signer != nil)
		if req.signer != nil {
			s.forget(req.npub)
		}
	case err != nil:
		logger.Log.Error("vault discovery failed", "npub", req.npub, "error", err)
		return
	}

	var mounts []*mount
	for _, v := range vaults {
		if !routable(v.Owner, v.Name) {
			logger.Log.Warn("skipping vault whose name will not form a route", "npub", v.Owner, "vault", v.Name)
			continue
		}
		src, srcErr := vaultsrc.NewSource(s.ctx, v, http.DefaultClient)
		if srcErr != nil {
			logger.Log.Error("failed to open vault", "npub", v.Owner, "vault", v.Name, "error", srcErr)
			continue
		}
		m := newVaultMount(v, src)
		if req.signer != nil {
			m.owner = req.npub
		}
		mounts = append(mounts, m)
	}

	s.addMounts(mounts)
}

// addMounts brings new sources into the corpus: each is scanned once, the
// indexes are rebuilt over everything mounted, and clients are told the
// corpus changed shape. A prefix already mounted is skipped, so an owner
// offered twice costs one fetch and no duplicate tree.
func (s *Server) addMounts(mounts []*mount) {
	s.treeMux.Lock()
	fresh := make([]*mount, 0, len(mounts))
	for _, m := range mounts {
		if slices.ContainsFunc(s.mounts, func(e *mount) bool { return e.prefix == m.prefix }) {
			continue
		}
		s.mounts = append(s.mounts, m)
		fresh = append(fresh, m)
	}
	s.treeMux.Unlock()

	if len(fresh) == 0 {
		return
	}

	for _, m := range fresh {
		s.scanMount(m)
		logger.Log.Info("vault mounted", "vault", m.src.Name(), "route", m.prefix, "private", m.owner != "")
	}

	s.rebuildIndexes()
	s.broadcastReload(true)
}

// observeSession offers the signed-in reader's own npub to the loader, once,
// with a signer acting as that reader. Configuration names whose public
// vaults to watch; a reader who signed in has named themselves, and their
// own vaults, private ones included, are the ones they most expect to find.
// Every request refreshes the signer the loader holds for the reader.
func (s *Server) observeSession(r *http.Request) {
	if s.loadVaults == nil || s.site == nil {
		return
	}

	npub := session.PubkeyFromContext(r.Context())
	if npub == "" {
		return
	}
	if _, err := btknostr.NpubToHex(npub); err != nil {
		logger.Log.Warn("session carries an invalid npub", "error", err)
		return
	}

	signer, ok := s.site.Signer(r)
	if !ok {
		return
	}

	s.observe(npub, signer)
}

// observe brings the npub's signer up to date and queues the npub for
// discovery unless it is already queued. The npub is recorded only once it
// is on the queue, so a full queue leaves it unrecorded and the reader's
// next request offers it again.
func (s *Server) observe(npub string, signer nostr.Signer) {
	s.observedMux.Lock()
	defer s.observedMux.Unlock()

	holder, ok := s.signers[npub]
	if !ok {
		holder = &latestSigner{}
		s.signers[npub] = holder
	}
	holder.set(signer)

	if s.observed[npub] {
		return
	}

	select {
	case s.owners <- ownerRequest{npub: npub, signer: holder}:
		s.observed[npub] = true
		logger.Log.Info("observed vault owner", "npub", npub)
	default:
		logger.Log.Warn("vault discovery queue is full, deferring owner", "npub", npub)
	}
}

// forget drops the record that an npub was queued, so its next request
// queues it again.
func (s *Server) forget(npub string) {
	s.observedMux.Lock()
	defer s.observedMux.Unlock()
	delete(s.observed, npub)
}
