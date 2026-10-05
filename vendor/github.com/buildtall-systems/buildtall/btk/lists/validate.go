package lists

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

const (
	KindFollowSet     = 30000
	KindPeopleList    = 30001
	KindRelaySet      = 30002
	KindBookmarkSet   = 30003
	KindCurationSet   = 30004
	KindInterestSet   = 30015
	KindEmojiSet      = 30030
	KindListSet       = 30101
	KindFileSet       = 30102
	KindSiteSet       = btknostr.KindSiteSet
	KindProvenanceSet = btknostr.KindProvenanceSet
	KindFileMetadata  = 1063
	KindLongFormNote  = btknostr.KindLongForm
	KindTextNote      = 1
)

var ListKinds = []int{
	KindFollowSet,
	KindPeopleList,
	KindRelaySet,
	KindBookmarkSet,
	KindCurationSet,
	KindInterestSet,
	KindEmojiSet,
	KindListSet,
	KindFileSet,
	KindSiteSet,
	KindProvenanceSet,
}

func ValidateEvent(event *nostr.Event) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	if event.PubKey == "" {
		return fmt.Errorf("event pubkey is empty")
	}

	if len(event.PubKey) != 64 {
		return fmt.Errorf("invalid pubkey length: %d", len(event.PubKey))
	}

	if event.Kind < 0 {
		return fmt.Errorf("invalid event kind: %d", event.Kind)
	}

	if event.ID == "" {
		return fmt.Errorf("event ID is empty")
	}

	if event.Sig == "" {
		return fmt.Errorf("event signature is empty")
	}

	ok, err := event.CheckSignature()
	if err != nil {
		return fmt.Errorf("signature check error: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

func ValidateListEvent(event *nostr.Event) error {
	if err := ValidateEvent(event); err != nil {
		return err
	}

	if event.Kind < 30000 || event.Kind >= 40000 {
		return fmt.Errorf("invalid list kind: %d (must be 30000-39999)", event.Kind)
	}

	hasDTag := false
	for _, tag := range event.Tags {
		if len(tag) >= 1 && tag[0] == "d" {
			hasDTag = true
			break
		}
	}

	if !hasDTag {
		return fmt.Errorf("list event missing d tag")
	}

	return nil
}

func ValidateDeletionEvent(event *nostr.Event) error {
	if err := ValidateEvent(event); err != nil {
		return err
	}

	if event.Kind != 5 {
		return fmt.Errorf("invalid deletion event kind: %d (must be 5)", event.Kind)
	}

	hasTarget := false
	for _, tag := range event.Tags {
		if len(tag) >= 2 && (tag[0] == "e" || tag[0] == "a") {
			hasTarget = true
			break
		}
	}

	if !hasTarget {
		return fmt.Errorf("deletion event missing target (e or a tag)")
	}

	return nil
}

func ValidateEventPubkey(event *nostr.Event, expectedPubkey string) error {
	if event.PubKey != expectedPubkey {
		return fmt.Errorf("event pubkey mismatch: got %s, expected %s", event.PubKey, expectedPubkey)
	}
	return nil
}

func AllowedTagsForKind(kind int) []string {
	return kindAllowedTags[kind]
}

func ValidateItemForKind(item *Item, kind int) error {
	if item == nil {
		return fmt.Errorf("item is nil")
	}

	if kind == KindPeopleList {
		return fmt.Errorf("kind 30001 (People List) is deprecated and does not accept new items")
	}

	// The kit refuses to write what it would refuse to read: parseSavedAt
	// discards a non-positive save time, and it reads the fourth position of
	// "a" and "e" tags alone. A save time anywhere else would be written and
	// never recovered.
	if item.SavedAt < 0 {
		return fmt.Errorf("save time %d is negative", item.SavedAt)
	}
	if item.SavedAt > 0 && !item.hasSaveTimePosition() {
		return fmt.Errorf("only an addressable or event item carries a save time, got type %q", item.Type)
	}

	allowed := AllowedTagsForKind(kind)
	if allowed == nil {
		return fmt.Errorf("unknown or unsupported list kind: %d", kind)
	}

	if !slices.Contains(allowed, item.Type) {
		return fmt.Errorf("tag type %q not allowed for kind %d (allowed: %v)", item.Type, kind, allowed)
	}

	switch kind {
	case KindBookmarkSet, KindCurationSet:
		if item.Type == "a" && item.SourceKind != 0 && item.SourceKind != KindLongFormNote {
			return fmt.Errorf("bookmark/curation sets only accept long-form articles (kind 30023), got kind %d", item.SourceKind)
		}
		if item.Type == "e" && item.SourceKind != 0 && item.SourceKind != KindTextNote {
			return fmt.Errorf("bookmark/curation sets only accept text notes (kind 1), got kind %d", item.SourceKind)
		}

	case KindListSet:
		if item.SourceKind != 0 && !AllowedCompositionKind(item.SourceKind) {
			return fmt.Errorf("list sets only accept other set kinds, got kind %d", item.SourceKind)
		}

	case KindFileSet:
		if item.SourceKind != 0 && item.SourceKind != KindFileMetadata {
			return fmt.Errorf("file sets only accept file metadata events (kind %d), got kind %d", KindFileMetadata, item.SourceKind)
		}
		if item.Identifier != "" {
			return fmt.Errorf("a file item carries no identifier: positions three and four are the relay hint and the save time")
		}

	case KindSiteSet:
		if item.SourceKind != 0 && item.SourceKind != btknostr.KindNsiteNamed {
			return fmt.Errorf("site sets only accept named sites (kind %d), got kind %d", btknostr.KindNsiteNamed, item.SourceKind)
		}
		kind, dTag, ok := splitCoordinate(item.Value)
		if !ok || kind != btknostr.KindNsiteNamed {
			return fmt.Errorf("site sets only accept named sites (kind %d), got coordinate %q", btknostr.KindNsiteNamed, item.Value)
		}
		if dTag == "" {
			return fmt.Errorf("a site set item is labeled by its d tag, and a root site has none: %q", item.Value)
		}

	case KindProvenanceSet:
		if item.SourceKind != 0 && item.SourceKind != btknostr.KindProvenance {
			return fmt.Errorf("provenance sets only accept provenance records (kind %d), got kind %d", btknostr.KindProvenance, item.SourceKind)
		}
		kind, dTag, ok := splitCoordinate(item.Value)
		if !ok || kind != btknostr.KindProvenance || dTag == "" {
			return fmt.Errorf("provenance sets only accept provenance records (kind %d), got coordinate %q", btknostr.KindProvenance, item.Value)
		}
	}

	return nil
}

// splitCoordinate reads the kind and d tag out of a kind:pubkey:d coordinate.
func splitCoordinate(value string) (kind int, dTag string, ok bool) {
	parts := strings.SplitN(value, ":", 3)
	if len(parts) != 3 {
		return 0, "", false
	}
	kind, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", false
	}
	return kind, parts[2], true
}

// ValidateItemsForKind validates a complete item set as one statement: each
// item under ValidateItemForKind, plus the rules only a whole set can state.
// A file set refuses the same id written twice, which no single item can see.
func ValidateItemsForKind(items []Item, kind int) error {
	seen := make(map[string]bool, len(items))
	for i := range items {
		if err := ValidateItemForKind(&items[i], kind); err != nil {
			return err
		}
		if kind != KindFileSet {
			continue
		}
		if seen[items[i].Value] {
			return fmt.Errorf("file set writes id %s twice", items[i].Value)
		}
		seen[items[i].Value] = true
	}
	return nil
}
