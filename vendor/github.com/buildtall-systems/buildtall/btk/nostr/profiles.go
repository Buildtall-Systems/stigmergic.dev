package nostr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"golang.org/x/sync/errgroup"
)

// NIP-65 relay list markers.
const (
	relayMarkerRead  = "read"
	relayMarkerWrite = "write"
)

// MaxWriteRelaysPerAuthor caps how many of an author's declared NIP-65 write
// relays are used for outbox routing, bounding a per-author fan-out so a few
// unreachable relays cannot starve it. Exported as the fleet rule's single
// authority: consumers that cap relay derivation reference this constant
// rather than declaring their own.
const MaxWriteRelaysPerAuthor = 5

// RelayListAggregators are the relays buildtall queries to discover an author's
// kind-10002 relay list (NIP-65) when it is absent from both the home and the
// fallback relays — tier-2 of the outbox resolution ladder. Declared once here
// as the single source of truth rather than inherited from any library default;
// callers opt in with WithRelayListRelays.
var RelayListAggregators = []string{
	"wss://relay.damus.io",
	"wss://purplepag.es",
	"wss://nos.lol",
}

// relayBatch is the demuxed result of one kind-widened query against a single
// relay: kind-0 profiles, and — when the resolver harvests them — the newest
// kind-10002 relay list and kind-3 follow list per author.
type relayBatch struct {
	profiles   map[string]*Profile
	relayLists map[string]*nostr.Event
	contacts   map[string]*nostr.Event
}

type Profile struct {
	Event       *nostr.Event
	Npub        string
	Name        string
	DisplayName string
	Picture     string
	About       string
	NIP05       string
	Banner      string
	Website     string
	Lud16       string
}

type ProfileResolverOption func(*ProfileResolver)

func WithNsecFile(path string) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.nsecFile = path
	}
}

func WithCacheToHome(enabled bool) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.cacheToHome = enabled
	}
}

func WithTimeout(d time.Duration) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.timeout = d
	}
}

func WithLogger(l *slog.Logger) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.logger = l
	}
}

// WithPool shares an externally-owned SimplePool for home-relay queries.
// When set, queries against homeRelay reuse the pool's connection (and its
// proactive-auth handshake, if configured) instead of opening a fresh one.
func WithPool(p *nostr.SimplePool) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.pool = p
	}
}

// WithTTL sets how long a resolved profile stays cached before it is re-fetched.
// The default of 0 means entries never expire, preserving cache-forever behavior
// for callers that do not opt in.
func WithTTL(d time.Duration) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.ttl = d
	}
}

// WithMaxOutboxRelays caps how many outbox relays a resolution queries and how
// many relay queries run concurrently. Relays are selected coverage-maximizing:
// the ones cited by the most unresolved pubkeys' relay lists win. The default
// of 0 means unlimited, preserving query-every-relay behavior for callers that
// do not opt in.
func WithMaxOutboxRelays(n int) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.maxOutboxRelays = n
	}
}

// WithMaxDuration bounds an entire resolution — home, fallback, and outbox
// waves share the one deadline — so per-relay timeouts cannot stack. The
// default of 0 means unbounded, preserving existing behavior for callers that
// do not opt in.
func WithMaxDuration(d time.Duration) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.maxDuration = d
	}
}

// WithHarvestContacts widens every ladder query to also request kind-3 follow
// lists, demuxed and exposed via ResolveManyStreamWithContacts, so contacts ride
// the same subscription and outbox connections already opened for profiles. The
// default of false keeps profile-only callers on their existing narrow query.
func WithHarvestContacts(enabled bool) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.harvestContacts = enabled
	}
}

// WithRelayListRelays sets the tier-2 aggregator relays queried for an author's
// kind-10002 relay list when it is absent from the home and fallback relays,
// before the outbox wave. The default of nil disables the fallback, preserving
// existing behavior for callers that do not opt in. See RelayListAggregators.
func WithRelayListRelays(relays []string) ProfileResolverOption {
	return func(r *ProfileResolver) {
		r.relayListRelays = relays
	}
}

type profileCacheEntry struct {
	profile *Profile
	// contacts is the author's newest harvested kind-3 follow list, captured
	// whenever harvesting is enabled — regardless of which resolution path
	// wrote the entry — so a cache hit can serve the harvest instead of
	// silently dropping it. Nil when harvesting is off or no list was found.
	contacts *nostr.Event
	expiry   time.Time
}

// valid reports whether the entry is still fresh. A zero expiry means the entry
// never expires (TTL disabled).
func (e profileCacheEntry) valid() bool {
	return e.expiry.IsZero() || time.Now().Before(e.expiry)
}

