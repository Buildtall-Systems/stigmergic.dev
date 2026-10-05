package lists

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// The organizer ceremony: the unsigned batches that build an owner's
// hierarchy in a domain. Create, rename, move, merge, and delete each compose
// as one ordered batch from the newest own-author editions, so a batch drss
// prepares for a browser signature and a batch btcli signs with a key are the
// same events. A batch touches each coordinate once (one pending edition per
// coordinate, emitted referenced-before-referencer) and ends with the one
// kind 5 that retires whatever the gesture removed.

// workingSet accumulates a gesture's revisions over the newest own editions.
// A coordinate revised twice in one gesture (the old parent of a moved list
// that also referenced the promoted destination) still yields one edition.
type workingSet struct {
	now       time.Time
	pending   map[string]*nostr.Event
	userNpub  string
	userHex   string
	rootCoord string
	domain    Domain
	existing  []*nostr.Event
	order     []string
	deleted   []*nostr.Event
}

func newWorkingSet(domain Domain, existing []*nostr.Event, userNpub string) (*workingSet, error) {
	userHex, err := btknostr.NpubToHex(userNpub)
	if err != nil {
		return nil, fmt.Errorf("converting user npub: %w", err)
	}
	return &workingSet{
		domain:    domain,
		userNpub:  userNpub,
		userHex:   userHex,
		rootCoord: FormatCoordinate(KindListSet, userHex, domain.RootDTag),
		existing:  existing,
		pending:   make(map[string]*nostr.Event),
		now:       time.Now(),
	}, nil
}

// base returns the newest fetched edition of the coordinate, or nil.
func (ws *workingSet) base(coord string) *nostr.Event {
	return newestByCoord(ws.existing, coord)
}

// isDeleted reports whether the gesture has already retired the coordinate.
func (ws *workingSet) isDeleted(coord string) bool {
	for _, ev := range ws.deleted {
		if CoordinateFromEvent(ev) == coord {
			return true
		}
	}
	return false
}

// current returns the edition the gesture sees for the coordinate: the
// pending revision when one exists, else the newest fetched edition, and nil
// for a coordinate the gesture has retired or never held.
func (ws *workingSet) current(coord string) *nostr.Event {
	if ev, ok := ws.pending[coord]; ok {
		return ev
	}
	if ws.isDeleted(coord) {
		return nil
	}
	return ws.base(coord)
}

// stage records ev as the coordinate's pending edition. The stamp is derived
// once against the fetched base, so a coordinate revised several times in one
// gesture carries one truthful created_at that outranks what the relay holds.
func (ws *workingSet) stage(ev *nostr.Event) {
	coord := CoordinateFromEvent(ev)
	if base := ws.base(coord); base != nil {
		ev.CreatedAt = NextCreatedAt(base, ws.now)
	} else {
		ev.CreatedAt = nostr.Timestamp(ws.now.Unix())
	}
	if _, ok := ws.pending[coord]; !ok {
		ws.order = append(ws.order, coord)
	}
	ws.pending[coord] = ev
}

// dTagHeld reports the own list already holding the d-tag on any kind, apart
// from editions the gesture retires; one name on two kinds is two lists a
// picker cannot tell apart.
func (ws *workingSet) dTagHeld(dTag string) *nostr.Event {
	for _, ev := range ws.pending {
		if GetDTag(ev) == dTag {
			return ev
		}
	}
	for _, ev := range ws.existing {
		if ev.PubKey == ws.userHex && GetDTag(ev) == dTag && !ws.isDeleted(CoordinateFromEvent(ev)) {
			return ev
		}
	}
	return nil
}

// mint stages a new list, refusing a d-tag the owner already holds.
func (ws *workingSet) mint(kind int, dTag, title, description, image string, items []Item) (*nostr.Event, error) {
	if held := ws.dTagHeld(dTag); held != nil {
		return nil, fmt.Errorf("%w: %q is a %s", ErrListExists, dTag, KindName(held.Kind))
	}
	ev, err := NewListEvent(kind, ws.userNpub, dTag, title, description, image, items)
	if err != nil {
		return nil, fmt.Errorf("building %s: %w", dTag, err)
	}
	ws.stage(ev)
	return ev, nil
}

// revised copies the edition with new tags for staging.
func revised(ev *nostr.Event, tags nostr.Tags) *nostr.Event {
	return &nostr.Event{
		Kind:    ev.Kind,
		PubKey:  ev.PubKey,
		Tags:    tags,
		Content: ev.Content,
	}
}

