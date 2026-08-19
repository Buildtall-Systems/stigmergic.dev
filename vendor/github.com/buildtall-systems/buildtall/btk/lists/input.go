package lists

import (
	"fmt"
	"strings"

	gonostr "github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// itemTypeRelay is the Item.Type discriminator for relay-URL inputs.
const itemTypeRelay = "relay"

func ParseInput(input string) (*Item, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("input is empty")
	}

	if strings.HasPrefix(input, "npub1") ||
		strings.HasPrefix(input, "note1") ||
		strings.HasPrefix(input, "nprofile1") ||
		strings.HasPrefix(input, "nevent1") ||
		strings.HasPrefix(input, "naddr1") {
		return parseNIP19Input(input)
	}

	if rest, ok := strings.CutPrefix(input, "nostr:"); ok {
		return parseNIP19Input(rest)
	}

	if gonostr.IsValidPublicKey(input) {
		npub, err := btknostr.HexToNpub(input)
		if err != nil {
			return nil, fmt.Errorf("encoding npub: %w", err)
		}
		return &Item{Type: "p", Value: npub}, nil
	}

	if gonostr.IsValid32ByteHex(input) {
		return &Item{Type: "e", Value: input}, nil
	}

	if isCoordinate(input) {
		return &Item{Type: "a", Value: input}, nil
	}

	if strings.HasPrefix(input, "wss://") || strings.HasPrefix(input, "ws://") {
		return &Item{Type: itemTypeRelay, Value: input}, nil
	}

	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return &Item{Type: "r", Value: input}, nil
	}

	if item, ok := parseEmoji(input); ok {
		return item, nil
	}

	if isHashtag(input) {
		tag := strings.TrimPrefix(input, "#")
		return &Item{Type: "t", Value: strings.ToLower(tag)}, nil
	}

	return nil, fmt.Errorf("unrecognized input format: %s", input)
}

func parseNIP19Input(input string) (*Item, error) {
	entity, err := DecodeNIP19(input)
	if err != nil {
		return nil, err
	}

	var relayHint string
	if len(entity.RelayHints) > 0 {
		relayHint = entity.RelayHints[0]
	}

	switch entity.Type {
	case nip19Npub, nip19Nprofile:
		return &Item{
			Type:      "p",
			Value:     entity.Npub,
			RelayHint: relayHint,
		}, nil

	case nip19Note, nip19Nevent:
		return &Item{
			Type:       "e",
			Value:      entity.EventID,
			RelayHint:  relayHint,
			SourceKind: entity.Kind,
		}, nil

	case nip19Naddr:
		pubkey, err := btknostr.NpubToHex(entity.Npub)
		if err != nil {
			return nil, fmt.Errorf("invalid npub in %s: %w", nip19Naddr, err)
		}
		coord := fmt.Sprintf("%d:%s:%s", entity.Kind, pubkey, entity.DTag)
		return &Item{
			Type:       "a",
			Value:      coord,
			RelayHint:  relayHint,
			SourceKind: entity.Kind,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported entity type: %s", entity.Type)
	}
}

func isCoordinate(s string) bool {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return false
	}

	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			return false
		}
	}

	return gonostr.IsValidPublicKey(parts[1])
}

func isHashtag(s string) bool {
	if !strings.HasPrefix(s, "#") {
		return false
	}
	tag := strings.TrimPrefix(s, "#")
	if tag == "" {
		return false
	}
	for _, c := range tag {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func parseEmoji(input string) (*Item, bool) {
	if !strings.HasPrefix(input, ":") {
		return nil, false
	}

	parts := strings.SplitN(input, " ", 2)
	if len(parts) != 2 {
		return nil, false
	}

	shortcode := strings.Trim(parts[0], ":")
	if shortcode == "" {
		return nil, false
	}

	url := strings.TrimSpace(parts[1])
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, false
	}

	return &Item{
		Type:       tagEmoji,
		Value:      shortcode,
		Identifier: url,
	}, true
}

func ParseInputForKind(input string, kind int) (*Item, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("input is empty")
	}

	if kind == KindPeopleList {
		return nil, fmt.Errorf("kind 30001 (People List) is deprecated and does not accept new items")
	}

	if kind == KindFollowSet {
		if gonostr.IsValidPublicKey(input) {
			return nil, fmt.Errorf("follow sets require npub format, not raw hex pubkey")
		}
		if gonostr.IsValid32ByteHex(input) {
			return nil, fmt.Errorf("follow sets require npub or note format, not raw hex")
		}
	}

	item, err := ParseInput(input)
	if err != nil {
		return nil, err
	}

	if err := ValidateItemForKind(item, kind); err != nil {
		return nil, err
	}

	return item, nil
}
