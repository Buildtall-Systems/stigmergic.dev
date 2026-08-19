package nostr

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v3"
)

var subscriptionIDCounter atomic.Int64

// Relay represents a connection to a Nostr relay.
type Relay struct {
	closeMutex sync.Mutex

	URL           string
	requestHeader http.Header  // e.g. for origin header
	httpClient    *http.Client // optional caller-supplied HTTP client (e.g. SOCKS5-routed)

	Connection    *Connection
	Subscriptions *xsync.MapOf[int64, *Subscription]

	ConnectionError         error
	connectionContext       context.Context // will be canceled when the connection closes
	connectionContextCancel context.CancelCauseFunc

	challengeMu                   sync.Mutex   // guards challenge; not closeMutex, so auth never orders against teardown
	challenge                     string       // NIP-42 challenge, we only keep the last
	noticeHandler                 func(string) // NIP-01 NOTICEs
	customHandler                 func(string) // nonstandard unparseable messages
	okCallbacks                   *xsync.MapOf[string, func(bool, string)]
	writeQueue                    chan writeRequest
	subscriptionChannelCloseQueue chan *Subscription

	authDone    chan struct{} // closed exactly once when auth succeeds (proactive or reactive)
	authOnce    sync.Once    // guards authDone close to prevent double-close panic
	challengeCh chan string   // buffered(1), notifies when a new challenge arrives from relay

	// custom things that aren't often used
	//
	AssumeValid bool // this will skip verifying signatures for events received from this relay
}

type writeRequest struct {
	msg    []byte
	answer chan error
}

// NewRelay returns a new relay. It takes a context that, when canceled, will close the relay connection.
func NewRelay(ctx context.Context, url string, opts ...RelayOption) *Relay {
	ctx, cancel := context.WithCancelCause(ctx)
	r := &Relay{
		URL:                           NormalizeURL(url),
		connectionContext:             ctx,
		connectionContextCancel:       cancel,
		Subscriptions:                 xsync.NewMapOf[int64, *Subscription](),
		okCallbacks:                   xsync.NewMapOf[string, func(bool, string)](),
		writeQueue:                    make(chan writeRequest),
		subscriptionChannelCloseQueue: make(chan *Subscription),
		requestHeader:                 nil,
		authDone:                      make(chan struct{}),
		challengeCh:                   make(chan string, 1),
	}

	for _, opt := range opts {
		opt.ApplyRelayOption(r)
	}

	return r
}

// RelayConnect returns a relay object connected to url.
//
// The given subscription is only used during the connection phase. Once successfully connected, cancelling ctx has no effect.
//
// The ongoing relay connection uses a background context. To close the connection, call r.Close().
// If you need fine grained long-term connection contexts, use NewRelay() instead.
func RelayConnect(ctx context.Context, url string, opts ...RelayOption) (*Relay, error) {
	r := NewRelay(context.Background(), url, opts...)
	err := r.Connect(ctx)
	return r, err
}

// RelayOption is the type of the argument passed when instantiating relay connections.
type RelayOption interface {
	ApplyRelayOption(*Relay)
}

var (
	_ RelayOption = (WithNoticeHandler)(nil)
	_ RelayOption = (WithCustomHandler)(nil)
	_ RelayOption = (WithRequestHeader)(nil)
	_ RelayOption = (*WithHTTPClient)(nil)
)

// WithNoticeHandler just takes notices and is expected to do something with them.
// when not given, defaults to logging the notices.
type WithNoticeHandler func(notice string)

func (nh WithNoticeHandler) ApplyRelayOption(r *Relay) {
	r.noticeHandler = nh
}

// WithCustomHandler must be a function that handles any relay message that couldn't be
// parsed as a standard envelope.
type WithCustomHandler func(data string)

func (ch WithCustomHandler) ApplyRelayOption(r *Relay) {
	r.customHandler = ch
}

// WithRequestHeader sets the HTTP request header of the websocket preflight request.
type WithRequestHeader http.Header

func (ch WithRequestHeader) ApplyRelayOption(r *Relay) {
	r.requestHeader = http.Header(ch)
}

// WithHTTPClient sets a custom HTTP client used for the WebSocket dial.
// Use this to route connections through a proxy (e.g. SOCKS5).
type WithHTTPClient struct{ Client *http.Client }

