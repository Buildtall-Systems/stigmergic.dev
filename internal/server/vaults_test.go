package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"

	"github.com/buildtall-systems/buildtall/btk/lists"
	"github.com/buildtall-systems/buildtall/btk/okf"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	vaultsrc "github.com/Buildtall-Systems/stigmergic.dev/internal/source/vault"
)

const (
	fixtureVaultName = "notes"
	fixtureConceptID = "thoughts/vault-note"
	fixtureBody      = "# Vault note\n\nthe vault's own words about coordination\n"
	fixtureLocalDoc  = "local.md"
	fixtureLocalBody = "# Local\n\nlocal words linking [[vault-note]] across the corpus\n"
)

func fixturePublishTime() time.Time { return time.Unix(1800000000, 0).UTC() }

// syntheticVault publishes one document onto an empty wire and assembles the
// Vault a fetch would have produced. The key pair is derived at run time and
// its npub encoded by the library, so no fixture carries an identifier no key
// ever signed.
func syntheticVault(t *testing.T) *vaultsrc.Vault {
	t.Helper()

	pub, err := nostr.GetPublicKey(nostr.GeneratePrivateKey())
	if err != nil {
		t.Fatalf("deriving the vault owner: %v", err)
	}
	owner, err := nip19.EncodePublicKey(pub)
	if err != nil {
		t.Fatalf("encoding the vault owner: %v", err)
	}

	domain, err := lists.VaultDomain(fixtureVaultName)
	if err != nil {
		t.Fatalf("VaultDomain: %v", err)
	}

	bundle, err := okf.NewBundle(domain.RootDTag)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	dir, err := bundle.Dir("thoughts")
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if addErr := dir.AddConcept(&okf.Concept{
		ConceptID:   fixtureConceptID,
		Body:        fixtureBody,
		Frontmatter: okf.Frontmatter{Type: "Document"},
	}); addErr != nil {
		t.Fatalf("AddConcept: %v", addErr)
	}

	plan, err := okf.ReconcileVault(domain, owner, bundle, okf.VaultEvents{}, okf.PublishOptions{Now: fixturePublishTime()})
	if err != nil {
		t.Fatalf("ReconcileVault: %v", err)
	}

	events := okf.VaultEvents{Sets: map[string]*nostr.Event{}, Documents: map[string]*nostr.Event{}}
	for _, ev := range plan.Events {
		switch ev.Kind {
		case lists.KindListSet:
			events.Root = ev
		case lists.KindCurationSet:
			events.Sets[lists.GetDTag(ev)] = ev
		case lists.KindLongFormNote:
			events.Documents[lists.FormatCoordinate(lists.KindLongFormNote, ev.PubKey, lists.GetDTag(ev))] = ev
		default:
			t.Fatalf("the plan emitted kind %d, which no vault holds", ev.Kind)
		}
	}

	exported, err := okf.ExportVault(domain, owner, events, logger.Log)
	if err != nil {
		t.Fatalf("ExportVault: %v", err)
	}

	return &vaultsrc.Vault{
		Descriptor: vaultsrc.Descriptor{Owner: owner, Name: fixtureVaultName},
		Domain:     domain,
		Events:     events,
		Bundle:     exported,
		Resolver:   okf.NewResolver(domain, events.Sets, ""),
	}
}

// serverWithVault starts a server over a local tree and one synthetic vault,
// and waits until both are indexed.
func serverWithVault(t *testing.T) (*Server, *vaultsrc.Vault) {
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
	cfg.Vault.Npubs = []string{v.Owner}

	srv := NewServerWithVaults(cfg, src, func(context.Context, string) ([]*vaultsrc.Vault, error) {
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
	waitForVaultMount(t, srv)

	return srv, v
}

// waitForVaultMount blocks until discovery has mounted the vault, which runs
// on its own goroutine so the server answers while relays are still talking.
func waitForVaultMount(t *testing.T, srv *Server) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.mountList()) > 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the vault to mount")
}

func vaultRoute(v *vaultsrc.Vault, rel string) string {
	return vaultMount(v.Owner, v.Name) + rel
}

func getRoute(t *testing.T, srv *Server, route string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, route, nil))
	return rec
}

