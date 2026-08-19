package okf

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/buildtall-systems/buildtall/btk/lists"
)

// A bundle is the on-disk dual of a region of the ontology: the directory tree
// a person edits and git tracks. Under projection D its shape is the
// ontology's shape, because a concept's path from the bundle root is its d-tag
// verbatim. Writing a bundle is therefore a materialization, not a translation.

const (
	// IndexFileName is the reserved per-directory file a bundle carries. It
	// holds no frontmatter, so it is not a concept and OKF's type requirement
	// does not reach it. The leading underscore keeps the reservation off the
	// concept namespace: a person's own index.md is an ordinary concept.
	IndexFileName = "_index.md"

	// conceptFileExt is appended to a concept's final path segment to name its
	// file, and removed again to recover the ConceptID.
	conceptFileExt = ".md"

	// bundlePathSeparator separates the segments of a bundle-relative path. A
	// bundle is a directory tree, so the separator is "/"; a name-encoded
	// domain whose members materialize as one must declare the same separator,
	// which lists.VaultPathSeparator does.
	bundlePathSeparator = lists.VaultPathSeparator

	bundleDirPerm  os.FileMode = 0750
	bundleFilePerm os.FileMode = 0600
)

// Citation records something a directory references but does not contain: an
// item authored by someone other than the bundle's owner, or one whose kind
// has no concept form. Citations are rendered into the directory's index
// rather than materialized as files, so an export never presents another
// person's work as the owner's and never fabricates a local copy of what lives
// on a relay.
type Citation struct {
	// Title is the human-readable label, empty when the reference carried none.
	Title string
	// Ref is the NIP-19 identifier a reader resolves: naddr for an addressable
	// item, nevent for an event item.
	Ref string
	// AuthorNpub attributes the item, empty when the reference names an event
	// rather than a person.
	AuthorNpub string
}

// Attachment records one non-markdown file a directory holds: the file's
// name and the SHA-256 of its bytes. The bytes themselves are not model
// state: they sit beside the bundle on disk and in a Blossom store on the
// wire, and name plus hash is the whole of what the directory's set states
// about them.
type Attachment struct {
	// Name is the file's name inside its directory, a single path segment.
	Name string
	// SHA256 is the lowercase hex digest of the file's bytes: the identity
	// the store serves them under and the check both directions verify.
	SHA256 string
}

// Directory is one node of a bundle's tree. A directory holding nothing is
// still written, because an empty directory is a fact about the vault and
// dropping it would make the export lossy.
type Directory struct {
	// subdirs leads for field alignment: a map is one pointer word, so placing
	// it first keeps the GC's pointer-scan region off the trailing len/cap
	// words of the slices below.
	subdirs map[string]*Directory
	// Path is the directory's bundle-relative path, empty at the root.
	Path      string
	Concepts  []*Concept
	Citations []Citation
	// Attachments are the non-markdown files the directory holds, sorted by
	// name so one tree states one tag list whichever way it was assembled.
	Attachments []Attachment
	// Node is what the event this directory stands for carries beyond the tree
	// itself. Kept last for field alignment: it ends in a slice whose trailing
	// len/cap words fall outside the GC pointer-scan region.
	Node NodeMetadata
}

// SkipReasonUnreferenced is the reason tier 1 records for every skip it
// makes, and the only reason it has: no included document references the
// file, so the publish does not state it.
const SkipReasonUnreferenced = "unreferenced by any included document"

// SkippedFile records one non-markdown file the tier 1 rule left out of a
// publish: the bundle-relative path and why. The record is the observability
// half of the rule, because a skip the operator cannot see is a file that
// silently vanished from the wire.
type SkippedFile struct {
	Path   string
	Reason string
}

