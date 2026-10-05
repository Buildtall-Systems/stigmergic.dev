package bunker

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/nbd-wtf/go-nostr/nip46"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

type pendingLogin struct {
	expiresAt time.Time
	client    *nip46.BunkerClient
	cancel    context.CancelFunc
	id        string
	state     State
	authURL   string
	failure   string
	userNpub  string
	userHex   string
	uri       string
}

type clientMetadata struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// StartBunkerLogin begins the paste flow: it builds a bunker client for the
// URI's remote signer and drives connect plus get_public_key in the
// background. The returned pending id is polled via Status.
func (m *Manager) StartBunkerLogin(bunkerURI string) (string, error) {
	if !nip46.IsValidBunkerURL(bunkerURI) {
		return "", fmt.Errorf("invalid bunker URI")
	}
	parsed, err := url.Parse(bunkerURI)
	if err != nil {
		return "", fmt.Errorf("parsing bunker URI: %w", err)
	}
	target := parsed.Host
	secret := parsed.Query().Get("secret")
	relays := parsed.Query()["relay"]
	if len(relays) == 0 {
		return "", fmt.Errorf("bunker URI names no relays")
	}

	clientSecret := nostr.GeneratePrivateKey()
	id := rand.Text()

	sessionCtx, cancel := context.WithCancel(context.Background())
	p := &pendingLogin{
		id:        id,
		state:     StateWaiting,
		cancel:    cancel,
		expiresAt: time.Now().Add(pendingExpiration),
	}
	m.mu.Lock()
	m.pendings[id] = p
	m.mu.Unlock()

	client := nip46.NewBunker(sessionCtx, clientSecret, target, relays, m.pool, func(authURL string) {
		m.setAuthURL(id, authURL)
	})

	go m.connectAndFinish(sessionCtx, p, client, target, secret)
	return id, nil
}

// StartNostrConnectLogin begins the QR flow: it subscribes for the signer's
// connect response before exposing the nostrconnect URI, then completes the
// handshake in the background. The URI carries the client pubkey in hex, the
// wire format NIP-46 prescribes, and is read back through Status so the QR
// handler and the caller share one source.
func (m *Manager) StartNostrConnectLogin() (string, error) {
	clientSecret := nostr.GeneratePrivateKey()
	clientPub, err := nostr.GetPublicKey(clientSecret)
	if err != nil {
		return "", fmt.Errorf("deriving client key: %w", err)
	}
	secret := rand.Text()
	id := rand.Text()

	sessionCtx, cancel := context.WithCancel(context.Background())
	p := &pendingLogin{
		id:        id,
		state:     StateWaiting,
		cancel:    cancel,
		expiresAt: time.Now().Add(pendingExpiration),
	}
	m.mu.Lock()
	m.pendings[id] = p
	m.mu.Unlock()

	waitCtx, waitCancel := context.WithDeadline(sessionCtx, p.expiresAt)
	events := m.pool.SubscribeMany(waitCtx, m.transportRelays, nostr.Filter{
		Kinds: []int{nostr.KindNostrConnect},
		Tags:  nostr.TagMap{"p": []string{clientPub}},
	}, nostr.WithLabel("bunker46wait"))

	uri := nostrConnectURI(clientPub, m.transportRelays, secret, m.perms, m.name, m.url)
	m.mu.Lock()
	p.uri = uri
	m.mu.Unlock()

	go m.awaitNostrConnect(sessionCtx, waitCtx, waitCancel, p, events, clientSecret, secret)
	return id, nil
}

func (m *Manager) connectAndFinish(sessionCtx context.Context, p *pendingLogin, client *nip46.BunkerClient, target, secret string) {
	rpcCtx, rpcCancel := context.WithDeadline(sessionCtx, p.expiresAt)
	defer rpcCancel()

	metadata, err := json.Marshal(clientMetadata{Name: m.name, URL: m.url})
	if err != nil {
		m.failPending(p.id, fmt.Errorf("marshaling client metadata: %w", err))
		return
	}

	if _, err := client.RPC(rpcCtx, "connect", []string{target, secret, m.perms, string(metadata)}); err != nil {
		m.failPending(p.id, fmt.Errorf("connect: %w", err))
		return
	}

	m.finishWithPublicKey(rpcCtx, p, client)
}

func (m *Manager) awaitNostrConnect(
	sessionCtx, waitCtx context.Context,
	waitCancel context.CancelFunc,
	p *pendingLogin,
	events chan nostr.RelayEvent,
	clientSecret, secret string,
) {
	defer waitCancel()
	for {
		select {
		case <-waitCtx.Done():
			m.failPending(p.id, fmt.Errorf("timed out waiting for signer"))
			return
		case ie, ok := <-events:
			if !ok || ie.Event == nil {
				m.failPending(p.id, fmt.Errorf("transport subscription closed"))
				return
			}

			resp, decrypted := decryptResponse(ie.Event, clientSecret)
			if !decrypted {
				continue
			}
			if resp.Result != secret {
				continue
			}

			remote := ie.PubKey
			client := nip46.NewBunker(sessionCtx, clientSecret, remote, m.transportRelays, m.pool, func(authURL string) {
				m.setAuthURL(p.id, authURL)
			})

			rpcCtx, rpcCancel := context.WithDeadline(sessionCtx, p.expiresAt)
			m.finishWithPublicKey(rpcCtx, p, client)
			rpcCancel()
			return
		}
	}
}

// finishWithPublicKey completes either flow: the user npub comes from
// get_public_key, which may differ from the remote signer's own key.
func (m *Manager) finishWithPublicKey(ctx context.Context, p *pendingLogin, client *nip46.BunkerClient) {
	userHex, err := client.GetPublicKey(ctx)
	if err != nil {
		m.failPending(p.id, fmt.Errorf("get_public_key: %w", err))
		return
	}
	userNpub, err := btknostr.HexToNpub(userHex)
	if err != nil {
		m.failPending(p.id, fmt.Errorf("encoding user npub: %w", err))
		return
	}
	if err := m.markReady(p.id, userHex, userNpub, client); err != nil {
		p.cancel()
	}
}

// decryptResponse decrypts a kind 24133 event addressed to the client,
// trying NIP-44 first and falling back to NIP-04, mirroring the fork's
// client dispatch.
func decryptResponse(evt *nostr.Event, clientSecret string) (nip46.Response, bool) {
	var resp nip46.Response

	plain := ""
	if conversationKey, err := nip44.GenerateConversationKey(evt.PubKey, clientSecret); err == nil {
		if decrypted, decErr := nip44.Decrypt(evt.Content, conversationKey); decErr == nil {
			plain = decrypted
		}
	}
	if plain == "" {
		sharedSecret, err := nip04.ComputeSharedSecret(evt.PubKey, clientSecret)
		if err != nil {
			return resp, false
		}
		decrypted, err := nip04.Decrypt(evt.Content, sharedSecret)
		if err != nil {
			return resp, false
		}
		plain = decrypted
	}

	if err := json.Unmarshal([]byte(plain), &resp); err != nil {
		return resp, false
	}
	return resp, true
}

func nostrConnectURI(clientPubHex string, relays []string, secret, perms, name, serviceURL string) string {
	q := url.Values{}
	for _, relay := range relays {
		q.Add("relay", relay)
	}
	q.Set("secret", secret)
	if perms != "" {
		q.Set("perms", perms)
	}
	if name != "" {
		q.Set("name", name)
	}
	if serviceURL != "" {
		q.Set("url", serviceURL)
	}
	u := url.URL{Scheme: "nostrconnect", Host: clientPubHex, RawQuery: q.Encode()}
	return u.String()
}
