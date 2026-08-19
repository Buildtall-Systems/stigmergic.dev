package nip98

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// NIP-98 tag names.
const (
	tagMethod  = "method"
	tagPayload = "payload"
)

const nonceBytes = 16

func SignRequest(nsec, url, method string, payloadHash ...string) (*nostr.Event, error) {
	var sk string

	switch {
	case len(nsec) == 64:
		sk = nsec
	case strings.HasPrefix(nsec, "nsec1"):
		prefix, value, err := nip19.Decode(nsec)
		if err != nil {
			return nil, fmt.Errorf("decoding nsec: %w", err)
		}
		if prefix != "nsec" {
			return nil, fmt.Errorf("expected nsec prefix, got %s", prefix)
		}
		var ok bool
		sk, ok = value.(string)
		if !ok {
			return nil, fmt.Errorf("decoded nsec value is not a string")
		}
	default:
		return nil, fmt.Errorf("invalid secret key format: must be 64-char hex or nsec1 bech32")
	}

	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		return nil, fmt.Errorf("deriving public key: %w", err)
	}

	// A random nonce makes each signed event's ID unique, so server-side
	// replay caches never conflate two otherwise-identical requests signed
	// within the same second. Servers ignore unknown tags.
	nonce := make([]byte, nonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	tags := nostr.Tags{
		{"u", url},
		{tagMethod, method},
		{"nonce", hex.EncodeToString(nonce)},
	}
	if len(payloadHash) > 0 && payloadHash[0] != "" {
		tags = append(tags, nostr.Tag{tagPayload, payloadHash[0]})
	}

	ev := nostr.Event{
		Kind:      KindHTTPAuth,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Tags:      tags,
		Content:   "",
		PubKey:    pk,
	}

	if err := ev.Sign(sk); err != nil {
		return nil, fmt.Errorf("signing event: %w", err)
	}

	return &ev, nil
}

func HeaderFromEvent(event *nostr.Event) (string, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshaling event: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	return "Nostr " + encoded, nil
}
