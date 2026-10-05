package bunker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

var errForeignAuthor = errors.New("signer answered with a foreign author")

// Session is a live bunker-backed signing channel for one logged-in user.
// Its lifetime is the context handed to the underlying nip46 client; Close
// cancels it.
type Session struct {
	signer    nostr.Signer
	cancel    context.CancelFunc
	lastUsed  time.Time
	expiresAt time.Time
	userNpub  string
	userHex   string
	mu        sync.Mutex
}

var _ nostr.Signer = (*Session)(nil)

func (s *Session) UserNpub() string {
	return s.userNpub
}

// UserPubkeyHex is the wire form of the session user's key, for the browser
// signing facade, which fills events and filters.
func (s *Session) UserPubkeyHex() string {
	return s.userHex
}

// GetPublicKey asks the remote signer, and refuses an answer that is not the
// logged-in user: a hostile transport relay is in the threat model.
func (s *Session) GetPublicKey(ctx context.Context) (string, error) {
	pk, err := s.signer.GetPublicKey(ctx)
	if err != nil {
		return "", fmt.Errorf("bunker get_public_key: %w", err)
	}
	if pk != s.userHex {
		return "", errForeignAuthor
	}
	return pk, nil
}

// SignEvent signs through the remote signer. The nip46 client verifies id and
// signature; the author check is ours, for the same threat as GetPublicKey.
func (s *Session) SignEvent(ctx context.Context, evt *nostr.Event) error {
	if err := s.signer.SignEvent(ctx, evt); err != nil {
		return fmt.Errorf("bunker sign: %w", err)
	}
	if evt.PubKey != s.userHex {
		return errForeignAuthor
	}
	return nil
}

func (s *Session) Close() {
	s.cancel()
}

func (s *Session) touch() {
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

func (s *Session) expired(now time.Time, idleTimeout time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.After(s.expiresAt) || now.Sub(s.lastUsed) > idleTimeout
}
