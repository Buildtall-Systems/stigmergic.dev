package lists

import "github.com/nbd-wtf/go-nostr"

// Read-side deletion. A kind 5 the owner signed names the coordinates and
// edition ids it retires; per NIP-09 it covers every edition whose created_at
// is not newer than its own. A reader applies the tombstones itself, so a
// deleted list vanishes from every page whether or not the relay honors "a"
// deletions.

// KindsWithDeletion returns the kinds plus kind 5, so one filter fetches the
// editions and the tombstones that retire them in a single round trip.
func KindsWithDeletion(kinds []int) []int {
	out := make([]int, 0, len(kinds)+1)
	out = append(out, kinds...)
	return append(out, nostr.KindDeletion)
}

// SplitTombstones separates the kind 5 events from the editions in a mixed
// fetch, preserving each side's order. The editions are what
// DedupeNewestAddressable takes; a kind 5 carries no d-tag and would collapse
// into one entry there.
func SplitTombstones(events []*nostr.Event) (editions, tombstones []*nostr.Event) {
	for _, ev := range events {
		if ev.Kind == nostr.KindDeletion {
			tombstones = append(tombstones, ev)
			continue
		}
		editions = append(editions, ev)
	}
	return editions, tombstones
}

// ApplyTombstones drops every edition a tombstone by the same author names:
// by coordinate when the tombstone's created_at is at or after the edition's,
// and by edition id outright. A tombstone by another author retires nothing,
// because only the author may delete what the author signed.
func ApplyTombstones(editions, tombstones []*nostr.Event) []*nostr.Event {
	if len(tombstones) == 0 {
		return editions
	}
	retiredCoords := make(map[string]nostr.Timestamp)
	retiredIDs := make(map[string]bool)
	for _, tomb := range tombstones {
		for _, tag := range tomb.Tags {
			if len(tag) < 2 {
				continue
			}
			switch tag[0] {
			case "a":
				key := tomb.PubKey + "|" + tag[1]
				if tomb.CreatedAt > retiredCoords[key] {
					retiredCoords[key] = tomb.CreatedAt
				}
			case "e":
				if tag[1] != "" {
					retiredIDs[tomb.PubKey+"|"+tag[1]] = true
				}
			}
		}
	}

	kept := make([]*nostr.Event, 0, len(editions))
	for _, ev := range editions {
		if retiredIDs[ev.PubKey+"|"+ev.ID] {
			continue
		}
		if stamp, ok := retiredCoords[ev.PubKey+"|"+CoordinateFromEvent(ev)]; ok && stamp >= ev.CreatedAt {
			continue
		}
		kept = append(kept, ev)
	}
	return kept
}
