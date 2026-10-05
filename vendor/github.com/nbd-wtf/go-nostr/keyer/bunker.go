package keyer

import (
	"context"
	"errors"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip46"
)

var _ nostr.Keyer = (*BunkerSigner)(nil)

// defaultBunkerTimeout bounds get_public_key and sign_event when no
// SignerOptions.BunkerSignTimeout was given; encrypt and decrypt stay unbounded
// in that case, as they always were.
const defaultBunkerTimeout = 30 * time.Second

// BunkerSigner is a signer that delegates operations to a remote bunker using NIP-46.
// It communicates with the bunker for all cryptographic operations rather than
// handling the private key locally.
type BunkerSigner struct {
	bunker  *nip46.BunkerClient
	timeout time.Duration
}

// NewBunkerSignerFromBunkerClient creates a new BunkerSigner from an existing BunkerClient.
func NewBunkerSignerFromBunkerClient(bc *nip46.BunkerClient) BunkerSigner {
	return BunkerSigner{bunker: bc}
}

// WithTimeout returns a copy of the signer that bounds every bunker operation
// at timeout, as SignerOptions.BunkerSignTimeout does through New.
func (bs BunkerSigner) WithTimeout(timeout time.Duration) BunkerSigner {
	bs.timeout = timeout
	return bs
}

func (bs BunkerSigner) bounded(ctx context.Context, op string, fallback time.Duration) (context.Context, context.CancelFunc) {
	timeout := bs.timeout
	if timeout == 0 {
		timeout = fallback
	}
	if timeout == 0 {
		return ctx, func() {}
	}
	return context.WithTimeoutCause(ctx, timeout, errors.New(op+" took too long"))
}

// GetPublicKey retrieves the public key from the remote bunker.
// It uses a timeout to prevent hanging indefinitely.
func (bs BunkerSigner) GetPublicKey(ctx context.Context) (string, error) {
	ctx, cancel := bs.bounded(ctx, "get_public_key", defaultBunkerTimeout)
	defer cancel()
	pk, err := bs.bunker.GetPublicKey(ctx)
	if err != nil {
		return "", err
	}
	return pk, nil
}

// SignEvent sends the event to the remote bunker for signing.
// It uses a timeout to prevent hanging indefinitely.
func (bs BunkerSigner) SignEvent(ctx context.Context, evt *nostr.Event) error {
	ctx, cancel := bs.bounded(ctx, "sign_event", defaultBunkerTimeout)
	defer cancel()
	return bs.bunker.SignEvent(ctx, evt)
}

// Encrypt encrypts a plaintext message for a recipient using the remote bunker.
func (bs BunkerSigner) Encrypt(ctx context.Context, plaintext string, recipient string) (string, error) {
	ctx, cancel := bs.bounded(ctx, "nip44_encrypt", 0)
	defer cancel()
	return bs.bunker.NIP44Encrypt(ctx, recipient, plaintext)
}

// Decrypt decrypts a base64-encoded ciphertext from a sender using the remote bunker.
func (bs BunkerSigner) Decrypt(ctx context.Context, base64ciphertext string, sender string) (plaintext string, err error) {
	ctx, cancel := bs.bounded(ctx, "nip44_decrypt", 0)
	defer cancel()
	return bs.bunker.NIP44Decrypt(ctx, sender, base64ciphertext)
}
