package nostr

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/keyer"

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
// send AUTH, so pools that only read should keep NewPoolWithAuth. Further
// options apply after the auth option, so a caller can give every relay a
// hardened HTTP client or request headers through nostr.WithRelayOptions.
func NewPoolWithProactiveAuth(ctx context.Context, authHandler nostr.WithAuthHandler, opts ...nostr.PoolOption) *nostr.SimplePool {
	return nostr.NewSimplePool(ctx, append([]nostr.PoolOption{nostr.WithProactiveAuth(authHandler)}, opts...)...)
}

// SignerAuthHandler returns a NIP-42 auth handler that signs each relay
// auth challenge through signer, logging the outcome. It composes with
// NewPoolWithAuth (or NewSimplePool) as a PoolOption, so a local key and a
// remote bunker answer a relay the same way.
func SignerAuthHandler(signer nostr.Signer, log *slog.Logger) nostr.WithAuthHandler {
	return func(ctx context.Context, authEvent nostr.RelayEvent) error {
		if err := signer.SignEvent(ctx, authEvent.Event); err != nil {
			log.Error("NIP-42 auth sign failed", "relay", authEvent.Relay.URL, "error", err)
			return fmt.Errorf("signing auth event: %w", err)
		}
		log.Debug("NIP-42 auth success", "relay", authEvent.Relay.URL)
		return nil
	}
}

// NsecAuthHandler is SignerAuthHandler for a service that holds its key as
// an nsec. The key is decoded at challenge time, as it always was, so a
// malformed nsec fails the auth rather than the pool's construction.
func NsecAuthHandler(nsec string, log *slog.Logger) nostr.WithAuthHandler {
	return func(ctx context.Context, authEvent nostr.RelayEvent) error {
		secHex, err := NsecToHex(nsec)
		if err != nil {
			log.Error("NIP-42 nsec conversion failed", "relay", authEvent.Relay.URL, "error", err)
			return fmt.Errorf("converting nsec: %w", err)
		}
		signer, err := keyer.NewPlainKeySigner(secHex)
		if err != nil {
			log.Error("NIP-42 signer construction failed", "relay", authEvent.Relay.URL, "error", err)
			return fmt.Errorf("constructing signer: %w", err)
		}
		return SignerAuthHandler(signer, log)(ctx, authEvent)
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
