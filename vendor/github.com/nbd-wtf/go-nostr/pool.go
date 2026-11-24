package nostr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nbd-wtf/go-nostr/nip45/hyperloglog"
	"github.com/puzpuzpuz/xsync/v3"
)

const (
	seenAlreadyDropTick = time.Minute
)

// SimplePool manages connections to multiple relays, ensures they are reopened when necessary and not duplicated.
type SimplePool struct {
	Relays  *xsync.MapOf[string, *Relay]
	Context context.Context

	authHandler func(context.Context, RelayEvent) error
	cancel      context.CancelCauseFunc

	eventMiddleware     func(RelayEvent)
	duplicateMiddleware func(relay string, id string)
	queryMiddleware     func(relay string, pubkey string, kind int)

	proactiveAuth bool

	// relayLocks serializes EnsureRelay per relay URL. It belongs to this
	// pool: a pool shared across every pool let one pool's proactive auth,
	// held under the lock, block a signer whose own pool connects to the
	// same relay, until the auth timed out.
	relayLocks namedLocks

	// custom things not often used
	penaltyBoxMu sync.Mutex
	penaltyBox   map[string][2]float64
	relayOptions []RelayOption
}

// DirectedFilter combines a Filter with a specific relay URL.
type DirectedFilter struct {
	Filter
	Relay string
}

// RelayEvent represents an event received from a specific relay.
type RelayEvent struct {
	*Event
	Relay *Relay
}

func (ie RelayEvent) String() string { return fmt.Sprintf("[%s] >> %s", ie.Relay.URL, ie.Event) }

// PoolOption is an interface for options that can be applied to a SimplePool.
type PoolOption interface {
	ApplyPoolOption(*SimplePool)
}

// NewSimplePool creates a new SimplePool with the given context and options.
func NewSimplePool(ctx context.Context, opts ...PoolOption) *SimplePool {
	ctx, cancel := context.WithCancelCause(ctx)

	pool := &SimplePool{
		Relays: xsync.NewMapOf[string, *Relay](),

		Context: ctx,
		cancel:  cancel,
	}

	for _, opt := range opts {
		opt.ApplyPoolOption(pool)
	}

	return pool
}

// WithRelayOptions sets options that will be used on every relay instance created by this pool.
func WithRelayOptions(ropts ...RelayOption) withRelayOptionsOpt {
	return ropts
}

type withRelayOptionsOpt []RelayOption

func (h withRelayOptionsOpt) ApplyPoolOption(pool *SimplePool) {
	pool.relayOptions = h
}

// WithAuthHandler must be a function that signs the auth event when called.
// it will be called whenever any relay in the pool returns a `CLOSED` message
// with the "auth-required:" prefix, only once for each relay
type WithAuthHandler func(ctx context.Context, authEvent RelayEvent) error

func (h WithAuthHandler) ApplyPoolOption(pool *SimplePool) {
	pool.authHandler = h
}

// WithProactiveAuth enables proactive NIP-42 authentication: after EnsureRelay
// connects to a relay, PerformAuth is called automatically. The signing callback
// is the same as WithAuthHandler and also serves as the reactive auth fallback.
type WithProactiveAuth func(ctx context.Context, authEvent RelayEvent) error

func (h WithProactiveAuth) ApplyPoolOption(pool *SimplePool) {
	pool.authHandler = h
	pool.proactiveAuth = true
}

// WithPenaltyBox just sets the penalty box mechanism so relays that fail to connect
// or that disconnect will be ignored for a while and we won't attempt to connect again.
func WithPenaltyBox() withPenaltyBoxOpt { return withPenaltyBoxOpt{} }

type withPenaltyBoxOpt struct{}

func (h withPenaltyBoxOpt) ApplyPoolOption(pool *SimplePool) {
	pool.penaltyBox = make(map[string][2]float64)
	go func() {
		sleep := 30.0
		for {
			time.Sleep(time.Duration(sleep) * time.Second)

			pool.penaltyBoxMu.Lock()
			nextSleep := 300.0
			for url, v := range pool.penaltyBox {
				remainingSeconds := v[1]
				remainingSeconds -= sleep
				if remainingSeconds <= 0 {
					pool.penaltyBox[url] = [2]float64{v[0], 0}
					continue
				} else {
					pool.penaltyBox[url] = [2]float64{v[0], remainingSeconds}
				}

				if remainingSeconds < nextSleep {
					nextSleep = remainingSeconds
				}
			}

			sleep = nextSleep
			pool.penaltyBoxMu.Unlock()
		}
	}()
}

