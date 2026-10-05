package nostr

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// FetchAuthorNames reads the given authors' kind 0 profiles from relays and
// returns the name each one states, keyed by npub. It is the confined
// counterpart to FetchArticlesByAuthorsSince: the same chunk bound, the same
// subscription seam, and no ladder at all. It reaches the relays it is handed
// and nothing else, publishes nothing, and keeps no cache between calls.
//
// An author who states neither display_name nor name yields no entry, so a
// caller can tell absence from emptiness. Nothing is substituted for a name
// that does not resolve: a truncated npub is not a name, and a caller that
// speaks its corpus aloud must never be handed one. ProfileResolver does
// substitute one, which is a second reason this read is not built on it; the
// first is that its outbox wave cannot be switched off by configuration.
//
// The map and the error are independent. A relay that fails before EOSE is
// reported and still leaves whatever landed in the map, and a failed chunk does
// not end the read, because a name that will not resolve costs a label and
// never a corpus. log must be non-nil.
func FetchAuthorNames(ctx context.Context, pool *nostr.SimplePool, relays []string, npubs []string, log *slog.Logger) (map[string]string, error) {
	return fetchAuthorNames(ctx, poolSubscribe(pool), relays, npubs, log)
}

func fetchAuthorNames(ctx context.Context, subscribe subscribeFunc, relays []string, npubs []string, log *slog.Logger) (map[string]string, error) {
	hexToNpub := make(map[string]string, len(npubs))
	authorsHex := make([]string, 0, len(npubs))
	for _, npub := range npubs {
		hex, err := NpubToHex(npub)
		if err != nil {
			// A member that will not decode has already failed the pull by the
			// time this read runs. Skipping it here costs one label rather than
			// the whole sweep's labels.
			log.Warn("skipping an author whose npub will not decode", "npub", npub, "error", err)
			continue
		}
		if _, seen := hexToNpub[hex]; seen {
			continue
		}
		hexToNpub[hex] = npub
		authorsHex = append(authorsHex, hex)
	}

	names := make(map[string]string, len(authorsHex))
	if len(authorsHex) == 0 {
		return names, nil
	}

	newest := make(map[string]*nostr.Event, len(authorsHex))
	var errs []error
	for chunk := range slices.Chunk(authorsHex, AuthorChunkMax) {
		events, err := fetchUntilEOSE(ctx, subscribe, relays, nostr.Filter{
			Kinds:   []int{nostr.KindProfileMetadata},
			Authors: chunk,
		}, log)
		if err != nil {
			errs = append(errs, err)
		}
		for _, ev := range events {
			if prior, seen := newest[ev.PubKey]; seen && ev.CreatedAt <= prior.CreatedAt {
				continue
			}
			newest[ev.PubKey] = ev
		}
	}

	for hex, ev := range newest {
		name := profileName(ev.Content)
		if name == "" {
			continue
		}
		names[hexToNpub[hex]] = name
	}

	if len(errs) > 0 {
		return names, errors.Join(errs...)
	}
	return names, nil
}

// profileName is the name a kind 0 event states: display_name when it carries
// one, then name. It returns the empty string when the content will not parse
// or states neither, which the caller reads as absence. Surrounding whitespace
// is not a statement, so it is trimmed before the test.
func profileName(contentJSON string) string {
	var data map[string]any
	if err := json.Unmarshal([]byte(contentJSON), &data); err != nil {
		return ""
	}
	if displayName := strings.TrimSpace(extractStr(data, "display_name")); displayName != "" {
		return displayName
	}
	return strings.TrimSpace(extractStr(data, "name"))
}
