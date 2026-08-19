package nostr

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/retry"
)

func NewPool(ctx context.Context) *nostr.SimplePool {
	return nostr.NewSimplePool(ctx)
}

func NewPoolWithAuth(ctx context.Context, authHandler nostr.WithAuthHandler) *nostr.SimplePool {
	return nostr.NewSimplePool(ctx, authHandler)
}

// NewPoolWithProactiveAuth authenticates every relay at connect time, keeping
// the same handler as the reactive fallback. Required when the pool must WRITE
// to a relay that gates only writes: such relays never send CLOSED
// auth-required on reads, so reactive auth never fires and publishes are
// rejected. Proactive auth stalls EnsureRelay briefly on relays that never
// send AUTH, so pools that only read should keep NewPoolWithAuth.
func NewPoolWithProactiveAuth(ctx context.Context, authHandler nostr.WithAuthHandler) *nostr.SimplePool {
	return nostr.NewSimplePool(ctx, nostr.WithProactiveAuth(authHandler))
}

// NsecAuthHandler returns a NIP-42 auth handler that signs each relay auth challenge
// with the key derived from nsec, logging the outcome. It composes with NewPoolWithAuth
// (or NewSimplePool) as a PoolOption, replacing the hand-rolled nsec→hex→sign closure
// every buildtall NIP-42 service otherwise duplicates.
func NsecAuthHandler(nsec string, log *slog.Logger) nostr.WithAuthHandler {
	return func(_ context.Context, authEvent nostr.RelayEvent) error {
		secHex, err := NsecToHex(nsec)
		if err != nil {
			log.Error("NIP-42 nsec conversion failed", "relay", authEvent.Relay.URL, "error", err)
			return fmt.Errorf("converting nsec: %w", err)
		}
		if err := authEvent.Sign(secHex); err != nil {
			log.Error("NIP-42 auth sign failed", "relay", authEvent.Relay.URL, "error", err)
			return fmt.Errorf("signing auth event: %w", err)
		}
		log.Debug("NIP-42 auth success", "relay", authEvent.Relay.URL)
		return nil
	}
}

const (
	ensureMaxRetries = 5
	ensureBaseDelay  = 1 * time.Second
)

func EnsureRelays(ctx context.Context, pool *nostr.SimplePool, relays []string, log *slog.Logger) error {
	for _, relayURL := range relays {
		url := relayURL
		err := retry.Do(ctx, ensureMaxRetries, ensureBaseDelay, func(_ context.Context) error {
			if _, err := pool.EnsureRelay(url); err != nil {
				log.Warn("relay connection failed, retrying",
					"relay", url,
					"error", err,
				)
				return err
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("ensuring relay %s: %w", url, err)
		}
	}
	return nil
}
