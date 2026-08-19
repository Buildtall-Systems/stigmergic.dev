package vault

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// listSet builds one synthetic kind 30101 for the classification cases.
func listSet(ownerHex, dTag string) *nostr.Event {
	return &nostr.Event{
		Kind:   lists.KindListSet,
		PubKey: ownerHex,
		Tags:   nostr.Tags{{btknostr.TagD, dTag}},
	}
}

func TestVaultsOfClassifiesRoots(t *testing.T) {
	owner := testOwner(t)
	ownerHex := mustHexOf(t, owner)
	sets := map[string]*nostr.Event{}
	for _, dTag := range []string{
		"vault-notes",          // a vault root
		"vault-work",           // another vault root
		"vault-notes/thoughts", // a member, not a root
		"vault-Bad_Name",       // instance name outside the grammar
		"drss",                 // another family entirely
		"vault-notes/_root",    // the companion set
	} {
		sets[dTag] = listSet(ownerHex, dTag)
	}

	got := vaultsOf(owner, sets)
	names := map[string]bool{}
	for _, d := range got {
		if d.Owner != owner {
			t.Errorf("descriptor owner = %q, want %q", d.Owner, owner)
		}
		names[d.Name] = true
	}
	if len(got) != 2 || !names[testVaultName] || !names["work"] {
		t.Errorf("vaults = %v, want exactly notes and work", got)
	}
}

func TestVaultsOfAnEmptySet(t *testing.T) {
	if got := vaultsOf(testOwner(t), nil); len(got) != 0 {
		t.Errorf("vaults = %v, want nothing", got)
	}
}
