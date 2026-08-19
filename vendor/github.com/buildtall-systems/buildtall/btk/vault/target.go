package vault

import (
	"fmt"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
	"github.com/buildtall-systems/buildtall/btk/okf"
)

// Target is the vault an invocation names: which vault, whose, and any relays
// the naming itself carried.
type Target struct {
	Vault  string
	Owner  string
	Relays []string
}

// TargetFromNaddr admits only an address that classifies into the vault family.
// The scope here is vaults, and an naddr naming any other region of the
// ontology is rejected by name rather than exported into something misshapen.
func TargetFromNaddr(naddr string) (Target, error) {
	pointer, err := btknostr.DecodeNaddr(naddr)
	if err != nil {
		return Target{}, fmt.Errorf("reading the vault address: %w", err)
	}
	if pointer.Kind != lists.KindListSet {
		return Target{}, fmt.Errorf(
			"that address names a kind %d event; vault export handles vaults only, whose roots are kind %d",
			pointer.Kind, lists.KindListSet)
	}

	name, class := lists.ClassifyVaultDTag(pointer.Identifier)
	if class != lists.DTagRoot {
		return Target{}, fmt.Errorf(
			"that address names %q; vault export handles vaults only, whose roots are named %q followed by the vault name",
			pointer.Identifier, lists.VaultFamilyPrefix)
	}

	owner, err := btknostr.HexToNpub(pointer.PublicKey)
	if err != nil {
		return Target{}, fmt.Errorf("reading the vault owner from the address: %w", err)
	}
	return Target{Vault: name, Owner: owner, Relays: pointer.Relays}, nil
}

// Summarize counts what an export wrote, so the verb's last word is what landed
// on disk rather than what was asked for.
func Summarize(d *okf.Directory) (dirs, concepts, citations int) {
	dirs = 1
	concepts = len(d.Concepts)
	citations = len(d.Citations)
	for _, child := range d.Children() {
		childDirs, childConcepts, childCitations := Summarize(child)
		dirs += childDirs
		concepts += childConcepts
		citations += childCitations
	}
	return dirs, concepts, citations
}
