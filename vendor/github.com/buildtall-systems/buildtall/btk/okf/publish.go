package okf

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// Publishing a bundle is a reconciliation, not a translation. The bundle is
// authoritative for the whole tag list of every event it describes, and the
// reconciler derives only what the tree already states: the d tag, which under
// projection D is the path verbatim, and the a coordinates naming the owner's
// own members, which are the directory tree. Every event is built from the
// bundle alone, so a document created where nothing existed carries exactly
// what one rewritten over a live event carries.
//
// A tag the bundle does not state is a tag the bundle says is gone. That rule
// needs no exception list, which is the whole of the point: an enumeration of
// owned names can never close, and every name missing from one is a tag some
// vault loses without saying so.
//
// Tag order is part of what the bundle states. A live event carrying the same
// tags in another order is republished once, into the order stated here, and
// is silent from then on.
//
// Nothing here performs I/O, signs, or reads a clock. The reference time
// arrives in PublishOptions so the same inputs always produce the same events.

// PublishOptions carries the decisions a reconciliation cannot make for
// itself.
type PublishOptions struct {
	// BlobsPresent is the store state the caller gathered before reconciling:
	// true for the lowercase hex SHA-256 of every stated blob the store
	// already holds. It arrives as data rather than as a client so the
	// reconciler performs no I/O and the same inputs always produce the same
	// plan. A hash absent from the map is planned for upload; the worst a
	// stale entry costs is one idempotent re-upload. It leads for field
	// alignment: its pointer word ahead of Now's trailing location pointer
	// keeps the GC's pointer-scan region to the words that carry one.
	BlobsPresent map[string]bool
	// Now is the reference time for every created_at this plan stamps. An
	// event replacing a live one is stamped to strictly outrank it, so a
	// future-dated live event still loses.
	Now time.Time
	// Delete enables tombstones for members present on the wire and absent
	// from the bundle. Off by default: a file missing from a bundle is more
	// likely an incomplete copy than an intent to erase, and a kind 5 cannot
	// be recalled.
	Delete bool
}

// PublishPlan is what a reconciliation decides.
type PublishPlan struct {
	// Events are ordered documents, then sets, then the root, so nothing is
	// ever referenced by an event that lands before it.
	Events []*nostr.Event
	// Deletions come last, so nothing is erased before its replacement exists.
	Deletions []*nostr.Event
	// Unchanged names the d-tags whose live events already match the bundle
	// and are therefore not republished at all.
	Unchanged []string
	// Uploads are the blobs the store lacks, one per distinct hash, sorted by
	// path. The publisher carries them before any event that states them, the
	// ordering rule documents already follow against sets.
	Uploads []BlobUpload
	// Orphans are the blob hashes the live vault states and the bundle no
	// longer does. They are report material for a manual `blossom delete`,
	// never a planned deletion: blobs are not events, one hash may be stated
	// by several directories or several vaults under one owner, and erasing
	// shared bytes is the operator's call alone.
	Orphans []string
}

// BlobUpload names one blob a publish must carry to the store before any
// event states it: the bundle-relative path of the file whose bytes go up,
// and the SHA-256 the store must echo back. One hash stated by several
// files uploads once, under the lexically first path that states it.
type BlobUpload struct {
	Path   string
	SHA256 string
}

// reconciler carries the fixed context of one reconciliation so each step
// reads as what it decides rather than as what it threads through.
type reconciler struct {
	documents map[string]*nostr.Event
	sets      map[string]*nostr.Event
	root      *nostr.Event
	plan      *PublishPlan
	ownerNpub string
	ownerHex  string
	now       time.Time
	// domain trails for field alignment: it ends in an int, so sixteen
	// pointer-free bytes fall outside the GC's pointer-scan region.
	domain lists.Domain
}

// ReconcileVault decides what publishing a bundle into the owner's vault would
// change. It emits an event only where the bundle and the wire disagree, so a
// bundle published twice in succession produces nothing the second time.
func ReconcileVault(domain lists.Domain, ownerNpub string, b *Bundle, events VaultEvents, opts PublishOptions) (*PublishPlan, error) {
	if b == nil || b.Root == nil {
		return nil, errors.New("okf: cannot publish a nil bundle")
	}
	if domain.PathSeparator == "" {
		return nil, fmt.Errorf("okf: domain %q does not declare name-encoded structure", domain.Name)
	}
	ownerHex, err := btknostr.NpubToHex(ownerNpub)
	if err != nil {
		return nil, fmt.Errorf("okf: vault owner: %w", err)
	}
	if events.Root != nil && events.Root.PubKey != ownerHex {
		return nil, fmt.Errorf("okf: root %q is authored by someone other than the named owner", domain.RootDTag)
	}

	r := &reconciler{
		documents: events.Documents,
		sets:      events.Sets,
		root:      events.Root,
		plan:      &PublishPlan{},
		ownerNpub: ownerNpub,
		ownerHex:  ownerHex,
		domain:    domain,
		now:       opts.Now,
	}
	if err := r.reconcile(b, opts); err != nil {
		return nil, err
	}
	return r.plan, nil
}

