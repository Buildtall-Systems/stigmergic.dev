package okf

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// Export under projection D needs no traversal. A vault's shape is carried in
// the d-tags of its sets, so the assembly reads the root only to learn which
// sets belong to the vault, and rebuilds the tree lexically from the paths
// those sets already name. Where the two sources of truth disagree, the export
// says so: a set the root does not reference, a reference no set answers, and
// a document filed under a set that does not name its parent are each a drift
// warning rather than a silent correction.

const (
	logKeyPath  = "path"
	logKeyCoord = "coordinate"
	logKeyDTag  = "d-tag"
	logKeyTag   = "tag"
	logKeyError = "error"
)

// VaultEvents is the fetched material an export assembles: the vault's kind
// 30101 root, the owner's kind 30004 sets keyed by d-tag, and the long-form
// documents those sets reference. Everything an export needs arrives here, so
// the assembly itself reads no relay.
type VaultEvents struct {
	Sets map[string]*nostr.Event
	// Documents are the long-form events keyed by coordinate, kept as they
	// arrived. Both directions need them whole. An export states every tag a
	// document carries, and a publish rebuilds the document from what the
	// bundle states, so a projection standing between the wire and the bundle
	// is a tag list nobody can recover.
	Documents map[string]*nostr.Event
	Root      *nostr.Event
}

// exporter carries the assembly's fixed context so each step reads as what it
// decides rather than as what it threads through.
type exporter struct {
	documents map[string]*nostr.Event
	bundle    *Bundle
	log       *slog.Logger
	ownerHex  string
	domain    lists.Domain
}

// ExportVault assembles a bundle from one vault's events. The owner is named
// by npub; the hex it decodes to is used only to compare against the pubkey
// field of a coordinate, which NIP-01 fixes as hex.
func ExportVault(domain lists.Domain, ownerNpub string, events VaultEvents, log *slog.Logger) (*Bundle, error) {
	if log == nil {
		return nil, errors.New("okf: export requires a logger")
	}
	if domain.PathSeparator == "" {
		return nil, fmt.Errorf("okf: domain %q does not declare name-encoded structure", domain.Name)
	}
	if events.Root == nil {
		return nil, fmt.Errorf("okf: no root event for %q", domain.RootDTag)
	}
	if events.Root.Kind != lists.KindListSet {
		return nil, fmt.Errorf("okf: root of %q is kind %d, want %d", domain.RootDTag, events.Root.Kind, lists.KindListSet)
	}
	if d := lists.GetDTag(events.Root); d != domain.RootDTag {
		return nil, fmt.Errorf("okf: root carries d-tag %q, want %q", d, domain.RootDTag)
	}

	ownerHex, err := btknostr.NpubToHex(ownerNpub)
	if err != nil {
		return nil, fmt.Errorf("okf: vault owner: %w", err)
	}
	if events.Root.PubKey != ownerHex {
		return nil, fmt.Errorf("okf: root %q is authored by someone other than the named owner", domain.RootDTag)
	}

	bundle, err := NewBundle(domain.RootDTag)
	if err != nil {
		return nil, err
	}

	e := &exporter{
		documents: events.Documents,
		bundle:    bundle,
		log:       log,
		ownerHex:  ownerHex,
		domain:    domain,
	}
	e.assemble(events)
	return bundle, nil
}

// DocumentAddresses names the long-form documents a vault's sets reference, so
// a caller fetches exactly what an assembly can place rather than the owner's
// whole long-form corpus. It is deliberately generous: a set the assembly
// later drops as drift still contributes its documents here, because one
// unused address in a batched fetch costs less than a second round trip.
func DocumentAddresses(domain lists.Domain, ownerNpub string, sets map[string]*nostr.Event) ([]btknostr.ArticleAddress, error) {
	ownerHex, err := btknostr.NpubToHex(ownerNpub)
	if err != nil {
		return nil, fmt.Errorf("okf: vault owner: %w", err)
	}

	seen := map[string]bool{}
	addrs := make([]btknostr.ArticleAddress, 0, len(sets))
	for _, dTag := range slices.Sorted(maps.Keys(sets)) {
		if domain.ClassifyDTag(dTag) != lists.DTagMember {
			continue
		}
		for _, item := range lists.GetItems(sets[dTag]) {
			if !item.IsAddressable() || seen[item.Value] {
				continue
			}
			kind, pubkeyHex, docDTag, err := btknostr.ParseCoordinate(item.Value)
			if err != nil || kind != lists.KindLongFormNote || pubkeyHex != ownerHex {
				continue
			}
			seen[item.Value] = true
			addrs = append(addrs, btknostr.ArticleAddress{Pubkey: pubkeyHex, DTag: docDTag})
		}
	}
	return addrs, nil
}