type ProfileResolver struct {
	cache map[string]profileCacheEntry
	// queryBatch is the per-relay query seam: r.queryRelayBatch in production,
	// substituted by tests so parallelism and deadline behavior are exercised
	// without the network.
	queryBatch func(ctx context.Context, relayURL string, pubkeys []string, includeRelayLists bool) relayBatch
	// queryKindsBatch is the outbox seam's scoped per-relay query — only the
	// given authors, only the given kinds: r.queryRelayKinds in production,
	// substituted by tests so chunking and per-relay author scoping are
	// observable without the network.
	queryKindsBatch func(ctx context.Context, relayURL string, authors []string, kinds []int) []*nostr.Event
	// queryStreamBatch is the stream path's scoped per-relay query — one
	// author, the article kind, a hard item limit: r.queryRelayStream in
	// production, substituted by tests so per-author bounding is observable
	// without the network.
	queryStreamBatch func(ctx context.Context, relayURL, author string, limit int) []*nostr.Event
	// publishHome is the unconditional home write-back seam:
	// r.writeEventsToHome in production, substituted by tests so the exact
	// write-back payload and its context are observable without the network.
	publishHome     func(ctx context.Context, events []*nostr.Event)
	logger          *slog.Logger
	pool            *nostr.SimplePool
	homeRelay       string
	nsecFile        string
	fallbackRelays  []string
	relayListRelays []string
	mu              sync.RWMutex
	timeout         time.Duration
	ttl             time.Duration
	maxDuration     time.Duration
	maxOutboxRelays int
	cacheToHome     bool
	harvestContacts bool
}

// entryExpiry returns the expiry timestamp for a freshly cached entry, or the
// zero time when TTL is disabled.
func (r *ProfileResolver) entryExpiry() time.Time {
	if r.ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(r.ttl)
}

