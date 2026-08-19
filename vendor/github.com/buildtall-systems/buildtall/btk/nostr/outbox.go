package nostr

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"sync"

	"github.com/nbd-wtf/go-nostr"
	"golang.org/x/sync/errgroup"
)

// AuthorChunkMax caps how many authors one relay request may carry.
// Public relays silently drop oversized author batches — measured zero results
// for 567-author requests the same relays answer when split — and a dropped
// batch is indistinguishable from "author has no event here". 50 is the
// ratified fleet value, not a tuning knob; a relay dropping batches this size
// is a finding to report, not a reason to retune.
const AuthorChunkMax = 50

// OutboxReport names the authors an outbox fetch could not reach, split by
// cause so the gap is legible rather than silent.
type OutboxReport struct {
	// NoRelayList holds the npubs with no kind-10002 found anywhere on the
	// discovery ladder — unreachable by outbox routing no matter how relays
	// are selected.
	NoRelayList []string
	// Uncovered holds the npubs that declared usable write relays, none of
	// which were selected before the coverage target or relay ceiling was
	// reached.
	Uncovered []string
}

// FetchByOutbox fetches the newest event per requested kind for each author,
// queried from the authors' declared NIP-65 write relays. Relay lists are
// discovered home-first with the aggregator tier descending only for authors
// the home relay is missing; relays are then selected by greedy coverage
// until coverageTarget (a fraction; 1 or more means full coverage) or the
// WithMaxOutboxRelays ceiling, and each selected relay is queried with only
// its own authors, chunked at AuthorChunkMax. Every event obtained from
// a relay other than home — discovered relay lists included — is written back
// to the home relay. Results are newest-per-kind, so the call is meant for
// replaceable kinds.
func (r *ProfileResolver) FetchByOutbox(ctx context.Context, npubs []string, kinds []int, coverageTarget float64) (map[string]map[int]*nostr.Event, OutboxReport, error) {
	hexToNpub := make(map[string]string, len(npubs))
	authors := make([]string, 0, len(npubs))
	for _, npub := range npubs {
		hexPubkey, err := NpubToHex(npub)
		if err != nil {
			return nil, OutboxReport{}, fmt.Errorf("decoding npub %q: %w", npub, err)
		}
		if _, ok := hexToNpub[hexPubkey]; !ok {
			authors = append(authors, hexPubkey)
		}
		hexToNpub[hexPubkey] = npub
	}

	writeBackCtx := ctx
	if r.maxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.maxDuration)
		defer cancel()
	}

	events, noRelayList, uncovered := r.fetchByOutboxHex(ctx, writeBackCtx, authors, kinds, coverageTarget)

	byNpub := make(map[string]map[int]*nostr.Event, len(events))
	for hexPubkey, kindEvents := range events {
		byNpub[hexToNpub[hexPubkey]] = kindEvents
	}
	return byNpub, OutboxReport{
		NoRelayList: npubsOf(noRelayList, hexToNpub),
		Uncovered:   npubsOf(uncovered, hexToNpub),
	}, nil
}

// OutboxStream is the result of one bounded article-stream read-through:
// the articles fetched from the authors' declared write relays, the newest
// kind-0 profile per author keyed by npub, and the coverage report.
type OutboxStream struct {
	Articles []*nostr.Event
	Profiles map[string]*nostr.Event
	Report   OutboxReport
}

