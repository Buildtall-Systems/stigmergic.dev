package nostr

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// FetchLatestAddressable returns the newest addressable (parameterized-replaceable)
// event of the given kind and d-tag authored by authorHex, or nil when none exists.
// The fetch is best-effort and bounded by ctx; callers should supply a deadline.
func FetchLatestAddressable(ctx context.Context, pool *nostr.SimplePool, relays []string, authorHex string, kind int, dtag string) *nostr.Event {
	filter := nostr.Filter{
		Kinds:   []int{kind},
		Authors: []string{authorHex},
		Tags:    nostr.TagMap{"d": []string{dtag}},
		Limit:   1,
	}

	var latest *nostr.Event
	for ev := range pool.FetchMany(ctx, relays, filter) {
		latest = pickNewer(latest, ev.Event)
	}
	return latest
}

// maxAddressableSetEvents bounds the bulk addressable fetch. A filter carrying no limit
// is subject to the relay's own default cap, which silently truncates the result set;
// an explicit generous bound keeps the full set flowing while still bounding a
// misbehaving relay.
const maxAddressableSetEvents = 5000

// FetchAddressableSet returns every addressable (parameterized-replaceable) event of the
// given kind authored by authorHex, keyed by d-tag with the newest event winning per key.
// The fetch is EOSE-bounded and best-effort; callers should supply a ctx deadline. An
// empty map means no events were found (or none were reachable).
func FetchAddressableSet(ctx context.Context, pool *nostr.SimplePool, relays []string, authorHex string, kind int) map[string]*nostr.Event {
	filter := nostr.Filter{
		Kinds:   []int{kind},
		Authors: []string{authorHex},
		Limit:   maxAddressableSetEvents,
	}

	set := make(map[string]*nostr.Event)
	pool.FetchManyReplaceable(ctx, relays, filter).Range(func(key nostr.ReplaceableKey, ev *nostr.Event) bool {
		storeAddressable(set, authorHex, key, ev)
		return true
	})
	return set
}

// storeAddressable records ev under its d-tag key, guarding against events another
// author smuggled into a single-author fetch and keeping the newer event when a key
// somehow repeats.
func storeAddressable(set map[string]*nostr.Event, authorHex string, key nostr.ReplaceableKey, ev *nostr.Event) {
	if key.PubKey != authorHex {
		return
	}
	set[key.D] = pickNewer(set[key.D], ev)
}

// AddToAddressableList fetches the owner's latest kind:dtag list, adds targetHex as a
// "p" tag when absent, and republishes to the first reachable relay via PublishWithAuth.
// It is idempotent: when targetHex is already present it reports (false, nil) without
// publishing. A successful publish reports (true, nil). sign signs both the list event
// and any NIP-42 auth challenge.
func AddToAddressableList(ctx context.Context, pool *nostr.SimplePool, relays []string, ownerHex, targetHex string, kind int, dtag string, sign func(*nostr.Event) error, log *slog.Logger) (bool, error) {
	existing := FetchLatestAddressable(ctx, pool, relays, ownerHex, kind, dtag)

	tags, changed := tagsWithMemberAdded(existing, dtag, targetHex)
	if !changed {
		return false, nil
	}

	if err := republishList(ctx, pool, relays, kind, tags, sign, log); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveFromAddressableList fetches the owner's latest kind:dtag list, drops targetHex's
// "p" tag while preserving the d-tag and every other tag, and republishes via
// PublishWithAuth. When the target is absent (or no list exists) it reports (false, nil)
// without publishing; a successful publish reports (true, nil).
func RemoveFromAddressableList(ctx context.Context, pool *nostr.SimplePool, relays []string, ownerHex, targetHex string, kind int, dtag string, sign func(*nostr.Event) error, log *slog.Logger) (bool, error) {
	existing := FetchLatestAddressable(ctx, pool, relays, ownerHex, kind, dtag)
	if existing == nil {
		return false, nil
	}

	tags, changed := tagsWithMemberRemoved(existing, dtag, targetHex)
	if !changed {
		return false, nil
	}

	if err := republishList(ctx, pool, relays, kind, tags, sign, log); err != nil {
		return false, err
	}
	return true, nil
}

// pickNewer returns whichever event has the greater CreatedAt. Ties keep latest (the
// incumbent), matching a strictly-greater reduction over fetched events; a nil latest is
// always replaced.
func pickNewer(latest, candidate *nostr.Event) *nostr.Event {
	if latest == nil || candidate.CreatedAt > latest.CreatedAt {
		return candidate
	}
	return latest
}

// tagsWithMemberAdded builds the tag set for a list that adds targetHex as a "p" tag.
// It starts from a fresh d-tag, copies every non-d tag from existing (preserving other
// members and any auxiliary tags), and appends the new member. changed is false when
// targetHex is already present, signalling an idempotent no-op.
func tagsWithMemberAdded(existing *nostr.Event, dtag, targetHex string) (tags nostr.Tags, changed bool) {
	if existing != nil {
		for _, tag := range existing.Tags {
			if len(tag) >= 2 && tag[0] == "p" && tag[1] == targetHex {
				return nil, false
			}
		}
	}

	tags = nostr.Tags{{"d", dtag}}
	if existing != nil {
		for _, tag := range existing.Tags {
			if len(tag) >= 1 && tag[0] != "d" {
				tags = append(tags, tag)
			}
		}
	}
	tags = append(tags, nostr.Tag{"p", targetHex})
	return tags, true
}

// tagsWithMemberRemoved builds the tag set for a list that drops targetHex's "p" tag.
// It starts from a fresh d-tag and copies every other tag, omitting the target member.
// changed is false when the target is not present, signalling nothing to publish.
func tagsWithMemberRemoved(existing *nostr.Event, dtag, targetHex string) (tags nostr.Tags, changed bool) {
	tags = nostr.Tags{{"d", dtag}}
	found := false
	for _, tag := range existing.Tags {
		if len(tag) >= 1 && tag[0] == "d" {
			continue
		}
		if len(tag) >= 2 && tag[0] == "p" && tag[1] == targetHex {
			found = true
			continue
		}
		tags = append(tags, tag)
	}
	return tags, found
}

// republishList signs a fresh addressable event carrying tags and publishes it to the
// first reachable relay, authenticating via NIP-42 when challenged. Per-relay failures
// are logged and skipped; an error is returned only when no relay accepts the event.
func republishList(ctx context.Context, pool *nostr.SimplePool, relays []string, kind int, tags nostr.Tags, sign func(*nostr.Event) error, log *slog.Logger) error {
	ev := nostr.Event{
		Kind:      kind,
		Tags:      tags,
		Content:   "",
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
	}
	if err := sign(&ev); err != nil {
		return fmt.Errorf("signing list event: %w", err)
	}

	for _, relayURL := range relays {
		relay, err := pool.EnsureRelay(relayURL)
		if err != nil {
			log.Warn("connecting to relay for list update", "relay", relayURL, "error", err)
			continue
		}
		if err := PublishWithAuth(ctx, relay, ev, sign); err != nil {
			log.Warn("publishing list update", "relay", relayURL, "error", err)
			continue
		}
		log.Info("published list update", "kind", kind, "dtag", listDTag(tags), "relay", relayURL)
		return nil
	}

	return fmt.Errorf("failed to publish list update to any relay")
}

// listDTag returns the value of the d-tag in tags, or "" when absent — used only for
// log annotation.
func listDTag(tags nostr.Tags) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == "d" {
			return tag[1]
		}
	}
	return ""
}
