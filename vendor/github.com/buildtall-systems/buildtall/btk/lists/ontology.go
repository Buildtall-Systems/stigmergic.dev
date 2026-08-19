package lists

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// This file is the declarative model of the personal ontology: the ratified
// specification (operations repo, concepts/nip-101.md) rendered as data.
// Validation and traversal derive their rules from these declarations.

const (
	// DefaultMaxDepth is buildtall's declared traversal depth limit. The root
	// is at depth 0; nodes deeper than the limit are pruned. nip-101 mandates
	// that a limit exist but leaves the value to the implementation, so this
	// constant is the declaration rather than a restatement of the spec. It is
	// 7 to accommodate the leaf-list hop three-tier composition requires.
	DefaultMaxDepth = 7
	// DepthCeiling is the hard bound no configured depth may exceed.
	DepthCeiling = 10
)

// NormalizeDepth resolves a configured depth against the declared policy:
// non-positive takes the default; above the ceiling clamps to it.
func NormalizeDepth(depth int) int {
	switch {
	case depth <= 0:
		return DefaultMaxDepth
	case depth > DepthCeiling:
		return DepthCeiling
	default:
		return depth
	}
}

// compositionKinds is the set of kinds a kind 30101 "a" tag may reference:
// the NIP-51 set kinds plus 30101 itself. 30101 events compose, leaves
// contain — application content is never referenced directly.
var compositionKinds = map[int]bool{
	KindFollowSet:   true,
	KindRelaySet:    true,
	KindBookmarkSet: true,
	KindCurationSet: true,
	KindInterestSet: true,
	KindEmojiSet:    true,
	KindListSet:     true,
}

// AllowedCompositionKind reports whether a kind 30101 "a" tag may reference
// events of the given kind.
func AllowedCompositionKind(kind int) bool {
	return compositionKinds[kind]
}

// feedServableKinds is the set of kinds whose harvest yields readable
// content: a follow set names authors, a curation set names long-form
// documents, and a composition node carries whatever its leaves carry. Every
// other set kind names something no feed can render.
//
// The set lives here rather than in either service because both read it: n2r
// fetches exactly these kinds and refuses the rest, and drss offers a feed
// URL for exactly these coordinates. A link one offers and the other refuses
// is the defect this single declaration prevents.
var feedServableKinds = map[int]bool{
	KindFollowSet:   true,
	KindCurationSet: true,
	KindListSet:     true,
}

// FeedServableKind reports whether a list of the given kind can be rendered
// as a feed.
func FeedServableKind(kind int) bool {
	return feedServableKinds[kind]
}