// appendTags stages the coordinate with every tag it lacks, matched on type
// and value, and reports whether anything was appended. Tags are carried
// verbatim so a moved member keeps its relay hint and petname.
func (ws *workingSet) appendTags(coord string, tags []nostr.Tag) (bool, error) {
	ev := ws.current(coord)
	if ev == nil {
		return false, fmt.Errorf("%w: %s", ErrListNotFound, coord)
	}
	present := make(map[[2]string]bool, len(ev.Tags))
	for _, tag := range ev.Tags {
		if len(tag) >= 2 {
			present[[2]string{tag[0], tag[1]}] = true
		}
	}
	newTags := make(nostr.Tags, len(ev.Tags), len(ev.Tags)+len(tags))
	copy(newTags, ev.Tags)
	appended := 0
	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}
		key := [2]string{tag[0], tag[1]}
		if present[key] {
			continue
		}
		present[key] = true
		newTags = append(newTags, tag)
		appended++
	}
	if appended == 0 {
		return false, nil
	}
	ws.stage(revised(ev, newTags))
	return true, nil
}

// removeTags stages the coordinate without every tag of the type whose value
// is in values, and reports whether anything was removed.
func (ws *workingSet) removeTags(coord, tagType string, values map[string]bool) (bool, error) {
	ev := ws.current(coord)
	if ev == nil {
		return false, fmt.Errorf("%w: %s", ErrListNotFound, coord)
	}
	newTags := make(nostr.Tags, 0, len(ev.Tags))
	removed := 0
	for _, tag := range ev.Tags {
		if len(tag) >= 2 && tag[0] == tagType && values[tag[1]] {
			removed++
			continue
		}
		newTags = append(newTags, tag)
	}
	if removed == 0 {
		return false, nil
	}
	ws.stage(revised(ev, newTags))
	return true, nil
}

// replaceReference stages the coordinate with its "a" reference to oldCoord
// pointing at newCoord in place, hint preserved; a referencer already holding
// newCoord drops the old reference instead of doubling.
func (ws *workingSet) replaceReference(coord, oldCoord, newCoord string) (bool, error) {
	ev := ws.current(coord)
	if ev == nil {
		return false, fmt.Errorf("%w: %s", ErrListNotFound, coord)
	}
	holdsNew := slices.ContainsFunc(ev.Tags, func(tag nostr.Tag) bool {
		return len(tag) >= 2 && tag[0] == "a" && tag[1] == newCoord
	})
	newTags := make(nostr.Tags, 0, len(ev.Tags))
	changed := false
	for _, tag := range ev.Tags {
		if len(tag) >= 2 && tag[0] == "a" && tag[1] == oldCoord {
			changed = true
			if holdsNew {
				continue
			}
			swapped := make(nostr.Tag, len(tag))
			copy(swapped, tag)
			swapped[1] = newCoord
			newTags = append(newTags, swapped)
			continue
		}
		newTags = append(newTags, tag)
	}
	if !changed {
		return false, nil
	}
	ws.stage(revised(ev, newTags))
	return true, nil
}

// setTitle stages the coordinate under the title and reports whether it
// differs from the one it carries.
func (ws *workingSet) setTitle(coord, title string) (bool, error) {
	ev := ws.current(coord)
	if ev == nil {
		return false, fmt.Errorf("%w: %s", ErrListNotFound, coord)
	}
	if GetTitle(ev) == title {
		return false, nil
	}
	ws.stage(UpdateListTitle(ev, title))
	return true, nil
}

// tombstone retires the coordinate's fetched edition. A pending revision of
// it is dropped: nothing is published for a list the same batch deletes.
func (ws *workingSet) tombstone(coord string) error {
	base := ws.base(coord)
	if base == nil {
		return fmt.Errorf("%w: %s", ErrListNotFound, coord)
	}
	if ws.isDeleted(coord) {
		return nil
	}
	if _, ok := ws.pending[coord]; ok {
		delete(ws.pending, coord)
		if i := slices.Index(ws.order, coord); i >= 0 {
			ws.order = slices.Delete(ws.order, i, i+1)
		}
	}
	ws.deleted = append(ws.deleted, base)
	return nil
}

