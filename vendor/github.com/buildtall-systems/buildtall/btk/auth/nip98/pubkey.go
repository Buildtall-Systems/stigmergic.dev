package nip98

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nbd-wtf/go-nostr/nip19"
)

func NormalizePubkey(pubkey string) (string, error) {
	if len(pubkey) == 64 {
		return pubkey, nil
	}

	if strings.HasPrefix(pubkey, "npub1") {
		_, v, err := nip19.Decode(pubkey)
		if err != nil {
			return "", fmt.Errorf("failed to decode npub: %w", err)
		}
		hex, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("unexpected npub decode result type")
		}
		return hex, nil
	}

	return "", fmt.Errorf("invalid pubkey format: must be 64-char hex or npub1 bech32")
}

func NormalizePubkeys(pubkeys []string) ([]string, error) {
	result := make([]string, len(pubkeys))
	for i, pk := range pubkeys {
		normalized, err := NormalizePubkey(pk)
		if err != nil {
			return nil, fmt.Errorf("invalid pubkey at index %d (%s): %w", i, pk, err)
		}
		result[i] = normalized
	}
	return result, nil
}

func IsPubkeyAllowed(pubkey string, allowedPubkeys []string) bool {
	if len(allowedPubkeys) == 0 {
		return false
	}
	return slices.Contains(allowedPubkeys, pubkey)
}
