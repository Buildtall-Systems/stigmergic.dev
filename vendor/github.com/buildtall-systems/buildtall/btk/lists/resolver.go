package lists

import (
	"context"
	"log/slog"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

const (
	ResolveCoordCeiling = 50
	resolveTimeout      = 5 * time.Second

	ReasonNotFound  = "not found"
	ReasonTimedOut  = "timed out"
	ReasonTruncated = "truncated"
)

type CoordQuerier interface {
	QueryBlocking(ctx context.Context, filter nostr.Filter, relays []string) ([]*nostr.Event, error)
}

type coordRef struct {
	coord  string
	pubkey string
	dTag   string
	kind   int
}

func ResolveForeign(ctx context.Context, q CoordQuerier, seed []*nostr.Event, relays []string) ([]*nostr.Event, map[string]string) {
	return ResolveForeignDepth(ctx, q, seed, relays, DefaultMaxDepth)
}

// ResolveForeignDepth is ResolveForeign with a configured traversal depth,
// normalized against the declared policy.
func ResolveForeignDepth(ctx context.Context, q CoordQuerier, seed []*nostr.Event, relays []string, maxDepth int) ([]*nostr.Event, map[string]string) {
	maxDepth = NormalizeDepth(maxDepth)

	rctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	known := make(map[string]bool, len(seed))
	for _, ev := range seed {
		known[CoordinateFromEvent(ev)] = true
	}

	unresolved := make(map[string]string)
	frontier := referencedListCoords(seed, known, unresolved)

	var resolved []*nostr.Event
	attempted := 0

	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		if attempted+len(frontier) > ResolveCoordCeiling {
			allowed := max(ResolveCoordCeiling-attempted, 0)
			for _, ref := range frontier[allowed:] {
				unresolved[ref.coord] = ReasonTruncated
			}
			slog.Warn("resolve: coordinate ceiling reached, truncating",
				"ceiling", ResolveCoordCeiling, "dropped", len(frontier)-allowed)
			frontier = frontier[:allowed]
			if len(frontier) == 0 {
				break
			}
		}

		fetched := fetchCoords(rctx, q, frontier, relays, unresolved)
		attempted += len(frontier)

		for _, ev := range fetched {
			coord := CoordinateFromEvent(ev)
			if known[coord] {
				continue
			}
			known[coord] = true
			resolved = append(resolved, ev)
		}
		for coord := range unresolved {
			known[coord] = true
		}

		frontier = referencedListCoords(fetched, known, unresolved)
	}

	if len(frontier) > 0 {
		for _, ref := range frontier {
			unresolved[ref.coord] = ReasonTruncated
		}
		slog.Warn("resolve: depth limit reached, truncating",
			"depth", maxDepth, "dropped", len(frontier))
	}

	return resolved, unresolved
}

func fetchCoords(ctx context.Context, q CoordQuerier, refs []coordRef, relays []string, unresolved map[string]string) []*nostr.Event {
	byAuthor := make(map[string][]coordRef)
	for _, ref := range refs {
		byAuthor[ref.pubkey] = append(byAuthor[ref.pubkey], ref)
	}

	var fetched []*nostr.Event
	for author, authorRefs := range byAuthor {
		kindSet := make(map[int]bool, len(authorRefs))
		kinds := make([]int, 0, len(authorRefs))
		dTags := make([]string, 0, len(authorRefs))
		wanted := make(map[string]bool, len(authorRefs))
		for _, ref := range authorRefs {
			if !kindSet[ref.kind] {
				kindSet[ref.kind] = true
				kinds = append(kinds, ref.kind)
			}
			dTags = append(dTags, ref.dTag)
			wanted[ref.coord] = true
		}

		filter := nostr.Filter{
			Kinds:   kinds,
			Authors: []string{author},
			Tags:    nostr.TagMap{"d": dTags},
		}

		events, err := q.QueryBlocking(ctx, filter, relays)
		if err != nil {
			reason := ReasonNotFound
			if ctx.Err() != nil {
				reason = ReasonTimedOut
			}
			for _, ref := range authorRefs {
				unresolved[ref.coord] = reason
			}
			slog.Warn("resolve: fetch failed", "author_coords", len(authorRefs), "err", err)
			continue
		}

		got := make(map[string]bool)
		for _, ev := range events {
			coord := CoordinateFromEvent(ev)
			if wanted[coord] && !got[coord] {
				got[coord] = true
				fetched = append(fetched, ev)
			}
		}
		for _, ref := range authorRefs {
			if !got[ref.coord] {
				reason := ReasonNotFound
				if ctx.Err() != nil {
					reason = ReasonTimedOut
				}
				unresolved[ref.coord] = reason
			}
		}
	}

	return fetched
}

func referencedListCoords(events []*nostr.Event, known map[string]bool, unresolved map[string]string) []coordRef {
	listKinds := make(map[int]bool, len(ListKinds))
	for _, k := range ListKinds {
		listKinds[k] = true
	}

	seen := make(map[string]bool)
	var refs []coordRef
	for _, ev := range events {
		for _, tag := range ev.Tags {
			if len(tag) < 2 || tag[0] != "a" {
				continue
			}
			coord := tag[1]
			if known[coord] || seen[coord] {
				continue
			}
			if unresolved[coord] != "" {
				continue
			}
			kind, pubkey, dTag, err := btknostr.ParseCoordinate(coord)
			if err != nil || !listKinds[kind] {
				continue
			}
			seen[coord] = true
			refs = append(refs, coordRef{coord: coord, kind: kind, pubkey: pubkey, dTag: dTag})
		}
	}
	return refs
}