// emit renders the batch: every pending edition once, each after the pending
// editions it references, then the kind 5 retiring the deleted editions,
// stamped one past the newest of them. Nothing pending and nothing deleted
// emits nil.
func (ws *workingSet) emit() ([]*nostr.Event, error) {
	events := make([]*nostr.Event, 0, len(ws.order)+1)
	emitted := make(map[string]bool, len(ws.order))
	remaining := append([]string(nil), ws.order...)
	for len(remaining) > 0 {
		var held []string
		for _, coord := range remaining {
			if ws.awaitsPending(ws.pending[coord], emitted, coord) {
				held = append(held, coord)
				continue
			}
			events = append(events, ws.pending[coord])
			emitted[coord] = true
		}
		if len(held) == len(remaining) {
			for _, coord := range held {
				events = append(events, ws.pending[coord])
			}
			break
		}
		remaining = held
	}

	if len(ws.deleted) > 0 {
		coords := make([]string, 0, len(ws.deleted))
		ids := make([]string, 0, len(ws.deleted))
		var newest *nostr.Event
		for _, ev := range ws.deleted {
			coords = append(coords, CoordinateFromEvent(ev))
			ids = append(ids, ev.ID)
			if newest == nil || ev.CreatedAt > newest.CreatedAt {
				newest = ev
			}
		}
		tomb, err := NewDeletionEvent(ws.userNpub, coords, ids, newest)
		if err != nil {
			return nil, fmt.Errorf("building the deletion: %w", err)
		}
		events = append(events, tomb)
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events, nil
}

// awaitsPending reports whether the edition references a pending coordinate
// not yet emitted, other than itself.
func (ws *workingSet) awaitsPending(ev *nostr.Event, emitted map[string]bool, self string) bool {
	for _, tag := range ev.Tags {
		if len(tag) < 2 || tag[0] != "a" || tag[1] == self {
			continue
		}
		if _, pending := ws.pending[tag[1]]; pending && !emitted[tag[1]] {
			return true
		}
	}
	return false
}

// subject resolves a list the gesture acts on or places into: own-authored,
// of the domain's leaf kind or kind 30101, and present. The d-tag is not
// consulted: the domain's prefix names the lists a writer creates, and an
// own list of any name the owner placed in the domain is the domain's too.
// The canonical root passes; callers that must not touch it use mutable.
func (ws *workingSet) subject(coord string) (*nostr.Event, error) {
	kind, _, err := ws.ownCoord(coord)
	if err != nil {
		return nil, err
	}
	if kind != ws.domain.LeafKind && kind != KindListSet {
		return nil, fmt.Errorf("%w: %s is a %s", ErrKindMismatch, coord, KindName(kind))
	}
	ev := ws.current(coord)
	if ev == nil {
		return nil, fmt.Errorf("%w: %s", ErrListNotFound, coord)
	}
	return ev, nil
}

// mutable resolves a subject the gesture will rename, move, merge, or
// delete; the canonical root is refused.
func (ws *workingSet) mutable(coord string) (*nostr.Event, error) {
	ev, err := ws.subject(coord)
	if err != nil {
		return nil, err
	}
	if coord == ws.rootCoord {
		return nil, ErrRootImmutable
	}
	return ev, nil
}

// ownCoord parses a coordinate the user must author.
func (ws *workingSet) ownCoord(coord string) (kind int, dTag string, err error) {
	kind, author, dTag, err := btknostr.ParseCoordinate(coord)
	if err != nil {
		return 0, "", fmt.Errorf("parsing coordinate: %w", err)
	}
	if author != ws.userHex {
		return 0, "", fmt.Errorf("list %s is not authored by the user", coord)
	}
	return kind, dTag, nil
}

// container resolves a placement target: a list or folder the gesture puts
// something under, of any d-tag, as subject takes it. The canonical root may
// be absent, because the first placement mints it; anything else must exist.
func (ws *workingSet) container(coord string) (kind int, dTag string, err error) {
	kind, dTag, err = ws.ownCoord(coord)
	if err != nil {
		return 0, "", err
	}
	if kind != ws.domain.LeafKind && kind != KindListSet {
		return 0, "", fmt.Errorf("%w: cannot place into a %s", ErrKindMismatch, KindName(kind))
	}
	if ws.current(coord) == nil && coord != ws.rootCoord {
		return 0, "", fmt.Errorf("%w: %s", ErrListNotFound, coord)
	}
	return kind, dTag, nil
}

// companion returns the node's companion leaf as the gesture sees it, or nil.
func (ws *workingSet) companion(node *nostr.Event) *nostr.Event {
	return ws.current(ws.companionCoord(GetDTag(node)))
}

func (ws *workingSet) companionCoord(nodeDTag string) string {
	return FormatCoordinate(ws.domain.LeafKind, ws.userHex, nodeDTag+ws.domain.CompanionSuffix)
}

// folderChildren lists the coordinates a node references other than its
// companion.
func (ws *workingSet) folderChildren(node *nostr.Event) []string {
	companion := ws.companionCoord(GetDTag(node))
	var children []string
	for _, item := range GetItems(node) {
		if item.IsAddressable() && item.Value != companion {
			children = append(children, item.Value)
		}
	}
	return children
}

// refTag renders an "a" reference with an optional relay hint.
func refTag(coord, relayHint string) nostr.Tag {
	tag := nostr.Tag{"a", coord}
	if relayHint != "" {
		tag = append(tag, relayHint)
	}
	return tag
}

// memberTags returns the leaf's "p" tags verbatim, all of them when hexes is
// nil and otherwise those whose value is in hexes.
func memberTags(leaf *nostr.Event, hexes map[string]bool) []nostr.Tag {
	var tags []nostr.Tag
	for _, tag := range leaf.Tags {
		if len(tag) >= 2 && tag[0] == "p" && (hexes == nil || hexes[tag[1]]) {
			tags = append(tags, tag)
		}
	}
	return tags
}

// depositLeaf resolves the leaf a feed lands in when placed into the target:
// the target itself when it is a leaf, and the target node's companion
// otherwise, minted and referenced on first use as the follow ceremony does.
// The canonical root is minted when absent.
func (ws *workingSet) depositLeaf(targetCoord string, targetKind int, targetDTag, listsRelayHint string) (string, error) {
	if targetKind == ws.domain.LeafKind {
		return targetCoord, nil
	}
	companionCoord := ws.companionCoord(targetDTag)
	if ws.current(companionCoord) == nil {
		if _, err := ws.mint(ws.domain.LeafKind, targetDTag+ws.domain.CompanionSuffix, ws.domain.CompanionTitle, "", "", nil); err != nil {
			return "", err
		}
	}
	if err := ws.reference(targetCoord, companionCoord, listsRelayHint); err != nil {
		return "", err
	}
	return companionCoord, nil
}

// reference stages the node referencing the child, minting the canonical
// root when it is the node and absent. A node already holding the reference
// is left alone.
func (ws *workingSet) reference(nodeCoord, childCoord, listsRelayHint string) error {
	if ws.current(nodeCoord) == nil {
		if nodeCoord != ws.rootCoord {
			return fmt.Errorf("%w: %s", ErrListNotFound, nodeCoord)
		}
		item := Item{Type: "a", Value: childCoord, RelayHint: listsRelayHint}
		_, err := ws.mint(KindListSet, ws.domain.RootDTag, ws.domain.RootTitle, "", "", []Item{item})
		return err
	}
	_, err := ws.appendTags(nodeCoord, []nostr.Tag{refTag(childCoord, listsRelayHint)})
	return err
}

// promote turns a bare leaf into a folder: a companion leaf under the
// domain's suffix carries the leaf's members verbatim, a node with the leaf's
// d-tag and title references the companion, every own referencer of the leaf
// is rewritten to the node, and the leaf is retired. The companion is derived
// by name, never by adopting the leaf's own d-tag, because every consumer
// re-derives it the same way and one name may not sit on two kinds.
func (ws *workingSet) promote(leafCoord, listsRelayHint string) (string, error) {
	leaf := ws.current(leafCoord)
	if leaf == nil {
		return "", fmt.Errorf("%w: %s", ErrListNotFound, leafCoord)
	}
	dTag := GetDTag(leaf)
	title := GetTitle(leaf)
	if err := ws.tombstone(leafCoord); err != nil {
		return "", err
	}
	companion, err := ws.mint(ws.domain.LeafKind, dTag+ws.domain.CompanionSuffix, title, "", "", nil)
	if err != nil {
		return "", err
	}
	companionCoord := CoordinateFromEvent(companion)
	if _, err = ws.appendTags(companionCoord, memberTags(leaf, nil)); err != nil {
		return "", err
	}
	ref := Item{Type: "a", Value: companionCoord, RelayHint: listsRelayHint}
	node, err := ws.mint(KindListSet, dTag, title, GetDescription(leaf), GetImage(leaf), []Item{ref})
	if err != nil {
		return "", err
	}
	nodeCoord := CoordinateFromEvent(node)
	for _, referencer := range ownReferencers(ws.existing, ws.userHex, leafCoord) {
		refCoord := CoordinateFromEvent(referencer)
		if ws.isDeleted(refCoord) {
			continue
		}
		if _, err = ws.replaceReference(refCoord, leafCoord, nodeCoord); err != nil {
			return "", err
		}
	}
	return nodeCoord, nil
}

// attach stages the child under the parent: a node takes the reference
// directly, and a leaf is promoted first. It reports whether the parent
// gained the reference.
func (ws *workingSet) attach(parentCoord string, parentKind int, childCoord, listsRelayHint string) (bool, error) {
	nodeCoord := parentCoord
	if parentKind == ws.domain.LeafKind {
		promoted, err := ws.promote(parentCoord, listsRelayHint)
		if err != nil {
			return false, err
		}
		nodeCoord = promoted
	}
	if ws.current(nodeCoord) == nil {
		return true, ws.reference(nodeCoord, childCoord, listsRelayHint)
	}
	return ws.appendTags(nodeCoord, []nostr.Tag{refTag(childCoord, listsRelayHint)})
}

// detach stages every own referencer of the coordinate without it, apart
// from the ones in keep, and reports whether any held it.
func (ws *workingSet) detach(coord string, keep map[string]bool) (bool, error) {
	detached := false
	for _, referencer := range ownReferencers(ws.existing, ws.userHex, coord) {
		refCoord := CoordinateFromEvent(referencer)
		if keep[refCoord] || ws.isDeleted(refCoord) {
			continue
		}
		removed, err := ws.removeTags(refCoord, "a", map[string]bool{coord: true})
		if err != nil {
			return false, err
		}
		detached = detached || removed
	}
	return detached, nil
}

// retire deletes a list per the deletion contract: every own referencer is
// revised without it, and its edition joins the batch's kind 5. A folder
// takes its companion along.
func (ws *workingSet) retire(ev *nostr.Event) error {
	coord := CoordinateFromEvent(ev)
	if _, err := ws.detach(coord, nil); err != nil {
		return err
	}
	if err := ws.tombstone(coord); err != nil {
		return err
	}
	if ev.Kind != KindListSet {
		return nil
	}
	companion := ws.companion(ev)
	if companion == nil {
		return nil
	}
	companionCoord := CoordinateFromEvent(companion)
	if _, err := ws.detach(companionCoord, nil); err != nil {
		return err
	}
	return ws.tombstone(companionCoord)
}

// refuseDesignated refuses a coordinate the preference designates, naming
// the role.
func refuseDesignated(roles ListRoles, coord string) error {
	if role := roles.DesignatedRole(coord); role != "" {
		return fmt.Errorf("%w: %s is the %s list", ErrRoleDesignated, coord, role)
	}
	return nil
}

// The edge check. A move is refused when the destination is the subject or
// lies inside its subtree, and when the destination's depth from the root
// plus the subject's height plus one would exceed the declared limit;
// promotion of the destination counts one level more. The walk is over own
// composition edges without the read side's pruning, so a descendant the
// builder would truncate still refuses the move.

// ownEdges maps each own node to the composable coordinates it references.
func ownEdges(existing []*nostr.Event, userHex string) map[string][]string {
	edges := make(map[string][]string)
	for _, ev := range existing {
		if ev.PubKey != userHex || ev.Kind != KindListSet {
			continue
		}
		var children []string
		for _, item := range GetItems(ev) {
			if !item.IsAddressable() {
				continue
			}
			kind, _, _, err := btknostr.ParseCoordinate(item.Value)
			if err != nil || !AllowedCompositionKind(kind) {
				continue
			}
			children = append(children, item.Value)
		}
		edges[CoordinateFromEvent(ev)] = children
	}
	return edges
}

// reaches reports whether target is start or lies below it.
func reaches(edges map[string][]string, start, target string) bool {
	visited := make(map[string]bool)
	var walk func(coord string) bool
	walk = func(coord string) bool {
		if coord == target {
			return true
		}
		if visited[coord] {
			return false
		}
		visited[coord] = true
		for _, child := range edges[coord] {
			if walk(child) {
				return true
			}
		}
		return false
	}
	return walk(start)
}

// height returns the longest downward path from the coordinate, cycles cut
// on the path.
func height(edges map[string][]string, coord string) int {
	onPath := make(map[string]bool)
	var walk func(coord string) int
	walk = func(coord string) int {
		if onPath[coord] {
			return 0
		}
		onPath[coord] = true
		defer delete(onPath, coord)
		deepest := 0
		for _, child := range edges[coord] {
			if h := walk(child) + 1; h > deepest {
				deepest = h
			}
		}
		return deepest
	}
	return walk(coord)
}

// depthOf returns the deepest position of the coordinate below the root, and
// zero when the root does not reach it: an unrooted list sits as a root of
// its own.
func depthOf(edges map[string][]string, root, coord string) int {
	deepest := 0
	onPath := make(map[string]bool)
	var walk func(coord string, depth int)
	walk = func(c string, depth int) {
		if c == coord && depth > deepest {
			deepest = depth
		}
		if onPath[c] {
			return
		}
		onPath[c] = true
		defer delete(onPath, c)
		for _, child := range edges[c] {
			walk(child, depth+1)
		}
	}
	walk(root, 0)
	return deepest
}

// checkPlacement applies the edge check to placing the subject (empty for a
// list not yet minted) under the destination.
func (ws *workingSet) checkPlacement(subjectCoord, destCoord string, promote bool) error {
	edges := ownEdges(ws.existing, ws.userHex)
	subjectHeight := 0
	if subjectCoord != "" {
		if reaches(edges, subjectCoord, destCoord) {
			return ErrCycle
		}
		subjectHeight = height(edges, subjectCoord)
	}
	levels := depthOf(edges, ws.rootCoord, destCoord) + 1 + subjectHeight
	if promote {
		levels++
	}
	if levels > DefaultMaxDepth {
		return fmt.Errorf("%w: the list would sit %d levels below the root, the limit is %d", ErrDepthExceeded, levels, DefaultMaxDepth)
	}
	return nil
}

// BuildCreateListEvents mints a list (the domain's leaf kind) or a folder
// (kind 30101) titled title under the parent, in publish order. The d-tag is
// the parent's path joined with the title's slug under the domain prefix; a
// name the owner already holds returns ErrListExists. A parent that is a bare
// leaf is promoted in the same batch. The language is the feed language the
// new list declares: blank writes no tag, and a value that is not a language
// code returns feedlang.ErrInvalid.
func BuildCreateListEvents(domain Domain, existing []*nostr.Event, userNpub string, kind int, title, language, parentCoord, listsRelayHint string) ([]*nostr.Event, error) {
	ws, err := newWorkingSet(domain, existing, userNpub)
	if err != nil {
		return nil, err
	}
	if kind != domain.LeafKind && kind != KindListSet {
		return nil, fmt.Errorf("%w: a new list is kind %d or kind %d, not %d", ErrKindMismatch, domain.LeafKind, KindListSet, kind)
	}
	parentKind, parentDTag, err := ws.container(parentCoord)
	if err != nil {
		return nil, err
	}
	dTag, err := memberDTag(domain, parentDTag, title)
	if err != nil {
		return nil, err
	}
	if err = ws.checkPlacement("", parentCoord, parentKind == domain.LeafKind); err != nil {
		return nil, err
	}
	list, err := ws.mint(kind, dTag, strings.TrimSpace(title), "", "", nil)
	if err != nil {
		return nil, err
	}
	if err = setLanguage(list, language); err != nil {
		return nil, err
	}
	if _, err = ws.attach(parentCoord, parentKind, CoordinateFromEvent(list), listsRelayHint); err != nil {
		return nil, err
	}
	return ws.emit()
}

// BuildRenameListEvents revises the subject under a new title. A folder's
// companion takes the title too, so the same-title rule holds. The root is
// refused, and the title it already carries returns ErrTitleUnchanged.
func BuildRenameListEvents(domain Domain, existing []*nostr.Event, userNpub, subjectCoord, title string) ([]*nostr.Event, error) {
	ws, err := newWorkingSet(domain, existing, userNpub)
	if err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("a list needs a title")
	}
	subject, err := ws.mutable(subjectCoord)
	if err != nil {
		return nil, err
	}
	changed, err := ws.setTitle(subjectCoord, title)
	if err != nil {
		return nil, err
	}
	if subject.Kind == KindListSet {
		if companion := ws.companion(subject); companion != nil {
			companionChanged, titleErr := ws.setTitle(CoordinateFromEvent(companion), title)
			if titleErr != nil {
				return nil, titleErr
			}
			changed = changed || companionChanged
		}
	}
	if !changed {
		return nil, ErrTitleUnchanged
	}
	return ws.emit()
}