// WithEventMiddleware is a function that will be called with all events received.
type WithEventMiddleware func(RelayEvent)

func (h WithEventMiddleware) ApplyPoolOption(pool *SimplePool) {
	pool.eventMiddleware = h
}

// WithDuplicateMiddleware is a function that will be called with all duplicate ids received.
type WithDuplicateMiddleware func(relay string, id string)

func (h WithDuplicateMiddleware) ApplyPoolOption(pool *SimplePool) {
	pool.duplicateMiddleware = h
}

// WithAuthorKindQueryMiddleware is a function that will be called with every combination of relay+pubkey+kind queried
// in a .SubMany*() call -- when applicable (i.e. when the query contains a pubkey and a kind).
type WithAuthorKindQueryMiddleware func(relay string, pubkey string, kind int)

func (h WithAuthorKindQueryMiddleware) ApplyPoolOption(pool *SimplePool) {
	pool.queryMiddleware = h
}

var (
	_ PoolOption = (WithAuthHandler)(nil)
	_ PoolOption = (WithEventMiddleware)(nil)
	_ PoolOption = WithPenaltyBox()
	_ PoolOption = WithRelayOptions(WithRequestHeader(http.Header{}))
)

// EnsureRelay ensures that a relay connection exists and is active.
// If the relay is not connected, it attempts to connect.
func (pool *SimplePool) EnsureRelay(url string) (*Relay, error) {
	nm := NormalizeURL(url)
	defer pool.relayLocks.lock(nm)()

	relay, ok := pool.Relays.Load(nm)
	if ok && relay == nil {
		if pool.penaltyBox != nil {
			pool.penaltyBoxMu.Lock()
			defer pool.penaltyBoxMu.Unlock()
			v, _ := pool.penaltyBox[nm]
			if v[1] > 0 {
				return nil, fmt.Errorf("in penalty box, %fs remaining", v[1])
			}
		}
	} else if ok && relay.IsConnected() {
		// already connected, unlock and return
		return relay, nil
	}

	// try to connect
	// we use this ctx here so when the pool dies everything dies
	ctx, cancel := context.WithTimeoutCause(
		pool.Context,
		time.Second*15,
		errors.New("connecting to the relay took too long"),
	)
	defer cancel()

	relay = NewRelay(context.Background(), url, pool.relayOptions...)
	if err := relay.Connect(ctx); err != nil {
		if pool.penaltyBox != nil {
			// putting relay in penalty box
			pool.penaltyBoxMu.Lock()
			defer pool.penaltyBoxMu.Unlock()
			v, _ := pool.penaltyBox[nm]
			pool.penaltyBox[nm] = [2]float64{v[0] + 1, 30.0 + math.Pow(2, v[0]+1)}
		}
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	if pool.proactiveAuth && pool.authHandler != nil {
		authCtx, authCancel := context.WithTimeout(pool.Context, 5*time.Second)
		defer authCancel()
		relay.PerformAuth(authCtx, func(event *Event) error {
			return pool.authHandler(authCtx, RelayEvent{Event: event, Relay: relay})
		})
	}

	pool.Relays.Store(nm, relay)
	return relay, nil
}

// PublishResult represents the result of publishing an event to a relay.
type PublishResult struct {
	Error    error
	RelayURL string
	Relay    *Relay
}

// PublishMany publishes an event to multiple relays and returns a channel of results emitted as they're received.
func (pool *SimplePool) PublishMany(ctx context.Context, urls []string, evt Event) chan PublishResult {
	ch := make(chan PublishResult, len(urls))

	wg := sync.WaitGroup{}
	wg.Add(len(urls))
	go func() {
		for _, url := range urls {
			go func() {
				defer wg.Done()

				relay, err := pool.EnsureRelay(url)
				if err != nil {
					ch <- PublishResult{err, url, nil}
					return
				}

				if err := relay.Publish(ctx, evt); err == nil {
					// success with no auth required
					ch <- PublishResult{nil, url, relay}
				} else if strings.HasPrefix(err.Error(), "msg: auth-required:") && pool.authHandler != nil {
					// try to authenticate if we can
					if authErr := relay.Auth(ctx, func(event *Event) error {
						return pool.authHandler(ctx, RelayEvent{Event: event, Relay: relay})
					}); authErr == nil {
						if err := relay.Publish(ctx, evt); err == nil {
							// success after auth
							ch <- PublishResult{nil, url, relay}
						} else {
							// failure after auth
							ch <- PublishResult{err, url, relay}
						}
					} else {
						// failure to auth
						ch <- PublishResult{fmt.Errorf("failed to auth: %w", authErr), url, relay}
					}
				} else {
					// direct failure
					ch <- PublishResult{err, url, relay}
				}
			}()
		}

		wg.Wait()
		close(ch)
	}()

	return ch
}

// SubscribeMany opens a subscription with the given filter to multiple relays
// the subscriptions ends when the context is canceled or when all relays return a CLOSED.
func (pool *SimplePool) SubscribeMany(
	ctx context.Context,
	urls []string,
	filter Filter,
	opts ...SubscriptionOption,
) chan RelayEvent {
	return pool.subMany(ctx, urls, Filters{filter}, nil, opts...)
}

// FetchMany opens a subscription, much like SubscribeMany, but it ends as soon as all Relays
// return an EOSE message.
func (pool *SimplePool) FetchMany(
	ctx context.Context,
	urls []string,
	filter Filter,
	opts ...SubscriptionOption,
) chan RelayEvent {
	return pool.SubManyEose(ctx, urls, Filters{filter}, opts...)
}

// Deprecated: SubMany is deprecated: use SubscribeMany instead.
func (pool *SimplePool) SubMany(
	ctx context.Context,
	urls []string,
	filters Filters,
	opts ...SubscriptionOption,
) chan RelayEvent {
	return pool.subMany(ctx, urls, filters, nil, opts...)
}

// SubscribeManyNotifyEOSE is like SubscribeMany, but takes a channel that is closed when
// all subscriptions have received an EOSE
func (pool *SimplePool) SubscribeManyNotifyEOSE(
	ctx context.Context,
	urls []string,
	filter Filter,
	eoseChan chan struct{},
	opts ...SubscriptionOption,
) chan RelayEvent {
	return pool.subMany(ctx, urls, Filters{filter}, eoseChan, opts...)
}

type ReplaceableKey struct {
	PubKey string
	D      string
}

// FetchManyReplaceable is like FetchMany, but deduplicates replaceable and addressable events and returns
// only the latest for each "d" tag.
func (pool *SimplePool) FetchManyReplaceable(
	ctx context.Context,
	urls []string,
	filter Filter,
	opts ...SubscriptionOption,
) *xsync.MapOf[ReplaceableKey, *Event] {
	ctx, cancel := context.WithCancelCause(ctx)

	results := xsync.NewMapOf[ReplaceableKey, *Event]()

	wg := sync.WaitGroup{}
	wg.Add(len(urls))

	seenAlreadyLatest := xsync.NewMapOf[ReplaceableKey, Timestamp]()
	opts = append(opts, WithCheckDuplicateReplaceable(func(rk ReplaceableKey, ts Timestamp) bool {
		updated := false
		seenAlreadyLatest.Compute(rk, func(latest Timestamp, _ bool) (newValue Timestamp, delete bool) {
			if ts > latest {
				updated = true // we are updating the most recent
				return ts, false
			}
			return latest, false // the one we had was already more recent
		})
		// WithCheckDuplicateReplaceable follows the "true == skip this event" contract
		// (see relay.go). Skip the staler duplicates; let the newest event through.
		return !updated
	}))

	for _, url := range urls {
		go func(nm string) {
			defer wg.Done()

			if mh := pool.queryMiddleware; mh != nil {
				if filter.Kinds != nil && filter.Authors != nil {
					for _, kind := range filter.Kinds {
						for _, author := range filter.Authors {
							mh(nm, author, kind)
						}
					}
				}
			}

			relay, err := pool.EnsureRelay(nm)
			if err != nil {
				debugLogf("error connecting to %s with %v: %s", nm, filter, err)
				return
			}

			pool.readStored(ctx, relay,
				func() (*Subscription, error) { return relay.Subscribe(ctx, Filters{filter}, opts...) },
				func(ie RelayEvent) bool {
					results.Store(ReplaceableKey{ie.PubKey, ie.Tags.GetD()}, ie.Event)
					return true
				},
			)
		}(NormalizeURL(url))
	}

	// this will happen when all subscriptions get an eose (or when they die)
	wg.Wait()
	cancel(errors.New("all subscriptions ended"))

	return results
}

// authRequiredPrefix is the NIP-42 prefix of a CLOSED or OK reason that asks for AUTH.
const authRequiredPrefix = "auth-required:"

// subscriptionEnd says why readSubscription stopped reading a subscription.
type subscriptionEnd int

const (
	// subscriptionDone: the context ended, the stored events ended, or the
	// caller's deliver function stopped the read.
	subscriptionDone subscriptionEnd = iota
	// subscriptionClosedByRelay: the relay sent CLOSED.
	subscriptionClosedByRelay
	// subscriptionBroken: the subscription ended for any other cause, for
	// example a dropped connection.
	subscriptionBroken
)

// readSubscription reads sub until it ends and passes each event to deliver.
// It is the one place where the pool waits on a subscription.
//
// A relay's CLOSED shows on two signals: the reason on sub.ClosedReason and the
// close of sub.Events. A select takes either one, so the closed-Events branch
// asks the subscription for the cause. The CLOSED is reported with its reason
// on both branches.
//
// A nil eose keeps the read going after the stored events end. A nil tick
// turns the periodic onTick call off.
func (pool *SimplePool) readSubscription(
	ctx context.Context,
	relay *Relay,
	sub *Subscription,
	eose <-chan struct{},
	tick <-chan time.Time,
	onTick func(),
	deliver func(RelayEvent) bool,
) (end subscriptionEnd, reason string) {
	for {
		select {
		case <-ctx.Done():
			return subscriptionDone, ""
		case <-eose:
			return subscriptionDone, ""
		case <-tick:
			onTick()
		case reason := <-sub.ClosedReason:
			return subscriptionClosedByRelay, reason
		case evt, more := <-sub.Events:
			if !more {
				if reason, closed := sub.ClosedByRelay(); closed {
					return subscriptionClosedByRelay, reason
				}
				return subscriptionBroken, ""
			}

			ie := RelayEvent{Event: evt, Relay: relay}
			if mh := pool.eventMiddleware; mh != nil {
				mh(ie)
			}

			if !deliver(ie) {
				return subscriptionDone, ""
			}
		}
	}
}

// authOnClosed answers a relay's CLOSED. When the reason asks for AUTH, the
// pool has an auth handler, and this read has not authenticated yet, it
// performs AUTH. It reports whether the caller must subscribe again.
func (pool *SimplePool) authOnClosed(ctx context.Context, relay *Relay, reason string, hasAuthed bool) bool {
	if hasAuthed || pool.authHandler == nil || !strings.HasPrefix(reason, authRequiredPrefix) {
		return false
	}

	err := relay.Auth(ctx, func(event *Event) error {
		return pool.authHandler(ctx, RelayEvent{Event: event, Relay: relay})
	})
	return err == nil
}

// readStored reads one relay's stored events: it subscribes, passes each event
// to deliver, and returns at EOSE. When the relay answers CLOSED with an
// auth-required reason it authenticates once and subscribes again.
func (pool *SimplePool) readStored(
	ctx context.Context,
	relay *Relay,
	subscribe func() (*Subscription, error),
	deliver func(RelayEvent) bool,
) {
	hasAuthed := false // so we don't keep doing AUTH again and again
	for {
		sub, err := subscribe()
		if err != nil {
			debugLogf("error subscribing to %s: %s", relay, err)
			return
		}

		end, reason := pool.readSubscription(ctx, relay, sub, sub.EndOfStoredEvents, nil, nil, deliver)
		if end != subscriptionClosedByRelay {
			return
		}
		if !pool.authOnClosed(ctx, relay, reason, hasAuthed) {
			debugLogf("CLOSED from %s: '%s'\n", relay, reason)
			return
		}
		hasAuthed = true
	}
}

func (pool *SimplePool) subMany(
	ctx context.Context,
	urls []string,
	filters Filters,
	eoseChan chan struct{},
	opts ...SubscriptionOption,
) chan RelayEvent {
	ctx, cancel := context.WithCancelCause(ctx)
	_ = cancel // do this so `go vet` will stop complaining
	events := make(chan RelayEvent)
	seenAlready := xsync.NewMapOf[string, Timestamp]()
	ticker := time.NewTicker(seenAlreadyDropTick)

	eoseWg := sync.WaitGroup{}
	eoseWg.Add(len(urls))
	if eoseChan != nil {
		go func() {
			eoseWg.Wait()
			close(eoseChan)
		}()
	}

	pending := xsync.NewCounter()
	pending.Add(int64(len(urls)))
	for i, url := range urls {
		url = NormalizeURL(url)
		urls[i] = url
		if idx := slices.Index(urls, url); idx != i {
			// skip duplicate relays in the list
			eoseWg.Done()
			continue
		}

		eosed := atomic.Bool{}

		go func(nm string) {
			defer func() {
				pending.Dec()
				if pending.Value() == 0 {
					close(events)
					cancel(fmt.Errorf("aborted: %w", context.Cause(ctx)))
				}
				if eosed.CompareAndSwap(false, true) {
					eoseWg.Done()
				}
			}()

			deliver := func(ie RelayEvent) bool {
				select {
				case events <- ie:
					return true
				case <-ctx.Done():
					return false
				}
			}
			dropOldSeen := func() {
				if eosed.Load() {
					old := Timestamp(time.Now().Add(-seenAlreadyDropTick).Unix())
					for id, value := range seenAlready.Range {
						if value < old {
							seenAlready.Delete(id)
						}
					}
				}
			}

			hasAuthed := false
			interval := 3 * time.Second
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				var sub *Subscription
				var end subscriptionEnd
				var reason string

				if mh := pool.queryMiddleware; mh != nil {
					for _, filter := range filters {
						if filter.Kinds != nil && filter.Authors != nil {
							for _, kind := range filter.Kinds {
								for _, author := range filter.Authors {
									mh(nm, author, kind)
								}
							}
						}
					}
				}

				relay, err := pool.EnsureRelay(nm)
				if err != nil {
					// otherwise (if we were connected and got disconnected) keep trying to reconnect
					debugLogf("%s reconnecting because connection failed\n", nm)
					goto reconnect
				}
				hasAuthed = false

			subscribe:
				sub, err = relay.Subscribe(ctx, filters, append(opts, WithCheckDuplicate(func(id, relay string) bool {
					_, exists := seenAlready.LoadAndStore(id, Timestamp(time.Now().Unix()))
					if exists && pool.duplicateMiddleware != nil {
						pool.duplicateMiddleware(relay, id)
					}
					return exists
				}))...)
				if err != nil {
					debugLogf("%s reconnecting because subscription died\n", nm)
					goto reconnect
				}

				// the subscription is an argument: the variable is assigned again on a resubscription
				go func(sub *Subscription) {
					<-sub.EndOfStoredEvents

					// guard here otherwise a resubscription will trigger a duplicate call to eoseWg.Done()
					if eosed.CompareAndSwap(false, true) {
						eoseWg.Done()
					}
				}(sub)

				// reset interval when we get a good subscription
				interval = 3 * time.Second

				end, reason = pool.readSubscription(ctx, relay, sub, nil, ticker.C, dropOldSeen, deliver)
				switch end {
				case subscriptionClosedByRelay:
					// a refusal, not a dropped connection: the filters stay as they are
					if pool.authOnClosed(ctx, relay, reason, hasAuthed) {
						hasAuthed = true // so we don't keep doing AUTH again and again
						goto subscribe
					}
					debugLogf("CLOSED from %s: '%s'\n", nm, reason)
					return
				case subscriptionBroken:
					// this means the connection was closed for weird reasons, like the server shut down
					// so we will update the filters here to include only events seem from now on
					// and try to reconnect until we succeed
					now := Now()
					for i := range filters {
						filters[i].Since = &now
					}
					debugLogf("%s reconnecting because sub.Events is broken\n", nm)
					goto reconnect
				default:
					return
				}

			reconnect:
				// we will go back to the beginning of the loop and try to connect again and again
				// until the context is canceled
				time.Sleep(interval)
				interval = min(time.Minute*5, interval*17/10) // the next time we try we will wait longer
			}
		}(url)
	}

	return events
}

