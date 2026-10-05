package challenge

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

const (
	ChallengeKind       = 22242
	ChallengeExpiration = 5 * time.Minute
)

// tagChallenge is the NIP-42 challenge tag name.
const tagChallenge = "challenge"

type pending struct {
	expiresAt time.Time
	npub      string
}

// Store tracks issued challenge nonces so that verification can confirm
// the server actually generated the challenge. Nonces are single-use:
// a successful verification consumes the nonce.
type Store struct {
	pending   map[string]pending // nonce hex → pending
	lastSweep time.Time
	expiry    time.Duration
	mu        sync.Mutex
}

func NewStore() *Store {
	return &Store{
		pending:   make(map[string]pending),
		expiry:    ChallengeExpiration,
		lastSweep: time.Now(),
	}
}

// NewChallenge generates a kind-22242 challenge event for the given npub
// and records the nonce so Verify can confirm it was server-issued.
func (s *Store) NewChallenge(npub string) (*nostr.Event, error) {
	hexPubkey, err := btknostr.NpubToHex(npub)
	if err != nil {
		return nil, fmt.Errorf("invalid npub: %w", err)
	}

	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return nil, fmt.Errorf("generating challenge: %w", err)
	}

	nonce := hex.EncodeToString(challengeBytes)

	s.mu.Lock()
	s.sweep()
	s.pending[nonce] = pending{
		npub:      npub,
		expiresAt: time.Now().Add(s.expiry),
	}
	s.mu.Unlock()

	event := &nostr.Event{
		PubKey:    hexPubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      ChallengeKind,
		Tags:      nostr.Tags{{tagChallenge, nonce}},
		Content:   "buildtall.systems authentication challenge",
	}

	return event, nil
}

// Verify validates a signed challenge event: signature, kind, pubkey,
// expiry, and — critically — that the challenge nonce was issued by this
// store for this npub. The nonce is consumed on success.
func (s *Store) Verify(signedEvent *nostr.Event, npub string) (string, error) {
	hexPubkey, err := btknostr.NpubToHex(npub)
	if err != nil {
		return "", fmt.Errorf("invalid npub: %w", err)
	}

	if signedEvent.Kind != ChallengeKind {
		return "", fmt.Errorf("invalid event kind: expected %d, got %d", ChallengeKind, signedEvent.Kind)
	}

	if signedEvent.PubKey != hexPubkey {
		return "", fmt.Errorf("pubkey mismatch")
	}

	eventTime := time.Unix(int64(signedEvent.CreatedAt), 0)
	if time.Since(eventTime) > ChallengeExpiration {
		return "", fmt.Errorf("challenge expired")
	}

	ok, sigErr := signedEvent.CheckSignature()
	if sigErr != nil {
		return "", fmt.Errorf("checking signature: %w", sigErr)
	}
	if !ok {
		return "", fmt.Errorf("invalid signature")
	}

	nonce := extractChallengeNonce(signedEvent)
	if nonce == "" {
		return "", fmt.Errorf("missing challenge nonce in event")
	}

	s.mu.Lock()
	p, found := s.pending[nonce]
	if found {
		delete(s.pending, nonce)
	}
	s.mu.Unlock()

	if !found {
		return "", fmt.Errorf("unknown or already-used challenge nonce")
	}

	if time.Now().After(p.expiresAt) {
		return "", fmt.Errorf("challenge expired")
	}

	if p.npub != npub {
		return "", fmt.Errorf("challenge was not issued for this npub")
	}

	return npub, nil
}

func extractChallengeNonce(event *nostr.Event) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == tagChallenge {
			return tag[1]
		}
	}
	return ""
}

// sweep removes expired nonces. Called under lock.
func (s *Store) sweep() {
	now := time.Now()
	if now.Sub(s.lastSweep) < time.Minute {
		return
	}
	for nonce, p := range s.pending {
		if now.After(p.expiresAt) {
			delete(s.pending, nonce)
		}
	}
	s.lastSweep = now
}
