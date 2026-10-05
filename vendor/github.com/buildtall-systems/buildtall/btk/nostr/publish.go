package nostr

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// relayPublisher is the narrow slice of *nostr.Relay that publishWithAuth
// exercises. Extracting it lets the auth-retry path be tested with a stub
// without a live relay; *nostr.Relay satisfies it directly.
type relayPublisher interface {
	Publish(ctx context.Context, event nostr.Event) error
	Auth(ctx context.Context, sign func(event *nostr.Event) error) error
}

func PublishWithAuth(ctx context.Context, relay *nostr.Relay, event nostr.Event, sign func(*nostr.Event) error) error {
	return publishWithAuth(ctx, relay, event, sign)
}

func publishWithAuth(ctx context.Context, relay relayPublisher, event nostr.Event, sign func(*nostr.Event) error) error {
	if err := relay.Publish(ctx, event); err != nil {
		if !strings.HasPrefix(err.Error(), "msg: auth-required:") {
			return err
		}

		if authErr := relay.Auth(ctx, sign); authErr != nil {
			return fmt.Errorf("NIP-42 auth: %w", authErr)
		}

		if err := relay.Publish(ctx, event); err != nil {
			return fmt.Errorf("publish after auth: %w", err)
		}
	}
	return nil
}

// EventSink is the slice of *nostr.SimplePool PublishSigned exercises, so a
// pool the caller already holds serves and a test can stand a double in.
type EventSink interface {
	PublishMany(ctx context.Context, urls []string, evt nostr.Event) chan nostr.PublishResult
}

var _ EventSink = (*nostr.SimplePool)(nil)

// PublishSigned signs an event through signer and offers it to every relay,
// accepting it as landed if any one of them took it. The refusals are
// reported only when all of them refuse, since an event reaching one relay
// is published and an event reaching none is not. Signing stamps the
// signer, so an event that names another author is refused before anything
// is sent: it would not fail, it would be stated under the wrong identity.
func PublishSigned(ctx context.Context, sink EventSink, relays []string, signer nostr.Signer, ev *nostr.Event, log *slog.Logger) error {
	author, err := signer.GetPublicKey(ctx)
	if err != nil {
		return fmt.Errorf("signing key: %w", err)
	}
	if ev.PubKey != "" && ev.PubKey != author {
		return fmt.Errorf("kind %d names author %s, not the signing key", ev.Kind, ev.PubKey)
	}
	if err := signer.SignEvent(ctx, ev); err != nil {
		return fmt.Errorf("signing kind %d: %w", ev.Kind, err)
	}

	accepted := 0
	var refusals []string
	for res := range sink.PublishMany(ctx, relays, *ev) {
		if res.Error == nil {
			accepted++
			continue
		}
		refusals = append(refusals, fmt.Sprintf("%s: %v", res.RelayURL, res.Error))
	}
	if accepted == 0 {
		return fmt.Errorf("no relay accepted kind %d: %s", ev.Kind, strings.Join(refusals, "; "))
	}
	log.Debug("published", "kind", ev.Kind, "id", ev.ID, "accepted", accepted, "refused", len(refusals))
	return nil
}