func (wh *WithHTTPClient) ApplyRelayOption(r *Relay) {
	r.httpClient = wh.Client
}

// String just returns the relay URL.
func (r *Relay) String() string {
	return r.URL
}

// Context retrieves the context that is associated with this relay connection.
// It will be closed when the relay is disconnected.
func (r *Relay) Context() context.Context { return r.connectionContext }

// IsConnected returns true if the connection to this relay seems to be active.
func (r *Relay) IsConnected() bool { return r.connectionContext.Err() == nil }

// AuthDone returns a channel that is closed when NIP-42 authentication
// succeeds, whether via proactive PerformAuth or reactive Auth.
func (r *Relay) AuthDone() <-chan struct{} { return r.authDone }

// setChallenge records the newest NIP-42 challenge. Called only from the
// message read loop.
func (r *Relay) setChallenge(challenge string) {
	r.challengeMu.Lock()
	r.challenge = challenge
	r.challengeMu.Unlock()
}

// currentChallenge returns the newest NIP-42 challenge, or "" if none has
// arrived. The read loop writes the field while callers read it from their own
// goroutines, so every access is guarded: an unsynchronized read can tear the
// string header, and a stale one gets stamped into an AUTH event the relay then
// rejects.
func (r *Relay) currentChallenge() string {
	r.challengeMu.Lock()
	defer r.challengeMu.Unlock()
	return r.challenge
}

// Connect tries to establish a websocket connection to r.URL.
// If the context expires before the connection is complete, an error is returned.
// Once successfully connected, context expiration has no effect: call r.Close
// to close the connection.
//
// The given context here is only used during the connection phase. The long-living
// relay connection will be based on the context given to NewRelay().
func (r *Relay) Connect(ctx context.Context) error {
	return r.ConnectWithTLS(ctx, nil)
}