func (r *reconciler) reconcile(b *Bundle, opts PublishOptions) error {
	dirs, concepts := flattenBundle(b)
	if err := CheckDTags(r.domain, concepts); err != nil {
		return err
	}
	if err := r.checkNodes(dirs); err != nil {
		return err
	}

	// Documents first: a set may not reference one that has not landed.
	held := map[string][]string{}
	publishable := map[string]bool{}
	for _, c := range concepts {
		dTag, err := r.checkConcept(c)
		if err != nil {
			return err
		}
		parent, _ := splitPath(c.ConceptID)
		coord := lists.FormatCoordinate(lists.KindLongFormNote, r.ownerHex, dTag)
		held[parent] = append(held[parent], coord)
		publishable[coord] = true
		if err := r.reconcileDocument(c, dTag); err != nil {
			return err
		}
	}

	for _, dir := range dirs {
		if dir.Path == "" {
			continue // the bundle root is the kind 30101, not a set
		}
		r.reconcileSet(dir, held[dir.Path])
	}
	rootSetDTag := r.reconcileRootSet(b.Root, held[""])
	r.reconcileRoot(b.Root, dirs, rootSetDTag)
	r.reconcileBlobs(dirs, opts.BlobsPresent)

	if opts.Delete {
		r.reconcileDeletions(dirs, publishable, rootSetDTag)
	}
	return nil
}

// checkConcept applies every refusal the write direction owns before anything
// is built from the concept. A root-level concept passes: the reserved root
// set holds it, and its d-tag is its single-segment path, verbatim.
func (r *reconciler) checkConcept(c *Concept) (string, error) {
	if err := CheckDTagMatchesPath(r.domain, c); err != nil {
		return "", err
	}
	if err := c.Frontmatter.CheckNostrTags(); err != nil {
		return "", fmt.Errorf("okf: concept %q: %w", c.ConceptID, err)
	}
	return MemberDTagForConcept(r.domain, c)
}

// checkNodes applies the refusals a directory's sidecar owes. Reading one from
// disk already refuses a stated d, but only a reconciliation knows the owner and
// the domain, so a coordinate the tree derives can be caught nowhere else: it is
// the one refusal in the package that has to wait for a publish to know it.
func (r *reconciler) checkNodes(dirs []*Directory) error {
	for _, dir := range dirs {
		if err := dir.Node.CheckNostrTags(); err != nil {
			return fmt.Errorf("okf: directory %q: %w", dir.Path, err)
		}
		if err := dir.Node.CheckOwnedMembers(r.domain, r.ownerHex); err != nil {
			return fmt.Errorf("okf: directory %q: %w", dir.Path, err)
		}
	}
	return nil
}

// reconcileDocument emits a kind 30023 only where the bundle and the live
// document disagree. The bundle states the whole event, so the live one is read
// to decide whether anything changed and for nothing else.
//
// The body is compared as it would look after a write and a read, because the
// serializer terminates a document with a newline the parser then keeps:
// without that, every document would appear changed on the first publish and a
// second run would never be quiet.
func (r *reconciler) reconcileDocument(c *Concept, dTag string) error {
	coord := lists.FormatCoordinate(lists.KindLongFormNote, r.ownerHex, dTag)
	live := r.documents[coord]
	tags, err := documentTags(c, dTag)
	if err != nil {
		return err
	}

	if live != nil && tagsEqual(live.Tags, tags) && bodyAsWritten(live.Content) == bodyAsWritten(c.Body) {
		r.plan.Unchanged = append(r.plan.Unchanged, dTag)
		return nil
	}
	r.plan.Events = append(r.plan.Events, &nostr.Event{
		Kind:      lists.KindLongFormNote,
		PubKey:    r.ownerHex,
		CreatedAt: r.stamp(live),
		Tags:      tags,
		Content:   c.Body,
	})
	return nil
}

