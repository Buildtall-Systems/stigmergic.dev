package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/buildtall-systems/buildtall/btk/lists"
	"github.com/buildtall-systems/buildtall/btk/okf"
)

// Ingest is the projection from a raw source tree, an Obsidian vault as it
// sits on disk, into the OKF bundle the unmodified publish machinery accepts.
// The projection is deterministic: one tree yields one bundle and one census,
// whichever run produces them. The bundle is a generated artifact, regenerated
// whole on every run and never hand-edited, so nothing here merges; and the
// source is opened read-only, so nothing here writes.
//
// The reading is lenient where the strict reader refuses, because a raw vault
// predates the format: a file with no frontmatter gains the two statements OKF
// requires and humans never wrote (the fence and a Document type) rather than
// aborting the walk, and a member the projection cannot carry is excluded and
// censused by name rather than refused with everything beside it. Bodies are
// carried byte-verbatim, wikilinks and all: the projection adds statements
// around a note, never edits within one.

// conceptFileExt is the suffix that makes a source file a concept candidate.
// The format spells it inside btk/okf without exporting it; the test suite
// pins this spelling to the reserved index name, which the format composes
// from the same suffix.
const conceptFileExt = ".md"

// Exclusion names one member the projection refused and the rule it broke.
type Exclusion struct {
	Path   string
	Reason string
}

// The exclusion reasons, one per projection rule a member can break.
const (
	// ReasonReservedName refuses a member named as a file the format reserves,
	// which the parent directory's own materialization writes.
	ReasonReservedName = "reserved name"
	// ReasonUnnameablePath refuses a member whose path will not render as a
	// d-tag.
	ReasonUnnameablePath = "path unnameable as a d-tag"
	// ReasonMalformedFrontmatter refuses a fence that will not parse: opened
	// and never closed, or holding YAML that will not decode.
	ReasonMalformedFrontmatter = "malformed frontmatter"
	// ReasonInvalidFrontmatter refuses frontmatter that parsed but states
	// what the format derives or another field already states, which the
	// strict reader would refuse on the way back in.
	ReasonInvalidFrontmatter = "frontmatter restates a derived fact"
	// ReasonForeignDTag refuses a stamped d that differs from what the file's
	// own path derives: a generated bundle carries no foreign identity claims.
	ReasonForeignDTag = "foreign d-tag stamp"
)

// Census is the ingest's account of every decision the projection took: what
// became a concept and how, and what did not and why. Nothing the walk
// touches is dropped silently; a name absent from the bundle appears here
// instead. Every list is sorted, so one tree renders one census.
type Census struct {
	// Synthesized names the concepts whose frontmatter this run invented
	// because the source file carried none.
	Synthesized []string
	// Preserved names the concepts whose source files carried a frontmatter
	// block of their own.
	Preserved []string
	// Excluded names the members the projection refused, each with its reason.
	Excluded []Exclusion
	// Hidden names the leading-dot entries skipped without being descended.
	Hidden []string
	// Filtered names the top-level entries outside every include filter.
	Filtered []string
	// Attachments names the non-markdown files recorded for verbatim copy.
	Attachments []string
	// Owner is the npub the bundle claims.
	Owner string
	// Filters is the active include list, sorted: the published-scope
	// statement. The bundle is authoritative for its root's reference set,
	// so a run under a narrower list than what is live drops the missing
	// slices from the root on the next publish. Kept last for field
	// alignment: its trailing len/cap words fall outside the GC pointer-scan
	// region.
	Filters []string
}

