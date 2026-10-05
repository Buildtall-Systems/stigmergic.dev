// Package lists implements the NIP-51 leaf-list and kind 30101 List of Lists
// machinery: domain model, protocol policy, event builders, parsing, and
// DAG traversal. It is the single shared implementation consumed by
// cmd/listoflists and cmd/drss, and the reference implementation of the
// ratified personal-ontology specification (operations repo,
// nuds/list-of-lists.md); ontology.go carries the spec's declarative model.
package lists

type List struct {
	Coord string
	DTag  string
	Title string
	// Description and Image are the remaining two NIP-51 set metadata fields.
	// Both are optional: an absent tag leaves the field empty, and an empty
	// field writes no tag.
	Description string
	Image       string
	// Language is the feed language the list declares in its language tag,
	// raw as written: an absent tag leaves it empty, and a reader resolves
	// it through feedlang.
	Language   string
	AuthorNpub string
	Items      []Item
	CreatedAt  int64
	Kind       int
	Foreign    bool
}

const tagEmoji = "emoji"

// Item is one member of a list. Values are npub-canonical: a "p" item carries
// an npub, converted to hex only inside the event builders — the protocol
// boundary. Coordinates ("a") and event ids ("e") are protocol strings and
// stay as-is.
type Item struct {
	Type       string `json:"type"`
	Value      string `json:"value"`
	RelayHint  string `json:"relayHint,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	SourceKind int    `json:"sourceKind,omitempty"`
	// SavedAt is the unix second at which the owner saved this item into the
	// list, carried in the fourth position of an "a" tag per the drss domain
	// convention in list-of-lists.md. Zero means absent, which every foreign writer
	// and every list written before the convention will be.
	SavedAt int64 `json:"savedAt,omitempty"`
}

func (i Item) IsAddressable() bool {
	return i.Type == "a"
}

// hasSaveTimePosition reports whether the item's tag type has a fourth
// position for a save time: "a" per the drss convention and "e" per the file
// set, both defined in list-of-lists.md. Writer and reader share this predicate so
// neither side accepts what the other refuses.
func (i Item) hasSaveTimePosition() bool {
	return i.IsAddressable() || i.IsEvent()
}

func (i Item) IsEvent() bool {
	return i.Type == "e"
}

func (i Item) IsHashtag() bool {
	return i.Type == "t"
}

func (i Item) IsURL() bool {
	return i.Type == "r"
}

// IsRelay reports whether the item is a relay URL, the one item type a kind
// 30002 relay set carries.
func (i Item) IsRelay() bool {
	return i.Type == itemTypeRelay
}

func (i Item) IsEmoji() bool {
	return i.Type == tagEmoji
}

func (i Item) IsPubkey() bool {
	return i.Type == "p"
}