// assemble keeps the sets that are both referenced by the root and classified
// into the vault's namespace, and reports each set that satisfies exactly one
// of the two. A set satisfying neither belongs to some other region of the
// owner's ontology and is not this vault's business.
func (e *exporter) assemble(events VaultEvents) {
	e.bundle.Root.Node = e.nodeFrom(events.Root)
	// The root states which format wrote it, so a publish can tell a bundle
	// that states every tag from one that predates the question.
	e.bundle.Root.Node.FormatVersion = BundleFormatVersion
	rootRefs := referencedCoordinates(events.Root)
	answered := make(map[string]bool, len(rootRefs))

	for _, dTag := range slices.Sorted(maps.Keys(events.Sets)) {
		set := events.Sets[dTag]
		coord := lists.CoordinateFromEvent(set)
		classified := e.domain.ClassifyDTag(dTag) == lists.DTagMember
		referenced := rootRefs[coord]
		if referenced {
			answered[coord] = true
		}

		switch {
		case classified && referenced:
			e.addSet(dTag, set)
		case classified:
			e.log.Warn("vault set is not referenced by the vault root", logKeyDTag, dTag)
		case referenced:
			e.log.Warn("vault root references a set outside the vault namespace", logKeyDTag, dTag)
		}
	}

	for _, coord := range slices.Sorted(maps.Keys(rootRefs)) {
		if !answered[coord] {
			e.log.Warn("vault root references a set no fetched event answers", logKeyCoord, coord)
		}
	}
}

// addSet materializes one directory and files what it references. The
// reserved root set materializes no directory at all: its holdings are the
// bundle root's own files and are routed there, so nothing named for it
// exists on disk for the next read to republish as a genuine nested set. Its
// tags are discarded rather than stated in a sidecar, since the reconciler
// mints every one of them from the tree and the domain's declaration.
func (e *exporter) addSet(dTag string, set *nostr.Event) {
	dir, path := e.bundle.Root, ""
	if dTag != e.domain.RootDTag+e.domain.CompanionSuffix {
		var err error
		path, err = DTagToPath(e.domain, dTag)
		if err != nil {
			e.log.Warn("skipping a vault set whose d-tag names no bundle path", logKeyDTag, dTag, logKeyError, err)
			return
		}
		dir, err = e.bundle.Dir(path)
		if err != nil {
			e.log.Warn("skipping a vault set the bundle will not hold", logKeyPath, path, logKeyError, err)
			return
		}
		e.checkDepth(path)
		dir.Node = e.nodeFrom(set)
	}

	for _, item := range lists.GetItems(set) {
		switch {
		case item.IsAddressable():
			e.addAddressable(dir, path, item)
		case item.IsEvent():
			e.addEventCitation(dir, path, item)
		default:
			e.log.Warn("ignoring an item type a curation set does not define", logKeyPath, path, "type", item.Type)
		}
	}

	for _, tag := range set.Tags {
		if len(tag) > 0 && tag[0] == TagOKFAttachment {
			e.addAttachment(dir, dTag, tag)
		}
	}
}

// addAttachment restates one okf-attachment statement into the model, the
// exact inverse of composedTags' emission. AddAttachment applies the gates
// both directions share, so a statement the bundle refuses is dropped with a
// warning naming the set and the tag rather than carried toward a write it
// could collide at: a stated path is validated before anything is written at
// it.
func (e *exporter) addAttachment(dir *Directory, dTag string, tag nostr.Tag) {
	if len(tag) < 3 {
		e.log.Warn("dropping an attachment statement that carries no hash", logKeyDTag, dTag, logKeyTag, tag)
		return
	}
	if err := dir.AddAttachment(Attachment{Name: tag[1], SHA256: tag[2]}); err != nil {
		e.log.Warn("dropping an attachment statement the bundle refuses", logKeyDTag, dTag, logKeyTag, tag, logKeyError, err)
	}
}

