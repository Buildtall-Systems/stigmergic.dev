package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"

	"github.com/buildtall-systems/buildtall/btk/auth/session"
	"github.com/buildtall-systems/buildtall/btk/views/login"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	vaultsrc "github.com/Buildtall-Systems/stigmergic.dev/internal/source/vault"
)

const (
	// aliceNpub is the shared test key alice's npub.
	aliceNpub = "npub1paydacp4r4njzewfxr0xjjs9g9gyw5jtr04qxv8ranpdrqk25ves7sp2vp"
	// testSessionMaxAge stands in for the session lifetime the configuration
	// defaults, which a config built in a test does not carry.
	testSessionMaxAge = "24h"
)

// testKeySigner signs as a key generated for the test.
type testKeySigner struct {
	sk string
}

func keySigner(t *testing.T) *testKeySigner {
	t.Helper()
	return &testKeySigner{sk: nostr.GeneratePrivateKey()}
}

func (k *testKeySigner) GetPublicKey(context.Context) (string, error) {
	return nostr.GetPublicKey(k.sk)
}

func (k *testKeySigner) SignEvent(_ context.Context, evt *nostr.Event) error {
	return evt.Sign(k.sk)
}

// serverWithPrivateVault starts a server whose loader gives the synthetic
// vault only to a read that carries a signer, the way an author-only relay
// answers, and reports every read the loader saw. No owner is configured,
// so the vault mounts only when its owner signs in.
func serverWithPrivateVault(t *testing.T) (*Server, *vaultsrc.Vault) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fixtureLocalDoc), []byte(fixtureLocalBody), 0o600); err != nil {
		t.Fatalf("writing the local document: %v", err)
	}
	src, err := source.NewFilesystem(dir, false, nil)
	if err != nil {
		t.Fatalf("NewFilesystem: %v", err)
	}

	v := syntheticVault(t)
	cfg := &config.Config{
		Port:             8080,
		Host:             testHost,
		Theme:            testThemeName,
		RecentFilesCount: 5,
	}
	cfg.Auth.SessionMaxAge = testSessionMaxAge

	srv := NewServerWithVaults(cfg, src, func(_ context.Context, owner string, signer nostr.Signer) ([]*vaultsrc.Vault, error) {
		if signer == nil || owner != v.Owner {
			return nil, ErrAuthRequired
		}
		return []*vaultsrc.Vault{v}, nil
	})
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			t.Errorf("failed to shut down server: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.WaitForIndexReady(ctx); err != nil {
		t.Fatalf("timed out waiting for the index: %v", err)
	}

	srv.observe(v.Owner, keySigner(t))
	waitForVaultMount(t, srv)

	return srv, v
}

// getRouteAs requests route as the session npub reader, or anonymously for
// an empty reader.
func getRouteAs(t *testing.T, srv *Server, route, reader string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, route, nil)
	if reader != "" {
		ctx := session.ContextWithPubkey(req.Context(), reader)
		ctx = session.ContextWithSessionID(ctx, "sid-"+reader)
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	return rec
}

func searchPathsAs(t *testing.T, srv *Server, query, reader string) []string {
	t.Helper()

	rec := getRouteAs(t, srv, "/api/search?q="+url.QueryEscape(query), reader)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from search, got %d", rec.Code)
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding the search response: %v", err)
	}
	paths := make([]string, 0, len(resp.Results))
	for _, m := range resp.Results {
		paths = append(paths, m.Path)
	}
	return paths
}

func filePathsAs(t *testing.T, srv *Server, reader string) []string {
	t.Helper()

	rec := getRouteAs(t, srv, "/api/files", reader)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from the files API, got %d", rec.Code)
	}
	var files []models.SearchableFile
	if err := json.Unmarshal(rec.Body.Bytes(), &files); err != nil {
		t.Fatalf("decoding the files response: %v", err)
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths
}

// TestPrivateVaultMountsForItsOwner is the read with a signer: the vault an
// author-only relay holds back from an anonymous read mounts, and it is
// marked private to the npub whose signer read it.
func TestPrivateVaultMountsForItsOwner(t *testing.T) {
	t.Parallel()

	srv, v := serverWithPrivateVault(t)

	m, ok := srv.mountAt(vaultMount(v.Owner, v.Name), v.Owner)
	if !ok {
		t.Fatal("the owner cannot reach their own vault mount")
	}
	if m.owner != v.Owner {
		t.Errorf("the vault read with a signer is owned by %q, want %q", m.owner, v.Owner)
	}

	rec := getRouteAs(t, srv, vaultRoute(v, fixtureConceptID+".md"), v.Owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for the owner, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "the vault's own words about coordination") {
		t.Error("the owner's page does not render the vault document")
	}
}