// documentTags is the complete tag list of the event a concept describes: the
// derived d first, then the fields OKF defines in the order a concept file
// states them, then the OKF-owned projections, then every raw tag verbatim.
//
// The order is fixed rather than incidental. A live event is compared against
// this list as a sequence, so an order that varied between two runs would
// republish the whole vault on each of them.
//
// The projections are emitted only when their field is stated: a type is a
// tag only when it is not the kind's canonical one, so every Document-typed,
// resource-less, extra-less event already on a relay republishes quietly. The
// symmetry is ruling 8's: a field the bundle does not state is a tag the wire
// loses.
func documentTags(c *Concept, dTag string) (nostr.Tags, error) {
	tags := nostr.Tags{{btknostr.TagD, dTag}}
	tags = appendValued(tags, btknostr.TagTitle, c.Frontmatter.Title)
	tags = appendValued(tags, btknostr.TagSummary, c.Frontmatter.Description)
	for _, topic := range c.Frontmatter.Tags {
		tags = append(tags, nostr.Tag{btknostr.TagTopic, topic})
	}
	tags = appendValued(tags, btknostr.TagPublishedAt, publishedAtWire(c.Frontmatter.Timestamp))
	if c.Frontmatter.Type != TypeDocument {
		tags = appendValued(tags, TagOKFType, c.Frontmatter.Type)
	}
	tags = appendValued(tags, TagOKFResource, c.Frontmatter.Resource)
	if len(c.Frontmatter.Extra) > 0 {
		extra, err := marshalExtra(c.Frontmatter.Extra)
		if err != nil {
			return nil, fmt.Errorf("okf: concept %q: %w", c.ConceptID, err)
		}
		tags = append(tags, nostr.Tag{TagOKFExtra, extra})
	}
	return appendRaw(tags, c.Frontmatter.NostrTags), nil
}

// appendValued states a sugared field as its tag and states nothing where the
// field is empty. A field left blank is a tag the event does not carry, which is
// what makes clearing one from disk possible at all.
func appendValued(tags nostr.Tags, name, value string) nostr.Tags {
	if value == "" {
		return tags
	}
	return append(tags, nostr.Tag{name, value})
}

// appendRaw copies a stated tag list onto an event's tags. The clone matters:
// the bundle outlives the plan, and an event sharing its backing array would let
// a signer's mutation reach back into what the operator has on disk.
func appendRaw(tags nostr.Tags, raw [][]string) nostr.Tags {
	for _, tag := range raw {
		tags = append(tags, slices.Clone(tag))
	}
	return tags
}

// publishedAtWire renders OKF's timestamp field back to what NIP-23 specifies,
// which is unix seconds. It is the inverse of the export's publishedAtISO: an
// instant survives the trip exactly, a bare calendar date is normalized to the
// instant it names, and a value neither form parses is carried verbatim, since a
// bundle unable to restate what it read would publish a document with no
// publication time at all.
func publishedAtWire(timestamp string) string {
	if instant, err := time.Parse(time.RFC3339, timestamp); err == nil {
		return strconv.FormatInt(instant.Unix(), 10)
	}
	if day, err := time.Parse(time.DateOnly, timestamp); err == nil {
		return strconv.FormatInt(day.UTC().Unix(), 10)
	}
	return timestamp
}

// reconcileSet emits a kind 30004 for one directory: its sidecar states
// everything the set carries, and the concepts beside it state its membership.
func (r *reconciler) reconcileSet(dir *Directory, held []string) {
	dTag := r.domain.DTagPrefix + dir.Path
	r.reconcileComposed(r.sets[dTag], lists.KindCurationSet, dTag, dir.Node, held, dir.Attachments)
}

// reconcileRootSet emits the reserved set holding the bundle root's own
// documents and attachments, and returns its d-tag, empty when the root holds
// no files and the set therefore does not exist, mirroring how a sidecar is
// written only when it states something. The set is derived, never authored:
// no sidecar exists to state its title, so the title is minted from the
// domain's declaration, the same one every deposit ceremony mints.
func (r *reconciler) reconcileRootSet(root *Directory, held []string) string {
	if len(held) == 0 && len(root.Attachments) == 0 {
		return ""
	}
	dTag := r.domain.RootDTag + r.domain.CompanionSuffix
	node := NodeMetadata{NostrTags: [][]string{{btknostr.TagTitle, r.domain.CompanionTitle}}}
	r.reconcileComposed(r.sets[dTag], lists.KindCurationSet, dTag, node, held, root.Attachments)
	return dTag
}