// addAddressable files an owned long-form document as a concept and everything
// else as a citation. A document belonging to another author is cited rather
// than written, so an export never presents another person's work as a local
// file the owner could republish under their own key.
func (e *exporter) addAddressable(dir *Directory, dirPath string, item lists.Item) {
	kind, pubkeyHex, dTag, err := btknostr.ParseCoordinate(item.Value)
	if err != nil {
		e.log.Warn("ignoring an unparseable coordinate", logKeyPath, dirPath, logKeyCoord, item.Value, logKeyError, err)
		return
	}
	if kind != lists.KindLongFormNote || pubkeyHex != e.ownerHex {
		e.cite(dir, item, dTag, pubkeyHex)
		return
	}

	doc, ok := e.documents[lists.FormatCoordinate(kind, pubkeyHex, dTag)]
	if !ok {
		e.log.Warn("vault set references a document no fetched event answers", logKeyPath, dirPath, logKeyCoord, item.Value)
		return
	}

	conceptPath, err := DTagToPath(e.domain, dTag)
	if err != nil {
		e.log.Warn("citing a document whose d-tag names no bundle path", logKeyPath, dirPath, logKeyDTag, dTag, logKeyError, err)
		e.cite(dir, item, dTag, pubkeyHex)
		return
	}

	// A document's path is its identity, so it is filed where its own d-tag
	// says it lives even when a set elsewhere is what referenced it.
	parent, _ := splitPath(conceptPath)
	if parent != dirPath {
		e.log.Warn("filing a document by its own path rather than by the set referencing it",
			logKeyPath, conceptPath, "referencing set", dirPath)
	}

	home, err := e.bundle.Dir(parent)
	if err != nil {
		e.log.Warn("skipping a document the bundle will not hold", logKeyPath, conceptPath, logKeyError, err)
		return
	}
	e.checkDepth(conceptPath)
	if err := home.AddConcept(e.conceptFromEvent(conceptPath, doc)); err != nil {
		e.log.Warn("skipping a document the bundle will not hold", logKeyPath, conceptPath, logKeyError, err)
	}
}

// cite records an addressable reference the bundle does not contain. A
// reference that will not encode as a naddr is dropped with a warning rather
// than cited as a raw coordinate: a citation a reader cannot resolve is worse
// than an honest absence, and hex belongs on the wire, not in a bundle.
func (e *exporter) cite(dir *Directory, item lists.Item, dTag, pubkeyHex string) {
	var relays []string
	if item.RelayHint != "" {
		relays = []string{item.RelayHint}
	}
	naddr, err := btknostr.CoordToNaddr(item.Value, relays)
	if err != nil {
		e.log.Warn("dropping a reference that will not encode as a naddr", logKeyCoord, item.Value, logKeyError, err)
		return
	}
	dir.AddCitation(Citation{Title: dTag, Ref: naddr, AuthorNpub: e.authorNpub(pubkeyHex)})
}

// addEventCitation records a reference to a specific event. Such a reference
// names no author and no addressable identity, so it can only ever be cited.
func (e *exporter) addEventCitation(dir *Directory, dirPath string, item lists.Item) {
	var relays []string
	if item.RelayHint != "" {
		relays = []string{item.RelayHint}
	}
	nevent, err := btknostr.HexToNevent(item.Value, relays)
	if err != nil {
		e.log.Warn("dropping a reference that will not encode as a nevent", logKeyPath, dirPath, logKeyError, err)
		return
	}
	dir.AddCitation(Citation{Ref: nevent})
}

// authorNpub renders a coordinate's hex author as an npub. An unencodable key
// yields no attribution rather than a hex string leaking into the bundle.
func (e *exporter) authorNpub(pubkeyHex string) string {
	npub, err := btknostr.HexToNpub(pubkeyHex)
	if err != nil {
		e.log.Warn("citing an item whose author key will not encode", logKeyError, err)
		return ""
	}
	return npub
}

// checkDepth is the tripwire the plan calls for. Name-encoded structure needs
// no traversal, so this export prunes nothing; the warning says that a reader
// who does traverse would lose everything below the limit.
func (e *exporter) checkDepth(path string) {
	depth := pathDepth(path)
	if depth <= lists.DefaultMaxDepth {
		return
	}
	e.log.Error("vault member lies deeper than the declared traversal limit; this export keeps it, a traversing reader would prune it",
		logKeyPath, path, "depth", depth, "limit", lists.DefaultMaxDepth)
}

// conceptFromEvent states a long-form document as a concept: the names OKF
// defines become the fields OKF defines, the OKF-owned projections return to
// the fields they project, and every other tag is stated raw. The d-tag is
// stamped explicitly rather than left to derivation, so a bundle republished
// from this export addresses the very same events it was read from, whatever
// the bundle root is later renamed to.
//
// Nothing is projected away. A document's tags are the document, so an export
// that kept only the names it could spell would produce a bundle that publishes
// as a poorer event than the one it came from.
func (e *exporter) conceptFromEvent(conceptPath string, ev *nostr.Event) *Concept {
	fm := Frontmatter{Type: TypeDocument, DTag: lists.GetDTag(ev)}

	for _, tag := range ev.Tags {
		if len(tag) == 0 || tag[0] == "" {
			continue
		}
		if e.projectTag(conceptPath, tag, &fm) {
			continue
		}
		_, sugared := sugaredTagNames[tag[0]]
		if !sugared && tag[0] != btknostr.TagD {
			// Stated verbatim, a tag of one element included: a name with no
			// value is a flag, and a flag the bundle drops is a flag the next
			// publish clears.
			fm.NostrTags = append(fm.NostrTags, slices.Clone(tag))
			continue
		}
		if len(tag) < 2 {
			continue // a name OKF sugars, carrying no value, states nothing
		}
		switch tag[0] {
		case btknostr.TagTitle:
			fm.Title = tag[1]
		case btknostr.TagSummary:
			fm.Description = tag[1]
		case btknostr.TagTopic:
			fm.Tags = append(fm.Tags, tag[1])
		case btknostr.TagPublishedAt:
			fm.Timestamp = publishedAtISO(tag[1])
		}
	}

	return &Concept{ConceptID: conceptPath, Body: ev.Content, Frontmatter: fm}
}

