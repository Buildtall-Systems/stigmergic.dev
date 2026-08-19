package lists

import (
	"context"

	"github.com/nbd-wtf/go-nostr"
)

// eventFetcher is the narrow slice of *nostr.SimplePool that PoolQuerier
// drives: one multi-relay read that ends at EOSE. Lifting it off the concrete
// type is what lets the adapter be exercised without a socket, since a
// *nostr.SimplePool cannot be pointed at a fake relay from a test.
type eventFetcher interface {
	FetchMany(ctx context.Context, urls []string, filter nostr.Filter, opts ...nostr.SubscriptionOption) chan nostr.RelayEvent
}

var _ eventFetcher = (*nostr.SimplePool)(nil)

// PoolQuerier adapts a SimplePool to CoordQuerier, so coordinate resolution
// runs over connections the caller already holds open, including a pool the
// caller authenticated. Every service that resolves foreign coordinates from a
// pool needs this same twelve lines; this is the one copy.
type PoolQuerier struct {
	fetcher eventFetcher
}

// NewPoolQuerier is the only way to build a usable PoolQuerier: the zero value
// has no pool to read from.
func NewPoolQuerier(pool *nostr.SimplePool) PoolQuerier {
	return PoolQuerier{fetcher: pool}
}

// QueryBlocking drains one fetch to EOSE. The error is always nil because the
// pool discards the reason a subscription ended: an unreachable relay and an
// empty relay are the same silence at this layer. Callers that must tell those
// apart read through btk/nostr.FetchUntilEOSE instead.
func (q PoolQuerier) QueryBlocking(ctx context.Context, filter nostr.Filter, relays []string) ([]*nostr.Event, error) {
	var events []*nostr.Event
	for ev := range q.fetcher.FetchMany(ctx, relays, filter) {
		events = append(events, ev.Event)
	}
	return events, nil
}