// BuildMoveFeedsEvents moves the feeds from the source leaf into the target:
// a leaf takes them directly, a folder takes them in its companion. Each
// feed's tag travels verbatim, hint and petname included; a feed the source
// does not hold is deposited bare. Nothing to add and nothing to remove
// returns ErrItemAlreadyPresent.
func BuildMoveFeedsEvents(domain Domain, existing []*nostr.Event, userNpub string, feedNpubs []string, fromCoord, toCoord, listsRelayHint string) ([]*nostr.Event, error) {
	ws, err := newWorkingSet(domain, existing, userNpub)
	if err != nil {
		return nil, err
	}
	if len(feedNpubs) == 0 {
		return nil, errors.New("no feeds to move")
	}
	from, err := ws.mutable(fromCoord)
	if err != nil {
		return nil, err
	}
	if from.Kind != domain.LeafKind {
		return nil, fmt.Errorf("%w: feeds move from a %s, not a %s", ErrKindMismatch, KindName(domain.LeafKind), KindName(from.Kind))
	}
	toKind, toDTag, err := ws.container(toCoord)
	if err != nil {
		return nil, err
	}
	hexes := make(map[string]bool, len(feedNpubs))
	tags := make([]nostr.Tag, 0, len(feedNpubs))
	for _, npub := range feedNpubs {
		hex, hexErr := btknostr.NpubToHex(npub)
		if hexErr != nil {
			return nil, fmt.Errorf("converting feed npub: %w", hexErr)
		}
		if hexes[hex] {
			continue
		}
		hexes[hex] = true
		held := memberTags(from, map[string]bool{hex: true})
		if len(held) == 0 {
			held = []nostr.Tag{{"p", hex}}
		}
		tags = append(tags, held...)
	}

	targetLeaf, err := ws.depositLeaf(toCoord, toKind, toDTag, listsRelayHint)
	if err != nil {
		return nil, err
	}
	if targetLeaf == fromCoord {
		return nil, ErrItemAlreadyPresent
	}
	added, err := ws.appendTags(targetLeaf, tags)
	if err != nil {
		return nil, err
	}
	removed, err := ws.removeTags(fromCoord, "p", hexes)
	if err != nil {
		return nil, err
	}
	if !added && !removed {
		return nil, ErrItemAlreadyPresent
	}
	return ws.emit()
}