// projectTag routes an OKF-owned tag back into the field it projects; consumed
// reports whether the name was one of the three. A projection carrying no
// value states nothing, the sugared rule. An okf-extra value that does not
// decode is carried into the raw list verbatim with a warning, where the
// publish gate's refusal will name it: corruption is surfaced loudly, never
// rewritten silently.
func (e *exporter) projectTag(conceptPath string, tag nostr.Tag, fm *Frontmatter) bool {
	if _, owned := okfTagSources[tag[0]]; !owned {
		return false
	}
	if len(tag) < 2 {
		return true
	}
	switch tag[0] {
	case TagOKFType:
		fm.Type = tag[1]
	case TagOKFResource:
		fm.Resource = tag[1]
	case TagOKFExtra:
		extra, err := unmarshalExtra(tag[1])
		if err != nil {
			e.log.Warn("carrying an okf-extra tag whose value does not decode",
				logKeyPath, conceptPath, logKeyError, err)
			fm.NostrTags = append(fm.NostrTags, slices.Clone(tag))
			return true
		}
		fm.Extra = extra
	}
	return true
}

// publishedAtISO renders the wire's publication time as the ISO 8601 instant
// OKF defines its timestamp field to hold. NIP-23 specifies unix seconds and
// some clients write a calendar date; both are read. A value that is neither is
// stated verbatim, since a bundle that cannot restate it would publish a
// document with no publication time at all.
//
// There is no fallback to created_at. That is a fact about the relay, it moves
// on every republish including a mechanical one, and inventing a publication
// time from it is what made an untouched document drift on disk.
func publishedAtISO(value string) string {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
	}
	if day, err := time.Parse(time.DateOnly, value); err == nil {
		return day.UTC().Format(time.RFC3339)
	}
	return value
}

// NormalizeTimestamp restates a timestamp in the spelling an export writes:
// the UTC ISO 8601 instant. An instant in any offset, a bare calendar date,
// and a bare unix-seconds count all name a moment; this is that moment spelled
// as the fixpoint, by running the value through the wire's own two directions.
// A value that names no moment returns verbatim, exactly as the wire would
// carry it. Ingest stamps this form so the first cycle is byte-quiet, the same
// rule the derived d follows.
func NormalizeTimestamp(value string) string {
	return publishedAtISO(publishedAtWire(value))
}

// nodeFrom states everything a directory's event carries that the tree does not
// derive. Its d-tag is the directory's own path, its owned coordinates are the
// files and directories themselves, and its attachment statements are derived
// from the directory's files at publish-read time, so all three are dropped;
// every other tag, a reference to somebody else's work included, is a fact
// only the sidecar can carry across a cold create. The attachment skip is
// unconditional, malformed statements included: a sidecar restating the tag is
// what checkRawTag refuses, so carrying one here would poison the next publish
// rather than surface anything.
func (e *exporter) nodeFrom(ev *nostr.Event) NodeMetadata {
	if ev == nil {
		return NodeMetadata{}
	}

	node := NodeMetadata{Content: ev.Content}
	for _, tag := range ev.Tags {
		if len(tag) == 0 || tag[0] == "" || tag[0] == btknostr.TagD || tag[0] == TagOKFAttachment {
			continue
		}
		if tag[0] == btknostr.TagCoordinate && len(tag) >= 2 && derivedCoordinate(e.domain, e.ownerHex, tag[1]) {
			continue
		}
		node.NostrTags = append(node.NostrTags, slices.Clone(tag))
	}
	return node
}

func referencedCoordinates(root *nostr.Event) map[string]bool {
	refs := map[string]bool{}
	for _, item := range lists.GetItems(root) {
		if item.IsAddressable() {
			refs[item.Value] = true
		}
	}
	return refs
}

// pathDepth counts a bundle-relative path's segments. The bundle root is depth
// zero, matching the ontology's own convention where the kind 30101 root is.
func pathDepth(path string) int {
	if path == "" {
		return 0
	}
	return strings.Count(path, bundlePathSeparator) + 1
}