// TestVaultDocumentRendersFromItsOwnMount is the integration walk: a document
// fetched from relays browses like a local one, at its own route.
func TestVaultDocumentRendersFromItsOwnMount(t *testing.T) {
	t.Parallel()

	srv, v := serverWithVault(t)

	rec := getRoute(t, srv, vaultRoute(v, fixtureConceptID+".md"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from the vault mount, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<p>the vault's own words about coordination</p>") {
		t.Errorf("the vault document did not render its own body: %s", body)
	}
	if !strings.Contains(body, fixtureVaultName) {
		t.Error("the page does not name the source it came from")
	}
}

// TestVaultDirectoryListsIntoItsOwnMount pins the routes a listing writes: a
// directory read from a vault links into that vault, never into the local
// tree that happens to be mounted beside it.
func TestVaultDirectoryListsIntoItsOwnMount(t *testing.T) {
	t.Parallel()

	srv, v := serverWithVault(t)

	rec := getRoute(t, srv, vaultRoute(v, "thoughts"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for the vault directory, got %d", rec.Code)
	}
	if want := vaultRoute(v, fixtureConceptID+".md"); !strings.Contains(rec.Body.String(), want) {
		t.Errorf("the listing lacks %s\nbody: %s", want, rec.Body.String())
	}
}

// TestSearchSpansEverySourceAndSaysWhere is ruling 8's window: one corpus,
// and every result carrying the name of the source holding it.
func TestSearchSpansEverySourceAndSaysWhere(t *testing.T) {
	t.Parallel()

	srv, v := serverWithVault(t)

	rec := getRoute(t, srv, "/api/search?q=words")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from search, got %d", rec.Code)
	}

	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding the search response: %v", err)
	}

	found := map[string]string{}
	for _, m := range resp.Results {
		found[m.Path] = m.Source
	}

	localRoute := markdown.FileMount + fixtureLocalDoc
	if _, ok := found[localRoute]; !ok {
		t.Errorf("search missed the local document: %+v", resp.Results)
	}
	vaultDoc := vaultRoute(v, fixtureConceptID+".md")
	if _, ok := found[vaultDoc]; !ok {
		t.Errorf("search missed the vault document: %+v", resp.Results)
	}
	if got := found[vaultDoc]; got != fixtureVaultName {
		t.Errorf("the vault result names source %q, want %q", got, fixtureVaultName)
	}
	if got := found[localRoute]; got == fixtureVaultName {
		t.Error("the local result claims the vault as its source")
	}
}

// TestBacklinksCrossMounts is the other half of one corpus: a local document
// naming a vault document shows up on that document's page, because index and
// render resolve the same way.
func TestBacklinksCrossMounts(t *testing.T) {
	t.Parallel()

	srv, v := serverWithVault(t)

	rec := getRoute(t, srv, vaultRoute(v, fixtureConceptID+".md"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from the vault mount, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Backlinks") {
		t.Fatalf("the vault document shows no backlinks block: %s", body)
	}
	if want := `href="` + markdown.FileMount + fixtureLocalDoc + `"`; !strings.Contains(body, want) {
		t.Errorf("the backlink does not point at the local document (%s)\nbody: %s", want, body)
	}
}

// TestLocalDocumentLinksIntoTheVault is the render side of the same claim: the
// link the backlink index recorded is a link the reader can follow.
func TestLocalDocumentLinksIntoTheVault(t *testing.T) {
	t.Parallel()

	srv, v := serverWithVault(t)

	rec := getRoute(t, srv, markdown.FileMount+fixtureLocalDoc)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for the local document, got %d", rec.Code)
	}
	if want := `href="` + vaultRoute(v, fixtureConceptID+".md") + `"`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("the local document does not link into the vault (%s)\nbody: %s", want, rec.Body.String())
	}
}

// TestSidebarShowsTheLocalTreeAlone holds Phase 4's boundary: the vault is
// reachable by route and searchable, and the panel that would show it is the
// next phase's work.
func TestSidebarShowsTheLocalTreeAlone(t *testing.T) {
	t.Parallel()

	srv, v := serverWithVault(t)

	rec := getRoute(t, srv, "/partial/sidebar")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for the sidebar, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), vaultMount(v.Owner, v.Name)) {
		t.Error("the sidebar shows a vault route, which no panel offers yet")
	}
}

// TestObserveQueuesAnOwnerOnce pins the dedupe: a reader browsing pays for
// discovery on their first request and never again.
func TestObserveQueuesAnOwnerOnce(t *testing.T) {
	t.Parallel()

	srv := &Server{
		owners:   make(chan string, 2),
		observed: map[string]bool{},
	}

	srv.observe(testOwnerNpub)
	srv.observe(testOwnerNpub)

	if got := len(srv.owners); got != 1 {
		t.Errorf("queued %d times, want 1", got)
	}
}

// TestObserveLeavesAFullQueueUnrecorded is why the npub is recorded only
// once it is queued: a burst of sign-ins must cost a deferred discovery, not
// a reader whose vaults never load.
func TestObserveLeavesAFullQueueUnrecorded(t *testing.T) {
	t.Parallel()

	srv := &Server{
		owners:   make(chan string, 1),
		observed: map[string]bool{},
	}

	srv.observe(testOwnerNpub)
	srv.observe("npub1other")

	if srv.observed["npub1other"] {
		t.Error("an owner that did not fit the queue was recorded as observed")
	}

	<-srv.owners
	srv.observe("npub1other")
	if !srv.observed["npub1other"] {
		t.Error("the next request did not queue the deferred owner")
	}
}
