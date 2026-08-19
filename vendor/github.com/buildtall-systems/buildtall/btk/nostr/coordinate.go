package nostr

import (
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func ParseCoordinate(coord string) (kind int, pubkey string, dTag string, err error) {
	parts := strings.SplitN(coord, ":", 3)
	if len(parts) != 3 {
		return 0, "", "", fmt.Errorf("invalid coordinate format: %s", coord)
	}

	if _, err := fmt.Sscanf(parts[0], "%d", &kind); err != nil {
		return 0, "", "", fmt.Errorf("invalid kind in coordinate: %s", coord)
	}

	return kind, parts[1], parts[2], nil
}

func CoordToNaddr(coord string, relays []string) (string, error) {
	kind, pubkey, dtag, err := ParseCoordinate(coord)
	if err != nil {
		return "", err
	}
	return nip19.EncodeEntity(pubkey, kind, dtag, relays)
}

// DecodeNaddr reads the addressable event a naddr names, along with the relay
// hints it carries. It is the read direction of CoordToNaddr, and the boundary
// where a bech32 identifier a person handed us becomes protocol fields.
func DecodeNaddr(naddr string) (nostr.EntityPointer, error) {
	prefix, value, err := nip19.Decode(naddr)
	if err != nil {
		return nostr.EntityPointer{}, fmt.Errorf("invalid naddr: %w", err)
	}
	if prefix != "naddr" {
		return nostr.EntityPointer{}, fmt.Errorf("expected naddr prefix, got %s", prefix)
	}
	pointer, ok := value.(nostr.EntityPointer)
	if !ok {
		return nostr.EntityPointer{}, fmt.Errorf("unexpected data type for naddr")
	}
	return pointer, nil
}
