package vault

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
	"github.com/buildtall-systems/buildtall/btk/okf"
)

// Fetch reads everything either verb needs: the root, the owner's curation
// sets, and the documents those sets name. A vault nobody has published yet
// comes back with a nil root and nothing else, which is an answer rather than a
// failure: exporting refuses it, publishing creates it.
func Fetch(ctx context.Context, pool *nostr.SimplePool, s Subject, log *slog.Logger) (okf.VaultEvents, error) {
	root := btknostr.FetchLatestAddressable(ctx, pool, s.Relays, s.OwnerHex, lists.KindListSet, s.Domain.RootDTag)
	sets := btknostr.FetchAddressableSet(ctx, pool, s.Relays, s.OwnerHex, lists.KindCurationSet)
	addrs, err := okf.DocumentAddresses(s.Domain, s.Owner, sets)
	if err != nil {
		return okf.VaultEvents{}, err
	}

	docs, err := btknostr.FetchLongFormByAddress(ctx, pool, s.Relays, addrs, log)
	if err != nil {
		return okf.VaultEvents{}, fmt.Errorf("fetching the vault's documents: %w", err)
	}

	events := vaultEvents(root, sets, docs)
	log.Debug("fetched the vault",
		"root", s.Domain.RootDTag, "sets", len(sets), "requested", len(addrs), "documents", len(events.Documents))
	return events, nil
}

// vaultEvents assembles fetched material into the form both verbs consume. The
// documents are kept as the events they arrived as, keyed by coordinate,
// because both directions need every tag: an export states them all in the
// bundle and a publish rebuilds the document from what the bundle states. A
// projection standing between the two, keeping the names it can spell and
// discarding the rest, is what made a vault lose tags it never asked to lose.
func vaultEvents(root *nostr.Event, sets map[string]*nostr.Event, docs []*nostr.Event) okf.VaultEvents {
	documents := make(map[string]*nostr.Event, len(docs))
	for _, ev := range docs {
		dTag := lists.GetDTag(ev)
		if dTag == "" {
			continue // a long-form note carrying no d-tag addresses nothing and replaces nothing
		}
		documents[lists.FormatCoordinate(lists.KindLongFormNote, ev.PubKey, dTag)] = ev
	}
	return okf.VaultEvents{Root: root, Sets: sets, Documents: documents}
}