// ConnectWithTLS is like Connect(), but takes a special tls.Config if you need that.
func (r *Relay) ConnectWithTLS(ctx context.Context, tlsConfig *tls.Config) error {
	if r.connectionContext == nil || r.Subscriptions == nil {
		return fmt.Errorf("relay must be initialized with a call to NewRelay()")
	}

	if r.URL == "" {
		return fmt.Errorf("invalid relay URL '%s'", r.URL)
	}

	if _, ok := ctx.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, 7*time.Second, errors.New("connection took too long"))
		defer cancel()
	}

	conn, err := NewConnection(ctx, r.URL, r.requestHeader, tlsConfig, r.httpClient)
	if err != nil {
		return fmt.Errorf("error opening websocket to '%s': %w", r.URL, err)
	}
	r.Connection = conn

	// ping every 29 seconds
	ticker := time.NewTicker(29 * time.Second)

	// queue all write operations here so we don't do mutex spaghetti
	go func() {
		defer func() {
			ticker.Stop()

			// close() reads and dereferences Connection under closeMutex, and
			// it is close() that cancels connectionContext and so schedules
			// this very cleanup. Without the lock the nil assignment can land
			// between close()'s nil check and its Close() call, dereferencing
			// nil.
			// ConnectionError is written by the read loop under the same lock,
			// so it is snapshotted here rather than read inside the loop below.
			r.closeMutex.Lock()
			r.Connection = nil
			connErr := r.ConnectionError
			r.closeMutex.Unlock()

			for _, sub := range r.Subscriptions.Range {
				sub.unsub(fmt.Errorf("relay connection closed: %w / %w", context.Cause(r.connectionContext), connErr))
			}
		}()

		pingAttempt := 0
		for {
			select {
			case <-r.connectionContext.Done():
				return

			case <-ticker.C:
				debugLogf("{%s} pinging relay", r.URL)
				err := r.Connection.Ping(r.connectionContext)
				if err != nil {
					pingAttempt++
					debugLogf("{%s} error writing ping (attempt %d): %v", r.URL, pingAttempt, err)

					if pingAttempt >= 3 {
						debugLogf("{%s} error writing ping after multiple attempts; closing websocket", r.URL)
						err = r.Close() // this should trigger a context cancelation
						if err != nil {
							debugLogf("{%s} failed to close relay: %v", r.URL, err)
						}
						return
					}
					continue
				}
				// ping was OK
				debugLogf("{%s} ping OK", r.URL)
				pingAttempt = 0

			case writeRequest := <-r.writeQueue:
				// all write requests will go through this to prevent races
				debugLogf("{%s} sending %v\n", r.URL, string(writeRequest.msg))
				if err := r.Connection.WriteMessage(r.connectionContext, writeRequest.msg); err != nil {
					writeRequest.answer <- err
				}
				close(writeRequest.answer)
			}
		}
	}()

	// general message reader loop
	go func() {
		buf := new(bytes.Buffer)
		mp := NewMessageParser()

		for {
			buf.Reset()

			if err := conn.ReadMessage(r.connectionContext, buf); err != nil {
				// Released before close(), which takes the same non-reentrant
				// lock.
				r.closeMutex.Lock()
				r.ConnectionError = err
				r.closeMutex.Unlock()

				r.close(err)
				break
			}

			message := string(buf.Bytes())
			debugLogf("{%s} received %v\n", r.URL, message)

			// if this is an "EVENT" we will have this preparser logic that should speed things up a little
			// as we skip handling duplicate events
			subid := extractSubID(message)
			sub, ok := r.Subscriptions.Load(subIdToSerial(subid))
			if ok {
				if sub.checkDuplicate != nil {
					if sub.checkDuplicate(extractEventID(message[10+len(subid):]), r.URL) {
						continue
					}
				} else if sub.checkDuplicateReplaceable != nil {
					if sub.checkDuplicateReplaceable(
						ReplaceableKey{extractEventPubKey(message), extractDTag(message)},
						extractTimestamp(message),
					) {
						continue
					}
				}
			}

			envelope, err := mp.ParseMessage(message)
			if envelope == nil {
				if r.customHandler != nil && err == UnknownLabel {
					r.customHandler(message)
				}
				continue
			}

			switch env := envelope.(type) {
			case *NoticeEnvelope:
				// see WithNoticeHandler
				if r.noticeHandler != nil {
					r.noticeHandler(string(*env))
				} else {
					log.Printf("NOTICE from %s: '%s'\n", r.URL, string(*env))
				}
			case *AuthEnvelope:
				if env.Challenge == nil {
					continue
				}
				r.setChallenge(*env.Challenge)
				select {
				case <-r.challengeCh:
				default:
				}
				r.challengeCh <- *env.Challenge
			case *EventEnvelope:
				// we already have the subscription from the pre-check above, so we can just reuse it
				if sub == nil {
					// InfoLogger.Printf("{%s} no subscription with id '%s'\n", r.URL, *env.SubscriptionID)
					continue
				} else {
					// check if the event matches the desired filter, ignore otherwise
					if !sub.match(&env.Event) {
						InfoLogger.Printf("{%s} filter does not match: %v ~ %v\n", r.URL, sub.Filters, env.Event)
						continue
					}

					// check signature, ignore invalid, except from trusted (AssumeValid) relays
					if !r.AssumeValid {
						if ok, _ := env.Event.CheckSignature(); !ok {
							InfoLogger.Printf("{%s} bad signature on %s\n", r.URL, env.Event.ID)
							continue
						}
					}

					// dispatch this to the internal .events channel of the subscription
					sub.dispatchEvent(&env.Event)
				}
			case *EOSEEnvelope:
				if subscription, ok := r.Subscriptions.Load(subIdToSerial(string(*env))); ok {
					subscription.dispatchEose()
				}
			case *ClosedEnvelope:
				if subscription, ok := r.Subscriptions.Load(subIdToSerial(env.SubscriptionID)); ok {
					subscription.handleClosed(env.Reason)
				}
			case *CountEnvelope:
				if subscription, ok := r.Subscriptions.Load(subIdToSerial(env.SubscriptionID)); ok && env.Count != nil && subscription.countResult != nil {
					subscription.countResult <- *env
				}
			case *OKEnvelope:
				if okCallback, exist := r.okCallbacks.Load(env.EventID); exist {
					okCallback(env.OK, env.Reason)
				} else {
					InfoLogger.Printf("{%s} got an unexpected OK message for event %s", r.URL, env.EventID)
				}
			}
		}
	}()

	return nil
}

// Write queues an arbitrary message to be sent to the relay.
func (r *Relay) Write(msg []byte) <-chan error {
	ch := make(chan error)
	select {
	case r.writeQueue <- writeRequest{msg: msg, answer: ch}:
	case <-r.connectionContext.Done():
		go func() { ch <- fmt.Errorf("connection closed") }()
	}
	return ch
}