// BuildRemoveFeedsEvents takes the feeds out of the leaf: one revision
// without their tags, nothing else touched. A leaf holding none of them
// returns ErrItemNotPresent.
func BuildRemoveFeedsEvents(domain Domain, existing []*nostr.Event, userNpub string, feedNpubs []string, fromCoord string) ([]*nostr.Event, error) {
	ws, err := newWorkingSet(domain, existing, userNpub)
	if err != nil {
		return nil, err
	}
	if len(feedNpubs) == 0 {
		return nil, errors.New("no feeds to remove")
	}
	from, err := ws.mutable(fromCoord)
	if err != nil {
		return nil, err
	}
	if from.Kind != domain.LeafKind {
		return nil, fmt.Errorf("%w: feeds leave a %s, not a %s", ErrKindMismatch, KindName(domain.LeafKind), KindName(from.Kind))
	}
	hexes := make(map[string]bool, len(feedNpubs))
	for _, npub := range feedNpubs {
		hex, hexErr := btknostr.NpubToHex(npub)
		if hexErr != nil {
			return nil, fmt.Errorf("converting feed npub: %w", hexErr)
		}
		hexes[hex] = true
	}
	removed, err := ws.removeTags(fromCoord, "p", hexes)
	if err != nil {
		return nil, err
	}
	if !removed {
		return nil, ErrItemNotPresent
	}
	return ws.emit()
}

