package okf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Reading a bundle inverts WriteBundle over everything the format states: the
// tree's shape, which concepts each directory holds, each concept's frontmatter
// and body, and the sidecar in which a directory states the event it stands
// for. Under projection D that is enough to recover a concept's identity,
// because its path from the bundle root is its d-tag verbatim and needs no
// lookup to reconstruct.
//
// A directory's references to items it does not contain are tags in that
// sidecar, so they survive the trip as the facts they are. index.md renders
// them a second time as prose for a human reader, and that rendering is
// recovered from nothing: index.md carries no frontmatter precisely so that it
// cannot be mistaken for a concept, and ParseConcept refuses it by design.

// ReadBundle reads the bundle rooted at dir, which is the directory WriteBundle
// created and named, not the directory it was written into. Every directory
// becomes a node including those holding nothing, since an empty directory is a
// fact about the vault and dropping it here would make the round trip lossy in
// the one direction the export was careful about.
func ReadBundle(dir string) (*Bundle, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("okf: reading bundle: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("okf: bundle %q is not a directory", dir)
	}
	b, err := NewBundle(filepath.Base(filepath.Clean(dir)))
	if err != nil {
		return nil, err
	}
	var pending []pendingAttachment
	if err := readDirectory(dir, "", b, &pending); err != nil {
		return nil, err
	}
	if err := pruneUnreferenced(b, pending); err != nil {
		return nil, err
	}
	return b, nil
}

// pendingAttachment is one non-markdown file the walk censused and tier 1
// has not yet ruled on: the directory node that will hold it if a link
// claims it, its bundle-relative path, its name, and its hash.
type pendingAttachment struct {
	dir  *Directory
	path string
	name string
	hash string
}

// pruneUnreferenced is the tier 1 skip rule: of the censused non-markdown
// files, only those some included document references publish. The graph
// resolves against the full census, so every real name participates in
// matching, and the attachment name rules run only for the files a link
// claims: a referenced dotfile still refuses, because a link must not
// launder a reserved name, while an unreferenced one skips before the rule
// can fire, which is the ordering that lets .last-update.json sit in a vault
// without blocking its publish. Skips and dangling references are recorded
// on the bundle, because a silent skip would be a file that vanished from
// the wire with no account of itself.
func pruneUnreferenced(b *Bundle, pending []pendingAttachment) error {
	concepts := bundleConcepts(b)
	idx := newFileIndex()
	for _, c := range concepts {
		idx.add(FileEntry{Path: c.ConceptID + conceptFileExt})
	}
	for _, p := range pending {
		idx.add(FileEntry{Path: p.path, SHA256: p.hash})
	}
	g := graphOver(concepts, idx)

	referenced := make(map[string]bool, len(g.Edges))
	for _, e := range g.Edges {
		referenced[e.Target.Path] = true
	}

	for _, p := range pending {
		if !referenced[p.path] {
			b.Skipped = append(b.Skipped, SkippedFile{Path: p.path, Reason: SkipReasonUnreferenced})
			continue
		}
		if err := CheckAttachmentName(p.name); err != nil {
			return fmt.Errorf("okf: attachment %q: %w", p.path, err)
		}
		if err := p.dir.AddAttachment(Attachment{Name: p.name, SHA256: p.hash}); err != nil {
			return err
		}
	}
	b.Dangling = g.Dangling
	return nil
}