// TestPrivateVaultIsInvisibleToOtherReaders walks every way a reader reaches
// a document and holds that a private vault answers each of them, for any
// reader but its owner, exactly as a vault never mounted would.
func TestPrivateVaultIsInvisibleToOtherReaders(t *testing.T) {
	t.Parallel()

	srv, v := serverWithPrivateVault(t)
	doc := vaultRoute(v, fixtureConceptID+".md")
	mount := vaultMount(v.Owner, v.Name)

	for _, reader := range []string{"", aliceNpub} {
		if rec := getRouteAs(t, srv, doc, reader); rec.Code != http.StatusNotFound {
			t.Errorf("reader %q: expected 404 for the private document, got %d", reader, rec.Code)
		}
		if rec := getRouteAs(t, srv, "/partial/tree/?mount="+url.QueryEscape(mount), reader); rec.Code != http.StatusNotFound {
			t.Errorf("reader %q: expected 404 expanding the private vault, got %d", reader, rec.Code)
		}
		if slices.Contains(searchPathsAs(t, srv, "words", reader), doc) {
			t.Errorf("reader %q: search found the private document", reader)
		}
		if slices.Contains(filePathsAs(t, srv, reader), doc) {
			t.Errorf("reader %q: the files API listed the private document", reader)
		}
		if entries := srv.vaultEntries(reader); len(entries) != 0 {
			t.Errorf("reader %q: the panel lists the private vault: %v", reader, entries)
		}
		if rec := getRouteAs(t, srv, "/partial/sidebar", reader); strings.Contains(rec.Body.String(), mount) {
			t.Errorf("reader %q: the sidebar names the private vault", reader)
		}
	}

	if !slices.Contains(searchPathsAs(t, srv, "words", v.Owner), doc) {
		t.Error("search hides the private document from its owner")
	}
	if !slices.Contains(filePathsAs(t, srv, v.Owner), doc) {
		t.Error("the files API hides the private document from its owner")
	}
	if entries := srv.vaultEntries(v.Owner); len(entries) != 1 {
		t.Errorf("expected the owner's panel to list one vault, got %v", entries)
	}
}

// TestPublicDocumentNeverLinksIntoAPrivateVault holds the link boundary: a
// public document resolves only into public sources, whoever reads it, so
// neither its page nor the private document's backlinks join the two.
func TestPublicDocumentNeverLinksIntoAPrivateVault(t *testing.T) {
	t.Parallel()

	srv, v := serverWithPrivateVault(t)
	doc := vaultRoute(v, fixtureConceptID+".md")

	for _, reader := range []string{"", v.Owner} {
		rec := getRouteAs(t, srv, markdown.FileMount+fixtureLocalDoc, reader)
		if rec.Code != http.StatusOK {
			t.Fatalf("reader %q: expected status 200 for the local document, got %d", reader, rec.Code)
		}
		if strings.Contains(rec.Body.String(), `href="`+doc+`"`) {
			t.Errorf("reader %q: the public document links into the private vault", reader)
		}
	}

	backlinks, ok := srv.cachedBacklinks.Load().(models.BacklinkIndex)
	if !ok {
		t.Fatal("no backlink index is stored")
	}
	if entries := backlinks[doc]; len(entries) != 0 {
		t.Errorf("the private document holds backlinks its public sources never resolved: %v", entries)
	}
	if rec := getRouteAs(t, srv, doc, v.Owner); strings.Contains(rec.Body.String(), "Backlinks") {
		t.Error("the private document's page shows a backlinks block")
	}
}

