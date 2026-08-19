package nostr

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/nbd-wtf/go-nostr"
)

const listManagerKind = 30000

type ListCallback func(npub string)

type ListManager struct {
	pool    *nostr.SimplePool
	log     *slog.Logger
	onAdd   ListCallback
	onRemov ListCallback
	members map[string]bool
	owner   string
	dtag    string
	relays  []string
	mu      sync.RWMutex
}

func NewListManager(pool *nostr.SimplePool, relays []string, ownerNpub string, dtag string, log *slog.Logger) *ListManager {
	return &ListManager{
		members: make(map[string]bool),
		pool:    pool,
		relays:  relays,
		log:     log,
		owner:   ownerNpub,
		dtag:    dtag,
	}
}

func (lm *ListManager) OnAdd(cb ListCallback) {
	lm.onAdd = cb
}

func (lm *ListManager) OnRemove(cb ListCallback) {
	lm.onRemov = cb
}

func (lm *ListManager) Start(ctx context.Context) error {
	_, err := lm.StartNotifyDone(ctx)
	return err
}

// StartNotifyDone behaves like Start and additionally returns a channel that
// closes when the list subscription's event stream terminates, so callers can
// detect watcher death and re-invoke.
func (lm *ListManager) StartNotifyDone(ctx context.Context) (<-chan struct{}, error) {
	ownerHex, err := NpubToHex(lm.owner)
	if err != nil {
		return nil, fmt.Errorf("converting owner npub to hex: %w", err)
	}

	filter := nostr.Filter{
		Authors: []string{ownerHex},
		Kinds:   []int{listManagerKind},
		Tags:    nostr.TagMap{"d": []string{lm.dtag}},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for evt := range lm.pool.SubscribeMany(ctx, lm.relays, filter) {
			lm.UpdateFromEvent(evt.Event)
		}
		if ctx.Err() == nil {
			lm.log.Warn("list subscription ended unexpectedly", "dtag", lm.dtag)
		}
	}()

	return done, nil
}

func (lm *ListManager) UpdateFromEvent(event *nostr.Event) {
	newMembers := ParseMembersFromEvent(event)

	lm.mu.Lock()
	oldMembers := lm.members
	lm.members = newMembers
	lm.mu.Unlock()

	for npub := range newMembers {
		if !oldMembers[npub] {
			lm.log.Info("member added", "npub", npub, "dtag", lm.dtag)
			if lm.onAdd != nil {
				lm.onAdd(npub)
			}
		}
	}

	for npub := range oldMembers {
		if !newMembers[npub] {
			lm.log.Info("member removed", "npub", npub, "dtag", lm.dtag)
			if lm.onRemov != nil {
				lm.onRemov(npub)
			}
		}
	}

	lm.log.Info("member list updated", "count", len(newMembers), "dtag", lm.dtag)
}

func (lm *ListManager) IsRegistered(npub string) bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.members[npub]
}

func (lm *ListManager) Members() []string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make([]string, 0, len(lm.members))
	for npub := range lm.members {
		result = append(result, npub)
	}
	return result
}

func ParseMembersFromEvent(event *nostr.Event) map[string]bool {
	members := make(map[string]bool)
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "p" {
			npub, err := HexToNpub(tag[1])
			if err != nil {
				continue
			}
			members[npub] = true
		}
	}
	return members
}