func NewProfileResolver(homeRelay string, fallbackRelays []string, opts ...ProfileResolverOption) *ProfileResolver {
	r := &ProfileResolver{
		homeRelay:      homeRelay,
		fallbackRelays: fallbackRelays,
		cacheToHome:    true,
		timeout:        5 * time.Second,
		logger:         slog.Default(),
		cache:          make(map[string]profileCacheEntry),
	}
	r.queryBatch = r.queryRelayBatch
	r.queryKindsBatch = r.queryRelayKinds
	r.queryStreamBatch = r.queryRelayStream
	r.publishHome = r.writeEventsToHome
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// harvestMap returns the map handed to batchResolve for the kind-3 harvest:
// the caller's contactsOut when set, a private map when harvesting is enabled
// (so cache entries capture contacts even when this caller did not ask for
// them), nil otherwise.
func (r *ProfileResolver) harvestMap(contactsOut map[string]*nostr.Event) map[string]*nostr.Event {
	if contactsOut != nil {
		return contactsOut
	}
	if r.harvestContacts {
		return make(map[string]*nostr.Event)
	}
	return nil
}

func (r *ProfileResolver) Resolve(ctx context.Context, npub string) (*Profile, error) {
	r.mu.RLock()
	if e, ok := r.cache[npub]; ok && e.valid() {
		r.mu.RUnlock()
		return e.profile, nil
	}
	r.mu.RUnlock()

	hexPubkey, err := NpubToHex(npub)
	if err != nil {
		return nil, fmt.Errorf("decoding npub: %w", err)
	}

	p, contacts := r.resolveOne(ctx, npub, hexPubkey)

	r.mu.Lock()
	r.cache[npub] = profileCacheEntry{profile: p, contacts: contacts, expiry: r.entryExpiry()}
	r.mu.Unlock()

	return p, nil
}

func (r *ProfileResolver) ResolveMany(ctx context.Context, npubs []string) (map[string]*Profile, error) {
	result := make(map[string]*Profile, len(npubs))

	var uncachedNpubs []string
	var uncachedHex []string
	r.mu.RLock()
	for _, npub := range npubs {
		if e, ok := r.cache[npub]; ok && e.valid() {
			result[npub] = e.profile
		} else {
			uncachedNpubs = append(uncachedNpubs, npub)
		}
	}
	r.mu.RUnlock()

	if len(uncachedNpubs) == 0 {
		return result, nil
	}

	npubToHex := make(map[string]string, len(uncachedNpubs))
	hexToNpub := make(map[string]string, len(uncachedNpubs))
	for _, npub := range uncachedNpubs {
		hex, err := NpubToHex(npub)
		if err != nil {
			continue
		}
		npubToHex[npub] = hex
		hexToNpub[hex] = npub
		uncachedHex = append(uncachedHex, hex)
	}

	if len(uncachedHex) == 0 {
		return result, nil
	}

	harvest := r.harvestMap(nil)
	profiles := r.batchResolve(ctx, uncachedHex, hexToNpub, nil, harvest)

	r.mu.Lock()
	expiry := r.entryExpiry()
	for npub, p := range profiles {
		r.cache[npub] = profileCacheEntry{profile: p, contacts: harvest[npub], expiry: expiry}
		result[npub] = p
	}
	r.mu.Unlock()

	return result, nil
}

// ResolveManyStream resolves npubs exactly as ResolveMany does — cache, then
// home, fallback, and outbox waves under the shared deadline — but emits each
// profile on the returned channel as it lands: cached and home-wave results as
// a burst, fallback/outbox results per relay batch, placeholders last. Every
// requested npub is emitted exactly once (resolved, cached, or placeholder;
// undecodable npubs emit placeholders), and the channel closes when resolution
// completes. The channel is buffered to the full emission count, so resolution
// never blocks on a slow consumer. The cache is populated identically to
// ResolveMany.
func (r *ProfileResolver) ResolveManyStream(ctx context.Context, npubs []string) <-chan *Profile {
	return r.resolveManyStream(ctx, npubs, nil)
}

// ResolveManyStreamWithContacts resolves npubs exactly as ResolveManyStream —
// profiles stream on the returned channel — and additionally harvests each
// resolved author's newest kind-3 follow list from the SAME ladder pass (one
// kind-widened subscription per relay). Cache-hit npubs contribute the
// follow list captured when their entry was written, so a warm cache serves
// the harvest rather than dropping it. The returned accessor returns the
// harvested contacts keyed by author npub; call it only after the channel is
// fully drained, which happens-before the harvest is complete. Harvesting
// requires WithHarvestContacts; without it the accessor returns an empty map.
func (r *ProfileResolver) ResolveManyStreamWithContacts(ctx context.Context, npubs []string) (<-chan *Profile, func() map[string]*nostr.Event) {
	contacts := make(map[string]*nostr.Event)
	ch := r.resolveManyStream(ctx, npubs, contacts)
	return ch, func() map[string]*nostr.Event { return contacts }
}

// resolveManyStream is the shared body of the streaming resolvers. When
// contactsOut is non-nil it is populated with the harvested kind-3 follow lists
// (keyed by author npub) before the channel closes.
func (r *ProfileResolver) resolveManyStream(ctx context.Context, npubs []string, contactsOut map[string]*nostr.Event) <-chan *Profile {
	ch := make(chan *Profile, len(npubs))

	var uncachedNpubs []string
	r.mu.RLock()
	for _, npub := range npubs {
		if e, ok := r.cache[npub]; ok && e.valid() {
			ch <- e.profile
			if contactsOut != nil && e.contacts != nil {
				contactsOut[npub] = e.contacts
			}
		} else {
			uncachedNpubs = append(uncachedNpubs, npub)
		}
	}
	r.mu.RUnlock()

	if len(uncachedNpubs) == 0 {
		close(ch)
		return ch
	}

	hexToNpub := make(map[string]string, len(uncachedNpubs))
	var uncachedHex []string
	for _, npub := range uncachedNpubs {
		hex, err := NpubToHex(npub)
		if err != nil {
			ch <- fallbackProfile(npub)
			continue
		}
		hexToNpub[hex] = npub
		uncachedHex = append(uncachedHex, hex)
	}

	if len(uncachedHex) == 0 {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)

		harvest := r.harvestMap(contactsOut)
		profiles := r.batchResolve(ctx, uncachedHex, hexToNpub, func(p *Profile) {
			ch <- p
		}, harvest)

		r.mu.Lock()
		expiry := r.entryExpiry()
		for npub, p := range profiles {
			r.cache[npub] = profileCacheEntry{profile: p, contacts: harvest[npub], expiry: expiry}
		}
		r.mu.Unlock()
	}()

	return ch
}

// Invalidate removes a single npub's cached profile, forcing the next resolve to
// re-fetch it from relays.
func (r *ProfileResolver) Invalidate(npub string) {
	r.mu.Lock()
	delete(r.cache, npub)
	r.mu.Unlock()
}

// InvalidateAll clears the entire profile cache.
func (r *ProfileResolver) InvalidateAll() {
	r.mu.Lock()
	r.cache = make(map[string]profileCacheEntry)
	r.mu.Unlock()
}

