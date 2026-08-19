package retry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const maxDelay = 60 * time.Second

// FatalError wraps an error that should not be retried. When fn returns a
// FatalError, Do stops immediately and returns the unwrapped inner error.
type FatalError struct {
	Err error
}

func (e *FatalError) Error() string { return e.Err.Error() }
func (e *FatalError) Unwrap() error { return e.Err }

// Fatal wraps err so that Do will not retry it.
func Fatal(err error) error {
	return &FatalError{Err: err}
}

// Do calls fn up to maxAttempts times with exponential backoff between failures.
// Backoff doubles each attempt: baseDelay, 2*baseDelay, 4*baseDelay, etc.,
// capped at 60 seconds. Returns nil on first success. If fn returns a
// FatalError, Do stops immediately and returns the inner error. Returns the
// last error wrapped with attempt count if all attempts are exhausted.
// Respects ctx cancellation during backoff waits.
func Do(ctx context.Context, maxAttempts int, baseDelay time.Duration, fn func(ctx context.Context) error) error {
	var lastErr error

	for attempt := range maxAttempts {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("retry cancelled: %w", err)
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		var fatal *FatalError
		if errors.As(lastErr, &fatal) {
			return fatal.Err
		}

		if attempt < maxAttempts-1 {
			delay := min(baseDelay<<attempt, maxDelay)

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return fmt.Errorf("retry cancelled during backoff: %w", ctx.Err())
			}
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", maxAttempts, lastErr)
}