// FetchStreamByOutbox fetches up to perAuthor kind-30023 articles and the
// newest kind-0 profile for each author, from the authors' declared NIP-65
// write relays under the same discovery ladder, coverage target, and relay
// ceiling as FetchByOutbox. This path is read-through by construction: it
// never touches the write-back seam, so nothing it fetches — discovered
// relay lists included — is ever published to the home relay.
func (r *ProfileResolver) FetchStreamByOutbox(ctx context.Context, npubs []string, perAuthor int, coverageTarget float64) (OutboxStream, error) {
	hexToNpub := make(map[string]string, len(npubs))
	authors := make([]string, 0, len(npubs))
	for _, npub := range npubs {
		hexPubkey, err := NpubToHex(npub)
		if err != nil {
			return OutboxStream{}, fmt.Errorf("decoding npub %q: %w", npub, err)
		}
		if _, ok := hexToNpub[hexPubkey]; !ok {
			authors = append(authors, hexPubkey)
		}
		hexToNpub[hexPubkey] = npub
	}

	if r.maxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.maxDuration)
		defer cancel()
	}

	relayLists := r.relayListsFromTier(ctx, []string{r.homeRelay}, authors)

	var missing []string
	for _, pk := range authors {
		if _, ok := relayLists[pk]; !ok {
			missing = append(missing, pk)
		}
	}
	if len(missing) > 0 && len(r.relayListRelays) > 0 {
		maps.Copy(relayLists, r.relayListsFromTier(ctx, r.relayListRelays, missing))
	}

	writeRelays := make(map[string][]string, len(relayLists))
	var noRelayList []string
	for _, pk := range authors {
		ev, ok := relayLists[pk]
		if !ok {
			noRelayList = append(noRelayList, pk)
			continue
		}
		writeRelays[pk] = ParseOutboxRelays(ev)
	}
	slices.Sort(noRelayList)

	sel := selectOutboxRelays(writeRelays, coverageTarget, r.maxOutboxRelays)

	articles, profiles := r.queryAssignedStreams(ctx, sel.assignment, perAuthor)

	byNpub := make(map[string]*nostr.Event, len(profiles))
	for hexPubkey, ev := range profiles {
		byNpub[hexToNpub[hexPubkey]] = ev
	}
	return OutboxStream{
		Articles: articles,
		Profiles: byNpub,
		Report: OutboxReport{
			NoRelayList: npubsOf(noRelayList, hexToNpub),
			Uncovered:   npubsOf(sel.uncovered, hexToNpub),
		},
	}, nil
}