// Ingest projects the tree rooted at source into a bundle for the named vault
// instance, owned by owner, whose key is found at nsecSource (empty for no
// claim; a caller stating one composes it with okf.EnvNsecSource). The
// include list names the top-level subtrees in scope; an empty list places
// the whole tree in scope. The returned bundle passes the publish gates
// unmodified, and the census accounts for every entry the walk saw.
func Ingest(source, name, owner, nsecSource string, include []string) (*okf.Bundle, *Census, error) {
	domain, err := lists.VaultDomain(name)
	if err != nil {
		return nil, nil, fmt.Errorf("vault name: %w", err)
	}
	cfg := &okf.VaultConfig{Owner: owner, NsecSource: nsecSource}
	if err = cfg.Validate(); err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(source)
	if err != nil {
		return nil, nil, fmt.Errorf("reading source: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("source %q is not a directory", source)
	}

	bundle, err := okf.NewBundle(domain.RootDTag)
	if err != nil {
		return nil, nil, err
	}
	bundle.VaultConfig = cfg
	// Stamped at assembly for the same reason the exporter stamps it: this
	// producer states every tag, and an unstamped bundle is refused by the
	// publish gate as one that may not.
	bundle.Root.Node.FormatVersion = okf.BundleFormatVersion

	w := &ingestWalk{
		bundle:  bundle,
		census:  &Census{Owner: owner, Filters: sortedStrings(include)},
		include: include,
		domain:  domain,
	}
	if err := w.directory(source, ""); err != nil {
		return nil, nil, err
	}
	if err := okf.CheckDTags(domain, w.concepts); err != nil {
		return nil, nil, err
	}
	w.census.sortLists()
	return bundle, w.census, nil
}

// ingestWalk carries one run's state: the bundle being assembled, the census
// being taken, and the concepts collected for the collision check.
type ingestWalk struct {
	bundle   *okf.Bundle
	census   *Census
	concepts []*okf.Concept
	include  []string
	domain   lists.Domain
}

// directory files everything the directory at diskPath holds under the bundle
// node at bundlePath, recursing into admitted subtrees. Creating the node
// before reading the entries is what carries an empty directory into the
// bundle: an empty directory is a fact about the vault.
func (w *ingestWalk) directory(diskPath, bundlePath string) error {
	dir, err := w.bundle.Dir(bundlePath)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(diskPath)
	if err != nil {
		return fmt.Errorf("reading directory %q: %w", diskPath, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		rel := joinMemberPath(bundlePath, name)
		if strings.HasPrefix(name, ".") {
			w.census.Hidden = append(w.census.Hidden, rel)
			continue
		}
		if bundlePath == "" && len(w.include) > 0 && !slices.Contains(w.include, name) {
			w.census.Filtered = append(w.census.Filtered, rel)
			continue
		}
		switch {
		case entry.IsDir():
			if err := w.subtree(filepath.Join(diskPath, name), rel, name); err != nil {
				return err
			}
		case !entry.Type().IsRegular():
			// The strict reader's posture, kept: a symlink resolves somewhere
			// this bundle does not describe, and nothing else irregular has a
			// concept form.
			return fmt.Errorf("%q is neither a regular file nor a directory", rel)
		case name == okf.IndexFileName:
			w.exclude(rel, ReasonReservedName)
		case !strings.HasSuffix(name, conceptFileExt):
			w.attachment(rel, name)
		default:
			if err := w.concept(filepath.Join(diskPath, name), rel, dir); err != nil {
				return err
			}
		}
	}
	return nil
}

// subtree admits one directory into the bundle, or refuses it whole: every
// member below an unnameable directory would fail the same way one at a time,
// and a directory bearing a reserved name would collide with the file its own
// parent's materialization writes.
func (w *ingestWalk) subtree(diskPath, rel, name string) error {
	if name == okf.IndexFileName {
		w.exclude(rel, ReasonReservedName)
		return nil
	}
	if _, err := w.domain.MemberDTag(rel); err != nil {
		w.exclude(rel, ReasonUnnameablePath)
		return nil
	}
	return w.directory(diskPath, rel)
}

// attachment censuses one non-markdown file for verbatim copy, or excludes
// it: the lenient half of the gate whose strict half is ReadBundle's refusal.
// A path the d-tag grammar cannot render cannot be stated on the wire, and a
// name CheckAttachmentName refuses would make the copied tree one the
// publish-read refuses whole. Given the walk's own skips (hidden entries, the
// reserved index, the concept suffix), the only name the last gate reaches is
// the vault-config near-miss, which is why its exclusion reason is the
// reserved name's. A root attachment passes: the reserved root set states it.
func (w *ingestWalk) attachment(rel, name string) {
	if _, err := w.domain.MemberDTag(rel); err != nil {
		w.exclude(rel, ReasonUnnameablePath)
		return
	}
	if err := okf.CheckAttachmentName(name); err != nil {
		w.exclude(rel, ReasonReservedName)
		return
	}
	w.census.Attachments = append(w.census.Attachments, rel)
}

// concept files one markdown source file as a concept, or censuses why not.
// The derived member d-tag is stamped as the explicit d, which is the fixpoint
// form the export writes: republishing what this run publishes stamps nothing
// new.
func (w *ingestWalk) concept(diskPath, rel string, dir *okf.Directory) error {
	conceptID := strings.TrimSuffix(rel, conceptFileExt)
	derived, err := w.domain.MemberDTag(conceptID)
	if err != nil {
		w.exclude(rel, ReasonUnnameablePath)
		return nil
	}
	data, err := os.ReadFile(filepath.Clean(diskPath))
	if err != nil {
		return fmt.Errorf("reading %q: %w", diskPath, err)
	}
	front, body, preserved, ok := w.frontmatter(data, rel)
	if !ok {
		return nil
	}
	if stamped := strings.TrimSpace(front.DTag); stamped != "" && stamped != derived {
		w.exclude(rel, ReasonForeignDTag)
		return nil
	}
	front.DTag = derived

	c := &okf.Concept{ConceptID: conceptID, Body: body, Frontmatter: front}
	if err := dir.AddConcept(c); err != nil {
		return err
	}
	w.concepts = append(w.concepts, c)
	if preserved {
		w.census.Preserved = append(w.census.Preserved, conceptID)
	} else {
		w.census.Synthesized = append(w.census.Synthesized, conceptID)
	}
	return nil
}

// frontmatter reads a source file's metadata leniently. A file with no fence
// synthesizes the statements OKF requires and humans never wrote. A fence
// that parses is preserved, unknown keys and all, a blank type defaulting
// rather than refusing. A fence that will not parse, or that states what the
// format derives, excludes the file: censused by name, ok false.
func (w *ingestWalk) frontmatter(data []byte, rel string) (front okf.Frontmatter, body string, preserved, ok bool) {
	fm, body, found, err := okf.SplitFrontmatter(data)
	if err != nil {
		w.exclude(rel, ReasonMalformedFrontmatter)
		return okf.Frontmatter{}, "", false, false
	}
	if !found {
		return okf.Frontmatter{Type: okf.TypeDocument}, string(data), false, true
	}
	if err := yaml.Unmarshal(fm, &front); err != nil {
		w.exclude(rel, ReasonMalformedFrontmatter)
		return okf.Frontmatter{}, "", false, false
	}
	if err := front.CheckNostrTags(); err != nil {
		w.exclude(rel, ReasonInvalidFrontmatter)
		return okf.Frontmatter{}, "", false, false
	}
	if strings.TrimSpace(front.Type) == "" {
		front.Type = okf.TypeDocument
	}
	// The timestamp is stamped in the export's fixpoint spelling, the rule the
	// derived d follows: the wire spells a moment in unix seconds and the
	// export restates it as the UTC instant, so any other spelling of the same
	// moment would diff on the first cycle though it states the same fact.
	front.Timestamp = okf.NormalizeTimestamp(front.Timestamp)
	return front, body, true, true
}

func (w *ingestWalk) exclude(path, reason string) {
	w.census.Excluded = append(w.census.Excluded, Exclusion{Path: path, Reason: reason})
}

// Render states the census in a stable order: the concept counts, then every
// refused, skipped, filtered, and accompanying name in full, ending with the
// active filter list and the owner. The filter line is deliberate: the
// include list is the published-scope statement, and the census restates it
// on every run so a narrowed scope is read before it is published.
func (c *Census) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "concepts: %d (%d synthesized, %d preserved)\n",
		len(c.Synthesized)+len(c.Preserved), len(c.Synthesized), len(c.Preserved))
	fmt.Fprintf(&b, "excluded: %d\n", len(c.Excluded))
	for _, e := range c.Excluded {
		fmt.Fprintf(&b, "  %s: %s\n", e.Path, e.Reason)
	}
	fmt.Fprintf(&b, "hidden entries skipped: %d\n", len(c.Hidden))
	for _, name := range c.Hidden {
		fmt.Fprintf(&b, "  %s\n", name)
	}
	fmt.Fprintf(&b, "filtered out of scope: %d\n", len(c.Filtered))
	for _, name := range c.Filtered {
		fmt.Fprintf(&b, "  %s\n", name)
	}
	fmt.Fprintf(&b, "attachments: %d\n", len(c.Attachments))
	for _, name := range c.Attachments {
		fmt.Fprintf(&b, "  %s\n", name)
	}
	if len(c.Filters) == 0 {
		b.WriteString("include filters: none; the whole tree is in scope\n")
	} else {
		fmt.Fprintf(&b, "include filters: %s\n", strings.Join(c.Filters, ", "))
	}
	fmt.Fprintf(&b, "owner: %s\n", c.Owner)
	return b.String()
}

// sortLists orders every census list so that rendering is a pure function of
// the tree rather than of the walk.
func (c *Census) sortLists() {
	sort.Strings(c.Synthesized)
	sort.Strings(c.Preserved)
	sort.Strings(c.Hidden)
	sort.Strings(c.Filtered)
	sort.Strings(c.Attachments)
	sort.Slice(c.Excluded, func(i, j int) bool {
		if c.Excluded[i].Path != c.Excluded[j].Path {
			return c.Excluded[i].Path < c.Excluded[j].Path
		}
		return c.Excluded[i].Reason < c.Excluded[j].Reason
	})
}

// joinMemberPath joins bundle-relative path segments with the separator the
// vault family declares, which is what keeps a member's census name equal to
// its name on the wire.
func joinMemberPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + lists.VaultPathSeparator + name
}

func sortedStrings(s []string) []string {
	out := slices.Clone(s)
	sort.Strings(out)
	return out
}
