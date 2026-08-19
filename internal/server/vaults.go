package server

import (
	"net/http"
	"slices"

	"github.com/buildtall-systems/buildtall/btk/auth/session"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	vaultsrc "github.com/Buildtall-Systems/stigmergic.dev/internal/source/vault"
)

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
		s.mountOwner(owner)
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case owner := <-s.owners:
			s.mountOwner(owner)
		}
	}
}

// mountOwner discovers one owner's vaults and mounts each of them. A failure
// is logged and dropped: a relay that will not answer costs the reader the
// vaults it holds, never the local tree they came to read.
func (s *Server) mountOwner(owner string) {
	vaults, err := s.loadVaults(s.ctx, owner)
	if err != nil {
		logger.Log.Error("vault discovery failed", "npub", owner, "error", err)
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
		mounts = append(mounts, newVaultMount(v, src))
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
		logger.Log.Info("vault mounted", "vault", m.src.Name(), "route", m.prefix)
	}

	s.rebuildIndexes()
	s.broadcastReload(true)
}

// observeSession offers the signed-in reader's own npub to the loader, once.
// Configuration names whose vaults to watch; a reader who signed in has
// named themselves, and their own vault is the one they most expect to
// find. With auth off no npub is observed, because no request carries one.
func (s *Server) observeSession(r *http.Request) {
	if s.loadVaults == nil || !s.config.Auth.Enabled {
		return
	}

	pubkey := session.PubkeyFromContext(r.Context())
	if pubkey == "" {
		return
	}

	npub, err := btknostr.HexToNpub(pubkey)
	if err != nil {
		logger.Log.Warn("session pubkey will not encode as an npub", "error", err)
		return
	}

	s.observe(npub)
}

// observe queues one owner for discovery unless it is already queued. The
// npub is recorded only once it is on the queue, so a full queue leaves it
// unrecorded and the reader's next request offers it again.
func (s *Server) observe(npub string) {
	s.observedMux.Lock()
	defer s.observedMux.Unlock()

	if s.observed[npub] {
		return
	}

	select {
	case s.owners <- npub:
		s.observed[npub] = true
		logger.Log.Info("observed vault owner", "npub", npub)
	default:
		logger.Log.Warn("vault discovery queue is full, deferring owner", "npub", npub)
	}
}