// queryAssignedStreams executes a stream selection: per selected relay, one
// per-author article query bounded by perAuthor, plus chunked kind-0
// batches for the relay's assigned authors, relays in parallel bounded by
// maxOutboxRelays. Articles are deduplicated by event ID across relays and
// capped at perAuthor newest per author; profiles keep the newest per
// author, hex-keyed.
func (r *ProfileResolver) queryAssignedStreams(ctx context.Context, assignment map[string][]string, perAuthor int) ([]*nostr.Event, map[string]*nostr.Event) {
	byAuthor := make(map[string][]*nostr.Event)
	profiles := make(map[string]*nostr.Event)
	seen := make(map[string]bool)
	var mu sync.Mutex
	var g errgroup.Group
	if r.maxOutboxRelays > 0 {
		g.SetLimit(r.maxOutboxRelays)
	}
	for relayURL, assigned := range assignment {
		g.Go(func() error {
			for _, author := range assigned {
				events := r.queryStreamBatch(ctx, relayURL, author, perAuthor)

				mu.Lock()
				for _, ev := range events {
					if ev.Kind != nostr.KindArticle || ev.PubKey != author || seen[ev.ID] {
						continue
					}
					seen[ev.ID] = true
					byAuthor[author] = append(byAuthor[author], ev)
				}
				mu.Unlock()
			}
			for chunk := range slices.Chunk(assigned, AuthorChunkMax) {
				for _, ev := range r.queryKindsBatch(ctx, relayURL, chunk, []int{nostr.KindProfileMetadata}) {
					if ev.Kind != nostr.KindProfileMetadata {
						continue
					}
					mu.Lock()
					if existing, ok := profiles[ev.PubKey]; !ok || ev.CreatedAt > existing.CreatedAt {
						profiles[ev.PubKey] = ev
					}
					mu.Unlock()
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		r.logger.Debug("outbox stream fetch", "error", err)
	}

	var articles []*nostr.Event
	for _, events := range byAuthor {
		slices.SortFunc(events, func(a, b *nostr.Event) int {
			return cmp.Compare(b.CreatedAt, a.CreatedAt)
		})
		if perAuthor > 0 && len(events) > perAuthor {
			events = events[:perAuthor]
		}
		articles = append(articles, events...)
	}
	return articles, profiles
}

func npubsOf(hexPubkeys []string, hexToNpub map[string]string) []string {
	if len(hexPubkeys) == 0 {
		return nil
	}
	npubs := make([]string, len(hexPubkeys))
	for i, pk := range hexPubkeys {
		npubs[i] = hexToNpub[pk]
	}
	return npubs
}

// fetchByOutboxHex is the hex-keyed core of FetchByOutbox: ladder, parse,
// select, scoped fetch, write-back. writeBackCtx is the caller's pre-deadline
// context — by the time the write-back runs, the shared fetch deadline is
// spent, so it must not be inherited.
func (r *ProfileResolver) fetchByOutboxHex(ctx, writeBackCtx context.Context, authors []string, kinds []int, coverageTarget float64) (map[string]map[int]*nostr.Event, []string, []string) {
	relayLists := r.relayListsFromTier(ctx, []string{r.homeRelay}, authors)

	var external []*nostr.Event

	var missing []string
	for _, pk := range authors {
		if _, ok := relayLists[pk]; !ok {
			missing = append(missing, pk)
		}
	}
	if len(missing) > 0 && len(r.relayListRelays) > 0 {
		for pk, ev := range r.relayListsFromTier(ctx, r.relayListRelays, missing) {
			relayLists[pk] = ev
			external = append(external, ev)
		}
	}

	writeRelays := make(map[string][]string, len(relayLists))
	var noRelayList []string
	for _, pk := range authors {
		ev, ok := relayLists[pk]
		if !ok {
			noRelayList = append(noRelayList, pk)
			continue
		}
		writeRelays[pk] = ParseOutboxRelays(ev)
	}
	slices.Sort(noRelayList)

	sel := selectOutboxRelays(writeRelays, coverageTarget, r.maxOutboxRelays)

	all, fetchedExternal := r.queryAssignedRelays(ctx, sel.assignment, kinds)
	external = append(external, fetchedExternal...)

	results := make(map[string]map[int]*nostr.Event)
	for _, ev := range all {
		kindEvents := results[ev.PubKey]
		if kindEvents == nil {
			kindEvents = make(map[int]*nostr.Event)
			results[ev.PubKey] = kindEvents
		}
		if existing, ok := kindEvents[ev.Kind]; !ok || ev.CreatedAt > existing.CreatedAt {
			kindEvents[ev.Kind] = ev
		}
	}

	if len(external) > 0 {
		r.publishHome(writeBackCtx, external)
	}

	return results, noRelayList, sel.uncovered
}

// queryAssignedRelays executes an outbox selection: each selected relay is
// queried for only its assigned authors, chunked at AuthorChunkMax,
// relays in parallel bounded by maxOutboxRelays. It returns every event
// received, plus the subset obtained from relays other than home
// (deduplicated by event ID), which is the write-back set.
func (r *ProfileResolver) queryAssignedRelays(ctx context.Context, assignment map[string][]string, kinds []int) (all, external []*nostr.Event) {
	seen := make(map[string]bool)
	var mu sync.Mutex
	var g errgroup.Group
	if r.maxOutboxRelays > 0 {
		g.SetLimit(r.maxOutboxRelays)
	}
	for relayURL, assigned := range assignment {
		g.Go(func() error {
			for chunk := range slices.Chunk(assigned, AuthorChunkMax) {
				events := r.queryKindsBatch(ctx, relayURL, chunk, kinds)

				mu.Lock()
				for _, ev := range events {
					all = append(all, ev)
					if relayURL != r.homeRelay && !seen[ev.ID] {
						seen[ev.ID] = true
						external = append(external, ev)
					}
				}
				mu.Unlock()
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		r.logger.Debug("outbox scoped fetch", "error", err)
	}
	return all, external
}

// relayListsFromTier queries one discovery-ladder tier for kind-10002 relay
// lists, fanning out across the tier's relays with every author batch chunked
// at AuthorChunkMax, merged newest-wins per author.
func (r *ProfileResolver) relayListsFromTier(ctx context.Context, relays, authors []string) map[string]*nostr.Event {
	merged := make(map[string]*nostr.Event)
	var mu sync.Mutex
	var g errgroup.Group
	if r.maxOutboxRelays > 0 {
		g.SetLimit(r.maxOutboxRelays)
	}
	for _, relayURL := range relays {
		g.Go(func() error {
			for chunk := range slices.Chunk(authors, AuthorChunkMax) {
				for _, ev := range r.queryKindsBatch(ctx, relayURL, chunk, []int{nostr.KindRelayListMetadata}) {
					if ev.Kind != nostr.KindRelayListMetadata {
						continue
					}
					mu.Lock()
					if existing, ok := merged[ev.PubKey]; !ok || ev.CreatedAt > existing.CreatedAt {
						merged[ev.PubKey] = ev
					}
					mu.Unlock()
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		r.logger.Debug("relay-list tier query", "error", err)
	}
	return merged
}

// outboxSelection maps each selected relay to the authors whose kind-10002
// names it, and lists the authors no selected relay covers. Uncovered authors
// are a reported fact rather than a silent gap: they are exactly the authors
// the outbox wave will not reach.
type outboxSelection struct {
	assignment map[string][]string
	uncovered  []string
}

// selectOutboxRelays greedily picks write relays until coverageTarget — a
// fraction of the authors holding at least one usable write relay; values of
// 1 or more mean full coverage — is met or maxRelays relays are selected,
// whichever comes first (0 means unbounded). Each pick is the relay covering
// the most not-yet-covered authors, ties broken toward the lexicographically
// smaller URL so selection is deterministic. Per-author relay lists are capped
// at MaxWriteRelaysPerAuthor in declaration order — the batch-path enforcement
// of the fleet rule. Authors with no usable write relay land directly in
// uncovered.
func selectOutboxRelays(writeRelays map[string][]string, coverageTarget float64, maxRelays int) outboxSelection {
	authorsFor := make(map[string][]string)
	coverableCount := 0
	var uncovered []string

	for author, relays := range writeRelays {
		if len(relays) > MaxWriteRelaysPerAuthor {
			relays = relays[:MaxWriteRelaysPerAuthor]
		}
		if len(relays) == 0 {
			uncovered = append(uncovered, author)
			continue
		}
		coverableCount++
		for _, url := range relays {
			authorsFor[url] = append(authorsFor[url], author)
		}
	}

	target := coverableCount
	if coverageTarget < 1 {
		target = int(math.Ceil(coverageTarget * float64(coverableCount)))
	}

	candidates := make([]string, 0, len(authorsFor))
	for url := range authorsFor {
		candidates = append(candidates, url)
	}
	slices.Sort(candidates)

	covered := make(map[string]bool, coverableCount)
	assignment := make(map[string][]string)

	for len(covered) < target {
		if maxRelays > 0 && len(assignment) >= maxRelays {
			break
		}
		best := ""
		bestGain := 0
		for _, url := range candidates {
			if _, taken := assignment[url]; taken {
				continue
			}
			gain := 0
			for _, author := range authorsFor[url] {
				if !covered[author] {
					gain++
				}
			}
			if gain > bestGain {
				best = url
				bestGain = gain
			}
		}
		if bestGain == 0 {
			break
		}
		authors := slices.Clone(authorsFor[best])
		slices.Sort(authors)
		assignment[best] = authors
		for _, author := range authors {
			covered[author] = true
		}
	}

	for _, authors := range authorsFor {
		for _, author := range authors {
			if !covered[author] {
				covered[author] = true
				uncovered = append(uncovered, author)
			}
		}
	}
	slices.Sort(uncovered)

	return outboxSelection{assignment: assignment, uncovered: uncovered}
}
