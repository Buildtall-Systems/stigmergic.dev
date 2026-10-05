package lists

import "slices"

// Harvest is the deduplicated membership of a list subtree: the npubs its
// leaves follow and the addressable content its leaves contain. The river
// reads Npubs as its author set; wave C reads Addresses as article
// addresses, so both are collected now.
type Harvest struct {
	// SavedAt maps an address to the unix second at which the owner saved it.
	// An address absent from the map carries no save time, which is every
	// address a foreign writer or a pre-convention writer wrote. A diamond
	// that reaches one address twice keeps the later save, so the value does
	// not depend on the order the walk happens to take.
	SavedAt map[string]int64
	// Relays maps a member value (npub or address) to the distinct non-empty
	// relay hints its items carried, in walk order. A member whose items
	// carried no hint is absent. The hints are the raw tag values: btk never
	// connects from them, so it does not normalize them; a client that
	// connects normalizes on its own side (buildtall android does).
	Relays    map[string][]string
	Npubs     []string
	Addresses []string
}

// HarvestSubtree locates the node bearing the target coordinate within the
// tree rooted at root and collects its subtree's membership. An "a" item on
// a composing kind 30101 node is a composition edge the walk already
// descends; on a leaf it is a content address and is collected. Dedupe
// spans the whole subtree because cycle detection is path-scoped, so a
// diamond duplicates its shared node. The boolean reports whether the
// target coordinate names a resolved list in the tree.
func HarvestSubtree(root *TreeNode, target string) (Harvest, bool) {
	node := FindSubtree(root, target)
	if node == nil {
		return Harvest{}, false
	}
	h := Harvest{SavedAt: make(map[string]int64), Relays: make(map[string][]string)}
	harvestNode(node, &h, make(map[string]bool), make(map[string]bool))
	return h, true
}

// FindSubtree locates the node bearing the target coordinate within the tree
// rooted at node, depth-first in item order. Unresolved placeholders never
// match: a caller holding the returned node holds a resolved list.
func FindSubtree(node *TreeNode, target string) *TreeNode {
	if node == nil {
		return nil
	}
	if node.List != nil && node.List.Coord == target {
		return node
	}
	for _, child := range node.Children {
		if found := FindSubtree(child, target); found != nil {
			return found
		}
	}
	return nil
}

func harvestNode(node *TreeNode, h *Harvest, seenNpub, seenAddr map[string]bool) {
	if node.List != nil {
		for _, item := range node.List.Items {
			switch {
			case item.IsPubkey():
				if !seenNpub[item.Value] {
					seenNpub[item.Value] = true
					h.Npubs = append(h.Npubs, item.Value)
				}
				h.retainHint(item.Value, item.RelayHint)
			case item.IsAddressable() && node.List.Kind != KindListSet:
				if !seenAddr[item.Value] {
					seenAddr[item.Value] = true
					h.Addresses = append(h.Addresses, item.Value)
				}
				if item.SavedAt > h.SavedAt[item.Value] {
					h.SavedAt[item.Value] = item.SavedAt
				}
				h.retainHint(item.Value, item.RelayHint)
			}
		}
	}
	for _, child := range node.Children {
		harvestNode(child, h, seenNpub, seenAddr)
	}
}

func (h *Harvest) retainHint(member, hint string) {
	if hint == "" || slices.Contains(h.Relays[member], hint) {
		return
	}
	h.Relays[member] = append(h.Relays[member], hint)
}
