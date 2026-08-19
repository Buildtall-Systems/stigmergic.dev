package lists

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// The follow deposit ceremony: the unsigned events that land one followed
// npub or one followed list coordinate in an owner's forest, and the
// revisions that take it back out. drss and btcli are peer consumers of one
// build, so a deposit prepared for a browser signature and a deposit signed
// by a CLI key are the same events.

// BuildFollowEvents produces the unsigned events for one follow into the
// target list, in publish order (referenced before referencer). A composing
// kind 30101 target deposits into its companion leaf, derived per the
// domain's companion-suffix declaration, minting and referencing the
// companion on first use; a leaf-kind target takes the deposit directly. A
// target not reachable from the canonical root gains a root revision
// referencing it, so every deposit stays discoverable from the domain root.
// A feed already present returns ErrItemAlreadyPresent.
func BuildFollowEvents(domain Domain, existing []*nostr.Event, userNpub, feedNpub, feedRelayHint, listsRelayHint, targetCoord string) ([]*nostr.Event, error) {
	userHex, err := btknostr.NpubToHex(userNpub)
	if err != nil {
		return nil, fmt.Errorf("converting user npub: %w", err)
	}
	targetKind, targetAuthor, targetDTag, err := btknostr.ParseCoordinate(targetCoord)
	if err != nil {
		return nil, fmt.Errorf("parsing target coordinate: %w", err)
	}
	if targetAuthor != userHex {
		return nil, fmt.Errorf("target list %s is not authored by the user", targetCoord)
	}

	member := Item{Type: "p", Value: feedNpub, RelayHint: feedRelayHint}
	rootCoord := FormatCoordinate(KindListSet, userHex, domain.RootDTag)

	var events []*nostr.Event
	switch targetKind {
	case KindListSet:
		events, err = companionDeposit(domain, existing, userNpub, userHex, targetDTag, member, listsRelayHint, rootCoord)
	case domain.LeafKind:
		events, err = leafDeposit(existing, targetCoord, member)
	default:
		err = fmt.Errorf("cannot deposit into kind %d list %s", targetKind, targetCoord)
	}
	if err != nil {
		return nil, err
	}

	if targetCoord != rootCoord && !rootedUnder(existing, userHex, rootCoord, targetCoord) {
		rootRevision, rootErr := rootReference(domain, existing, userNpub, targetCoord, listsRelayHint)
		if rootErr != nil {
			return nil, rootErr
		}
		if rootRevision != nil {
			events = append(events, rootRevision)
		}
	}
	return events, nil
}

// companionDeposit lands the member in the target node's companion leaf.
// The companion d-tag is the node's d-tag plus the domain suffix. A fresh
// companion is followed by the node revision referencing it; only the
// canonical root may be minted alongside, because any other absent node has
// nothing for the ceremony to revise.
func companionDeposit(domain Domain, existing []*nostr.Event, userNpub, userHex, targetDTag string, member Item, listsRelayHint, rootCoord string) ([]*nostr.Event, error) {
	companionDTag := targetDTag + domain.CompanionSuffix
	companion := newestByKindDTag(existing, domain.LeafKind, companionDTag)

	var events []*nostr.Event
	if companion == nil {
		leaf, err := NewListEvent(domain.LeafKind, userNpub, companionDTag, domain.CompanionTitle, "", "", []Item{member})
		if err != nil {
			return nil, fmt.Errorf("building companion leaf: %w", err)
		}
		events = append(events, leaf)
	} else {
		updated, err := AddItemToList(companion, member)
		if err != nil {
			return nil, err
		}
		events = append(events, updated)
	}

	companionRef := Item{
		Type:      "a",
		Value:     FormatCoordinate(domain.LeafKind, userHex, companionDTag),
		RelayHint: listsRelayHint,
	}
	targetCoord := FormatCoordinate(KindListSet, userHex, targetDTag)
	target := newestByCoord(existing, targetCoord)
	if target == nil {
		if targetCoord != rootCoord {
			return nil, fmt.Errorf("target list %s not found", targetCoord)
		}
		newRoot, err := NewListEvent(KindListSet, userNpub, domain.RootDTag, domain.RootTitle, "", "", []Item{companionRef})
		if err != nil {
			return nil, fmt.Errorf("building user root: %w", err)
		}
		return append(events, newRoot), nil
	}

	updatedTarget, err := AddItemToList(target, companionRef)
	if err != nil {
		if errors.Is(err, ErrItemAlreadyPresent) {
			// The node already references its companion; only the leaf changes.
			return events, nil
		}
		return nil, fmt.Errorf("referencing companion leaf: %w", err)
	}
	return append(events, updatedTarget), nil
}

