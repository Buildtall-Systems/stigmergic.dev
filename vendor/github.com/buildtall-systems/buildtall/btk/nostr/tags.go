package nostr

import "github.com/nbd-wtf/go-nostr"

func MakeTag(values ...string) nostr.Tag {
	return nostr.Tag(values)
}

func AppendTag(tags nostr.Tags, tag nostr.Tag) nostr.Tags {
	return append(tags, tag)
}