// Publish sends an "EVENT" command to the relay r as in NIP-01 and waits for an OK response.
func (r *Relay) Publish(ctx context.Context, event Event) error {
	return r.publish(ctx, event.ID, &EventEnvelope{Event: event})
}

// Auth sends an "AUTH" command client->relay as in NIP-42 and waits for an OK response.
//
// You don't have to build the AUTH event yourself, this function takes a function to which the
// event that must be signed will be passed, so it's only necessary to sign that.
func (r *Relay) Auth(ctx context.Context, sign func(event *Event) error) error {
	authEvent := Event{
		CreatedAt: Now(),
		Kind:      KindClientAuthentication,
		Tags: Tags{
			Tag{"relay", r.URL},
			Tag{"challenge", r.currentChallenge()},
		},
		Content: "",
	}
	if err := sign(&authEvent); err != nil {
		return fmt.Errorf("error signing auth event: %w", err)
	}

	if err := r.publish(ctx, authEvent.ID, &AuthEnvelope{Event: authEvent}); err != nil {
		return err
	}
	r.authOnce.Do(func() { close(r.authDone) })
	return nil
}

// PerformAuth blocks until a NIP-42 challenge is available (or uses one already
// received), signs and sends the AUTH event, and waits for the relay's OK.
// On success AuthDone() will be closed.
func (r *Relay) PerformAuth(ctx context.Context, sign func(event *Event) error) error {
	if r.currentChallenge() == "" {
		select {
		case <-r.challengeCh:
		case <-ctx.Done():
			return ctx.Err()
		case <-r.connectionContext.Done():
			return fmt.Errorf("connection closed while waiting for auth challenge")
		}
	}
	return r.Auth(ctx, sign)
}

func (r *Relay) publish(ctx context.Context, id string, env Envelope) error {
	var err error
	var cancel context.CancelFunc

	if _, ok := ctx.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		ctx, cancel = context.WithTimeoutCause(ctx, 7*time.Second, fmt.Errorf("given up waiting for an OK"))
		defer cancel()
	} else {
		// otherwise make the context cancellable so we can stop everything upon receiving an "OK"
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	// listen for an OK callback
	gotOk := false
	r.okCallbacks.Store(id, func(ok bool, reason string) {
		gotOk = true
		if !ok {
			err = fmt.Errorf("msg: %s", reason)
		}
		cancel()
	})
	defer r.okCallbacks.Delete(id)

	// publish event
	envb, _ := env.MarshalJSON()
	if err := <-r.Write(envb); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			// this will be called when we get an OK or when the context has been canceled
			if gotOk {
				return err
			}
			return ctx.Err()
		case <-r.connectionContext.Done():
			// this is caused when we lose connectivity
			return err
		}
	}
}

// Subscribe sends a "REQ" command to the relay r as in NIP-01.
// Events are returned through the channel sub.Events.
// The subscription is closed when context ctx is cancelled ("CLOSE" in NIP-01).
//
// Remember to cancel subscriptions, either by calling `.Unsub()` on them or ensuring their `context.Context` will be canceled at some point.
// Failure to do that will result in a huge number of halted goroutines being created.
func (r *Relay) Subscribe(ctx context.Context, filters Filters, opts ...SubscriptionOption) (*Subscription, error) {
	sub := r.PrepareSubscription(ctx, filters, opts...)

	// Read under closeMutex: the write pump nils Connection on teardown, so an
	// unguarded read here races a concurrent disconnect.
	r.closeMutex.Lock()
	connected := r.Connection != nil
	r.closeMutex.Unlock()

	if !connected {
		return nil, fmt.Errorf("not connected to %s", r.URL)
	}

	if err := sub.Fire(); err != nil {
		return nil, fmt.Errorf("couldn't subscribe to %v at %s: %w", filters, r.URL, err)
	}

	return sub, nil
}

