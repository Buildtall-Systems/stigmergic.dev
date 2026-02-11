package auth

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

const (
	KindHTTPAuth    = 27235
	DefaultTimeSkew = 60 * time.Second
)

func VerifyNIP98Event(event *nostr.Event, expectedURL, expectedMethod string) error {
	if event.Kind != KindHTTPAuth {
		return fmt.Errorf("invalid event kind: expected %d, got %d", KindHTTPAuth, event.Kind)
	}

	if event.Content != "" {
		return fmt.Errorf("event content must be empty")
	}

	ok, err := event.CheckSignature()
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid signature")
	}

	uTag := event.Tags.Find("u")
	if uTag == nil {
		return fmt.Errorf("missing required 'u' tag")
	}
	if uTag[1] != expectedURL {
		return fmt.Errorf("URL mismatch: expected %s, got %s", expectedURL, uTag[1])
	}

	methodTag := event.Tags.Find("method")
	if methodTag == nil {
		return fmt.Errorf("missing required 'method' tag")
	}
	if !strings.EqualFold(methodTag[1], expectedMethod) {
		return fmt.Errorf("method mismatch: expected %s, got %s", expectedMethod, methodTag[1])
	}

	now := time.Now()
	eventTime := event.CreatedAt.Time()
	diff := now.Sub(eventTime)
	if diff < 0 {
		diff = -diff
	}
	if diff > DefaultTimeSkew {
		return fmt.Errorf("event timestamp too far from current time: %v", diff)
	}

	return nil
}

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
	return slices.Contains(allowedPubkeys, pubkey)
}

func TimestampFromTime(t time.Time) nostr.Timestamp {
	return nostr.Timestamp(t.Unix())
}
