// Package vault moves a vault between the relays that hold it and an OKF
// bundle on disk. A vault is a region of the personal ontology: one kind 30101
// root, a kind 30004 curation set per directory, and a kind 30023 long-form
// note per document, every d-tag carrying the member's path verbatim.
//
// Both directions name a vault the same way, read the same relays, and fetch
// the same events. They differ in what they do with what they find, not in how
// they find it, so the naming, the tiers, the pool, and the fetch live here and
// the two verbs are left with what they each do alone.
package vault

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// Subject is the resolved subject of an invocation: which vault, whose, the
// domain its d-tags live in, and the relays it is read from. The domain trails
// for field alignment, as it ends in an int.
type Subject struct {
	Relays   []string
	Owner    string
	OwnerHex string
	Domain   lists.Domain
}

// Resolve completes a named vault into everything a verb needs before it
// touches a relay. The name is the instance name, without the family prefix.
//
// The tiers are passed rather than read from configuration, so this package
// stays clear of how any one caller spells its settings.
func Resolve(name, ownerNpub string, configured, hinted []string) (Subject, error) {
	domain, err := lists.VaultDomain(name)
	if err != nil {
		return Subject{}, fmt.Errorf("vault name: %w", err)
	}
	ownerHex, err := btknostr.NpubToHex(ownerNpub)
	if err != nil {
		return Subject{}, fmt.Errorf("vault owner: %w", err)
	}
	return Subject{
		Domain:   domain,
		Owner:    ownerNpub,
		OwnerHex: ownerHex,
		Relays:   readTiers(configured, hinted),
	}, nil
}

// readTiers puts the configured tiers first and appends whatever relays the
// address itself hinted at, so a hint widens a read without displacing the
// home relay.
func readTiers(configured, hinted []string) []string {
	tiers := make([]string, 0, len(configured)+len(hinted))
	seen := make(map[string]bool, len(configured)+len(hinted))
	for _, relay := range configured {
		if relay == "" || seen[relay] {
			continue
		}
		seen[relay] = true
		tiers = append(tiers, relay)
	}
	for _, relay := range hinted {
		if relay == "" || seen[relay] {
			continue
		}
		seen[relay] = true
		tiers = append(tiers, relay)
	}
	return tiers
}

// NewReadPool authenticates reactively: the handler fires only when a relay
// challenges, so an unauthenticated relay is never stalled by an AUTH that
// nobody asked for.
func NewReadPool(ctx context.Context, nsec string, log *slog.Logger) *nostr.SimplePool {
	if nsec == "" {
		return btknostr.NewPool(ctx)
	}
	return btknostr.NewPoolWithAuth(ctx, btknostr.NsecAuthHandler(nsec, log))
}

// NewWritePool authenticates at connect time. A relay that gates only writes
// never sends auth-required on a read, so the reactive handler would never
// fire and every publish would be rejected.
func NewWritePool(ctx context.Context, nsec string, log *slog.Logger) *nostr.SimplePool {
	return btknostr.NewPoolWithProactiveAuth(ctx, btknostr.NsecAuthHandler(nsec, log))
}
