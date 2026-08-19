package okf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// A directory has no concept file, so the event it stands for, a kind 30004 set
// or the kind 30101 at the bundle root, has nowhere to state what it carries.
// The sidecar is that place. It is a dotfile, so a renderer walking the bundle
// passes over it without being taught to, and it is written only where there is
// something to state, since an empty sidecar in every directory would be noise
// in a format whose whole argument is that it is browsable.

const (
	// NodeFileName is the reserved per-directory sidecar stating the event the
	// directory stands for.
	NodeFileName = ".okf.yaml"

	// NodeContentKey holds that event's content verbatim. It is spelled once
	// here and once in the decoder's struct tag, which the round trip binds
	// together.
	NodeContentKey = "content"

	// NodeFormatVersionKey holds the bundle format version. Only the bundle
	// root states it, since it is a fact about the bundle rather than about any
	// one directory.
	NodeFormatVersionKey = "format_version"

	// BundleFormatVersion is the version this exporter writes into the root
	// sidecar, and the only one a publish accepts. It is an integer rather than
	// a dotted string because the only question ever asked of it is whether a
	// bundle predates the exporter, and an integer answers that without a
	// parser deciding whether "0.1" and "0.1.0" are the same version.
	//
	// Version 1 is the first that states every tag. A bundle written before it
	// carries no version at all, which reads as zero and is refused: publishing
	// it under whole-tag-set authority would delete every tag the older format
	// could not express.
	//
	// Version 2 renames the generated index from index.md to _index.md, freeing
	// index.md as an ordinary concept. A version 1 bundle's generated index.md
	// read under version 2 rules would be misread as a concept with no
	// frontmatter, so the gate refuses it with a remedy instead.
	BundleFormatVersion = 2
)

// NodeMetadata is everything a directory's event carries that the tree does not
// derive. The tree derives exactly two things: the d tag, which under
// projection D is the directory's path verbatim, and the a coordinates naming
// the owner's own members, which are the concept files beside the directory and
// the subdirectories beneath it. Everything else states itself here.
type NodeMetadata struct {
	// Content is the event's content, empty for most sets and non-empty
	// wherever a person wrote prose into the list itself.
	Content string
	// NostrTags is every tag the event carries that the tree does not derive,
	// held verbatim: name first, then every remaining element, in the order the
	// wire gave them. A foreign reference lives here as the tag it is, which is
	// what carries it across a cold create.
	NostrTags [][]string
	// FormatVersion is the bundle format the exporter wrote, stated at the
	// bundle root and nowhere else, and zero everywhere it is not stated. It
	// trails the two fields holding pointers, which is what keeps the GC's
	// pointer-scan region to the words that actually carry one.
	FormatVersion int
}

// IsEmpty reports whether the node states nothing at all, which is when a
// bundle writes no sidecar for it. The version counts as something stated, so
// the root's sidecar is written even for a vault whose root carries nothing
// else, which is what keeps every exported bundle answerable about its format.
func (n NodeMetadata) IsEmpty() bool {
	return n.FormatVersion == 0 && n.Content == "" && len(n.NostrTags) == 0
}

// CheckNostrTags refuses a sidecar tag list that states what the directory's own
// location already states, for the same reason a concept's frontmatter may not:
// two statements of one fact need a precedence rule, and a silent precedence
// rule is how a lossy round trip returns.
func (n *NodeMetadata) CheckNostrTags() error {
	for _, tag := range n.NostrTags {
		if err := checkRawTag(tag); err != nil {
			return fmt.Errorf("%s: %w", NodeFileName, err)
		}
	}
	return nil
}

