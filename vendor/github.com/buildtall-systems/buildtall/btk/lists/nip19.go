package lists

import (
	"fmt"

	gonostr "github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

const (
	nip19Naddr    = "naddr"
	nip19Note     = "note"
	nip19Nevent   = "nevent"
	nip19Npub     = "npub"
	nip19Nprofile = "nprofile"
)

type DecodedEntity struct {
	Type       string
	Npub       string
	EventID    string
	DTag       string
	RelayHints []string
	Kind       int
}

func DecodeNIP19(input string) (*DecodedEntity, error) {
	prefix, data, err := nip19.Decode(input)
	if err != nil {
		return nil, fmt.Errorf("invalid NIP-19 encoding: %w", err)
	}

	entity := &DecodedEntity{Type: prefix}

	switch prefix {
	case nip19Npub:
		entity.Npub = input

	case nip19Note:
		eventID, ok := data.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected data type for note")
		}
		entity.EventID = eventID

	case nip19Nprofile:
		profile, ok := data.(gonostr.ProfilePointer)
		if !ok {
			return nil, fmt.Errorf("unexpected data type for nprofile")
		}
		npub, err := btknostr.HexToNpub(profile.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("encoding npub: %w", err)
		}
		entity.Npub = npub
		entity.RelayHints = profile.Relays

	case nip19Nevent:
		event, ok := data.(gonostr.EventPointer)
		if !ok {
			return nil, fmt.Errorf("unexpected data type for nevent")
		}
		entity.EventID = event.ID
		entity.RelayHints = event.Relays
		entity.Kind = event.Kind
		if event.Author != "" {
			npub, err := btknostr.HexToNpub(event.Author)
			if err != nil {
				return nil, fmt.Errorf("encoding author npub: %w", err)
			}
			entity.Npub = npub
		}

	case nip19Naddr:
		addr, ok := data.(gonostr.EntityPointer)
		if !ok {
			return nil, fmt.Errorf("unexpected data type for naddr")
		}
		entity.Kind = addr.Kind
		npub, err := btknostr.HexToNpub(addr.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("encoding npub: %w", err)
		}
		entity.Npub = npub
		entity.DTag = addr.Identifier
		entity.RelayHints = addr.Relays

	case "nsec":
		return nil, fmt.Errorf("secret keys should not be used as list items")

	default:
		return nil, fmt.Errorf("unsupported NIP-19 prefix: %s", prefix)
	}

	return entity, nil
}