// leafDeposit lands the member directly in the picked leaf. The leaf must
// exist: pickers offer only lists that do, and minting leaves is the
// companion ceremony's job.
func leafDeposit(existing []*nostr.Event, targetCoord string, member Item) ([]*nostr.Event, error) {
	target := newestByCoord(existing, targetCoord)
	if target == nil {
		return nil, fmt.Errorf("target list %s not found", targetCoord)
	}
	updated, err := AddItemToList(target, member)
	if err != nil {
		return nil, err
	}
	return []*nostr.Event{updated}, nil
}

// rootedUnder reports whether the target coordinate is reachable from the
// canonical root in the owner's forest.
func rootedUnder(existing []*nostr.Event, userHex, rootCoord, targetCoord string) bool {
	for _, root := range BuildHierarchyForOwner(existing, nil, userHex) {
		if root.List == nil || root.List.Coord != rootCoord {
			continue
		}
		return FindSubtree(root, targetCoord) != nil
	}
	return false
}

// rootReference produces the root revision referencing the target, minting
// the root when absent. A root already carrying the reference (the target
// was unreachable through pruning, not missing) yields no event.
func rootReference(domain Domain, existing []*nostr.Event, userNpub, targetCoord, listsRelayHint string) (*nostr.Event, error) {
	ref := Item{Type: "a", Value: targetCoord, RelayHint: listsRelayHint}
	root := newestByKindDTag(existing, KindListSet, domain.RootDTag)
	if root == nil {
		newRoot, err := NewListEvent(KindListSet, userNpub, domain.RootDTag, domain.RootTitle, "", "", []Item{ref})
		if err != nil {
			return nil, fmt.Errorf("building user root: %w", err)
		}
		return newRoot, nil
	}
	updated, err := AddItemToList(root, ref)
	if err != nil {
		if errors.Is(err, ErrItemAlreadyPresent) {
			return nil, nil
		}
		return nil, fmt.Errorf("rooting target list: %w", err)
	}
	return updated, nil
}

// BuildUnfollowEvents produces the unsigned revisions removing the feed from
// every own-author leaf under the write list. The touched leaves are
// siblings in the batch, so it carries no ordering constraint. A feed
// deposited nowhere returns ErrItemNotPresent.
func BuildUnfollowEvents(domain Domain, existing []*nostr.Event, userHex, writeCoord, feedNpub string) ([]*nostr.Event, error) {
	subtree := FindForestSubtree(BuildHierarchyForOwner(existing, nil, userHex), writeCoord)
	if subtree == nil {
		return nil, ErrItemNotPresent
	}

	var events []*nostr.Event
	seen := make(map[string]bool)
	var walk func(n *TreeNode) error
	walk = func(n *TreeNode) error {
		if n.List != nil && n.List.Kind == domain.LeafKind && !seen[n.List.Coord] {
			seen[n.List.Coord] = true
			if leafHasMember(n.List, feedNpub) {
				leaf := newestByCoord(existing, n.List.Coord)
				if leaf == nil {
					return fmt.Errorf("leaf %s resolved in the tree but missing from the fetch", n.List.Coord)
				}
				updated, err := RemoveItemFromList(leaf, "p", feedNpub)
				if err != nil {
					return err
				}
				events = append(events, updated)
			}
		}
		for _, child := range n.Children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(subtree); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, ErrItemNotPresent
	}
	return events, nil
}

