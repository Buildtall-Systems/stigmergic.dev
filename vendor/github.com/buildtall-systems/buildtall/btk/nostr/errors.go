package nostr

import (
	"errors"
	"fmt"
	"strings"
)

// NIP-01 machine-readable prefixes. Relays put one of these at the start of a
// CLOSED reason so clients can react without parsing prose.
const (
	PrefixAuthRequired = "auth-required:"
	PrefixRestricted   = "restricted:"
)

// RelayReadError attributes a failed relay read to the relay that failed and,
// when the relay terminated the subscription itself, to the CLOSED reason it
// gave. It exists because no pool or relay read method in go-nostr returns the
// reason a read produced nothing: an outage is otherwise indistinguishable from
// an empty result.
type RelayReadError struct {
	Err    error
	Relay  string
	Reason string
}

func (e *RelayReadError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("relay read %s: closed: %s", e.Relay, e.Reason)
	}
	return fmt.Sprintf("relay read %s: %v", e.Relay, e.Err)
}

func (e *RelayReadError) Unwrap() error { return e.Err }

// IsAuthRequired reports whether the relay refused the read pending NIP-42
// authentication.
func IsAuthRequired(err error) bool { return hasClosedPrefix(err, PrefixAuthRequired) }

// IsRestricted reports whether the relay refused the read for this client
// regardless of authentication.
func IsRestricted(err error) bool { return hasClosedPrefix(err, PrefixRestricted) }

func hasClosedPrefix(err error, prefix string) bool {
	var readErr *RelayReadError
	if !errors.As(err, &readErr) {
		return false
	}
	return strings.HasPrefix(readErr.Reason, prefix)
}
