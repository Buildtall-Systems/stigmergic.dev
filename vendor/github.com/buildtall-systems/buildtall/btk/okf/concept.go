package okf

import (
	"fmt"

	"gopkg.in/yaml.v3"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// NostrTagsKey is the frontmatter key holding a concept's complete Nostr tag
// list. It is not "tags": OKF v0.1 already defines that key as cross-cutting
// categories, which project onto the "t" tag and nothing else. A producer-
// defined key is the honest place for a projection OKF does not specify.
const NostrTagsKey = "nostr_tags"

// The frontmatter keys OKF v0.1 defines. They are named rather than spelled at
// each use because the mapping onto Nostr tag names is stated in three places:
// the canonical emission order, the refusal that keeps a raw tag from restating
// a field, and the serializer.
const (
	FieldType        = "type"
	FieldTitle       = "title"
	FieldDescription = "description"
	FieldResource    = "resource"
	FieldTags        = "tags"
	FieldTimestamp   = "timestamp"
	FieldDTag        = btknostr.TagD
)

// The OKF-owned wire tag names. Each projects one concept frontmatter field
// no sugared tag states, which is what makes every statement in a file
// reproducible from the wire: the producer type when it is not the kind's
// canonical one, the resource URI, and the producer-defined keys as one
// canonically serialized mapping. Each is emitted only when its field is
// stated, read back into its field on export, and refused in a raw tag list
// exactly as the sugared names are.
const (
	TagOKFType     = "okf-type"
	TagOKFResource = "okf-resource"
	TagOKFExtra    = "okf-extra"
)

// TagOKFAttachment is the OKF-owned wire tag stating one attachment a
// directory's set carries: the file's name inside the directory, then the
// SHA-256 of its bytes. It is not a frontmatter projection: its source is
// the attachment files themselves, hashed when the bundle is read for a
// publish, and it is emitted only where a directory holds one, so a set
// without attachments republishes exactly as it always did. Like the three
// concept-owned names, it is refused in every raw tag list: a statement
// whose source is the tree may not also arrive by hand.
const TagOKFAttachment = "okf-attachment"

// TagOKFServer is the OKF-owned wire tag naming one Blossom store the vault's
// attachment bytes serve from: its value is a Blossom base URL. It is carried
// on the vault root, one or more times in preference order. Unlike the tags
// above it projects no frontmatter field and derives from no tree: it is
// stated in the root sidecar's nostr tags and rides the round trip like any
// other sidecar tag, so no raw tag list refuses it.
const TagOKFServer = "okf-server"

// okfTagSources names, for each OKF-owned tag, where a concept states that
// fact instead. okf-extra names no single field: every frontmatter key
// outside the known set is its statement.
var okfTagSources = map[string]string{
	TagOKFType:     "the " + FieldType + " field",
	TagOKFResource: "the " + FieldResource + " field",
	TagOKFExtra:    "the producer-defined frontmatter keys",
}

// knownFrontmatterKeys are the keys OKF gives defined meaning. Everything else
// a producer writes is retained verbatim in Frontmatter.Extra. The order here
// is the canonical emission order used by the serializer.
var knownFrontmatterKeys = []string{
	FieldType,
	FieldTitle,
	FieldDescription,
	FieldResource,
	FieldTags,
	FieldTimestamp,
	FieldDTag,
	NostrTagsKey,
}

// sugaredTagNames are the tag names an OKF-defined field already states. A
// concept says each of these once, in the field OKF specifies for it, so that
// no precedence rule is ever needed between two statements of one fact.
var sugaredTagNames = map[string]string{
	btknostr.TagTitle:       FieldTitle,
	btknostr.TagSummary:     FieldDescription,
	btknostr.TagTopic:       FieldTags,
	btknostr.TagPublishedAt: FieldTimestamp,
}

// Frontmatter is the YAML metadata block of an OKF concept document. Only Type
// is required (OKF v0.1 conformance). The recommended fields are captured
// explicitly; any additional producer-defined keys are preserved in Extra so
// that parsing and re-serializing a concept never discards information.
type Frontmatter struct {
	// Type identifies the kind of concept (e.g. "Document"). Required.
	Type string
	// Title is the human-readable display name.
	Title string
	// Description is a one-line summary.
	Description string
	// Resource is the canonical URI of the asset the concept describes, if any.
	Resource string
	// Timestamp is the ISO 8601 time of last meaningful change.
	Timestamp string
	// DTag is an explicit addressable-event identifier ("d" in frontmatter).
	// When empty, consumers derive the d-tag from the concept's bundle path.
	DTag string
	// Extra holds producer-defined keys not covered above, preserved verbatim.
	Extra map[string]any
	// Tags are short cross-cutting categories.
	Tags []string
	// NostrTags is every tag the concept's event carries that no OKF-defined
	// field states, held verbatim: name first, then every remaining element, in
	// the order the wire gave them. A tag is a list rather than a pair, and one
	// name may appear many times, so nothing weaker than a list of lists can
	// carry it. Kept last for field alignment: its trailing len/cap words fall
	// outside the GC pointer-scan region.
	NostrTags [][]string
}

// Concept is one OKF knowledge document: a frontmatter block plus a markdown
// body, identified by ConceptID: its bundle-relative path with the ".md"
// suffix removed (e.g. "tables/orders").
type Concept struct {
	ConceptID string
	Body      string
	// Frontmatter is kept last for field alignment: it ends in a slice whose
	// trailing len/cap words fall outside the GC pointer-scan region.
	Frontmatter Frontmatter
}

// UnmarshalYAML routes the known keys into their typed fields and collects
// every remaining key into Extra, so unknown frontmatter survives a round trip.
func (f *Frontmatter) UnmarshalYAML(value *yaml.Node) error {
	var known struct {
		Type        string     `yaml:"type"`
		Title       string     `yaml:"title"`
		Description string     `yaml:"description"`
		Resource    string     `yaml:"resource"`
		Timestamp   string     `yaml:"timestamp"`
		DTag        string     `yaml:"d"`
		Tags        []string   `yaml:"tags"`
		NostrTags   [][]string `yaml:"nostr_tags"`
	}
	if err := value.Decode(&known); err != nil {
		return err
	}

	all := map[string]any{}
	if err := value.Decode(&all); err != nil {
		return err
	}
	for _, k := range knownFrontmatterKeys {
		delete(all, k)
	}

	f.Type = known.Type
	f.Title = known.Title
	f.Description = known.Description
	f.Resource = known.Resource
	f.Tags = known.Tags
	f.Timestamp = known.Timestamp
	f.DTag = known.DTag
	f.NostrTags = known.NostrTags
	if len(all) > 0 {
		f.Extra = all
	}
	return nil
}

// CheckNostrTags refuses a raw tag list that states what something else already
// states. A name an OKF field sugars would need a precedence rule against that
// field, and a name the tree derives would let a file's frontmatter contradict
// its own location. Both are refused rather than resolved, because a silent
// precedence rule is how a lossy round trip returns.
func (f *Frontmatter) CheckNostrTags() error {
	for _, tag := range f.NostrTags {
		if err := checkRawTag(tag); err != nil {
			return err
		}
		if field, ok := sugaredTagNames[tag[0]]; ok {
			return fmt.Errorf("%s states %q, which the %q field already states: give it once, in %q",
				NostrTagsKey, tag[0], field, field)
		}
		if source, ok := okfTagSources[tag[0]]; ok {
			return fmt.Errorf("%s states %q, which the projection derives from %s: give it once, there",
				NostrTagsKey, tag[0], source)
		}
	}
	return nil
}

// checkRawTag applies the refusals every raw tag list shares, whichever event it
// describes. A tag with no name cannot go on the wire at all, and a tag stating
// the d that the bundle tree already states could only ever contradict its own
// location: under projection D a path is the identity of whatever sits at it.
func checkRawTag(tag []string) error {
	if len(tag) == 0 || tag[0] == "" {
		return fmt.Errorf("%s holds a tag with no name", NostrTagsKey)
	}
	if tag[0] == btknostr.TagD {
		return fmt.Errorf("%s states %q, which the bundle path already states: move it rather than restating its identity",
			NostrTagsKey, btknostr.TagD)
	}
	if tag[0] == TagOKFAttachment {
		return fmt.Errorf("%s states %q, which the projection derives from the attachment files a directory holds: place the file rather than restating it",
			NostrTagsKey, TagOKFAttachment)
	}
	return nil
}