// BuildListFollowEvents produces the unsigned events for one list follow: an
// "a" reference to the followed coordinate deposited on the destination node.
// The subject may have any author, and its kind must be a legal composition
// kind. The destination is an own-author kind 30101 node and takes the
// reference directly, never through the companion path, because companion
// leaves admit only "p" members. A subject hint carried by the caller's naddr
// wins over the configured lists-relay hint. A destination not reachable from
// the canonical root gains a root revision, and the canonical root is minted
// when it is the destination and absent. A subject already referenced returns
// ErrItemAlreadyPresent.
func BuildListFollowEvents(domain Domain, existing []*nostr.Event, userNpub, subjectCoord, subjectRelayHint, listsRelayHint, targetCoord string) ([]*nostr.Event, error) {
	userHex, err := btknostr.NpubToHex(userNpub)
	if err != nil {
		return nil, fmt.Errorf("converting user npub: %w", err)
	}
	targetKind, targetAuthor, _, err := btknostr.ParseCoordinate(targetCoord)
	if err != nil {
		return nil, fmt.Errorf("parsing target coordinate: %w", err)
	}
	if targetAuthor != userHex {
		return nil, fmt.Errorf("target list %s is not authored by the user", targetCoord)
	}
	if targetKind != KindListSet {
		return nil, fmt.Errorf("cannot deposit a list reference into kind %d list %s", targetKind, targetCoord)
	}
	subjectKind, _, _, err := btknostr.ParseCoordinate(subjectCoord)
	if err != nil {
		return nil, fmt.Errorf("parsing subject coordinate: %w", err)
	}
	if !AllowedCompositionKind(subjectKind) {
		return nil, fmt.Errorf("kind %d list %s is not a composable list", subjectKind, subjectCoord)
	}
	if subjectCoord == targetCoord {
		return nil, fmt.Errorf("list %s cannot follow itself", targetCoord)
	}

	hint := subjectRelayHint
	if hint == "" {
		hint = listsRelayHint
	}
	member := Item{Type: "a", Value: subjectCoord, RelayHint: hint}
	rootCoord := FormatCoordinate(KindListSet, userHex, domain.RootDTag)

	target := newestByCoord(existing, targetCoord)
	if target == nil {
		if targetCoord != rootCoord {
			return nil, fmt.Errorf("target list %s not found", targetCoord)
		}
		newRoot, rootErr := NewListEvent(KindListSet, userNpub, domain.RootDTag, domain.RootTitle, "", "", []Item{member})
		if rootErr != nil {
			return nil, fmt.Errorf("building user root: %w", rootErr)
		}
		return []*nostr.Event{newRoot}, nil
	}
	updated, err := AddItemToList(target, member)
	if err != nil {
		return nil, err
	}
	events := []*nostr.Event{updated}

	if targetCoord != rootCoord && !rootedUnder(existing, userHex, rootCoord, targetCoord) {
		rootRevision, rootErr := rootReference(domain, existing, userNpub, targetCoord, listsRelayHint)
		if rootErr != nil {
			return nil, rootErr
		}
		if rootRevision != nil {
			events = append(events, rootRevision)
		}
	}
	return events, nil
}

// BuildListUnfollowEvents produces the unsigned revisions removing the "a"
// reference from every own-author kind 30101 node under the write list. The
// touched nodes are siblings in the batch, so it carries no ordering
// constraint. A reference deposited nowhere returns ErrItemNotPresent.
func BuildListUnfollowEvents(existing []*nostr.Event, userHex, writeCoord, subjectCoord string) ([]*nostr.Event, error) {
	return removeReferenceUnder(existing, userHex, writeCoord, KindListSet, subjectCoord)
}