// Bundle is an OKF bundle held in memory: a named root directory and the tree
// beneath it. Name is the directory written under the output directory; for a
// vault it is the root d-tag, which is what makes a concept's path from the
// output directory its d-tag verbatim.
type Bundle struct {
	Root *Directory
	// VaultConfig is what the bundle states about itself beyond its tree:
	// whose vault it is, and where that owner's key is found. It is nil where
	// the bundle states nothing, which is every bundle written before the file
	// existed and is why the absence is an answer rather than a failure.
	VaultConfig *VaultConfig
	Name        string
	// Skipped and Dangling are what ReadBundle's tier 1 post-pass observed:
	// the non-markdown files no included document references, left unstated,
	// and the references that resolved to nothing. Both are empty on a
	// programmatically built bundle, whose caller stated every attachment
	// deliberately.
	Skipped  []SkippedFile
	Dangling []DanglingLink
}

// NewBundle returns an empty bundle whose root directory is named name.
func NewBundle(name string) (*Bundle, error) {
	if err := CheckPathSegments(name, bundlePathSeparator); err != nil {
		return nil, fmt.Errorf("okf: bundle name: %w", err)
	}
	if strings.Contains(name, bundlePathSeparator) {
		return nil, fmt.Errorf("okf: bundle name %q contains %q; the name is one directory", name, bundlePathSeparator)
	}
	return &Bundle{Name: name, Root: newDirectory("")}, nil
}

// Dir returns the directory at the bundle-relative path, creating it and every
// intermediate directory not already present. The empty path is the root.
func (b *Bundle) Dir(path string) (*Directory, error) {
	if path == "" {
		return b.Root, nil
	}
	if err := CheckPathSegments(path, bundlePathSeparator); err != nil {
		return nil, fmt.Errorf("okf: directory: %w", err)
	}
	dir := b.Root
	for segment := range strings.SplitSeq(path, bundlePathSeparator) {
		dir = dir.child(segment)
	}
	return dir, nil
}

// AddConcept places a concept in the directory. The concept's path must name
// this directory as its parent: a concept's identity is its path, so a bundle
// that filed it elsewhere would republish it under a name it does not have.
func (d *Directory) AddConcept(c *Concept) error {
	if c == nil {
		return errors.New("okf: cannot add a nil concept to a bundle")
	}
	if err := CheckPathSegments(c.ConceptID, bundlePathSeparator); err != nil {
		return fmt.Errorf("okf: concept: %w", err)
	}
	parent, name := splitPath(c.ConceptID)
	if parent != d.Path {
		return fmt.Errorf("okf: concept %q does not belong to directory %q", c.ConceptID, d.Path)
	}
	if name+conceptFileExt == IndexFileName {
		return fmt.Errorf("okf: concept %q would overwrite the reserved %q", c.ConceptID, IndexFileName)
	}
	for _, existing := range d.Concepts {
		if existing.ConceptID == c.ConceptID {
			return fmt.Errorf("okf: concept %q is already in the bundle", c.ConceptID)
		}
	}
	d.Concepts = append(d.Concepts, c)
	return nil
}

// AddAttachment records a non-markdown file the directory holds, keeping the
// list sorted and refusing a name stated twice: two files cannot share one
// name on disk, so a duplicate can only be one statement arriving from two
// sources, and whichever hash lost would lose silently.
func (d *Directory) AddAttachment(a Attachment) error {
	if err := CheckAttachmentName(a.Name); err != nil {
		return fmt.Errorf("okf: attachment: %w", err)
	}
	if !isBlobHash(a.SHA256) {
		return fmt.Errorf("okf: attachment %q: %q is not a lowercase hex sha-256", a.Name, a.SHA256)
	}
	i, found := slices.BinarySearchFunc(d.Attachments, a, func(x, y Attachment) int {
		return strings.Compare(x.Name, y.Name)
	})
	if found {
		return fmt.Errorf("okf: attachment %q is already in directory %q", a.Name, d.Path)
	}
	d.Attachments = slices.Insert(d.Attachments, i, a)
	return nil
}