// FeedServableKinds returns the servable kinds in ascending order, for a
// relay filter or a refusal that must name them.
func FeedServableKinds() []int {
	kinds := make([]int, 0, len(feedServableKinds))
	for kind := range feedServableKinds {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return kinds
}

// kindAllowedTags declares, per list kind, the item tag types its events may
// carry. Kinds absent from the table accept no items (30001 is deprecated).
var kindAllowedTags = map[int][]string{
	KindFollowSet:   {"p"},
	KindRelaySet:    {itemTypeRelay},
	KindBookmarkSet: {"a", "e"},
	KindCurationSet: {"a", "e"},
	KindInterestSet: {"t"},
	KindEmojiSet:    {tagEmoji},
	KindListSet:     {"a"},
}

// Domain declares the convention set under which an application writes into
// a user's ontology: its canonical root, its slice of the owner's d-tag
// namespace, and the leaf kind its lists use. Entry-points are discoverable
// from the declaration, never from a writer's source.
type Domain struct {
	Name            string
	RootDTag        string
	DTagPrefix      string
	ImportPrefix    string
	CompanionSuffix string
	// PathSeparator is non-empty when the domain uses name-encoded structure:
	// a member carries its position in the hierarchy as a separated path in
	// its own d-tag rather than in composition edges, so the hierarchy is
	// reconstructed lexically instead of by traversal.
	PathSeparator string
	// RootTitle and CompanionTitle are the titles a deposit ceremony mints
	// when the canonical root or a companion leaf does not exist yet. They
	// live on the declaration so every writer mints the same names.
	RootTitle      string
	CompanionTitle string
	LeafKind       int
}

// DTagClass classifies a d-tag against a domain's declared namespace. A
// companion leaf is not name-distinguishable from a like-named member (the
// suffix is a writer convention, not a partition), so companions classify as
// members; the companion role is structural, read from composition.
type DTagClass int

const (
	DTagForeign DTagClass = iota
	DTagRoot
	DTagImport
	DTagMember
)

// ClassifyDTag places a d-tag within the domain's declared namespace.
func (d Domain) ClassifyDTag(dTag string) DTagClass {
	switch {
	case dTag == d.RootDTag:
		return DTagRoot
	case d.ImportPrefix != "" && strings.HasPrefix(dTag, d.ImportPrefix):
		return DTagImport
	case strings.HasPrefix(dTag, d.DTagPrefix):
		return DTagMember
	default:
		return DTagForeign
	}
}

// MemberDTag renders the d-tag of the member at path within a name-encoded
// domain. Paths are verbatim: case, spacing, and non-ASCII characters survive
// unaltered, so a member's name on disk and its name on the wire are one
// string.
func (d Domain) MemberDTag(path string) (string, error) {
	if d.PathSeparator == "" {
		return "", fmt.Errorf("domain %q does not declare name-encoded structure", d.Name)
	}
	if err := validateMemberPath(path, d.PathSeparator); err != nil {
		return "", err
	}
	return d.DTagPrefix + path, nil
}

// MemberPath recovers a member's path from its d-tag, inverting MemberDTag.
func (d Domain) MemberPath(dTag string) (string, error) {
	if d.PathSeparator == "" {
		return "", fmt.Errorf("domain %q does not declare name-encoded structure", d.Name)
	}
	if d.ClassifyDTag(dTag) != DTagMember {
		return "", fmt.Errorf("d-tag %q is not a member of domain %q", dTag, d.Name)
	}
	path := strings.TrimPrefix(dTag, d.DTagPrefix)
	if err := validateMemberPath(path, d.PathSeparator); err != nil {
		return "", err
	}
	return path, nil
}

// validateMemberPath enforces the rules the ontology imposes on a name-encoded
// path: it must name something, it may not carry the coordinate field
// delimiter, and every segment must name something. The vault family reserves
// one segment, the root set's own name, derived from its companion suffix: a
// path other than that exact segment may not carry it. Constraints belonging
// to a consumer's own medium, such as a filesystem's reading of "." and "..",
// are that consumer's to enforce when it materializes the path.
func validateMemberPath(path, separator string) error {
	if path == "" {
		return errors.New("member path is empty")
	}
	if strings.Contains(path, ":") {
		return fmt.Errorf(`member path %q contains ":", which delimits coordinate fields`, path)
	}
	segments := strings.Split(path, separator)
	if slices.Contains(segments, "") {
		return fmt.Errorf("member path %q has an empty segment", path)
	}
	if path != VaultRootSegment && slices.Contains(segments, VaultRootSegment) {
		return fmt.Errorf("member path %q carries the reserved segment %q, which names only the root set", path, VaultRootSegment)
	}
	return nil
}

var (
	slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
	slugRepeat   = regexp.MustCompile(`-+`)
)

// Slug lowercases a name and reduces it to a stable d-tag fragment: runs of
// non-alphanumeric characters collapse to a single hyphen, and leading and
// trailing hyphens are trimmed. An empty result becomes "untitled". The rule
// lives here because every writer that mints a d-tag from a human name owes
// the others the same name for the same title.
func Slug(name string) string {
	s := slugNonAlnum.ReplaceAllString(strings.ToLower(name), "-")
	s = slugRepeat.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "untitled"
	}
	return s
}