// CheckOwnedMembers refuses a coordinate naming something inside the owner's own
// vault. Those coordinates are the directory tree restating itself: a set's
// owned members are the files and directories beside it, and the root's are the
// directories themselves, all of them derived when the bundle is published. A
// reference to anybody else's work, or to the owner's work outside this vault,
// is exactly what the sidecar exists to carry and passes untouched.
func (n *NodeMetadata) CheckOwnedMembers(domain lists.Domain, ownerHex string) error {
	for _, tag := range n.NostrTags {
		if len(tag) < 2 || tag[0] != btknostr.TagCoordinate {
			continue
		}
		if derivedCoordinate(domain, ownerHex, tag[1]) {
			return fmt.Errorf("%s states the coordinate %q, which the bundle tree already states: move the directory rather than restating what holds it",
				NodeFileName, tag[1])
		}
	}
	return nil
}

// derivedCoordinate reports whether a coordinate is one the bundle tree states
// by existing: the owner's own work inside this vault, which is the concept
// files and the directories themselves. A coordinate that will not parse, one
// written by somebody else, and one naming the owner's work in another domain
// are all references a bundle carries rather than derives. Export drops what
// this names and the sidecar refuses it, from the one predicate, so the two
// cannot disagree about which half of a set is the tree.
func derivedCoordinate(domain lists.Domain, ownerHex, coord string) bool {
	_, pubkeyHex, dTag, err := btknostr.ParseCoordinate(coord)
	if err != nil || pubkeyHex != ownerHex {
		return false
	}
	class := domain.ClassifyDTag(dTag)
	return class == lists.DTagRoot || class == lists.DTagMember
}

// ParseNode reads sidecar bytes. A key the format does not define is refused
// rather than dropped: a misspelled key would otherwise be silent data loss on
// the next publish, which is the very class of defect the sidecar exists to end.
func ParseNode(data []byte) (*NodeMetadata, error) {
	var raw struct {
		Content       string     `yaml:"content"`
		NostrTags     [][]string `yaml:"nostr_tags"`
		FormatVersion int        `yaml:"format_version"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// An empty sidecar decodes to EOF and is a node that states nothing, which
	// is a shape a person can reach by emptying a file rather than deleting it.
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: %w", NodeFileName, err)
	}

	n := &NodeMetadata{FormatVersion: raw.FormatVersion, Content: raw.Content, NostrTags: raw.NostrTags}
	if err := n.CheckNostrTags(); err != nil {
		return nil, err
	}
	return n, nil
}

// WriteNode serializes node metadata to sidecar bytes: the version first where
// there is one, then the tag list, one tag per line, then the content, so the
// statements a reader scans for come before the prose they may have to scroll
// past.
func WriteNode(n NodeMetadata) ([]byte, error) {
	m := &yaml.Node{Kind: yaml.MappingNode}
	if n.FormatVersion != 0 {
		m.Content = append(m.Content, strScalar(NodeFormatVersionKey), intScalar(n.FormatVersion))
	}
	if len(n.NostrTags) > 0 {
		m.Content = append(m.Content, strScalar(NostrTagsKey), nostrTagsNode(n.NostrTags))
	}
	if n.Content != "" {
		m.Content = append(m.Content, strScalar(NodeContentKey), contentScalar(n.Content))
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("okf: marshaling %s: %w", NodeFileName, err)
	}
	return data, nil
}

// contentScalar renders content as a literal block wherever it spans lines, so
// prose stays prose in a diff. yaml.v3 falls back to a quoted scalar on its own
// where a block cannot represent the value exactly.
func contentScalar(s string) *yaml.Node {
	node := strScalar(s)
	if strings.Contains(s, "\n") {
		node.Style = yaml.LiteralStyle
	}
	return node
}

// readNode reads the sidecar at diskPath, naming the file in every failure
// because a bundle carries one per directory and the message is otherwise
// indistinguishable from every other one.
func readNode(diskPath string) (*NodeMetadata, error) {
	data, err := os.ReadFile(filepath.Clean(diskPath))
	if err != nil {
		return nil, fmt.Errorf("okf: reading %q: %w", diskPath, err)
	}
	n, err := ParseNode(data)
	if err != nil {
		return nil, fmt.Errorf("okf: %q: %w", diskPath, err)
	}
	return n, nil
}