// TestRouteSetResolvesByOwner pins the resolvers themselves: a private
// document sees the public sources and its owner's own, and no other
// owner's.
func TestRouteSetResolvesByOwner(t *testing.T) {
	t.Parallel()

	owner, other := npubFixture(t), npubFixture(t)
	mounts := []*mount{
		{prefix: markdown.FileMount},
		{prefix: vaultMount(owner, "notes"), owner: owner},
		{prefix: vaultMount(other, "notes"), owner: other},
	}
	entries := [][]markdown.RouteEntry{
		{{Path: "public.md", Route: markdown.FileMount + "public.md"}},
		{{Path: "mine.md", Route: vaultMount(owner, "notes") + "mine.md"}},
		{{Path: "theirs.md", Route: vaultMount(other, "notes") + "theirs.md"}},
	}

	routes := newRouteSet(mounts, entries)

	resolves := func(r *markdown.TreeResolver, target string) bool {
		_, ok := r.ResolveRoute(target)
		return ok
	}

	if !resolves(routes.forOwner(owner), "public") || !resolves(routes.forOwner(owner), "mine") {
		t.Error("the owner's resolver misses the public or its own documents")
	}
	if resolves(routes.forOwner(owner), "theirs") {
		t.Error("the owner's resolver reaches another owner's private document")
	}
	if resolves(routes.public, "mine") || resolves(routes.public, "theirs") {
		t.Error("the public resolver reaches a private document")
	}
	if routes.forOwner("") != routes.public {
		t.Error("a public document does not resolve through the public resolver")
	}
}

// TestUnansweredChallengeRetriesWithTheNextRequest is the deferral: a relay
// that challenged and got no signature leaves the owner free to queue again,
// so a request with a page now open to sign retries the read.
func TestUnansweredChallengeRetriesWithTheNextRequest(t *testing.T) {
	t.Parallel()

	var reads atomic.Int32
	srv := &Server{
		ctx:      t.Context(),
		owners:   make(chan ownerRequest, 1),
		observed: map[string]bool{testOwnerNpub: true},
		signers:  map[string]*latestSigner{},
		loadVaults: func(context.Context, string, nostr.Signer) ([]*vaultsrc.Vault, error) {
			reads.Add(1)
			return nil, ErrAuthRequired
		},
	}

	srv.mountOwner(ownerRequest{npub: testOwnerNpub, signer: keySigner(t)})
	if srv.observed[testOwnerNpub] {
		t.Error("an owner whose challenge went unanswered stays recorded as observed")
	}

	srv.observed[testOwnerNpub] = true
	srv.mountOwner(ownerRequest{npub: testOwnerNpub})
	if !srv.observed[testOwnerNpub] {
		t.Error("a read without a signer forgot the owner, so every request would queue it again")
	}
	if got := reads.Load(); got != 2 {
		t.Errorf("expected two reads, got %d", got)
	}
}

// TestUnansweredChallengeStillMountsWhatWasRead holds that one relay's
// challenge costs only that relay's events: the vaults the other relays gave
// mount, public, beside the error.
func TestUnansweredChallengeStillMountsWhatWasRead(t *testing.T) {
	t.Parallel()

	v := syntheticVault(t)
	srv := newTestServer(t, &config.Config{Port: 8080, Host: testHost, Theme: testThemeName})
	srv.loadVaults = func(context.Context, string, nostr.Signer) ([]*vaultsrc.Vault, error) {
		return []*vaultsrc.Vault{v}, ErrAuthRequired
	}

	srv.mountOwner(ownerRequest{npub: v.Owner})

	m, ok := srv.mountAt(vaultMount(v.Owner, v.Name), "")
	if !ok {
		t.Fatal("the vault read beside the challenge did not mount")
	}
	if m.owner != "" {
		t.Errorf("a vault read without a signer is private to %q", m.owner)
	}
}

// TestLatestSignerSignsThroughTheNewestSigner holds the holder's purpose: a
// challenge long after the read signs through the reader's current login.
func TestLatestSignerSignsThroughTheNewestSigner(t *testing.T) {
	t.Parallel()

	holder := &latestSigner{}
	evt := &nostr.Event{Kind: nostr.KindClientAuthentication, CreatedAt: nostr.Now()}
	if err := holder.SignEvent(t.Context(), evt); !errors.Is(err, errNoSigner) {
		t.Errorf("an empty holder signed, or failed with %v", err)
	}

	first, second := keySigner(t), keySigner(t)
	holder.set(first)
	holder.set(second)

	if err := holder.SignEvent(t.Context(), evt); err != nil {
		t.Fatalf("SignEvent: %v", err)
	}
	want, err := second.GetPublicKey(t.Context())
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	if evt.PubKey != want {
		t.Error("the holder signed with a signer older than the newest")
	}
}

