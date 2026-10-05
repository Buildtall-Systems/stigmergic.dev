package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/server"
)

// relayChallenge is the NIP-42 challenge the fake relay issues.
const relayChallenge = "stigmergic-test-challenge"

// authorOnlyRelay answers the way a relay with author_only_reads does: it
// challenges every connection, closes a REQ with auth-required until the
// connection authenticates, and serves an authenticated REQ an empty result.
// It records each connection and the npub each AUTH proved.
type authorOnlyRelay struct {
	authed      []string
	mu          sync.Mutex
	connections int
}

func (a *authorOnlyRelay) counts() (int, []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connections, append([]string(nil), a.authed...)
}

func (a *authorOnlyRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	a.mu.Lock()
	a.connections++
	a.mu.Unlock()

	ctx := r.Context()
	send := func(msg ...any) bool {
		data, err := json.Marshal(msg)
		if err != nil {
			return false
		}
		return conn.Write(ctx, websocket.MessageText, data) == nil
	}

	if !send("AUTH", relayChallenge) {
		return
	}

	authed := false
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var msg []json.RawMessage
		if json.Unmarshal(data, &msg) != nil || len(msg) < 2 {
			continue
		}
		var label, subID string
		if json.Unmarshal(msg[0], &label) != nil {
			continue
		}

		switch label {
		case "REQ":
			if json.Unmarshal(msg[1], &subID) != nil {
				continue
			}
			if !authed {
				send("CLOSED", subID, "auth-required: this relay serves only its author")
				continue
			}
			send("EOSE", subID)
		case "AUTH":
			var evt nostr.Event
			if json.Unmarshal(msg[1], &evt) != nil {
				continue
			}
			ok, sigErr := evt.CheckSignature()
			challenge := evt.Tags.Find("challenge")
			if !ok || sigErr != nil || evt.Kind != nostr.KindClientAuthentication || challenge == nil || challenge[1] != relayChallenge {
				send("OK", evt.ID, false, "invalid: the AUTH event does not answer this challenge")
				continue
			}
			npub, encErr := nip19.EncodePublicKey(evt.PubKey)
			if encErr != nil {
				continue
			}
			authed = true
			a.mu.Lock()
			a.authed = append(a.authed, npub)
			a.mu.Unlock()
			send("OK", evt.ID, true, "")
		}
	}
}

// testKey is a key generated for one test, with the npub it signs as.
type testKey struct {
	sk   string
	npub string
}

func newTestKey(t *testing.T) *testKey {
	t.Helper()
	sk := nostr.GeneratePrivateKey()
	pub, err := nostr.GetPublicKey(sk)
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	npub, err := nip19.EncodePublicKey(pub)
	if err != nil {
		t.Fatalf("EncodePublicKey: %v", err)
	}
	return &testKey{sk: sk, npub: npub}
}

func (k *testKey) GetPublicKey(context.Context) (string, error) {
	return nostr.GetPublicKey(k.sk)
}

func (k *testKey) SignEvent(_ context.Context, evt *nostr.Event) error {
	return evt.Sign(k.sk)
}

// refusingSigner stands for a reader whose page is closed: it cannot sign.
type refusingSigner struct{}

var errPageClosed = errors.New("no page is open to sign")

func (refusingSigner) GetPublicKey(context.Context) (string, error)  { return "", errPageClosed }
func (refusingSigner) SignEvent(context.Context, *nostr.Event) error { return errPageClosed }

func loaderAgainst(t *testing.T, relay *authorOnlyRelay) server.VaultLoader {
	t.Helper()

	srv := httptest.NewServer(relay)
	t.Cleanup(srv.Close)

	cfg := &config.Config{}
	cfg.Vault.Relays = []string{"ws" + strings.TrimPrefix(srv.URL, "http")}
	load := vaultLoader(cfg)
	if load == nil {
		t.Fatal("a configured relay yielded no loader")
	}
	return load
}

// TestLoaderReportsAnUnansweredChallenge is the anonymous read: the relay's
// challenge goes unanswered, so the loader reports the withheld events
// rather than an empty owner.
func TestLoaderReportsAnUnansweredChallenge(t *testing.T) {
	t.Parallel()

	relay := &authorOnlyRelay{}
	load := loaderAgainst(t, relay)
	owner := newTestKey(t)

	vaults, err := load(t.Context(), owner.npub, nil)
	if !errors.Is(err, server.ErrAuthRequired) {
		t.Fatalf("expected ErrAuthRequired, got %v", err)
	}
	if len(vaults) != 0 {
		t.Errorf("expected no vaults, got %d", len(vaults))
	}
	if _, authed := relay.counts(); len(authed) != 0 {
		t.Errorf("the anonymous read authenticated as %v", authed)
	}
}

// TestLoaderAuthenticatesThroughTheOwnersSigner is the read with a signer:
// the relay's challenge is answered as the owner, and the read completes.
func TestLoaderAuthenticatesThroughTheOwnersSigner(t *testing.T) {
	t.Parallel()

	relay := &authorOnlyRelay{}
	load := loaderAgainst(t, relay)
	owner := newTestKey(t)

	if _, err := load(t.Context(), owner.npub, owner); err != nil {
		t.Fatalf("the signed read failed: %v", err)
	}
	if _, authed := relay.counts(); len(authed) != 1 || authed[0] != owner.npub {
		t.Errorf("expected one AUTH as %s, got %v", owner.npub, authed)
	}
}

// TestLoaderReportsASignerThatCannotSign holds that a signer refusing the
// challenge reads as unanswered, so the owner is retried later.
func TestLoaderReportsASignerThatCannotSign(t *testing.T) {
	t.Parallel()

	relay := &authorOnlyRelay{}
	load := loaderAgainst(t, relay)

	if _, err := load(t.Context(), newTestKey(t).npub, refusingSigner{}); !errors.Is(err, server.ErrAuthRequired) {
		t.Fatalf("expected ErrAuthRequired, got %v", err)
	}
}

// TestLoaderKeepsEachOwnersConnectionApart holds the isolation: an
// anonymous read and each owner's signed read ride separate connections,
// and an owner's later read rides the connection already authenticated.
func TestLoaderKeepsEachOwnersConnectionApart(t *testing.T) {
	t.Parallel()

	relay := &authorOnlyRelay{}
	load := loaderAgainst(t, relay)
	alice, bob := newTestKey(t), newTestKey(t)

	if _, err := load(t.Context(), alice.npub, nil); !errors.Is(err, server.ErrAuthRequired) {
		t.Fatalf("anonymous read: expected ErrAuthRequired, got %v", err)
	}
	for _, k := range []*testKey{alice, bob, alice} {
		if _, err := load(t.Context(), k.npub, k); err != nil {
			t.Fatalf("signed read as %s: %v", k.npub, err)
		}
	}

	connections, authed := relay.counts()
	if connections != 3 {
		t.Errorf("expected three connections (anonymous, alice, bob), got %d", connections)
	}
	if len(authed) != 2 || authed[0] != alice.npub || authed[1] != bob.npub {
		t.Errorf("expected one AUTH each for alice then bob, got %v", authed)
	}
}
