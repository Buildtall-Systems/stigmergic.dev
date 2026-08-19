package nostr

import (
	"context"
	"fmt"
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
