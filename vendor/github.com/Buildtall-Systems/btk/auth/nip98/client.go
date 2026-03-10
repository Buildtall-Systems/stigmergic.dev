package nip98

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func SignRequest(nsec, url, method string) (*nostr.Event, error) {
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
		sk = value.(string)
	default:
		return nil, fmt.Errorf("invalid secret key format: must be 64-char hex or nsec1 bech32")
	}

	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		return nil, fmt.Errorf("deriving public key: %w", err)
	}

	ev := nostr.Event{
		Kind:      KindHTTPAuth,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Tags: nostr.Tags{
			{"u", url},
			{"method", method},
		},
		Content: "",
		PubKey:  pk,
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
