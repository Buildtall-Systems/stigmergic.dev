package nostr

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/retry"
)

// ConnectWithRetry connects to a relay with exponential backoff. On success
// the caller owns the returned *nostr.Relay and must close it.
func ConnectWithRetry(ctx context.Context, url string, maxAttempts int, baseDelay time.Duration, logger *slog.Logger) (*nostr.Relay, error) {
	var relay *nostr.Relay

	err := retry.Do(ctx, maxAttempts, baseDelay, func(ctx context.Context) error {
		r, err := nostr.RelayConnect(ctx, url)
		if err != nil {
			logger.Warn("relay connection failed, retrying",
				"relay", url,
				"error", err,
			)
			return err
		}
		relay = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", url, err)
	}

	return relay, nil
}