// Deprecated: SubManyEose is deprecated: use FetchMany instead.
func (pool *SimplePool) SubManyEose(
	ctx context.Context,
	urls []string,
	filters Filters,
	opts ...SubscriptionOption,
) chan RelayEvent {
	seenAlready := xsync.NewMapOf[string, struct{}]()
	return pool.subManyEoseNonOverwriteCheckDuplicate(ctx, urls, filters,
		WithCheckDuplicate(func(id, relay string) bool {
			_, exists := seenAlready.LoadOrStore(id, struct{}{})
			if exists && pool.duplicateMiddleware != nil {
				pool.duplicateMiddleware(relay, id)
			}
			return exists
		}),
		opts...)
}

func (pool *SimplePool) subManyEoseNonOverwriteCheckDuplicate(
	ctx context.Context,
	urls []string,
	filters Filters,
	wcd WithCheckDuplicate,
	opts ...SubscriptionOption,
) chan RelayEvent {
	ctx, cancel := context.WithCancelCause(ctx)

	events := make(chan RelayEvent)
	wg := sync.WaitGroup{}
	wg.Add(len(urls))

	opts = append(opts, wcd)

	go func() {
		// this will happen when all subscriptions get an eose (or when they die)
		wg.Wait()
		cancel(errors.New("all subscriptions ended"))
		close(events)
	}()

	for _, url := range urls {
		go func(nm string) {
			defer wg.Done()

			if mh := pool.queryMiddleware; mh != nil {
				for _, filter := range filters {
					if filter.Kinds != nil && filter.Authors != nil {
						for _, kind := range filter.Kinds {
							for _, author := range filter.Authors {
								mh(nm, author, kind)
							}
						}
					}
				}
			}

			relay, err := pool.EnsureRelay(nm)
			if err != nil {
				debugLogf("error connecting to %s with %v: %s", nm, filters, err)
				return
			}

			pool.readStored(ctx, relay,
				func() (*Subscription, error) { return relay.Subscribe(ctx, filters, opts...) },
				func(ie RelayEvent) bool {
					select {
					case events <- ie:
						return true
					case <-ctx.Done():
						return false
					}
				},
			)
		}(NormalizeURL(url))
	}

	return events
}

