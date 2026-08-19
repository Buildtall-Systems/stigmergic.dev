package okf

import (
	"github.com/nbd-wtf/go-nostr"
)

// Servers returns the Blossom base URLs the vault root states: the value of
// every okf-server tag, in tag order, which is the vault's preference order.
// An empty value is skipped, and a nil root or one stating no servers yields
// nothing; falling back to the owner's kind 10063 server list is the
// consumer's business.
func Servers(root *nostr.Event) []string {
	if root == nil {
		return nil
	}
	var servers []string
	for _, tag := range root.Tags {
		if len(tag) >= 2 && tag[0] == TagOKFServer && tag[1] != "" {
			servers = append(servers, tag[1])
		}
	}
	return servers
}
