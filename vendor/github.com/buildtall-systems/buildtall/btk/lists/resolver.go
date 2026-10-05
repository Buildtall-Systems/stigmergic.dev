package lists

import (
	"context"
	"log/slog"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

const (
	// DefaultResolveCoordCeiling caps how many foreign coordinates one
	// traversal fetches. It is a budget guard rather than a rule of the
	// ontology: it was chosen for a web page rendering a tree, where a partial
	// view with placeholders beats a slow page. A caller that needs total
	// coverage raises it or removes it through ResolvePolicy.
	DefaultResolveCoordCeiling = 50
	// DefaultResolveTimeout bounds one whole traversal, and exists for the
	// same reason and with the same escape.
	DefaultResolveTimeout = 5 * time.Second

	ReasonNotFound  = "not found"
	ReasonTimedOut  = "timed out"
	ReasonTruncated = "truncated"
)

// ResolvePolicy carries the three bounds on a cross-author traversal. Every
// field is optional: a zero takes the declared default, so a caller states
// only what it means to change.
//
// A negative CoordCeiling removes the ceiling and a negative Timeout removes
// the deadline, leaving the caller's own context as the only bound. Depth has
// no such escape, because the list-of-lists NUD mandates that a depth limit exist; NormalizeDepth
// clamps it to DepthCeiling.
//
// The bounds are a policy rather than a constant because btk serves consumers
// with opposite needs: a web page wants a fast partial answer, and a headless
// corpus pull wants a complete one at whatever cost.
type ResolvePolicy struct {
	Timeout      time.Duration
	MaxDepth     int
	CoordCeiling int
}

// Normalize resolves the policy against the declared defaults, leaving any
// negative escape intact for the caller that asked for it.
func (p ResolvePolicy) Normalize() ResolvePolicy {
	p.MaxDepth = NormalizeDepth(p.MaxDepth)
	if p.CoordCeiling == 0 {
		p.CoordCeiling = DefaultResolveCoordCeiling
	}
	if p.Timeout == 0 {
		p.Timeout = DefaultResolveTimeout
	}
	return p
}

type CoordQuerier interface {
	QueryBlocking(ctx context.Context, filter nostr.Filter, relays []string) ([]*nostr.Event, error)
}

type coordRef struct {
	coord  string
	pubkey string
	dTag   string
	kind   int
}

// ResolveForeign fetches the foreign list coordinates seed references, and
// recurses into what it fetches, under the bounds policy states. A zero policy
// takes the declared defaults; see ResolvePolicy for the escapes.
//
// It returns the events it resolved and the census of every reference it did
// not, keyed by coordinate with the reason. Nothing is dropped silently: a
// reference the bounds cut is stamped ReasonTruncated.
func ResolveForeign(ctx context.Context, q CoordQuerier, seed []*nostr.Event, relays []string, policy ResolvePolicy) ([]*nostr.Event, map[string]string) {
	policy = policy.Normalize()

	rctx := ctx
	if policy.Timeout > 0 {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(ctx, policy.Timeout)
		defer cancel()
	}

	known := make(map[string]bool, len(seed))
	for _, ev := range seed {
		known[CoordinateFromEvent(ev)] = true
	}

	unresolved := make(map[string]string)
	frontier := referencedListCoords(seed, known, unresolved)

	var resolved []*nostr.Event
	attempted := 0

	for depth := 1; depth <= policy.MaxDepth && len(frontier) > 0; depth++ {
		if policy.CoordCeiling > 0 && attempted+len(frontier) > policy.CoordCeiling {
			allowed := max(policy.CoordCeiling-attempted, 0)
			for _, ref := range frontier[allowed:] {
				unresolved[ref.coord] = ReasonTruncated
			}
			slog.Warn("resolve: coordinate ceiling reached, truncating",
				"ceiling", policy.CoordCeiling, "dropped", len(frontier)-allowed)
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
			"depth", policy.MaxDepth, "dropped", len(frontier))
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
