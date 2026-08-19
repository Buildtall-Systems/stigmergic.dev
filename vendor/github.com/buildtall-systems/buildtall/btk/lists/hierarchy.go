package lists

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

const MaxDepth = DefaultMaxDepth

type TreeNode struct {
	List       *List
	Unresolved *UnresolvedRef
	Children   []*TreeNode
	Depth      int
}

type UnresolvedRef struct {
	Coord  string
	Reason string
}

func NewTreeNode(list *List, depth int) *TreeNode {
	return &TreeNode{
		List:     list,
		Children: make([]*TreeNode, 0),
		Depth:    depth,
	}
}

func (n *TreeNode) AddChild(child *TreeNode) {
	n.Children = append(n.Children, child)
}

func (n *TreeNode) HasChildren() bool {
	return len(n.Children) > 0
}

func (n *TreeNode) ItemCount() int {
	if n.List == nil {
		return 0
	}

	count := 0
	for _, item := range n.List.Items {
		if !item.IsAddressable() {
			count++
		}
	}
	return count
}

func (n *TreeNode) ChildListCount() int {
	if n.List == nil {
		return 0
	}

	count := 0
	for _, item := range n.List.Items {
		if item.IsAddressable() {
			count++
		}
	}
	return count
}

type HierarchyBuilder struct {
	lists      map[string]*List
	unresolved map[string]string
	visited    map[string]bool
	maxDepth   int
}

func NewHierarchyBuilder(lists []*List) *HierarchyBuilder {
	return NewHierarchyBuilderWithUnresolved(lists, nil)
}

func NewHierarchyBuilderWithUnresolved(lists []*List, unresolved map[string]string) *HierarchyBuilder {
	listMap := make(map[string]*List)
	for _, l := range lists {
		listMap[l.Coord] = l
	}
	return &HierarchyBuilder{
		lists:      listMap,
		unresolved: unresolved,
		visited:    make(map[string]bool),
		maxDepth:   DefaultMaxDepth,
	}
}

// WithMaxDepth configures the builder's traversal depth limit, normalized
// against the declared policy (non-positive takes the default; the ceiling
// clamps), and returns the builder for chaining.
func (b *HierarchyBuilder) WithMaxDepth(depth int) *HierarchyBuilder {
	b.maxDepth = NormalizeDepth(depth)
	return b
}

func (b *HierarchyBuilder) Build() []*TreeNode {
	referenced := make(map[string]bool)

	for _, list := range b.lists {
		for _, item := range list.Items {
			if item.IsAddressable() {
				referenced[item.Value] = true
			}
		}
	}

	var roots []*TreeNode
	for coord, list := range b.lists {
		if !referenced[coord] {
			b.visited = make(map[string]bool)
			node := b.buildNode(list, 0)
			if node != nil {
				roots = append(roots, node)
			}
		}
	}

	return roots
}

func (b *HierarchyBuilder) buildNode(list *List, depth int) *TreeNode {
	if depth > b.maxDepth {
		return nil
	}

	if b.visited[list.Coord] {
		return nil
	}

	b.visited[list.Coord] = true
	defer func() { b.visited[list.Coord] = false }()

	node := NewTreeNode(list, depth)

	for _, item := range list.Items {
		if item.IsAddressable() {
			if childList, ok := b.lists[item.Value]; ok {
				if depth+1 > b.maxDepth {
					node.AddChild(&TreeNode{
						Unresolved: &UnresolvedRef{Coord: item.Value, Reason: ReasonTruncated},
						Children:   make([]*TreeNode, 0),
						Depth:      depth + 1,
					})
					continue
				}
				childNode := b.buildNode(childList, depth+1)
				if childNode != nil {
					node.AddChild(childNode)
				}
			} else if reason, ok := b.unresolved[item.Value]; ok && depth+1 <= b.maxDepth {
				node.AddChild(&TreeNode{
					Unresolved: &UnresolvedRef{Coord: item.Value, Reason: reason},
					Children:   make([]*TreeNode, 0),
					Depth:      depth + 1,
				})
			}
		}
	}

	return node
}

// DedupeNewestAddressable keeps the newest event per (kind, d-tag): relays
// should do this for addressable kinds, but a multi-relay fetch can still
// surface competing revisions.
func DedupeNewestAddressable(events []*nostr.Event) []*nostr.Event {
	newest := make(map[string]*nostr.Event, len(events))
	var order []string
	for _, ev := range events {
		key := fmt.Sprintf("%d:%s", ev.Kind, GetDTag(ev))
		have, ok := newest[key]
		if !ok {
			order = append(order, key)
			newest[key] = ev
			continue
		}
		if ev.CreatedAt > have.CreatedAt {
			newest[key] = ev
		}
	}
	out := make([]*nostr.Event, 0, len(order))
	for _, key := range order {
		out = append(out, newest[key])
	}
	return out
}

// UnresolvedRefs names every composition reference pointing outside the
// fetched set. A traversal that resolves nothing foreign treats any such
// reference as not found, so it lands in the census rather than
// disappearing silently. Only list kinds are references: other addressable
// items (article coordinates on curation sets) are content, not lists.
func UnresolvedRefs(events []*nostr.Event) map[string]string {
	listKinds := make(map[int]bool, len(ListKinds))
	for _, k := range ListKinds {
		listKinds[k] = true
	}
	present := make(map[string]bool, len(events))
	for _, ev := range events {
		present[CoordinateFromEvent(ev)] = true
	}
	refs := make(map[string]string)
	for _, ev := range events {
		for _, item := range GetItems(ev) {
			if !item.IsAddressable() || present[item.Value] {
				continue
			}
			kind, _, _, err := btknostr.ParseCoordinate(item.Value)
			if err != nil || !listKinds[kind] {
				continue
			}
			refs[item.Value] = ReasonNotFound
		}
	}
	return refs
}

func EventsToLists(events []*nostr.Event) []*List {
	var lists []*List

	for _, ev := range events {
		list, err := ParseList(ev)
		if err != nil {
			continue
		}
		lists = append(lists, list)
	}

	return lists
}

func BuildHierarchy(events []*nostr.Event) []*TreeNode {
	lists := EventsToLists(events)
	builder := NewHierarchyBuilder(lists)
	return builder.Build()
}

func BuildHierarchyForOwner(events []*nostr.Event, unresolved map[string]string, ownerPubkey string) []*TreeNode {
	var lists []*List
	for _, ev := range events {
		list, err := ParseList(ev)
		if err != nil {
			continue
		}
		list.Foreign = ev.PubKey != ownerPubkey
		lists = append(lists, list)
	}
	builder := NewHierarchyBuilderWithUnresolved(lists, unresolved)
	return builder.Build()
}