// batchResolve walks the home → fallback → outbox waves for hexPubkeys under
// one shared deadline (when configured). A non-nil emit receives every profile
// exactly once as it lands — home-wave results as a burst, fallback results
// per relay batch, outbox results as a burst after that wave, placeholders
// last — while the returned map is the complete result either way. When
// contactsOut is non-nil and the resolver harvests contacts, it receives every
// discovered kind-3 follow list keyed by author npub, merged newest-wins
// across the same waves.
func (r *ProfileResolver) batchResolve(ctx context.Context, hexPubkeys []string, hexToNpub map[string]string, emit func(*Profile), contactsOut map[string]*nostr.Event) map[string]*Profile {
	// The write-back runs after the waves, so it must not inherit the shared
	// deadline — by then it is spent. It gets the caller's context instead
	// (bounded internally by r.timeout).
	preDeadlineCtx := ctx
	if r.maxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.maxDuration)
		defer cancel()
	}

	result := make(map[string]*Profile, len(hexPubkeys))

	homeBatch := r.queryBatch(ctx, r.homeRelay, hexPubkeys, true)

	// An author is done at a rung only when everything the resolver was asked
	// for has landed: the kind-0, and the kind-3 as well when contacts are
	// harvested. Gating the descent on the kind-0 alone marks harvesting
	// authors done at the home rung, so their follow lists are never fetched
	// once their profiles are home-cached.
	resolvedAtHome := make(map[string]bool, len(hexPubkeys))
	var remaining []string
	for _, pk := range hexPubkeys {
		p, haveProfile := homeBatch.profiles[pk]
		if haveProfile {
			npub := hexToNpub[pk]
			p.Npub = npub
			result[npub] = p
			resolvedAtHome[pk] = true
			if emit != nil {
				emit(p)
			}
		}
		_, haveContacts := homeBatch.contacts[pk]
		if !haveProfile || (r.harvestContacts && !haveContacts) {
			remaining = append(remaining, pk)
		}
	}

	allContacts := homeBatch.contacts

	if len(remaining) == 0 {
		r.exportContacts(contactsOut, allContacts, hexToNpub)
		return result
	}

	var onFound func(map[string]*Profile)
	if emit != nil {
		onFound = func(delta map[string]*Profile) {
			for pk, p := range delta {
				// Home-resolved authors descend only for their missing
				// kind-3; their profile was already emitted at rung one.
				if resolvedAtHome[pk] {
					continue
				}
				if npub, ok := hexToNpub[pk]; ok {
					p.Npub = npub
				}
				emit(p)
			}
		}
	}

	r.logger.Debug("authors incomplete on home relay, trying fallback relays", "count", len(remaining))

	fallbackBatch := r.queryMultiRelaysBatch(ctx, r.fallbackRelays, remaining, onFound)

	for _, pk := range remaining {
		p, ok := fallbackBatch.profiles[pk]
		if !ok {
			continue
		}
		npub := hexToNpub[pk]
		if _, done := result[npub]; done {
			continue
		}
		p.Npub = npub
		result[npub] = p
	}

	allRelayLists := mergeRelayLists(homeBatch.relayLists, fallbackBatch.relayLists)
	allContacts = mergeRelayLists(allContacts, fallbackBatch.contacts)

	var needOutbox []string
	for _, pk := range remaining {
		_, haveProfile := result[hexToNpub[pk]]
		_, haveContacts := allContacts[pk]
		if !haveProfile || (r.harvestContacts && !haveContacts) {
			needOutbox = append(needOutbox, pk)
		}
	}

	var aggLists map[string]*nostr.Event
	var outboxExternal []*nostr.Event
	if len(needOutbox) > 0 {
		// Tier-2: authors whose kind-10002 was on neither the home nor the
		// fallback relays get one aggregator pass so the outbox wave can still
		// reach their declared write relays.
		if len(r.relayListRelays) > 0 {
			var missingList []string
			for _, pk := range needOutbox {
				if _, ok := allRelayLists[pk]; !ok {
					missingList = append(missingList, pk)
				}
			}
			if len(missingList) > 0 {
				r.logger.Debug("relay lists missing, trying aggregators", "count", len(missingList), "relays", len(r.relayListRelays))
				aggBatch := r.queryMultiRelaysBatch(ctx, r.relayListRelays, missingList, nil)
				aggLists = aggBatch.relayLists
				allRelayLists = mergeRelayLists(allRelayLists, aggLists)
			}
		}

		writeRelays := make(map[string][]string, len(needOutbox))
		for _, pk := range needOutbox {
			if ev, ok := allRelayLists[pk]; ok {
				writeRelays[pk] = ParseOutboxRelays(ev)
			}
		}
		sel := selectOutboxRelays(writeRelays, 1, r.maxOutboxRelays)
		if len(sel.assignment) > 0 {
			r.logger.Debug("trying outbox relays for incomplete authors", "count", len(needOutbox), "relays", len(sel.assignment))
			events, external := r.queryAssignedRelays(ctx, sel.assignment, r.queryKinds(true))
			outboxExternal = external
			outboxBatch := demuxProfileBatch(events)
			for pk, p := range outboxBatch.profiles {
				npub, ok := hexToNpub[pk]
				if !ok {
					continue
				}
				if _, done := result[npub]; done {
					continue
				}
				p.Npub = npub
				result[npub] = p
				if emit != nil {
					emit(p)
				}
			}
			allContacts = mergeRelayLists(allContacts, outboxBatch.contacts)
		}
	}

	for _, pk := range remaining {
		npub := hexToNpub[pk]
		if _, ok := result[npub]; !ok {
			placeholder := fallbackProfile(npub)
			result[npub] = placeholder
			if emit != nil {
				emit(placeholder)
			}
		}
	}

	if r.cacheToHome {
		if external := externalEvents(remaining, fallbackBatch, aggLists, outboxExternal, hexToNpub); len(external) > 0 {
			if emit != nil {
				// Streaming: don't delay the stream's done event, and don't let
				// the request's end kill the write-back mid-publish.
				go r.publishHome(context.WithoutCancel(preDeadlineCtx), external)
			} else {
				r.publishHome(preDeadlineCtx, external)
			}
		}
	}

	r.exportContacts(contactsOut, allContacts, hexToNpub)
	return result
}

