package nostr

import (
	"fmt"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func NewDeleteEvent(pubkey string, eventToDelete *nostr.Event) *nostr.Event {
	event := &nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      5,
		Tags:      nostr.Tags{},
		Content:   "",
	}

	event.Tags = append(event.Tags, nostr.Tag{"e", eventToDelete.ID})

	if eventToDelete.Kind >= 10000 && eventToDelete.Kind < 20000 {
		aTag := fmt.Sprintf("%d:%s", eventToDelete.Kind, eventToDelete.PubKey)
		event.Tags = append(event.Tags, nostr.Tag{"a", aTag})
	} else if eventToDelete.Kind >= 30000 && eventToDelete.Kind < 40000 {
		if dTag := eventToDelete.Tags.Find("d"); len(dTag) > 1 {
			aTag := fmt.Sprintf("%d:%s:%s", eventToDelete.Kind, eventToDelete.PubKey, dTag[1])
			event.Tags = append(event.Tags, nostr.Tag{"a", aTag})
		}
	}

	return event
}
