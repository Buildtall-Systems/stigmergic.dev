package lists

import "github.com/nbd-wtf/go-nostr"

// SavedCoords collects the deduped "a" references held by own-author
// curation sets within the write subtree: exactly the set an unsave would
// revise away, so Save-control state and the unsave walk can never disagree.
// It is the definition of a saved article wherever one is counted.
func SavedCoords(events []*nostr.Event, userHex, writeCoord string) map[string]bool {
	out := map[string]bool{}
	subtree := FindForestSubtree(BuildHierarchyForOwner(events, nil, userHex), writeCoord)
	if subtree == nil {
		return out
	}
	seen := map[string]bool{}
	var walk func(n *TreeNode)
	walk = func(n *TreeNode) {
		if n.Unresolved != nil {
			return
		}
		if n.List != nil && n.List.Kind == KindCurationSet && !n.List.Foreign && !seen[n.List.Coord] {
			seen[n.List.Coord] = true
			for _, item := range n.List.Items {
				if item.IsAddressable() {
					out[item.Value] = true
				}
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(subtree)
	return out
}