// readDirectory fills the bundle node at bundlePath from the directory at
// diskPath and recurses, so the tree is built top down and every intermediate
// node exists before anything is filed under it.
func readDirectory(diskPath, bundlePath string, b *Bundle, pending *[]pendingAttachment) error {
	dir, err := b.Dir(bundlePath)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(diskPath)
	if err != nil {
		return fmt.Errorf("okf: reading directory %q: %w", diskPath, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		childPath := joinPath(bundlePath, name)
		switch {
		case entry.IsDir():
			// A directory named as one of the files a bundle reserves would
			// collide with the file its own parent writes, so the tree could
			// not be materialized again.
			if name == IndexFileName || name == NodeFileName || name == VaultConfigFileName {
				return fmt.Errorf("okf: %q is a directory, but %q names a file this format reserves", childPath, name)
			}
			if err := readDirectory(filepath.Join(diskPath, name), childPath, b, pending); err != nil {
				return err
			}
		case !entry.Type().IsRegular():
			// A symlink resolves somewhere this bundle does not describe, and
			// nothing else irregular has a concept form. Refusing beats
			// following it out of the tree or dropping it in silence.
			return fmt.Errorf("okf: %q is neither a regular file nor a directory", childPath)
		case name == IndexFileName:
			// The index describes the bundle rather than adding to the
			// ontology. It carries no frontmatter and holds nothing to recover.
		case name == NodeFileName:
			// The sidecar states the event this directory stands for, which no
			// concept file can, since a directory has none.
			meta, err := readNode(filepath.Join(diskPath, name))
			if err != nil {
				return err
			}
			// The version describes the bundle, and a bundle has one root. A
			// subdirectory stating its own would be a second answer to a
			// question with one, and whichever answer lost would lose silently.
			if bundlePath != "" && meta.FormatVersion != 0 {
				return fmt.Errorf("okf: %q states %s, which only the bundle root states",
					childPath, NodeFormatVersionKey)
			}
			dir.Node = *meta
		case name == VaultConfigFileName:
			// Whose vault this is is a fact about the bundle, and a bundle has
			// one root. A subdirectory stating its own owner would be a second
			// answer to a question with one, and whichever answer lost would
			// lose silently.
			if bundlePath != "" {
				return fmt.Errorf("okf: %q states %s, which only the bundle root states",
					childPath, VaultConfigFileName)
			}
			cfg, err := readVaultConfig(filepath.Join(diskPath, name))
			if err != nil {
				return err
			}
			b.VaultConfig = cfg
		case isVaultConfigNearMiss(name):
			// Refused rather than tolerated as an attachment, because a bundle whose
			// owner is stated in a misspelled file states no owner at all and
			// falls back to whatever configuration is ambient, which is the
			// failure the file exists to prevent.
			return fmt.Errorf("okf: %q is not %q, and a bundle stating its owner in a file this format does not read states no owner: rename it",
				childPath, VaultConfigFileName)
		case !strings.HasSuffix(name, conceptFileExt):
			// A non-markdown file: censused leniently by path, name, and
			// hash, because a publish states what this read finds and a file
			// passed over in silence would be a file the round trip loses.
			// Whether it publishes is tier 1's decision, made after the whole
			// tree is read, since only the link graph can say whether any
			// included document references it; the name rules wait with it.
			hash, err := readAttachment(filepath.Join(diskPath, name))
			if err != nil {
				return fmt.Errorf("okf: attachment %q: %w", childPath, err)
			}
			*pending = append(*pending, pendingAttachment{dir: dir, path: childPath, name: name, hash: hash})
		default:
			c, err := readConcept(filepath.Join(diskPath, name), strings.TrimSuffix(childPath, conceptFileExt))
			if err != nil {
				return err
			}
			if err := dir.AddConcept(c); err != nil {
				return err
			}
		}
	}
	return nil
}

// readAttachment hashes one censused file's bytes. The digest is taken here,
// at publish-read time, so the tree on disk is the sole source of what a set
// states. The name is deliberately not checked here: tier 1 decides first
// whether the file publishes at all, and only a referenced file faces the
// name rules.
func readAttachment(diskPath string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(diskPath))
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// readConcept parses the file at diskPath as the concept identified by
// conceptID. The identity is checked before the file is opened, because a path
// that cannot name a concept cannot be published as one and the parse would
// only bury that under a second failure.
func readConcept(diskPath, conceptID string) (*Concept, error) {
	if err := CheckPathSegments(conceptID, bundlePathSeparator); err != nil {
		return nil, fmt.Errorf("okf: concept file %q: %w", diskPath, err)
	}
	data, err := os.ReadFile(filepath.Clean(diskPath))
	if err != nil {
		return nil, fmt.Errorf("okf: reading concept %q: %w", conceptID, err)
	}
	return ParseConcept(data, conceptID)
}
