package lists

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

const (
	tagTitle       = "title"
	tagDescription = "description"
	tagImage       = "image"
)

// ErrItemAlreadyPresent reports an add whose (type, value) pair is already in
// the list, so callers never blind-append a duplicate tag.
var ErrItemAlreadyPresent = errors.New("item already in list")

// ErrItemNotPresent reports a remove whose (type, value) pair is not in the
// list. Publishing would be a pure created_at bump, so callers short-circuit
// to an honest "already removed" instead.
var ErrItemNotPresent = errors.New("item not in list")

// NextCreatedAt guarantees a mutation strictly outranks its base: a tie would
// fall into NIP-01's lowest-id lottery, and a future-stamped base would win
// against a plain wall-clock stamp. The reference time is a parameter so that
// a caller assembling events without performing I/O stays deterministic, and
// so that the rule lives in one place rather than being restated by every
// package that has to replace a live event.
func NextCreatedAt(base *nostr.Event, now time.Time) nostr.Timestamp {
	stamp := nostr.Timestamp(now.Unix())
	if next := base.CreatedAt + 1; next > stamp {
		return next
	}
	return stamp
}

// nextCreatedAt applies NextCreatedAt at the current wall-clock time, which is
// what the in-package mutations want.
func nextCreatedAt(base *nostr.Event) nostr.Timestamp {
	return NextCreatedAt(base, time.Now())
}

// tagValue converts an item's npub-canonical value to its wire form. Items
// are npub-canonical in the domain layer; "p" tags carry hex. This is the
// protocol boundary where the conversion happens.
func tagValue(item Item) (string, error) {
	if item.Type == "p" {
		hex, err := btknostr.NpubToHex(item.Value)
		if err != nil {
			return "", fmt.Errorf("converting p item value: %w", err)
		}
		return hex, nil
	}
	return item.Value, nil
}

// itemTag renders one item as its wire tag. A save time occupies the fourth
// position, so position three is padded with an empty relay hint when the
// item carries none. Only an addressable item carries a save time.
func itemTag(item Item) (nostr.Tag, error) {
	value, err := tagValue(item)
	if err != nil {
		return nil, err
	}
	tag := nostr.Tag{item.Type, value}
	if item.RelayHint != "" {
		tag = append(tag, item.RelayHint)
	}
	if item.Identifier != "" {
		if item.RelayHint == "" {
			tag = append(tag, "")
		}
		tag = append(tag, item.Identifier)
	}
	if item.SavedAt > 0 && item.IsAddressable() {
		for len(tag) < 3 {
			tag = append(tag, "")
		}
		tag = append(tag, strconv.FormatInt(item.SavedAt, 10))
	}
	return tag, nil
}

// NewListEvent builds an unsigned list event. Title, description and image
// are the NIP-51 set metadata: each writes a tag only when it carries a
// value, so an omitted field never writes a blanking tag.
func NewListEvent(kind int, npub string, dTag string, title string, description string, image string, items []Item) (*nostr.Event, error) {
	pubkey, err := btknostr.NpubToHex(npub)
	if err != nil {
		return nil, err
	}

	tags := nostr.Tags{
		{"d", dTag},
	}

	if title != "" && title != dTag {
		tags = append(tags, nostr.Tag{tagTitle, title})
	}

	if description != "" {
		tags = append(tags, nostr.Tag{tagDescription, description})
	}

	if image != "" {
		tags = append(tags, nostr.Tag{tagImage, image})
	}

	for _, item := range items {
		tag, err := itemTag(item)
		if err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}

	return &nostr.Event{
		Kind:      kind,
		PubKey:    pubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Tags:      tags,
		Content:   "",
	}, nil
}

func UpdateListTitle(event *nostr.Event, newTitle string) *nostr.Event {
	newTags := make(nostr.Tags, 0, len(event.Tags))

	hasTitleTag := false
	for _, tag := range event.Tags {
		if len(tag) >= 1 && tag[0] == tagTitle {
			if newTitle != "" {
				newTags = append(newTags, nostr.Tag{tagTitle, newTitle})
			}
			hasTitleTag = true
		} else {
			newTags = append(newTags, tag)
		}
	}

	if !hasTitleTag && newTitle != "" {
		dTagIdx := -1
		for i, tag := range newTags {
			if len(tag) >= 1 && tag[0] == "d" {
				dTagIdx = i
				break
			}
		}
		if dTagIdx >= 0 {
			insertIdx := dTagIdx + 1
			newTags = append(newTags[:insertIdx], append(nostr.Tags{{tagTitle, newTitle}}, newTags[insertIdx:]...)...)
		} else {
			newTags = append(newTags, nostr.Tag{tagTitle, newTitle})
		}
	}

	return &nostr.Event{
		Kind:      event.Kind,
		PubKey:    event.PubKey,
		CreatedAt: nextCreatedAt(event),
		Tags:      newTags,
		Content:   event.Content,
	}
}