// CountMany aggregates count results from multiple relays using NIP-45 HyperLogLog
func (pool *SimplePool) CountMany(
	ctx context.Context,
	urls []string,
	filter Filter,
	opts []SubscriptionOption,
) int {
	hll := hyperloglog.New(0) // offset is irrelevant here

	wg := sync.WaitGroup{}
	wg.Add(len(urls))
	for _, url := range urls {
		go func(nm string) {
			defer wg.Done()
			relay, err := pool.EnsureRelay(url)
			if err != nil {
				return
			}
			ce, err := relay.countInternal(ctx, Filters{filter}, opts...)
			if err != nil {
				return
			}
			if len(ce.HyperLogLog) != 256 {
				return
			}
			hll.MergeRegisters(ce.HyperLogLog)
		}(NormalizeURL(url))
	}

	wg.Wait()
	return int(hll.Count())
}

// QuerySingle returns the first event returned by the first relay, cancels everything else.
func (pool *SimplePool) QuerySingle(
	ctx context.Context,
	urls []string,
	filter Filter,
	opts ...SubscriptionOption,
) *RelayEvent {
	ctx, cancel := context.WithCancelCause(ctx)
	for ievt := range pool.SubManyEose(ctx, urls, Filters{filter}, opts...) {
		cancel(errors.New("got the first event and ended successfully"))
		return &ievt
	}
	cancel(errors.New("SubManyEose() didn't get yield events"))
	return nil
}