// CanonicalRootCoord returns the domain's per-user root coordinate for the
// given owner. The npub decodes to hex here — the protocol boundary minting
// the wire value.
func (d Domain) CanonicalRootCoord(ownerNpub string) (string, error) {
	ownerHex, err := btknostr.NpubToHex(ownerNpub)
	if err != nil {
		return "", fmt.Errorf("owner npub: %w", err)
	}
	return FormatCoordinate(KindListSet, ownerHex, d.RootDTag), nil
}

// FormatCoordinate renders an addressable-event coordinate kind:pubkey:d-tag.
func FormatCoordinate(kind int, pubkeyHex, dTag string) string {
	return fmt.Sprintf("%d:%s:%s", kind, pubkeyHex, dTag)
}

// DomainDRSS is the declared drss domain — the RSS-feed region of a user's
// ontology written by cmd/drss.
var DomainDRSS = Domain{
	Name:            "drss",
	RootDTag:        "drss",
	DTagPrefix:      "drss-",
	ImportPrefix:    "drss-import-",
	CompanionSuffix: "-feeds",
	RootTitle:       "DRSS lists",
	CompanionTitle:  "Feeds",
	LeafKind:        KindFollowSet,
}

var domains = map[string]Domain{
	DomainDRSS.Name: DomainDRSS,
}

// The vault domain family. A vault's root is per-instance, so the family is a
// constructor rather than an entry in the static registry: the declaration
// fixes the family prefix, the separator, and the leaf kind, and an instance
// name supplied at construction completes the root and the member prefix.
const (
	// VaultFamilyPrefix identifies every d-tag belonging to the family; the
	// instance name follows it.
	VaultFamilyPrefix = "vault-"
	// VaultPathSeparator separates the segments of a vault member's path.
	VaultPathSeparator = "/"
	// VaultRootSegment is the final segment of the family's companion suffix.
	// The suffix leads with the separator, so the companion formula derives
	// vault-<name>/_root: a member-classified set holding the vault's
	// root-level files. The segment is reserved by exact enumeration; only the
	// companion's own path may carry it.
	VaultRootSegment = "_root"
	// VaultRootSetTitle is the title every writer mints for the root set.
	VaultRootSetTitle = "Root"
)

// VaultDomain constructs the declared domain for the named vault.
func VaultDomain(name string) (Domain, error) {
	if err := ValidateVaultName(name); err != nil {
		return Domain{}, err
	}
	root := VaultFamilyPrefix + name
	return Domain{
		Name:            root,
		RootDTag:        root,
		DTagPrefix:      root + VaultPathSeparator,
		CompanionSuffix: VaultPathSeparator + VaultRootSegment,
		CompanionTitle:  VaultRootSetTitle,
		LeafKind:        KindCurationSet,
		PathSeparator:   VaultPathSeparator,
	}, nil
}

// ValidateVaultName checks a name against the family's declared instance-name
// grammar: one or more lowercase alphanumerics and hyphens.
func ValidateVaultName(name string) error {
	if name == "" {
		return errors.New("vault name is empty")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return fmt.Errorf("vault name %q contains %q; names are lowercase alphanumerics and hyphens", name, r)
		}
	}
	return nil
}

// ClassifyVaultDTag places a d-tag within the vault family and names the
// instance it belongs to, which is what lets a reader classify a set without
// knowing in advance which vaults an owner keeps. A d-tag outside the family,
// one whose instance name violates the declared grammar, and one whose member
// path is unnameable are all foreign.
func ClassifyVaultDTag(dTag string) (string, DTagClass) {
	if !strings.HasPrefix(dTag, VaultFamilyPrefix) {
		return "", DTagForeign
	}
	name, path, hasPath := strings.Cut(strings.TrimPrefix(dTag, VaultFamilyPrefix), VaultPathSeparator)
	if ValidateVaultName(name) != nil {
		return "", DTagForeign
	}
	if !hasPath {
		return name, DTagRoot
	}
	if validateMemberPath(path, VaultPathSeparator) != nil {
		return "", DTagForeign
	}
	return name, DTagMember
}

// DomainByName returns the declared domain registered under name.
func DomainByName(name string) (Domain, bool) {
	d, ok := domains[name]
	return d, ok
}