// AddItemToList appends one item to the list, preserving the complete
// existing tag set. It is the single-item form of AddItemsToList.
func AddItemToList(event *nostr.Event, item Item) (*nostr.Event, error) {
	return AddItemsToList(event, []Item{item})
}

// AddItemsToList appends a batch of items to the list in one event: the
// complete existing tag set is preserved, items already present (and
// duplicates within the batch) are skipped, and the created_at strictly
// outranks the base. If every item is already present it returns
// ErrItemAlreadyPresent so callers never publish a pure created_at bump.
func AddItemsToList(event *nostr.Event, items []Item) (*nostr.Event, error) {
	present := make(map[[2]string]bool, len(event.Tags))
	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			present[[2]string{tag[0], tag[1]}] = true
		}
	}

	newTags := make(nostr.Tags, len(event.Tags), len(event.Tags)+len(items))
	copy(newTags, event.Tags)

	appended := 0
	for _, item := range items {
		value, err := tagValue(item)
		if err != nil {
			return nil, err
		}
		key := [2]string{item.Type, value}
		if present[key] {
			continue
		}
		present[key] = true

		tag, err := itemTag(item)
		if err != nil {
			return nil, err
		}
		newTags = append(newTags, tag)
		appended++
	}

	if appended == 0 {
		return nil, ErrItemAlreadyPresent
	}

	return &nostr.Event{
		Kind:      event.Kind,
		PubKey:    event.PubKey,
		CreatedAt: nextCreatedAt(event),
		Tags:      newTags,
		Content:   event.Content,
	}, nil
}

func RemoveItemFromList(event *nostr.Event, itemType string, itemValue string) (*nostr.Event, error) {
	wireValue, err := tagValue(Item{Type: itemType, Value: itemValue})
	if err != nil {
		return nil, err
	}

	newTags := make(nostr.Tags, 0, len(event.Tags))

	found := false
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == itemType && tag[1] == wireValue {
			found = true
			continue
		}
		newTags = append(newTags, tag)
	}

	if !found {
		return nil, ErrItemNotPresent
	}

	return &nostr.Event{
		Kind:      event.Kind,
		PubKey:    event.PubKey,
		CreatedAt: nextCreatedAt(event),
		Tags:      newTags,
		Content:   event.Content,
	}, nil
}

// RepublishAfterDeletion rebuilds a deleted list for the republish choice on
// the conflict surface: same state as the event the user signed, but with a
// created_at that outranks the tombstone. Per NIP-09 the deletion covers
// versions up to its own created_at, so the signed event's stamp generally
// cannot resurrect the list.
func RepublishAfterDeletion(list *nostr.Event, tombstone *nostr.Event) *nostr.Event {
	return &nostr.Event{
		Kind:      list.Kind,
		PubKey:    list.PubKey,
		CreatedAt: nextCreatedAt(tombstone),
		Tags:      list.Tags,
		Content:   list.Content,
	}
}

// NewDeletionEvent builds a kind-5 deletion. When target (the latest known
// version of the list being deleted) is non-nil, the deletion outranks it:
// per NIP-09 a deletion only covers versions up to its own created_at.
func NewDeletionEvent(npub string, coords []string, eventIDs []string, target *nostr.Event) (*nostr.Event, error) {
	pubkey, err := btknostr.NpubToHex(npub)
	if err != nil {
		return nil, err
	}

	createdAt := nostr.Timestamp(time.Now().Unix())
	if target != nil {
		createdAt = nextCreatedAt(target)
	}

	kindsSeen := make(map[string]bool)
	tags := make(nostr.Tags, 0, len(coords)*2+len(eventIDs))

	for _, eventID := range eventIDs {
		if eventID != "" {
			tags = append(tags, nostr.Tag{"e", eventID})
		}
	}

	for _, coord := range coords {
		tags = append(tags, nostr.Tag{"a", coord})

		kind, _, _, err := btknostr.ParseCoordinate(coord)
		if err == nil {
			kindStr := fmt.Sprintf("%d", kind)
			if !kindsSeen[kindStr] {
				tags = append(tags, nostr.Tag{"k", kindStr})
				kindsSeen[kindStr] = true
			}
		}
	}

	return &nostr.Event{
		Kind:      nostr.KindDeletion,
		PubKey:    pubkey,
		CreatedAt: createdAt,
		Tags:      tags,
		Content:   "",
	}, nil
}