// removeReferenceUnder produces the unsigned revisions dropping one "a"
// reference from every own-author list of the given kind under the write
// list. A composition node and a collection differ only in the kind they
// carry the reference on, so one walk serves both withdrawals. A reference
// deposited nowhere returns ErrItemNotPresent.
func removeReferenceUnder(existing []*nostr.Event, userHex, writeCoord string, kind int, coord string) ([]*nostr.Event, error) {
	subtree := FindForestSubtree(BuildHierarchyForOwner(existing, nil, userHex), writeCoord)
	if subtree == nil {
		return nil, ErrItemNotPresent
	}

	var events []*nostr.Event
	seen := make(map[string]bool)
	var walk func(n *TreeNode) error
	walk = func(n *TreeNode) error {
		if n.List != nil && n.List.Kind == kind && !n.List.Foreign && !seen[n.List.Coord] {
			seen[n.List.Coord] = true
			if nodeReferences(n.List, coord) {
				node := newestByCoord(existing, n.List.Coord)
				if node == nil {
					return fmt.Errorf("list %s resolved in the tree but missing from the fetch", n.List.Coord)
				}
				updated, err := RemoveItemFromList(node, "a", coord)
				if err != nil {
					return err
				}
				events = append(events, updated)
			}
		}
		for _, child := range n.Children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(subtree); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, ErrItemNotPresent
	}
	return events, nil
}

// The save ceremony: the unsigned events that put one article into a
// collection and take it back out. A collection is a NIP-51 kind 30004
// curation set under the domain's d-tag prefix, holding "a" coordinates that
// address long-form documents, each stamped with the second the owner saved
// it.

// ErrCollectionExists reports a mint whose d-tag is already held in the
// owner's forest. The caller offers the list holding the name instead of
// writing a second list over it.
var ErrCollectionExists = errors.New("a list already holds that name")

// Collection is the identity a minted collection carries: the three NIP-51
// set metadata fields. The image is optional and the title is not, because
// the title is also the name the d-tag is minted from.
type Collection struct {
	Title       string
	Description string
	Image       string
}

// BuildSaveEvents produces the unsigned events for one article save into the
// target collection, in publish order (referenced before referencer). The
// target is an own-author kind 30004 collection that already exists: minting
// one is BuildNewCollectionEvents's work. The item is an "a" carrying the
// article's own relay hint and the save time. A collection the canonical root
// cannot reach gains a root revision, so every save stays discoverable from
// the domain root. An article already in the collection returns
// ErrItemAlreadyPresent.
func BuildSaveEvents(domain Domain, existing []*nostr.Event, userNpub, articleCoord, articleRelayHint, listsRelayHint string, savedAt int64, targetCoord string) ([]*nostr.Event, error) {
	userHex, err := btknostr.NpubToHex(userNpub)
	if err != nil {
		return nil, fmt.Errorf("converting user npub: %w", err)
	}
	item, err := savedArticle(articleCoord, articleRelayHint, savedAt)
	if err != nil {
		return nil, err
	}
	if err = ownCollectionCoord(userHex, targetCoord); err != nil {
		return nil, err
	}

	target := newestByCoord(existing, targetCoord)
	if target == nil {
		return nil, fmt.Errorf("collection %s not found", targetCoord)
	}
	updated, err := AddItemToList(target, item)
	if err != nil {
		return nil, err
	}
	events := []*nostr.Event{updated}

	rootCoord := FormatCoordinate(KindListSet, userHex, domain.RootDTag)
	if !rootedUnder(existing, userHex, rootCoord, targetCoord) {
		rootRevision, rootErr := rootReference(domain, existing, userNpub, targetCoord, listsRelayHint)
		if rootErr != nil {
			return nil, rootErr
		}
		if rootRevision != nil {
			events = append(events, rootRevision)
		}
	}
	return events, nil
}

