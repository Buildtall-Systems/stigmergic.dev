package vault

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btkvault "github.com/buildtall-systems/buildtall/btk/vault"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
	"github.com/buildtall-systems/buildtall/btk/okf"
)

// Vault is one fetched vault, ready to serve: its wire events, the exported
// bundle, the resolver indexed over the same sets, and the Blossom stores it
// names in preference order.
type Vault struct {
	Events   okf.VaultEvents
	Bundle   *okf.Bundle
	Resolver *okf.Resolver
	Domain   lists.Domain
	Descriptor
	// Servers trails for field alignment: its len and cap words keep the
	// GC's pointer-scan region off the end of the struct.
	Servers []string
}

// Load fetches one discovered vault: resolve the subject, fetch the root,
// the sets, and the documents, take the store list from the root's
// okf-server tags with the owner's kind 10063 as the fallback, export the
// bundle, and index the resolver over the same sets with the first store as
// base. A vault naming no store still loads: attachments resolve to nothing
// and render dangling.
func Load(ctx context.Context, pool *nostr.SimplePool, relays []string, d Descriptor, log *slog.Logger) (*Vault, error) {
	subject, err := btkvault.Resolve(d.Name, d.Owner, relays, nil)
	if err != nil {
		return nil, fmt.Errorf("vault %q of %s: %w", d.Name, d.Owner, err)
	}
	events, err := btkvault.Fetch(ctx, pool, subject, log)
	if err != nil {
		return nil, fmt.Errorf("vault %q of %s: %w", d.Name, d.Owner, err)
	}
	if events.Root == nil {
		return nil, fmt.Errorf("vault %q of %s: no root event on the configured relays", d.Name, d.Owner)
	}
	servers := okf.Servers(events.Root)
	if len(servers) == 0 {
		servers = btknostr.FetchBlossomServers(ctx, pool, relays, subject.OwnerHex)
	}
	bundle, err := okf.ExportVault(subject.Domain, d.Owner, events, log)
	if err != nil {
		return nil, fmt.Errorf("vault %q of %s: %w", d.Name, d.Owner, err)
	}
	base := ""
	if len(servers) > 0 {
		base = servers[0]
	}
	return &Vault{
		Descriptor: d,
		Domain:     subject.Domain,
		Events:     events,
		Bundle:     bundle,
		Resolver:   okf.NewResolver(subject.Domain, events.Sets, base),
		Servers:    servers,
	}, nil
}

// docModTimes maps each concept file's path to its document event's
// created_at, which is what the filesystem reports as the file's ModTime.
// A coordinate that will not parse back to a path stamps nothing, matching
// the export's own permissiveness.
func (v *Vault) docModTimes() map[string]time.Time {
	times := make(map[string]time.Time, len(v.Events.Documents))
	for coord, ev := range v.Events.Documents {
		if ev == nil {
			continue
		}
		_, _, dTag, err := btknostr.ParseCoordinate(coord)
		if err != nil {
			continue
		}
		p, err := okf.DTagToPath(v.Domain, dTag)
		if err != nil {
			continue
		}
		times[p+conceptExt] = ev.CreatedAt.Time().UTC()
	}
	return times
}
