package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/server"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source/vault"
)

// authSignTimeout bounds how long a relay's challenge waits for the reader's
// signer. A browser signer answers only through an open page, and a person
// may have to approve the signature, so the wait is long. It bounds the
// signing alone: the relay's answer to the signed event keeps the pool's own
// timeout.
const authSignTimeout = time.Minute

// errNoAnonymousAuth is what the anonymous pool answers to a challenge.
var errNoAnonymousAuth = errors.New("this read carries no signer")

// unansweredKey carries, in a load's context, the flag a pool's auth handler
// raises when it could not answer a relay's challenge.
type unansweredKey struct{}

// noteUnanswered raises the load's flag, when the context carries one.
func noteUnanswered(ctx context.Context) {
	if flag, ok := ctx.Value(unansweredKey{}).(*atomic.Bool); ok {
		flag.Store(true)
	}
}

// vaultLoader is how the server reaches Nostr: a read pool over the
// configured relays, a discovery query per owner, and a fetch per vault
// found. Configuring no relays yields no loader at all, and the server
// serves the local tree alone, which is what an unconfigured install does.
//
// A read without a signer rides one anonymous pool, which refuses every
// challenge. A read with a signer rides a pool of that owner's own, which
// answers a challenge through the signer, so one reader's authenticated
// connection never carries another reader's query. Each pool is built on
// first use, so a serve that never discovers anything opens no connection,
// and is kept, so later reads ride the same relay conversation.
func vaultLoader(cfg *config.Config) server.VaultLoader {
	relays := cfg.Vault.Relays
	if len(relays) == 0 {
		return nil
	}

	var (
		once      sync.Once
		anonymous *nostr.SimplePool
		mu        sync.Mutex
		authed    = make(map[string]*nostr.SimplePool)
	)

	poolFor := func(ctx context.Context, owner string, signer nostr.Signer) *nostr.SimplePool {
		if signer == nil {
			once.Do(func() {
				anonymous = btknostr.NewPoolWithAuth(ctx, func(ctx context.Context, _ nostr.RelayEvent) error {
					noteUnanswered(ctx)
					return errNoAnonymousAuth
				})
			})
			return anonymous
		}

		mu.Lock()
		defer mu.Unlock()
		if pool, ok := authed[owner]; ok {
			return pool
		}
		answer := btknostr.SignerAuthHandler(signer, logger.Log)
		pool := btknostr.NewPoolWithAuth(ctx, func(ctx context.Context, authEvent nostr.RelayEvent) error {
			signCtx, cancel := context.WithTimeout(ctx, authSignTimeout)
			defer cancel()
			if err := answer(signCtx, authEvent); err != nil {
				noteUnanswered(ctx)
				return err
			}
			return nil
		})
		authed[owner] = pool
		return pool
	}

	return func(ctx context.Context, owner string, signer nostr.Signer) ([]*vault.Vault, error) {
		pool := poolFor(ctx, owner, signer)

		unanswered := &atomic.Bool{}
		ctx = context.WithValue(ctx, unansweredKey{}, unanswered)

		found, err := vault.Discover(ctx, pool, relays, []string{owner})
		if err != nil {
			return nil, err
		}

		vaults := make([]*vault.Vault, 0, len(found))
		for _, d := range found {
			v, loadErr := vault.Load(ctx, pool, relays, d, logger.Log)
			if loadErr != nil {
				// One unreadable vault is not the others' problem: an owner
				// publishing three vaults and one broken root event still
				// reads the three.
				logger.Log.Error("failed to load vault", "npub", d.Owner, "vault", d.Name, "error", loadErr)
				continue
			}
			vaults = append(vaults, v)
		}

		logger.Log.Info("vault discovery complete", "npub", owner, "found", len(found), "loaded", len(vaults), "signer", signer != nil)
		if unanswered.Load() {
			return vaults, server.ErrAuthRequired
		}
		return vaults, nil
	}
}
