package nostr

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func NpubToHex(npub string) (string, error) {
	prefix, v, err := nip19.Decode(npub)
	if err != nil {
		return "", fmt.Errorf("invalid npub: %w", err)
	}
	if prefix != "npub" {
		return "", fmt.Errorf("expected npub prefix, got %s", prefix)
	}
	pubkey, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("unexpected data type for npub")
	}
	return pubkey, nil
}

func NsecToHex(nsec string) (string, error) {
	prefix, v, err := nip19.Decode(nsec)
	if err != nil {
		return "", fmt.Errorf("invalid nsec: %w", err)
	}
	if prefix != "nsec" {
		return "", fmt.Errorf("expected nsec prefix, got %s", prefix)
	}
	seckey, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("unexpected data type for nsec")
	}
	return seckey, nil
}

func HexToNpub(pubkey string) (string, error) {
	return nip19.EncodePublicKey(pubkey)
}

func HexToNsec(seckey string) (string, error) {
	return nip19.EncodePrivateKey(seckey)
}

// NsecToNpub names the identity a secret key acts as. Signing stamps the
// signer's own pubkey onto whatever it signs, so a caller publishing events it
// composed for a named author compares this against that author first: the two
// disagreeing means the events would land under a key nobody asked for, with
// nothing on the wire to say so.
func NsecToNpub(nsec string) (string, error) {
	seckey, err := NsecToHex(nsec)
	if err != nil {
		return "", err
	}
	pubkey, err := nostr.GetPublicKey(seckey)
	if err != nil {
		return "", fmt.Errorf("deriving a public key from an nsec: %w", err)
	}
	return HexToNpub(pubkey)
}

// HexToNevent encodes an event id as a nevent, carrying whatever relay hints
// the reference offered. NIP-01 fixes an event id as hex on the wire; a
// nevent is how that id leaves the protocol boundary and reaches a person.
func HexToNevent(eventID string, relays []string) (string, error) {
	return nip19.EncodeEvent(eventID, relays, "")
}
