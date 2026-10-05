// Package site mounts a btk-nav website's whole authentication surface:
// extension login, remote-signer login, and the routes the browser signer
// calls.
package site

import (
	"fmt"
	"strings"

	"github.com/buildtall-systems/buildtall/btk/auth/bunker"
)

const secureRelayScheme = "wss://"

// NIP46Config names the relays that carry NIP-46 handshake and signing
// traffic. A service embeds it as its nip46 config section.
type NIP46Config struct {
	TransportRelays []string `mapstructure:"transport_relays"`
}

// Defaults fills an empty relay list with the default transport relay.
func (c *NIP46Config) Defaults() {
	if len(c.TransportRelays) == 0 {
		c.TransportRelays = []string{bunker.DefaultTransportRelay}
	}
}

// Validate refuses an empty list and any relay that is not wss, because the
// transport carries the signer's traffic.
func (c NIP46Config) Validate() error {
	if len(c.TransportRelays) == 0 {
		return fmt.Errorf("nip46.transport_relays needs at least one relay")
	}
	for _, relay := range c.TransportRelays {
		if !strings.HasPrefix(relay, secureRelayScheme) {
			return fmt.Errorf("nip46.transport_relays entry %q is not a %s relay", relay, secureRelayScheme)
		}
	}
	return nil
}