// externalEvents gathers every event obtained below the home rung during a
// batch resolution (fallback profiles, relay lists, and contacts, aggregator
// relay lists, and the outbox wave's events), deduplicated by event ID and
// scoped to the requested authors. This is the write-back set: everything
// external is republished to home unconditionally, including events for
// authors whose kind-0 was already home-resolved. Any narrower iteration
// ratchets, because whatever home already holds then blocks the write-back of
// what it lacks.
func externalEvents(remaining []string, fallbackBatch relayBatch, aggLists map[string]*nostr.Event, outboxExternal []*nostr.Event, hexToNpub map[string]string) []*nostr.Event {
	var events []*nostr.Event
	seen := make(map[string]bool)
	add := func(ev *nostr.Event) {
		if ev == nil || seen[ev.ID] {
			return
		}
		seen[ev.ID] = true
		events = append(events, ev)
	}
	for _, pk := range remaining {
		if p, ok := fallbackBatch.profiles[pk]; ok {
			add(p.Event)
		}
		add(fallbackBatch.relayLists[pk])
		add(fallbackBatch.contacts[pk])
		add(aggLists[pk])
	}
	for _, ev := range outboxExternal {
		if _, ok := hexToNpub[ev.PubKey]; ok {
			add(ev)
		}
	}
	return events
}

// exportContacts copies harvested kind-3 events, keyed by author hex, into the
// caller's contactsOut map re-keyed by author npub. It is a no-op when
// contactsOut is nil (the caller did not ask for contacts).
func (r *ProfileResolver) exportContacts(contactsOut, harvested map[string]*nostr.Event, hexToNpub map[string]string) {
	if contactsOut == nil {
		return
	}
	for pk, ev := range harvested {
		if npub, ok := hexToNpub[pk]; ok {
			contactsOut[npub] = ev
		}
	}
}

func (r *ProfileResolver) resolveOne(ctx context.Context, npub string, hexPubkey string) (*Profile, *nostr.Event) {
	hexToNpub := map[string]string{hexPubkey: npub}
	harvest := r.harvestMap(nil)
	profiles := r.batchResolve(ctx, []string{hexPubkey}, hexToNpub, nil, harvest)
	p, ok := profiles[npub]
	if !ok {
		p = fallbackProfile(npub)
	}
	return p, harvest[npub]
}

// WriteRelaysFor fetches npub's kind-10002 relay list across the home, fallback,
// and aggregator relays — descending only until one is found — and returns its
// declared write relays (markers "" or "write"). It returns nil when no relay
// list is found anywhere. This is the outbox-routing seam for callers that must
// subscribe to wherever an author actually publishes, independent of where that
// author's other events live. A relay list found below the home rung is
// written back to the home relay.
func (r *ProfileResolver) WriteRelaysFor(ctx context.Context, npub string) ([]string, error) {
	hexPubkey, err := NpubToHex(npub)
	if err != nil {
		return nil, fmt.Errorf("decoding npub: %w", err)
	}

	writeBackCtx := ctx
	if r.maxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.maxDuration)
		defer cancel()
	}

	authors := []string{hexPubkey}

	var relayList *nostr.Event
	for tierIdx, tier := range [][]string{{r.homeRelay}, r.fallbackRelays, r.relayListRelays} {
		if len(tier) == 0 {
			continue
		}
		if ev, ok := r.relayListsFromTier(ctx, tier, authors)[hexPubkey]; ok {
			relayList = ev
			if tierIdx > 0 {
				r.publishHome(writeBackCtx, []*nostr.Event{ev})
			}
			break
		}
	}

	if relayList == nil {
		return nil, nil
	}
	relays := ParseOutboxRelays(relayList)
	if len(relays) > MaxWriteRelaysPerAuthor {
		relays = relays[:MaxWriteRelaysPerAuthor]
	}
	return relays, nil
}