// BatchedSubManyEose performs batched subscriptions to multiple relays with different filters.
func (pool *SimplePool) BatchedSubManyEose(
	ctx context.Context,
	dfs []DirectedFilter,
	opts ...SubscriptionOption,
) chan RelayEvent {
	res := make(chan RelayEvent)
	wg := sync.WaitGroup{}
	wg.Add(len(dfs))
	seenAlready := xsync.NewMapOf[string, struct{}]()

	for _, df := range dfs {
		go func(df DirectedFilter) {
			for ie := range pool.subManyEoseNonOverwriteCheckDuplicate(ctx,
				[]string{df.Relay},
				Filters{df.Filter},
				WithCheckDuplicate(func(id, relay string) bool {
					_, exists := seenAlready.LoadOrStore(id, struct{}{})
					if exists && pool.duplicateMiddleware != nil {
						pool.duplicateMiddleware(relay, id)
					}
					return exists
				}), opts...,
			) {
				select {
				case res <- ie:
				case <-ctx.Done():
					wg.Done()
					return
				}
			}
			wg.Done()
		}(df)
	}

	go func() {
		wg.Wait()
		close(res)
	}()

	return res
}

// Close closes the pool with the given reason.
func (pool *SimplePool) Close(reason string) {
	pool.cancel(fmt.Errorf("pool closed with reason: '%s'", reason))
}