// BuildNewCollectionEvents mints a collection holding its first article, then
// hangs it on the owner's tree: on the write list when that is an own
// composing kind 30101 node, and on the canonical root otherwise, since a
// leaf kind composes nothing. The collection is published before the list
// referencing it. A d-tag the owner already holds returns ErrCollectionExists,
// so the caller can offer that list instead of writing over it.
func BuildNewCollectionEvents(domain Domain, existing []*nostr.Event, userNpub string, collection Collection, articleCoord, articleRelayHint, listsRelayHint string, savedAt int64, writeCoord string) ([]*nostr.Event, error) {
	userHex, err := btknostr.NpubToHex(userNpub)
	if err != nil {
		return nil, fmt.Errorf("converting user npub: %w", err)
	}
	item, err := savedArticle(articleCoord, articleRelayHint, savedAt)
	if err != nil {
		return nil, err
	}
	dTag, err := collectionDTag(domain, collection.Title)
	if err != nil {
		return nil, err
	}
	if held := ownDTagHolder(existing, userHex, dTag); held != nil {
		return nil, fmt.Errorf("%w: %q is a %s", ErrCollectionExists, dTag, KindName(held.Kind))
	}

	set, err := NewListEvent(KindCurationSet, userNpub, dTag, collection.Title, collection.Description, collection.Image, []Item{item})
	if err != nil {
		return nil, fmt.Errorf("building the collection: %w", err)
	}
	attachments, err := attachCollection(domain, existing, userNpub, userHex, CoordinateFromEvent(set), listsRelayHint, writeCoord)
	if err != nil {
		return nil, err
	}
	return append([]*nostr.Event{set}, attachments...), nil
}

// BuildUnsaveEvents produces the unsigned revisions removing the article from
// every own-author collection under the write list. The touched collections
// are siblings in the batch, so it carries no ordering constraint. An article
// saved nowhere returns ErrItemNotPresent.
func BuildUnsaveEvents(existing []*nostr.Event, userHex, writeCoord, articleCoord string) ([]*nostr.Event, error) {
	return removeReferenceUnder(existing, userHex, writeCoord, KindCurationSet, articleCoord)
}

// savedArticle renders the item one save deposits: an "a" naming the article,
// the relay hint pointing at it, and the save time. The kit's own validator
// decides what a curation set admits, so a coordinate of the wrong kind and
// an impossible save time are refused here rather than on the relay.
func savedArticle(articleCoord, articleRelayHint string, savedAt int64) (Item, error) {
	articleKind, _, _, err := btknostr.ParseCoordinate(articleCoord)
	if err != nil {
		return Item{}, fmt.Errorf("parsing article coordinate: %w", err)
	}
	item := Item{
		Type:       "a",
		Value:      articleCoord,
		RelayHint:  articleRelayHint,
		SourceKind: articleKind,
		SavedAt:    savedAt,
	}
	if err = ValidateItemForKind(&item, KindCurationSet); err != nil {
		return Item{}, err
	}
	return item, nil
}

// ownCollectionCoord refuses a save target that is not the user's own
// collection: a stranger's list is not ours to revise, and a set of any other
// kind holds something a collection does not.
func ownCollectionCoord(userHex, targetCoord string) error {
	kind, author, _, err := btknostr.ParseCoordinate(targetCoord)
	if err != nil {
		return fmt.Errorf("parsing target coordinate: %w", err)
	}
	if author != userHex {
		return fmt.Errorf("collection %s is not authored by the user", targetCoord)
	}
	if kind != KindCurationSet {
		return fmt.Errorf("cannot save into kind %d list %s; a collection is kind %d", kind, targetCoord, KindCurationSet)
	}
	return nil
}

// collectionDTag renders the d-tag a titled collection takes in the domain:
// the domain prefix and the title's slug. A name ending in the companion
// suffix is refused, because the follow ceremony derives that name from a
// node of its own and would collide with it.
func collectionDTag(domain Domain, title string) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", errors.New("a collection needs a title")
	}
	dTag := domain.DTagPrefix + Slug(title)
	if domain.CompanionSuffix != "" && strings.HasSuffix(dTag, domain.CompanionSuffix) {
		return "", fmt.Errorf("%q mints %q, which names a follow leaf the deposit ceremony owns", title, dTag)
	}
	return dTag, nil
}