// acquireRelay returns a connected relay for relayURL — the shared pool
// connection when a pool is configured and relayURL is the home relay, a
// fresh NIP-42-authenticated connection otherwise — plus a release func the
// caller must invoke when done. Pooled relays are shared, so release never
// closes them.
func (r *ProfileResolver) acquireRelay(ctx context.Context, relayURL string) (*nostr.Relay, func(), error) {
	if r.pool != nil && relayURL == r.homeRelay {
		pooled, err := r.pool.EnsureRelay(relayURL)
		if err != nil {
			return nil, nil, err
		}
		return pooled, func() {}, nil
	}

	fresh, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		return nil, nil, err
	}
	release := func() {
		if closeErr := fresh.Close(); closeErr != nil {
			r.logger.Debug("closing relay", "relay", fresh.URL, "error", closeErr)
		}
	}
	if r.nsecFile != "" {
		r.authenticateRelay(ctx, fresh)
	}
	return fresh, release, nil
}

func (r *ProfileResolver) queryRelayBatch(ctx context.Context, relayURL string, pubkeys []string, includeRelayLists bool) relayBatch {
	batch := relayBatch{
		profiles:   make(map[string]*Profile),
		relayLists: make(map[string]*nostr.Event),
		contacts:   make(map[string]*nostr.Event),
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	relay, release, err := r.acquireRelay(queryCtx, relayURL)
	if err != nil {
		r.logger.Debug("relay connect failed", "relay", relayURL, "error", err)
		return batch
	}
	defer release()

	events, err := relay.QuerySync(queryCtx, nostr.Filter{
		Kinds:   r.queryKinds(includeRelayLists),
		Authors: pubkeys,
	})
	if err != nil {
		r.logger.Debug("batch query failed", "relay", relayURL, "error", err)
		return batch
	}

	return demuxProfileBatch(events)
}

// queryRelayKinds runs one scoped query against a single relay: only the
// given authors, only the given kinds. Callers own chunking, so an oversized
// author batch never reaches a relay from here.
func (r *ProfileResolver) queryRelayKinds(ctx context.Context, relayURL string, authors []string, kinds []int) []*nostr.Event {
	queryCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	relay, release, err := r.acquireRelay(queryCtx, relayURL)
	if err != nil {
		r.logger.Debug("relay connect failed", "relay", relayURL, "error", err)
		return nil
	}
	defer release()

	events, err := relay.QuerySync(queryCtx, nostr.Filter{
		Kinds:   kinds,
		Authors: authors,
	})
	if err != nil {
		r.logger.Debug("scoped query failed", "relay", relayURL, "error", err)
		return nil
	}
	return events
}

// queryRelayStream runs one bounded stream query against a single relay:
// one author, the article kind, at most limit items. The single-author
// filter is what makes the limit per-author; a batched filter's limit
// would let one prolific author starve the rest.
func (r *ProfileResolver) queryRelayStream(ctx context.Context, relayURL, author string, limit int) []*nostr.Event {
	queryCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	relay, release, err := r.acquireRelay(queryCtx, relayURL)
	if err != nil {
		r.logger.Debug("relay connect failed", "relay", relayURL, "error", err)
		return nil
	}
	defer release()

	events, err := relay.QuerySync(queryCtx, nostr.Filter{
		Kinds:   []int{nostr.KindArticle},
		Authors: []string{author},
		Limit:   limit,
	})
	if err != nil {
		r.logger.Debug("stream query failed", "relay", relayURL, "error", err)
		return nil
	}
	return events
}

// queryKinds is the event-kind set one ladder query requests: always kind-0,
// plus kind-10002 when includeRelayLists, plus kind-3 when the resolver harvests
// contacts. Widening to kind-3 is opt-in so profile-only callers keep their
// narrow query.
func (r *ProfileResolver) queryKinds(includeRelayLists bool) []int {
	kinds := []int{0}
	if includeRelayLists {
		kinds = append(kinds, 10002)
	}
	if r.harvestContacts {
		kinds = append(kinds, nostr.KindFollowList)
	}
	return kinds
}

// demuxProfileBatch sorts a relay's returned events into a relayBatch — kind-0
// profiles, kind-10002 relay lists, and kind-3 follow lists — keeping the newest
// event per author for each kind.
func demuxProfileBatch(events []*nostr.Event) relayBatch {
	batch := relayBatch{
		profiles:   make(map[string]*Profile),
		relayLists: make(map[string]*nostr.Event),
		contacts:   make(map[string]*nostr.Event),
	}

	profileEvents := make(map[string]*nostr.Event)
	for _, ev := range events {
		switch ev.Kind {
		case 0:
			if existing, ok := profileEvents[ev.PubKey]; !ok || ev.CreatedAt > existing.CreatedAt {
				profileEvents[ev.PubKey] = ev
			}
		case 10002:
			if existing, ok := batch.relayLists[ev.PubKey]; !ok || ev.CreatedAt > existing.CreatedAt {
				batch.relayLists[ev.PubKey] = ev
			}
		case nostr.KindFollowList:
			if existing, ok := batch.contacts[ev.PubKey]; !ok || ev.CreatedAt > existing.CreatedAt {
				batch.contacts[ev.PubKey] = ev
			}
		}
	}

	for pk, ev := range profileEvents {
		p := ParseProfile(pk, ev.Content)
		p.Event = ev
		batch.profiles[pk] = p
	}

	return batch
}

// queryMultiRelaysBatch fans one author-batched query out to every relay in
// parallel, bounded by maxOutboxRelays when set. Profiles merge first-wins and
// relay lists newest-wins, exactly as the sequential walk did. A non-nil
// onFound receives each relay's newly-found profiles (the delta against relays
// that already answered) as that relay returns; deltas are disjoint, so a
// consumer sees each pubkey at most once.
func (r *ProfileResolver) queryMultiRelaysBatch(ctx context.Context, relayURLs []string, pubkeys []string, onFound func(map[string]*Profile)) relayBatch {
	all := relayBatch{
		profiles:   make(map[string]*Profile),
		relayLists: make(map[string]*nostr.Event),
		contacts:   make(map[string]*nostr.Event),
	}

	var mu sync.Mutex
	var g errgroup.Group
	if r.maxOutboxRelays > 0 {
		g.SetLimit(r.maxOutboxRelays)
	}

	for _, relayURL := range relayURLs {
		g.Go(func() error {
			batch := r.queryBatch(ctx, relayURL, pubkeys, true)

			mu.Lock()
			delta := make(map[string]*Profile, len(batch.profiles))
			for pk, p := range batch.profiles {
				if _, ok := all.profiles[pk]; !ok {
					all.profiles[pk] = p
					delta[pk] = p
				}
			}
			for pk, ev := range batch.relayLists {
				if existing, ok := all.relayLists[pk]; !ok || ev.CreatedAt > existing.CreatedAt {
					all.relayLists[pk] = ev
				}
			}
			for pk, ev := range batch.contacts {
				if existing, ok := all.contacts[pk]; !ok || ev.CreatedAt > existing.CreatedAt {
					all.contacts[pk] = ev
				}
			}
			mu.Unlock()

			if onFound != nil && len(delta) > 0 {
				onFound(delta)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		r.logger.Debug("multi-relay batch query", "error", err)
	}

	return all
}

func (r *ProfileResolver) authenticateRelay(ctx context.Context, relay *nostr.Relay) {
	nsec, err := r.readNsec()
	if err != nil {
		r.logger.Debug("cannot read nsec for auth", "error", err)
		return
	}

	secHex, err := NsecToHex(nsec)
	if err != nil {
		r.logger.Debug("invalid nsec for auth", "error", err)
		return
	}

	sign := func(ev *nostr.Event) error {
		return ev.Sign(secHex)
	}
	if err := relay.PerformAuth(ctx, sign); err != nil {
		r.logger.Debug("NIP-42 auth failed", "relay", relay.URL, "error", err)
	}
}

func (r *ProfileResolver) readNsec() (string, error) {
	if r.nsecFile == "" {
		return "", fmt.Errorf("no nsec file configured")
	}
	data, err := os.ReadFile(r.nsecFile)
	if err != nil {
		return "", fmt.Errorf("reading nsec file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// homePublisher is the transport seam for home-relay write-back: *nostr.Relay
// satisfies it in production; tests substitute a recorder.
type homePublisher interface {
	Publish(ctx context.Context, ev nostr.Event) error
}

// homeWriter returns a publisher connected to the home relay — the shared
// pool connection when configured, otherwise a fresh connection authenticated
// with the resolver's nsec — plus a release func the caller must invoke.
func (r *ProfileResolver) homeWriter(ctx context.Context) (homePublisher, func(), error) {
	if r.pool != nil {
		pooled, err := r.pool.EnsureRelay(r.homeRelay)
		if err != nil {
			return nil, nil, fmt.Errorf("pool EnsureRelay: %w", err)
		}
		return pooled, func() {}, nil
	}

	nsec, err := r.readNsec()
	if err != nil {
		return nil, nil, err
	}
	secHex, err := NsecToHex(nsec)
	if err != nil {
		return nil, nil, err
	}

	fresh, err := nostr.RelayConnect(ctx, r.homeRelay)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting home relay: %w", err)
	}
	release := func() {
		if closeErr := fresh.Close(); closeErr != nil {
			r.logger.Debug("closing relay", "relay", fresh.URL, "error", closeErr)
		}
	}
	sign := func(ev *nostr.Event) error {
		return ev.Sign(secHex)
	}
	if err := fresh.PerformAuth(ctx, sign); err != nil {
		r.logger.Debug("auth failed for home write", "error", err)
	}
	return fresh, release, nil
}

// writeEventsToHome publishes externally obtained events back to the home
// relay — the unconditional write-back rule: anything pulled from a rung
// below home is republished so the next query resolves at rung one.
func (r *ProfileResolver) writeEventsToHome(ctx context.Context, events []*nostr.Event) {
	if len(events) == 0 {
		return
	}

	writeCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	pub, release, err := r.homeWriter(writeCtx)
	if err != nil {
		r.logger.Debug("cannot reach home relay for write-back", "error", err)
		return
	}
	defer release()

	r.publishAllToHome(writeCtx, pub, events)
}

// publishAllToHome publishes each event exactly as received — the original
// signed bytes, never a reconstruction. Failures are aggregated into one
// summary log line per batch: a rejected or unreachable home relay fails
// every event the same way, and per-event lines only flood the log with
// hundreds of copies of the same few causes.
func (r *ProfileResolver) publishAllToHome(ctx context.Context, pub homePublisher, events []*nostr.Event) {
	published := 0
	failures := make(map[string]int)

	for _, ev := range events {
		if pubErr := pub.Publish(ctx, *ev); pubErr != nil {
			failures[pubErr.Error()]++
			continue
		}
		published++
	}

	if len(failures) > 0 {
		failed := 0
		for _, n := range failures {
			failed += n
		}
		r.logger.Debug("home write-back incomplete",
			"published", published,
			"failed", failed,
			"causes", failures,
		)
	}
}

func ParseProfile(hexPubkey, contentJSON string) *Profile {
	npub, err := HexToNpub(hexPubkey)
	if err != nil {
		npub = hexPubkey
	}
	p := &Profile{Npub: npub}

	var data map[string]any
	if err := json.Unmarshal([]byte(contentJSON), &data); err != nil {
		p.Name = TruncatedNpub(hexPubkey)
		return p
	}

	p.DisplayName = extractStr(data, "display_name")
	p.Name = extractStr(data, "name")
	p.Picture = extractStr(data, "picture")
	p.About = extractStr(data, "about")
	p.NIP05 = extractStr(data, "nip05")
	p.Banner = extractStr(data, "banner")
	p.Website = extractStr(data, "website")
	p.Lud16 = extractStr(data, "lud16")

	if p.DisplayName == "" && p.Name == "" {
		p.Name = TruncatedNpub(hexPubkey)
	}

	return p
}

func TruncatedNpub(pubkeyHex string) string {
	npub, err := nip19.EncodePublicKey(pubkeyHex)
	if err != nil {
		if len(pubkeyHex) > 16 {
			return pubkeyHex[:16] + "…"
		}
		return pubkeyHex
	}
	if len(npub) > 16 {
		return npub[:12] + "…" + npub[len(npub)-4:]
	}
	return npub
}

func fallbackProfile(npub string) *Profile {
	return &Profile{
		Npub: npub,
		Name: truncateNpub(npub),
	}
}

func truncateNpub(npub string) string {
	if len(npub) > 16 {
		return npub[:12] + "…" + npub[len(npub)-4:]
	}
	return npub
}

// ParseOutboxRelays returns the write relays (markers "" or "write") declared
// in a kind-10002 event, normalized and deduplicated: hosts are lowercased and
// trailing slashes stripped, so duplicate spellings of one relay collapse to a
// single entry instead of splitting citation counts and opening redundant
// connections. Only wss:// relays survive — non-TLS ws:// entries are an
// author's local addresses (LAN, mesh) that a server can never reach, and
// subscribing to them only stalls the fan-out.
func ParseOutboxRelays(event *nostr.Event) []string {
	var relays []string
	seen := make(map[string]bool)
	for _, tag := range event.Tags {
		if len(tag) < 2 || tag[0] != "r" {
			continue
		}
		marker := ""
		if len(tag) >= 3 {
			marker = strings.ToLower(tag[2])
		}
		if marker != "" && marker != relayMarkerWrite {
			continue
		}
		if !nostr.IsValidRelayURL(tag[1]) {
			continue
		}
		url := nostr.NormalizeURL(tag[1])
		if !strings.HasPrefix(url, "wss://") || seen[url] {
			continue
		}
		seen[url] = true
		relays = append(relays, url)
	}
	return relays
}

func mergeRelayLists(a, b map[string]*nostr.Event) map[string]*nostr.Event {
	merged := make(map[string]*nostr.Event, len(a)+len(b))
	maps.Copy(merged, a)
	for k, v := range b {
		if existing, ok := merged[k]; !ok || v.CreatedAt > existing.CreatedAt {
			merged[k] = v
		}
	}
	return merged
}

func extractStr(data map[string]any, key string) string {
	val, ok := data[key]
	if !ok {
		return ""
	}
	str, ok := val.(string)
	if !ok {
		return ""
	}
	return str
}
