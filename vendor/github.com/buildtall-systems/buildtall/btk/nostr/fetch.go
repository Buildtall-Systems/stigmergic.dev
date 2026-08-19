package nostr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/nbd-wtf/go-nostr"
)

// relayEnsurer is the narrow slice of *nostr.SimplePool the fetch helper uses.
// The pool is here for connection reuse only: every one of its read methods
// discards the reason a subscription ended, which is the whole failure this
// helper exists to fix.
type relayEnsurer interface {
	EnsureRelay(url string) (*nostr.Relay, error)
}

var _ relayEnsurer = (*nostr.SimplePool)(nil)

// relaySubscription is the channel surface of a live *nostr.Subscription,
// lifted off the concrete type so the drain loop can be driven without a
// socket. A *nostr.Subscription cannot be constructed by a test: its cancel
// func and relay back-reference are unexported.
type relaySubscription struct {
	events <-chan *nostr.Event
	eose   <-chan struct{}
	closed <-chan string
	ctx    context.Context
	unsub  func()
}

// subscribeFunc opens one subscription. poolSubscribe is the production
// implementation; tests script their own.
type subscribeFunc func(ctx context.Context, relayURL string, filter nostr.Filter) (relaySubscription, error)

// FetchUntilEOSE reads filter from every relay, returning once each has sent
// EOSE. Its contract, unlike every pool read method, distinguishes the three
// outcomes a caller must tell apart:
//
//   - events, nil: those events are the complete stored set.
//   - nil, nil: confirmed empty. Every relay answered and held nothing.
//   - events, err: at least one relay failed. err is a *RelayReadError per
//     failed relay, joined; events are only from relays that reached EOSE.
//
// A relay that fails before EOSE contributes no events, so a caller that
// ignores err renders a partial answer as if it were whole.
//
// The helper does not retry; callers own retry policy (see btk/retry). It
// imposes no liveness pre-check either, because Relay.IsConnected reports
// context liveness rather than socket health and lags a dead socket by up to
// two minutes. Failure detection is the subscription error plus the caller's
// context deadline.
func FetchUntilEOSE(ctx context.Context, pool *nostr.SimplePool, relays []string, filter nostr.Filter, log *slog.Logger) ([]*nostr.Event, error) {
	return fetchUntilEOSE(ctx, poolSubscribe(pool), relays, filter, log)
}

func fetchUntilEOSE(ctx context.Context, subscribe subscribeFunc, relays []string, filter nostr.Filter, log *slog.Logger) ([]*nostr.Event, error) {
	type outcome struct {
		err    error
		relay  string
		events []*nostr.Event
	}

	outcomes := make([]outcome, len(relays))
	var wg sync.WaitGroup
	for i, relayURL := range relays {
		wg.Go(func() {
			events, err := fetchFromRelay(ctx, subscribe, relayURL, filter)
			outcomes[i] = outcome{err: err, relay: relayURL, events: events}
		})
	}
	wg.Wait()

	var (
		events []*nostr.Event
		errs   []error
		seen   = make(map[string]struct{})
	)
	for _, o := range outcomes {
		if o.err != nil {
			log.Warn("relay read failed", "relay", o.relay, "error", o.err)
			errs = append(errs, o.err)
			continue
		}
		for _, event := range o.events {
			if _, duplicate := seen[event.ID]; duplicate {
				continue
			}
			seen[event.ID] = struct{}{}
			events = append(events, event)
		}
	}
	if len(errs) > 0 {
		return events, errors.Join(errs...)
	}
	return events, nil
}

func fetchFromRelay(ctx context.Context, subscribe subscribeFunc, relayURL string, filter nostr.Filter) ([]*nostr.Event, error) {
	// The pool rewrites caller filter slices when it reconnects a relay, so no
	// filter reaches a subscription without being copied first.
	sub, err := subscribe(ctx, relayURL, filter.Clone())
	if err != nil {
		return nil, &RelayReadError{Relay: relayURL, Err: err}
	}
	defer sub.unsub()

	return drainUntilEOSE(ctx, relayURL, sub)
}

func drainUntilEOSE(ctx context.Context, relayURL string, sub relaySubscription) ([]*nostr.Event, error) {
	var events []*nostr.Event
	for {
		select {
		case event, ok := <-sub.events:
			// The events channel is closed only when the subscription context is
			// cancelled, which means the subscription died before EOSE.
			if !ok {
				return nil, subscriptionDied(relayURL, sub)
			}
			events = append(events, event)
		case <-sub.eose:
			// The relay guarantees every stored event is delivered before it
			// signals EOSE, so events is complete here.
			return events, nil
		case reason := <-sub.closed:
			return nil, &RelayReadError{
				Relay:  relayURL,
				Reason: reason,
				Err:    fmt.Errorf("relay closed subscription: %s", reason),
			}
		case <-sub.ctx.Done():
			return nil, subscriptionDied(relayURL, sub)
		case <-ctx.Done():
			return nil, &RelayReadError{
				Relay: relayURL,
				Err:   fmt.Errorf("read did not reach EOSE: %w", context.Cause(ctx)),
			}
		}
	}
}

// subscriptionDied builds the error for a subscription that ended before EOSE.
// A CLOSED reason, when there was one, is already buffered on the channel by
// the time the subscription context is cancelled, so this drain always finds it
// and never blocks.
func subscriptionDied(relayURL string, sub relaySubscription) error {
	reason := ""
	select {
	case reason = <-sub.closed:
	default:
	}

	cause := context.Cause(sub.ctx)
	if cause == nil {
		cause = errors.New("subscription ended before EOSE")
	}
	return &RelayReadError{Relay: relayURL, Reason: reason, Err: cause}
}

func poolSubscribe(pool relayEnsurer) subscribeFunc {
	return func(ctx context.Context, relayURL string, filter nostr.Filter) (relaySubscription, error) {
		relay, err := pool.EnsureRelay(relayURL)
		if err != nil {
			return relaySubscription{}, fmt.Errorf("ensuring relay: %w", err)
		}

		sub, err := relay.Subscribe(ctx, nostr.Filters{filter})
		if err != nil {
			return relaySubscription{}, fmt.Errorf("subscribing: %w", err)
		}

		return relaySubscription{
			events: sub.Events,
			eose:   sub.EndOfStoredEvents,
			closed: sub.ClosedReason,
			ctx:    sub.Context,
			unsub:  sub.Unsub,
		}, nil
	}
}