// BuildMoveListEvents moves the subject under the destination: a folder takes
// the reference, a bare leaf is promoted first. With fromCoord named, only
// that parent releases the subject; empty, every own referencer does. The
// edge check refuses the subject itself, its own subtree, and a placement
// past the depth limit. A subject already under the destination and held
// nowhere else returns ErrItemAlreadyPresent.
func BuildMoveListEvents(domain Domain, existing []*nostr.Event, userNpub, subjectCoord, fromCoord, toCoord, listsRelayHint string) ([]*nostr.Event, error) {
	ws, err := newWorkingSet(domain, existing, userNpub)
	if err != nil {
		return nil, err
	}
	if _, err = ws.mutable(subjectCoord); err != nil {
		return nil, err
	}
	toKind, _, err := ws.container(toCoord)
	if err != nil {
		return nil, err
	}
	if fromCoord != "" {
		from, fromErr := ws.subject(fromCoord)
		if fromErr != nil {
			return nil, fromErr
		}
		if from.Kind != KindListSet {
			return nil, fmt.Errorf("%w: %s is a %s and holds no lists", ErrKindMismatch, fromCoord, KindName(from.Kind))
		}
		if fromCoord == toCoord {
			return nil, ErrItemAlreadyPresent
		}
	}
	if err = ws.checkPlacement(subjectCoord, toCoord, toKind == domain.LeafKind); err != nil {
		return nil, err
	}
	attached, err := ws.attach(toCoord, toKind, subjectCoord, listsRelayHint)
	if err != nil {
		return nil, err
	}
	var detached bool
	if fromCoord != "" {
		detached, err = ws.removeTags(fromCoord, "a", map[string]bool{subjectCoord: true})
	} else {
		detached, err = ws.detach(subjectCoord, map[string]bool{toCoord: true})
	}
	if err != nil {
		return nil, err
	}
	if !attached && !detached {
		return nil, ErrItemAlreadyPresent
	}
	return ws.emit()
}