// CheckAttachmentName rejects a name no attachment can carry. An attachment
// is one path segment inside its directory, so the segment rules apply. A
// leading dot is the namespace the format reserves for its own sidecars, and
// the one ingest skips as hidden, so a dotted attachment could never survive
// a round trip through a raw tree. The concept suffix names a concept, and
// the vault-config near-miss rule holds here for the same reason it holds in
// the reader: both directions of the bundle must refuse exactly the same
// names, or an export could write a file the next read refuses.
func CheckAttachmentName(name string) error {
	if err := CheckPathSegments(name, bundlePathSeparator); err != nil {
		return err
	}
	if strings.Contains(name, bundlePathSeparator) {
		return fmt.Errorf("attachment name %q contains %q; the name is one segment inside its directory", name, bundlePathSeparator)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("attachment name %q begins with a dot, which the format reserves for its own sidecars", name)
	}
	if strings.HasSuffix(name, conceptFileExt) {
		return fmt.Errorf("attachment name %q ends in %q, which names a concept rather than an attachment", name, conceptFileExt)
	}
	if isVaultConfigNearMiss(name) {
		return fmt.Errorf("attachment name %q is nearly %q, and a bundle stating its owner in a file this format does not read states no owner: rename it", name, VaultConfigFileName)
	}
	return nil
}

// isBlobHash reports whether s is a lowercase hex SHA-256, the one spelling
// the wire and the store agree on. Uppercase is rejected rather than folded:
// a hash is compared byte-for-byte in tags, so two spellings of one digest
// would be two statements of one fact.
func isBlobHash(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for _, c := range []byte(s) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// AddCitation records a reference the directory carries but does not contain.
func (d *Directory) AddCitation(c Citation) {
	d.Citations = append(d.Citations, c)
}

// Children returns the subdirectories in lexical order, so a written bundle
// never depends on the order events arrived from a relay.
func (d *Directory) Children() []*Directory {
	names := make([]string, 0, len(d.subdirs))
	for name := range d.subdirs {
		names = append(names, name)
	}
	sort.Strings(names)

	children := make([]*Directory, 0, len(names))
	for _, name := range names {
		children = append(children, d.subdirs[name])
	}
	return children
}

// WriteBundle materializes the bundle under outDir: every directory is
// created, including the empty ones; every concept is written at its path; and
// every directory receives an index of what it holds.
func WriteBundle(b *Bundle, outDir string) error {
	if b == nil || b.Root == nil {
		return errors.New("okf: cannot write a nil bundle")
	}
	if err := CheckPathSegments(b.Name, bundlePathSeparator); err != nil {
		return fmt.Errorf("okf: bundle name: %w", err)
	}
	rootPath := filepath.Join(outDir, b.Name)
	if err := writeDirectory(rootPath, b.Name, b.Root); err != nil {
		return err
	}
	return writeVaultConfig(rootPath, b.VaultConfig)
}

// writeVaultConfig writes the vault-local file at the bundle root and nowhere
// else, so the tree carries one answer to whose vault it is. A bundle claiming
// nothing writes no file, which is what an older bundle read back and written
// out again looks like.
func writeVaultConfig(rootPath string, v *VaultConfig) error {
	if v == nil {
		return nil
	}
	data, err := WriteVaultConfig(*v)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(rootPath, VaultConfigFileName), data, bundleFilePerm); err != nil {
		return fmt.Errorf("okf: writing %q: %w", VaultConfigFileName, err)
	}
	return nil
}

func newDirectory(path string) *Directory {
	return &Directory{Path: path, subdirs: map[string]*Directory{}}
}

// child returns the named subdirectory, creating it on first reference so an
// intermediate directory exists whether or not the wire carried a set for it.
func (d *Directory) child(name string) *Directory {
	if existing, ok := d.subdirs[name]; ok {
		return existing
	}
	child := newDirectory(joinPath(d.Path, name))
	d.subdirs[name] = child
	return child
}

