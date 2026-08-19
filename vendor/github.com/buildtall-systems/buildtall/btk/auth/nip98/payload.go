package nip98

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// VerifyPayloadTag checks that the event's "payload" tag matches the SHA-256
// hex digest of body. A request without a body requires no tag; a request
// with a body must carry a matching tag.
func VerifyPayloadTag(event *nostr.Event, body []byte) error {
	if len(body) == 0 {
		return nil
	}

	payloadTag := event.Tags.Find(tagPayload)
	if len(payloadTag) < 2 {
		return fmt.Errorf("missing required 'payload' tag for request with body")
	}

	sum := sha256.Sum256(body)
	expected := hex.EncodeToString(sum[:])
	if !strings.EqualFold(payloadTag[1], expected) {
		return fmt.Errorf("payload hash mismatch")
	}

	return nil
}
