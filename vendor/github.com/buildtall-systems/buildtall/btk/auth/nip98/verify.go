package nip98

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

const KindHTTPAuth = nostr.KindHTTPAuth

const DefaultMaxSkew = 60 * time.Second

func VerifyNIP98Event(event *nostr.Event, expectedURL, expectedMethod string, maxSkew time.Duration) error {
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
	if len(uTag) < 2 {
		return fmt.Errorf("missing required 'u' tag")
	}
	if uTag[1] != expectedURL {
		return fmt.Errorf("URL mismatch: expected %s, got %s", expectedURL, uTag[1])
	}

	methodTag := event.Tags.Find(tagMethod)
	if len(methodTag) < 2 {
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
	if diff > maxSkew {
		return fmt.Errorf("event timestamp too far from current time: %v", diff)
	}

	return nil
}

func ParseNIP98FromHeader(authHeader string) (*nostr.Event, error) {
	if !strings.HasPrefix(authHeader, "Nostr ") {
		return nil, fmt.Errorf("authorization header must start with 'Nostr '")
	}

	encoded := strings.TrimPrefix(authHeader, "Nostr ")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	var event nostr.Event
	if err := json.Unmarshal(decoded, &event); err != nil {
		return nil, fmt.Errorf("JSON unmarshal failed: %w", err)
	}

	return &event, nil
}
