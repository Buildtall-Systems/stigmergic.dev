// Package vault materializes Nostr vaults as content sources. Discovery turns
// relay URLs and owner npubs into vault descriptors, fetch turns one
// descriptor into wire events, an exported bundle, and a link resolver, and
// the synthetic filesystem serves the result read-only: concepts as markdown
// files, attachment bytes from the Blossom store the vault names on its own
// root event.
package vault

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// Descriptor names one discovered vault: whose it is and what it is called.
type Descriptor struct {
	Owner string
	Name  string
}

// Discover finds every vault of every watched owner: one kind 30101 query
// per owner over the configured relays, each root d-tag in the vault family
// yielding one descriptor. Zero relays or zero owners yields an empty list,
// not an error; an owner npub that will not decode is an error, mirroring
// how the auth allowlist treats a bad npub.
func Discover(ctx context.Context, pool *nostr.SimplePool, relays, owners []string) ([]Descriptor, error) {
	if len(relays) == 0 || len(owners) == 0 {
		return nil, nil
	}
	var found []Descriptor
	seen := map[Descriptor]bool{}
	for _, owner := range owners {
		ownerHex, err := btknostr.NpubToHex(owner)
		if err != nil {
			return nil, fmt.Errorf("vault owner %q: %w", owner, err)
		}
		sets := btknostr.FetchAddressableSet(ctx, pool, relays, ownerHex, lists.KindListSet)
		for _, d := range vaultsOf(owner, sets) {
			if !seen[d] {
				seen[d] = true
				found = append(found, d)
			}
		}
	}
	slices.SortFunc(found, func(a, b Descriptor) int {
		if c := strings.Compare(a.Owner, b.Owner); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return found, nil
}

// vaultsOf classifies one owner's kind 30101 d-tags: every DTagRoot in the
// vault family is one vault, named by its instance name. Everything else,
// other list families and malformed names alike, is passed over.
func vaultsOf(owner string, sets map[string]*nostr.Event) []Descriptor {
	var vaults []Descriptor
	for dTag := range sets {
		name, class := lists.ClassifyVaultDTag(dTag)
		if class != lists.DTagRoot {
			continue
		}
		vaults = append(vaults, Descriptor{Owner: owner, Name: name})
	}
	return vaults
}