func writeDirectory(diskPath, title string, d *Directory) error {
	if err := os.MkdirAll(diskPath, bundleDirPerm); err != nil {
		return fmt.Errorf("okf: creating directory %q: %w", diskPath, err)
	}

	for _, c := range sortedConcepts(d.Concepts) {
		data, err := WriteConcept(c)
		if err != nil {
			return err
		}
		_, name := splitPath(c.ConceptID)
		if err := os.WriteFile(filepath.Join(diskPath, name+conceptFileExt), data, bundleFilePerm); err != nil {
			return fmt.Errorf("okf: writing concept %q: %w", c.ConceptID, err)
		}
	}

	if !d.Node.IsEmpty() {
		data, err := WriteNode(d.Node)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(diskPath, NodeFileName), data, bundleFilePerm); err != nil {
			return fmt.Errorf("okf: writing %q for directory %q: %w", NodeFileName, d.Path, err)
		}
	}

	index := []byte(renderIndex(title, d))
	if err := os.WriteFile(filepath.Join(diskPath, IndexFileName), index, bundleFilePerm); err != nil {
		return fmt.Errorf("okf: writing %q for directory %q: %w", IndexFileName, d.Path, err)
	}

	for _, child := range d.Children() {
		_, name := splitPath(child.Path)
		if err := writeDirectory(filepath.Join(diskPath, name), name, child); err != nil {
			return err
		}
	}
	return nil
}

// renderIndex builds a directory's index: a frontmatter-free listing of the
// subdirectories and concepts it holds, followed by the references it carries
// without containing. The absence of frontmatter is deliberate, since an index
// describes the bundle rather than adding to the ontology, and a file with no
// frontmatter cannot be mistaken for a concept.
func renderIndex(title string, d *Directory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", title)

	if children := d.Children(); len(children) > 0 {
		b.WriteString("\n## Directories\n\n")
		for _, child := range children {
			_, name := splitPath(child.Path)
			// The link targets the child's index rather than the directory,
			// so it resolves to a file in any renderer, not only in one that
			// knows to look inside a directory for an index.
			fmt.Fprintf(&b, "- %s\n", markdownLink(name, name+bundlePathSeparator+IndexFileName))
		}
	}

	if concepts := sortedConcepts(d.Concepts); len(concepts) > 0 {
		b.WriteString("\n## Concepts\n\n")
		for _, c := range concepts {
			_, name := splitPath(c.ConceptID)
			line := "- " + markdownLink(conceptLabel(c, name), name+conceptFileExt)
			if desc := c.Frontmatter.Description; desc != "" {
				line += ": " + desc
			}
			b.WriteString(line + "\n")
		}
	}

	// Citations keep the order the referencing event listed them in, which is
	// the owner's own ordering and is already deterministic.
	if len(d.Citations) > 0 {
		b.WriteString("\n## Citations\n\n")
		for _, c := range d.Citations {
			b.WriteString(renderCitation(c) + "\n")
		}
	}

	return b.String()
}

// conceptLabel prefers the concept's title, falling back to its file name so a
// titleless concept still reads as something rather than as an empty link.
func conceptLabel(c *Concept, name string) string {
	if title := strings.TrimSpace(c.Frontmatter.Title); title != "" {
		return title
	}
	return name
}

func renderCitation(c Citation) string {
	label := c.Title
	if label == "" {
		label = c.Ref
	}
	line := "- " + label
	if c.Ref != "" && c.Ref != label {
		line += ": " + c.Ref
	}
	if c.AuthorNpub != "" {
		line += " by " + c.AuthorNpub
	}
	return line
}

// markdownLink renders an angle-bracketed link target, which is what keeps a
// verbatim path (spaces and all) resolvable without percent-encoding it into
// something that no longer matches the name on the wire.
func markdownLink(text, target string) string {
	return fmt.Sprintf("[%s](<%s>)", text, target)
}

// sortedConcepts orders concepts lexically by path without disturbing the
// caller's slice, so writing a bundle twice yields the same bytes.
func sortedConcepts(concepts []*Concept) []*Concept {
	ordered := make([]*Concept, len(concepts))
	copy(ordered, concepts)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].ConceptID < ordered[j].ConceptID
	})
	return ordered
}

// splitPath separates a bundle-relative path into its parent directory and
// final segment. A top-level path's parent is the root, whose path is empty.
func splitPath(path string) (parent, name string) {
	i := strings.LastIndex(path, bundlePathSeparator)
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+len(bundlePathSeparator):]
}

func joinPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + bundlePathSeparator + name
}