// PrepareSubscription creates a subscription, but doesn't fire it.
//
// Remember to cancel subscriptions, either by calling `.Unsub()` on them or ensuring their `context.Context` will be canceled at some point.
// Failure to do that will result in a huge number of halted goroutines being created.
func (r *Relay) PrepareSubscription(ctx context.Context, filters Filters, opts ...SubscriptionOption) *Subscription {
	current := subscriptionIDCounter.Add(1)
	ctx, cancel := context.WithCancelCause(ctx)

	sub := &Subscription{
		Relay:             r,
		Context:           ctx,
		cancel:            cancel,
		counter:           current,
		Events:            make(chan *Event),
		EndOfStoredEvents: make(chan struct{}, 1),
		ClosedReason:      make(chan string, 1),
		Filters:           filters,
		match:             filters.Match,
	}

	label := ""
	for _, opt := range opts {
		switch o := opt.(type) {
		case WithLabel:
			label = string(o)
		case WithCheckDuplicate:
			sub.checkDuplicate = o
		case WithCheckDuplicateReplaceable:
			sub.checkDuplicateReplaceable = o
		}
	}

	// subscription id computation
	buf := subIdPool.Get().([]byte)[:0]
	buf = strconv.AppendInt(buf, sub.counter, 10)
	buf = append(buf, ':')
	buf = append(buf, label...)
	defer subIdPool.Put(buf)
	sub.id = string(buf)

	// we track subscriptions only by their counter, no need for the full id
	r.Subscriptions.Store(int64(sub.counter), sub)

	// start handling events, eose, unsub etc:
	go sub.start()

	return sub
}

// QueryEvents subscribes to events matching the given filter and returns a channel of events.
//
// In most cases it's better to use SimplePool instead of this method.
func (r *Relay) QueryEvents(ctx context.Context, filter Filter) (chan *Event, error) {
	sub, err := r.Subscribe(ctx, Filters{filter})
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case <-sub.ClosedReason:
			case <-sub.EndOfStoredEvents:
			case <-ctx.Done():
			case <-r.Context().Done():
			}
			sub.unsub(errors.New("QueryEvents() ended"))
			return
		}
	}()

	return sub.Events, nil
}

// QuerySync subscribes to events matching the given filter and returns a slice of events.
// This method blocks until all events are received or the context is canceled.
//
// In most cases it's better to use SimplePool instead of this method.
func (r *Relay) QuerySync(ctx context.Context, filter Filter) ([]*Event, error) {
	if _, ok := ctx.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, 7*time.Second, errors.New("QuerySync() took too long"))
		defer cancel()
	}

	events := make([]*Event, 0, max(filter.Limit, 250))
	ch, err := r.QueryEvents(ctx, filter)
	if err != nil {
		return nil, err
	}

	for evt := range ch {
		events = append(events, evt)
	}

	return events, nil
}

// Count sends a "COUNT" command to the relay and returns the count of events matching the filters.
func (r *Relay) Count(
	ctx context.Context,
	filters Filters,
	opts ...SubscriptionOption,
) (int64, []byte, error) {
	v, err := r.countInternal(ctx, filters, opts...)
	if err != nil {
		return 0, nil, err
	}

	return *v.Count, v.HyperLogLog, nil
}

func (r *Relay) countInternal(ctx context.Context, filters Filters, opts ...SubscriptionOption) (CountEnvelope, error) {
	sub := r.PrepareSubscription(ctx, filters, opts...)
	sub.countResult = make(chan CountEnvelope)

	if err := sub.Fire(); err != nil {
		return CountEnvelope{}, err
	}

	defer sub.unsub(errors.New("countInternal() ended"))

	if _, ok := ctx.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, 7*time.Second, errors.New("countInternal took too long"))
		defer cancel()
	}

	for {
		select {
		case count := <-sub.countResult:
			return count, nil
		case <-ctx.Done():
			return CountEnvelope{}, ctx.Err()
		}
	}
}

// Close closes the relay connection.
func (r *Relay) Close() error {
	return r.close(errors.New("Close() called"))
}

func (r *Relay) close(reason error) error {
	r.closeMutex.Lock()
	defer r.closeMutex.Unlock()

	if r.connectionContextCancel == nil {
		return fmt.Errorf("relay already closed")
	}
	r.connectionContextCancel(reason)
	r.connectionContextCancel = nil

	if r.Connection == nil {
		return fmt.Errorf("relay not connected")
	}

	err := r.Connection.Close()
	if err != nil {
		return err
	}

	return nil
}

var subIdPool = sync.Pool{
	New: func() any { return make([]byte, 0, 15) },
}