// TestObserveSessionOffersTheReadersSigner is the request side: a signed-in
// request queues its npub with a signer that acts as it, and an anonymous
// request queues nothing.
func TestObserveSessionOffersTheReadersSigner(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &config.Config{Port: 8080, Host: testHost, Theme: testThemeName})
	srv.loadVaults = func(context.Context, string, nostr.Signer) ([]*vaultsrc.Vault, error) { return nil, nil }
	if srv.site == nil {
		mounted, err := mountLogin(http.NewServeMux(), srv.config, mustSessions(t), srv.theme, srv.themes)
		if err != nil {
			t.Fatalf("mountLogin: %v", err)
		}
		t.Cleanup(mounted.Close)
		srv.site = mounted
	}

	anonymous := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	srv.observeSession(anonymous)
	if len(srv.observed) != 0 {
		t.Fatal("an anonymous request queued an owner")
	}

	ctx := session.ContextWithPubkey(t.Context(), aliceNpub)
	ctx = session.ContextWithSessionID(ctx, "sid-alice")
	srv.observeSession(httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil))

	if !srv.observed[aliceNpub] {
		t.Fatal("the signed-in reader's npub was not queued")
	}
	if holder := srv.signers[aliceNpub]; holder == nil || holder.current() == nil {
		t.Error("the queued npub carries no signer")
	}
}

func mustSessions(t *testing.T) *session.Manager {
	t.Helper()
	sm, err := session.NewManager(sessionCookie, "", testSessionMaxAge)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return sm
}

// TestWhitelistTakesNpubsAndHex holds the configured whitelist's two forms:
// an npub stands, a hex key becomes its npub, and anything else is refused.
func TestWhitelistTakesNpubsAndHex(t *testing.T) {
	t.Parallel()

	_, hexValue, err := nip19.Decode(aliceNpub)
	if err != nil {
		t.Fatalf("decoding alice: %v", err)
	}
	hexKey, ok := hexValue.(string)
	if !ok {
		t.Fatal("alice's npub did not decode to a hex key")
	}

	got, err := normalizeWhitelist([]string{aliceNpub, hexKey})
	if err != nil {
		t.Fatalf("normalizeWhitelist: %v", err)
	}
	if !slices.Equal(got, []string{aliceNpub, aliceNpub}) {
		t.Errorf("expected both entries as alice's npub, got %v", got)
	}

	for _, bad := range []string{"npub1notavalidkey", strings.Repeat("z", hexKeyLength), "alice"} {
		if _, err := normalizeWhitelist([]string{bad}); err == nil {
			t.Errorf("the whitelist accepted %q", bad)
		}
	}
}

// TestWhitelistAdmitsOnlyItsNpubs holds the login's refusal, and that an
// empty whitelist admits no one.
func TestWhitelistAdmitsOnlyItsNpubs(t *testing.T) {
	t.Parallel()

	admit := whitelistAdmit([]string{aliceNpub})
	if err := admit(t.Context(), aliceNpub); err != nil {
		t.Errorf("the whitelist refused its own npub: %v", err)
	}
	if err := admit(t.Context(), npubFixture(t)); !errors.Is(err, errNotWhitelisted) {
		t.Errorf("the whitelist admitted another npub, or failed with %v", err)
	}
	if err := whitelistAdmit(nil)(t.Context(), aliceNpub); err == nil {
		t.Error("an empty whitelist admitted an npub")
	}
}

// TestAuthGateSendsAnonymousReadersToTheLogin is the gate end to end: with
// auth on, an anonymous page request lands on the btk login, and the login
// page and its scripts answer without a session.
func TestAuthGateSendsAnonymousReadersToTheLogin(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Port: 8080, Host: testHost, Theme: testThemeName}
	cfg.Auth.Enabled = true
	cfg.Auth.SessionMaxAge = testSessionMaxAge
	cfg.Auth.AllowedNpubs = []string{aliceNpub}
	srv := newTestServer(t, cfg)

	serve := func(route string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, route, nil))
		return rec
	}

	rec := serve(markdown.FileMount + "notes.md")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 for an anonymous page request, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, login.LoginPath+"?") {
		t.Errorf("the gate sent the reader to %q, not the login", loc)
	}

	if rec := serve(login.LoginPath); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), login.DefaultPaths().Start) {
		t.Errorf("the login page did not render the btk panel: status %d", rec.Code)
	}
	if rec := serve("/static/btk/js/nostr.js"); rec.Code != http.StatusOK {
		t.Errorf("expected the btk script to load without a session, got %d", rec.Code)
	}
}
