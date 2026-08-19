package nostr

import (
	"context"

	"github.com/nbd-wtf/go-nostr"
)

// FetchBlossomServers returns the "server" tag values of the newest
// KindBlossomServerList event authorHex published, in tag order, or nothing
// when the author published none. The kind is replaceable rather than
// addressable, so the reduction is newest-wins by CreatedAt across relays with
// no d tag in the filter. The fetch is best-effort and bounded by ctx; callers
// should supply a deadline.
func FetchBlossomServers(ctx context.Context, pool *nostr.SimplePool, relays []string, authorHex string) []string {
	filter := nostr.Filter{
		Kinds:   []int{KindBlossomServerList},
		Authors: []string{authorHex},
		Limit:   1,
	}

	var latest *nostr.Event
	for ev := range pool.FetchMany(ctx, relays, filter) {
		latest = pickNewer(latest, ev.Event)
	}
	return blossomServers(latest)
}

// blossomServers extracts the server URLs of one server-list event, in tag
// order, skipping empty values; a nil event states none.
func blossomServers(ev *nostr.Event) []string {
	if ev == nil {
		return nil
	}
	var servers []string
	for _, tag := range ev.Tags {
		if len(tag) >= 2 && tag[0] == TagServer && tag[1] != "" {
			servers = append(servers, tag[1])
		}
	}
	return servers
}
