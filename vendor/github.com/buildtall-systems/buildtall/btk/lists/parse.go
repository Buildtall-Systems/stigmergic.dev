package lists

import (
	"fmt"
	"strconv"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

func ParseList(event *nostr.Event) (*List, error) {
	npub, err := btknostr.HexToNpub(event.PubKey)
	if err != nil {
		return nil, fmt.Errorf("encoding npub: %w", err)
	}
	list := &List{
		Kind:        event.Kind,
		AuthorNpub:  npub,
		DTag:        GetDTag(event),
		Title:       GetTitle(event),
		Description: GetDescription(event),
		Image:       GetImage(event),
		CreatedAt:   int64(event.CreatedAt),
		Items:       GetItems(event),
	}
	list.Coord = CoordinateFromEvent(event)
	return list, nil
}

// firstTagValue reads the value of the first tag bearing the given name. An
// absent tag and a valueless one both read as empty.
func firstTagValue(event *nostr.Event, name string) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == name {
			return tag[1]
		}
	}
	return ""
}

func GetDTag(event *nostr.Event) string {
	return firstTagValue(event, "d")
}

// GetTitle falls back to the d tag only when no title tag is present. A
// present but empty title tag reads as the empty title its writer wrote.
func GetTitle(event *nostr.Event) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == tagTitle {
			return tag[1]
		}
	}
	return GetDTag(event)
}

func GetDescription(event *nostr.Event) string {
	return firstTagValue(event, tagDescription)
}

func GetImage(event *nostr.Event) string {
	return firstTagValue(event, tagImage)
}

// parseSavedAt reads the drss save time from the fourth position of an "a"
// tag. A value that is not a positive integer of seconds is a fact about a
// foreign writer, not a parse failure, so it reads as absent.
func parseSavedAt(tag nostr.Tag) int64 {
	if len(tag) < 4 {
		return 0
	}
	seconds, err := strconv.ParseInt(tag[3], 10, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return seconds
}

func GetItems(event *nostr.Event) []Item {
	var items []Item

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "p":
			npub, err := btknostr.HexToNpub(tag[1])
			if err != nil {
				continue
			}
			item := Item{Type: "p", Value: npub}
			if len(tag) >= 3 {
				item.RelayHint = tag[2]
			}
			items = append(items, item)

		case "e":
			item := Item{Type: "e", Value: tag[1]}
			if len(tag) >= 3 {
				item.RelayHint = tag[2]
			}
			items = append(items, item)

		case "a":
			item := Item{Type: "a", Value: tag[1], SavedAt: parseSavedAt(tag)}
			if len(tag) >= 3 {
				item.RelayHint = tag[2]
			}
			items = append(items, item)

		case "t":
			items = append(items, Item{Type: "t", Value: tag[1]})

		case "r":
			item := Item{Type: "r", Value: tag[1]}
			if len(tag) >= 3 {
				item.Identifier = tag[2]
			}
			items = append(items, item)

		case tagEmoji:
			if len(tag) >= 3 {
				items = append(items, Item{Type: tagEmoji, Value: tag[1], Identifier: tag[2]})
			}
		}
	}

	return items
}

func CoordinateFromEvent(event *nostr.Event) string {
	return FormatCoordinate(event.Kind, event.PubKey, GetDTag(event))
}

func KindName(kind int) string {
	switch kind {
	case KindFollowSet:
		return "Follow Set"
	case KindPeopleList:
		return "People List"
	case KindRelaySet:
		return "Relay Set"
	case KindBookmarkSet:
		return "Bookmark Set"
	case KindCurationSet:
		return "Curation Set"
	case KindInterestSet:
		return "Interest Set"
	case KindEmojiSet:
		return "Emoji Set"
	case KindListSet:
		return "List Set"
	default:
		return fmt.Sprintf("Kind %d", kind)
	}
}

func IsDeprecatedKind(kind int) bool {
	return kind == KindPeopleList
}

func IsEncrypted(event *nostr.Event) bool {
	return event.Content != ""
}
