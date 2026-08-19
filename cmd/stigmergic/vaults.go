package main

import (
	"context"
	"sync"

	"github.com/nbd-wtf/go-nostr"

	btkvault "github.com/buildtall-systems/buildtall/btk/vault"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/server"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source/vault"
)

// vaultLoader is how the server reaches Nostr: one anonymous read pool over
// the configured relays, a discovery query per owner, and a fetch per vault
// found. Configuring no relays yields no loader at all, and the server
// serves the local tree alone, which is what an unconfigured install does.
//
// The pool is built on the first call rather than here, so a serve that
// never discovers anything opens no connection, and it is built once, so
// every owner rides the same relay conversation.
func vaultLoader(cfg *config.Config) server.VaultLoader {
	relays := cfg.Vault.Relays
	if len(relays) == 0 {
		return nil
	}

	var (
		once sync.Once
		pool *nostr.SimplePool
	)

	return func(ctx context.Context, owner string) ([]*vault.Vault, error) {
		once.Do(func() {
			pool = btkvault.NewReadPool(ctx, "", logger.Log)
		})

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

		logger.Log.Info("vault discovery complete", "npub", owner, "found", len(found), "loaded", len(vaults))
		return vaults, nil
	}
}