// reconcileRoot emits the vault's single kind 30101, which references every
// set the bundle describes. Nesting is lexical from the d-tags, so the root is
// a flat list of every directory at every depth, plus the reserved root set
// when the root holds files of its own.
func (r *reconciler) reconcileRoot(root *Directory, dirs []*Directory, rootSetDTag string) {
	sets := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if dir.Path == "" {
			continue
		}
		sets = append(sets, lists.FormatCoordinate(lists.KindCurationSet, r.ownerHex, r.domain.DTagPrefix+dir.Path))
	}
	if rootSetDTag != "" {
		sets = append(sets, lists.FormatCoordinate(lists.KindCurationSet, r.ownerHex, rootSetDTag))
	}
	r.reconcileComposed(r.root, lists.KindListSet, r.domain.RootDTag, root.Node, sets, nil)
}

// reconcileComposed decides one composing event. A set and the root differ only
// in which kind they compose and where their members come from, so both are
// this: what the sidecar states, plus the members the tree derives.
func (r *reconciler) reconcileComposed(live *nostr.Event, kind int, dTag string, node NodeMetadata, coords []string, attachments []Attachment) {
	slices.Sort(coords)
	tags := composedTags(node, dTag, coords, attachments)

	if live != nil && tagsEqual(live.Tags, tags) && live.Content == node.Content {
		r.plan.Unchanged = append(r.plan.Unchanged, dTag)
		return
	}
	r.plan.Events = append(r.plan.Events, &nostr.Event{
		Kind:      kind,
		PubKey:    r.ownerHex,
		CreatedAt: r.stamp(live),
		Tags:      tags,
		Content:   node.Content,
	})
}

// composedTags is the complete tag list of a composing event: the derived d
// first, then everything the directory's sidecar states, then the coordinates of
// the members the tree derives, which arrive sorted so that the same bundle
// always yields the same event, then one okf-attachment statement per file
// the directory holds, name then hash, in the model's sorted order. The
// attachment position is as fixed as every other: tagsEqual compares whole
// ordered sequences, so a position that varied between runs would republish
// every attachment-bearing set on each of them.
func composedTags(node NodeMetadata, dTag string, coords []string, attachments []Attachment) nostr.Tags {
	tags := appendRaw(nostr.Tags{{btknostr.TagD, dTag}}, node.NostrTags)
	for _, coord := range coords {
		tags = append(tags, nostr.Tag{btknostr.TagCoordinate, coord})
	}
	for _, a := range attachments {
		tags = append(tags, nostr.Tag{TagOKFAttachment, a.Name, a.SHA256})
	}
	return tags
}

// reconcileBlobs decides the store half of the plan. Uploads are the stated
// hashes the caller did not find on the store, one per distinct hash however
// many files state it, sorted by path with the lexically first path naming
// the blob. Orphans are the hashes the live vault states and the bundle no
// longer does, gathered for the report and nothing else: the event layer's
// absence-deletes discipline stops at statements, and the bytes behind a
// hash are dissociated only by the operator's own hand.
func (r *reconciler) reconcileBlobs(dirs []*Directory, present map[string]bool) {
	stated := map[string]bool{}
	var candidates []BlobUpload
	for _, dir := range dirs {
		for _, a := range dir.Attachments {
			stated[a.SHA256] = true
			if !present[a.SHA256] {
				candidates = append(candidates, BlobUpload{Path: joinPath(dir.Path, a.Name), SHA256: a.SHA256})
			}
		}
	}
	slices.SortFunc(candidates, func(x, y BlobUpload) int { return strings.Compare(x.Path, y.Path) })
	planned := map[string]bool{}
	for _, u := range candidates {
		if planned[u.SHA256] {
			continue
		}
		planned[u.SHA256] = true
		r.plan.Uploads = append(r.plan.Uploads, u)
	}

	orphaned := map[string]bool{}
	for _, dTag := range slices.Sorted(maps.Keys(r.sets)) {
		if r.domain.ClassifyDTag(dTag) != lists.DTagMember {
			continue
		}
		for _, tag := range r.sets[dTag].Tags {
			if len(tag) >= 3 && tag[0] == TagOKFAttachment && !stated[tag[2]] && !orphaned[tag[2]] {
				orphaned[tag[2]] = true
				r.plan.Orphans = append(r.plan.Orphans, tag[2])
			}
		}
	}
	slices.Sort(r.plan.Orphans)
}

