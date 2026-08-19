package nostr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// countDeadline bounds one relay's COUNT round trip. Without a caller
// deadline go-nostr imposes its own 7 seconds; this tighter bound keeps a
// dead relay from stalling a whole enrichment pass behind one query.
const countDeadline = 5 * time.Second

// countFunc counts filter on one relay. poolCount is the production
// implementation; tests script their own.
type countFunc func(ctx context.Context, relayURL string, filter nostr.Filter) (int64, error)

// CountEvents asks relays in order for a NIP-45 count of filter and
// returns the first success. Each relay answers authoritatively for its
// own store, so no cross-relay merge is attempted (the HLL merge path is
// undefined for tag-less filters and unused here). If every relay fails,
// the result is a *RelayReadError per relay, joined.
func CountEvents(ctx context.Context, pool *nostr.SimplePool, relays []string, filter nostr.Filter, log *slog.Logger) (int64, error) {
	return countEvents(ctx, poolCount(pool), relays, filter, log)
}

func countEvents(ctx context.Context, count countFunc, relays []string, filter nostr.Filter, log *slog.Logger) (int64, error) {
	if len(relays) == 0 {
		return 0, errors.New("no relays to count against")
	}
	var errs []error
	for _, relayURL := range relays {
		n, err := count(ctx, relayURL, filter)
		if err != nil {
			log.Warn("relay count failed", "relay", relayURL, "error", err)
			errs = append(errs, err)
			continue
		}
		return n, nil
	}
	return 0, errors.Join(errs...)
}

func poolCount(pool relayEnsurer) countFunc {
	return func(ctx context.Context, relayURL string, filter nostr.Filter) (int64, error) {
		relay, err := pool.EnsureRelay(relayURL)
		if err != nil {
			return 0, &RelayReadError{Relay: relayURL, Err: fmt.Errorf("ensuring relay: %w", err)}
		}
		countCtx, cancel := context.WithTimeout(ctx, countDeadline)
		defer cancel()
		n, _, err := relay.Count(countCtx, nostr.Filters{filter.Clone()})
		if err != nil {
			return 0, &RelayReadError{Relay: relayURL, Err: fmt.Errorf("counting: %w", err)}
		}
		return n, nil
	}
}