// BuildMergeListEvents moves every feed of the source into the target and
// deletes the source. A folder source contributes its companion's feeds and
// must hold no other child; a folder target takes the feeds in its
// companion. The target revision is omitted when it already holds every
// feed. A source the preference designates is refused, naming the role.
func BuildMergeListEvents(domain Domain, existing []*nostr.Event, userNpub, sourceCoord, targetCoord, listsRelayHint string, roles ListRoles) ([]*nostr.Event, error) {
	ws, err := newWorkingSet(domain, existing, userNpub)
	if err != nil {
		return nil, err
	}
	source, err := ws.mutable(sourceCoord)
	if err != nil {
		return nil, err
	}
	if err = refuseDesignated(roles, sourceCoord); err != nil {
		return nil, err
	}
	if sourceCoord == targetCoord {
		return nil, fmt.Errorf("list %s cannot merge into itself", sourceCoord)
	}
	sourceLeaf := source
	if source.Kind == KindListSet {
		if children := ws.folderChildren(source); len(children) > 0 {
			return nil, fmt.Errorf("%w: %s holds %d other lists", ErrFolderNotEmpty, sourceCoord, len(children))
		}
		sourceLeaf = ws.companion(source)
		if sourceLeaf != nil {
			if err = refuseDesignated(roles, CoordinateFromEvent(sourceLeaf)); err != nil {
				return nil, err
			}
		}
	}
	toKind, toDTag, err := ws.container(targetCoord)
	if err != nil {
		return nil, err
	}
	if sourceLeaf != nil {
		sourceLeafCoord := CoordinateFromEvent(sourceLeaf)
		if targetCoord == sourceLeafCoord || (toKind == KindListSet && ws.companionCoord(toDTag) == sourceLeafCoord) {
			return nil, fmt.Errorf("list %s cannot merge into its own folder", sourceCoord)
		}
		targetLeaf, leafErr := ws.depositLeaf(targetCoord, toKind, toDTag, listsRelayHint)
		if leafErr != nil {
			return nil, leafErr
		}
		if _, err = ws.appendTags(targetLeaf, memberTags(sourceLeaf, nil)); err != nil {
			return nil, err
		}
	}
	if err = ws.retire(source); err != nil {
		return nil, err
	}
	return ws.emit()
}

// BuildDeleteListEvents deletes the subject per the deletion contract: every
// own referencer revised without it, then one kind 5 naming its coordinate
// and edition, stamped one past it. A folder must hold no child other than
// its companion, which is retired with it. The root and a designated list
// are refused.
func BuildDeleteListEvents(domain Domain, existing []*nostr.Event, userNpub, subjectCoord string, roles ListRoles) ([]*nostr.Event, error) {
	ws, err := newWorkingSet(domain, existing, userNpub)
	if err != nil {
		return nil, err
	}
	subject, err := ws.mutable(subjectCoord)
	if err != nil {
		return nil, err
	}
	if err = refuseDesignated(roles, subjectCoord); err != nil {
		return nil, err
	}
	if subject.Kind == KindListSet {
		if children := ws.folderChildren(subject); len(children) > 0 {
			return nil, fmt.Errorf("%w: %s holds %d other lists", ErrFolderNotEmpty, subjectCoord, len(children))
		}
		if companion := ws.companion(subject); companion != nil {
			if err = refuseDesignated(roles, CoordinateFromEvent(companion)); err != nil {
				return nil, err
			}
		}
	}
	if err = ws.retire(subject); err != nil {
		return nil, err
	}
	return ws.emit()
}