// reconcileDeletions names what is on the wire, inside this vault, and absent
// from the bundle. A set whose directory is gone and a document whose file is
// gone are both erasures the operator has to ask for explicitly. The reserved
// root set is an ordinary member here: an emptied root stops minting it, so
// the live one falls to the same rule as any removed directory's set.
func (r *reconciler) reconcileDeletions(dirs []*Directory, publishable map[string]bool, rootSetDTag string) {
	keep := map[string]bool{}
	if rootSetDTag != "" {
		keep[rootSetDTag] = true
	}
	for _, dir := range dirs {
		if dir.Path != "" {
			keep[r.domain.DTagPrefix+dir.Path] = true
		}
	}

	for _, dTag := range slices.Sorted(maps.Keys(r.sets)) {
		if r.domain.ClassifyDTag(dTag) != lists.DTagMember || keep[dTag] {
			continue
		}
		r.deletion(r.sets[dTag], lists.FormatCoordinate(lists.KindCurationSet, r.ownerHex, dTag))
	}
	for _, coord := range slices.Sorted(maps.Keys(r.documents)) {
		if publishable[coord] || !r.owns(coord, lists.KindLongFormNote) {
			continue
		}
		r.deletion(r.documents[coord], coord)
	}
}

func (r *reconciler) deletion(target *nostr.Event, coord string) {
	var ids []string
	if target != nil && target.ID != "" {
		ids = append(ids, target.ID)
	}
	// nostr-rs-relay 0.9.0 honors NIP-09 by "e" tag, so the event id matters
	// as much as the coordinate; NewDeletionEvent derives the "k" tag from the
	// coordinate's kind. Its stamp is replaced because it reads the wall clock
	// and a plan must be deterministic.
	ev, err := lists.NewDeletionEvent(r.ownerNpub, []string{coord}, ids, target)
	if err != nil {
		return
	}
	ev.CreatedAt = r.stamp(target)
	r.plan.Deletions = append(r.plan.Deletions, ev)
}

// stamp dates an event so it strictly outranks the one it replaces. A tie
// falls into NIP-01's lowest-id lottery and a future-dated live event would
// otherwise survive the publish.
func (r *reconciler) stamp(live *nostr.Event) nostr.Timestamp {
	if live == nil {
		return nostr.Timestamp(r.now.Unix())
	}
	return lists.NextCreatedAt(live, r.now)
}

// owns reports whether a coordinate names an event of the given kind that this
// owner wrote inside this vault's namespace. Anything else in a set is a
// foreign reference and is not the bundle's to rewrite.
func (r *reconciler) owns(coord string, kind int) bool {
	gotKind, pubkeyHex, dTag, err := btknostr.ParseCoordinate(coord)
	if err != nil {
		return false
	}
	return gotKind == kind && pubkeyHex == r.ownerHex && r.domain.ClassifyDTag(dTag) == lists.DTagMember
}

// tagsEqual compares two tag lists as ordered sequences, which is what makes a
// tag the bundle stopped stating visible at all: a comparison per name can only
// ever see the names it was taught, and the names it was not taught are exactly
// the ones a vault loses.
func tagsEqual(a, b nostr.Tags) bool {
	return slices.EqualFunc(a, b, func(x, y nostr.Tag) bool { return slices.Equal(x, y) })
}

// bodyAsWritten is what a body looks like once it has been through a file.
// WriteConcept terminates every document with a newline and ParseConcept keeps
// it, so a body that did not end in one gains one, exactly once. Comparing
// bodies in this form is what lets a publish of an unedited bundle be silent.
func bodyAsWritten(body string) string {
	if body == "" || strings.HasSuffix(body, "\n") {
		return body
	}
	return body + "\n"
}

// flattenBundle lists every directory in the bundle and every concept it holds,
// both in a deterministic order so a plan never depends on map iteration. The
// directories arrive rather than their paths alone, because what a directory
// states about its own event now travels with it.
func flattenBundle(b *Bundle) (dirs []*Directory, concepts []*Concept) {
	var walk func(*Directory)
	walk = func(d *Directory) {
		dirs = append(dirs, d)
		concepts = append(concepts, sortedConcepts(d.Concepts)...)
		for _, child := range d.Children() {
			walk(child)
		}
	}
	walk(b.Root)
	slices.SortFunc(dirs, func(a, b *Directory) int { return strings.Compare(a.Path, b.Path) })
	return dirs, concepts
}

func tagValue(tags nostr.Tags, name string) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == name {
			return tag[1]
		}
	}
	return ""
}

func tagValues(tags nostr.Tags, name string) []string {
	var values []string
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == name {
			values = append(values, tag[1])
		}
	}
	return values
}