// attachCollection produces the revisions hanging a minted collection on the
// owner's tree. An own composing node takes the reference directly, and an
// unrooted one is rooted in the same batch; anything else sends the reference
// to the canonical root, which is minted when it does not exist.
func attachCollection(domain Domain, existing []*nostr.Event, userNpub, userHex, coord, listsRelayHint, writeCoord string) ([]*nostr.Event, error) {
	writeKind, writeAuthor, _, err := btknostr.ParseCoordinate(writeCoord)
	if err != nil {
		return nil, fmt.Errorf("parsing write list coordinate: %w", err)
	}
	if writeAuthor != userHex {
		return nil, fmt.Errorf("write list %s is not authored by the user", writeCoord)
	}

	rootCoord := FormatCoordinate(KindListSet, userHex, domain.RootDTag)
	if writeKind != KindListSet || writeCoord == rootCoord {
		root, rootErr := rootReference(domain, existing, userNpub, coord, listsRelayHint)
		if rootErr != nil {
			return nil, rootErr
		}
		if root == nil {
			return nil, nil
		}
		return []*nostr.Event{root}, nil
	}

	node := newestByCoord(existing, writeCoord)
	if node == nil {
		return nil, fmt.Errorf("write list %s not found", writeCoord)
	}
	revision, err := AddItemToList(node, Item{Type: "a", Value: coord, RelayHint: listsRelayHint})
	if err != nil {
		return nil, fmt.Errorf("referencing the collection: %w", err)
	}
	events := []*nostr.Event{revision}

	if !rootedUnder(existing, userHex, rootCoord, writeCoord) {
		rootRevision, rootErr := rootReference(domain, existing, userNpub, writeCoord, listsRelayHint)
		if rootErr != nil {
			return nil, rootErr
		}
		if rootRevision != nil {
			events = append(events, rootRevision)
		}
	}
	return events, nil
}

// ownDTagHolder returns the owner's own list already holding the d-tag, or
// nil. The search spans kinds, because one name on two kinds is two lists a
// picker cannot tell apart.
func ownDTagHolder(events []*nostr.Event, userHex, dTag string) *nostr.Event {
	for _, ev := range events {
		if ev.PubKey == userHex && GetDTag(ev) == dTag {
			return ev
		}
	}
	return nil
}

// FindForestSubtree locates the node bearing the target coordinate across a
// forest of trees, or nil when no tree holds it.
func FindForestSubtree(roots []*TreeNode, target string) *TreeNode {
	for _, root := range roots {
		if subtree := FindSubtree(root, target); subtree != nil {
			return subtree
		}
	}
	return nil
}

// nodeReferences reports whether the resolved node carries the coordinate.
func nodeReferences(l *List, coord string) bool {
	for _, item := range l.Items {
		if item.IsAddressable() && item.Value == coord {
			return true
		}
	}
	return false
}

// leafHasMember reports whether the resolved leaf carries the npub.
func leafHasMember(l *List, feedNpub string) bool {
	for _, item := range l.Items {
		if item.IsPubkey() && item.Value == feedNpub {
			return true
		}
	}
	return false
}

// newestByKindDTag returns the newest (kind, d-tag) event from an
// already-deduplicated fetch, or nil.
func newestByKindDTag(events []*nostr.Event, kind int, dTag string) *nostr.Event {
	for _, ev := range events {
		if ev.Kind == kind && GetDTag(ev) == dTag {
			return ev
		}
	}
	return nil
}

// newestByCoord returns the newest event bearing the coordinate from an
// already-deduplicated fetch, or nil.
func newestByCoord(events []*nostr.Event, coord string) *nostr.Event {
	for _, ev := range events {
		if CoordinateFromEvent(ev) == coord {
			return ev
		}
	}
	return nil
}
