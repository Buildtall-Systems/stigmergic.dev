package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
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

// TestVaultRowExpandsThroughTheTreePartial walks what a click on a vault row
// does. The row ships one empty container naming the vault's root and its
// mount; the client asks the tree partial for that mount's root, and the
// answer is the vault's own top level, every row linking back into the same
// mount rather than into the local tree beside it.
func TestVaultRowExpandsThroughTheTreePartial(t *testing.T) {
	t.Parallel()

	srv, v := serverWithVault(t)
	mount := vaultMount(v.Owner, v.Name)

	rec := getRoute(t, srv, "/partial/tree/?mount="+url.QueryEscape(mount))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 expanding the vault root, got %d", rec.Code)
	}

	html := rec.Body.String()
	if !strings.Contains(html, `data-children-path="thoughts" data-mount="`+mount+`"`) {
		t.Errorf("the vault's top level does not carry its own mount\nbody: %s", html)
	}
	if strings.Contains(html, `data-mount="`+markdown.FileMount+`"`) {
		t.Error("a vault's rows must not point at the local tree's mount")
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

// TestSidebarShowsBothHalves is the panel end to end: the local tree above
// and the mounted vault below, each expandable through its own mount.
func TestSidebarShowsBothHalves(t *testing.T) {
	t.Parallel()

	srv, v := serverWithVault(t)

	rec := getRoute(t, srv, "/partial/sidebar")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for the sidebar, got %d", rec.Code)
	}

	html := rec.Body.String()
	mount := vaultMount(v.Owner, v.Name)
	if !strings.Contains(html, `aria-label="File tree"`) {
		t.Error("expected the local tree to keep its landmark")
	}
	if !strings.Contains(html, `aria-label="Vaults"`) {
		t.Error("expected the vault panel to render its own landmark")
	}
	if strings.Index(html, `aria-label="Vaults"`) < strings.Index(html, `aria-label="File tree"`) {
		t.Error("the vault panel belongs below the local tree, not above it")
	}
	if !strings.Contains(html, `data-children-path="." data-mount="`+mount+`" data-loaded="false"`) {
		t.Errorf("expected a collapsed container pointed at %s", mount)
	}
	if !strings.Contains(html, `title="`+v.Owner+`"`) {
		t.Error("expected the whole npub in the row's title attribute")
	}
	if !strings.Contains(html, ">"+v.Name+"<") {
		t.Errorf("expected the vault to be labeled %q", v.Name)
	}
	if !strings.Contains(html, ">"+models.ShortNpub(v.Owner)+"<") {
		t.Error("expected the shortened npub beside the vault name")
	}
	if strings.Contains(html, ">"+v.Owner+"<") {
		t.Error("the whole npub belongs in the title attribute, not in the label")
	}
}

// TestSidebarWithoutVaultsIsTodaysPanel holds the promise the split makes to
// a reader who configured no vaults: the lower half is absent rather than
// empty, and the upper half renders exactly as it did before.
func TestSidebarWithoutVaultsIsTodaysPanel(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fixtureLocalDoc), []byte(fixtureLocalBody), 0o600); err != nil {
		t.Fatalf("writing the local document: %v", err)
	}
	src, err := source.NewFilesystem(dir, false, nil)
	if err != nil {
		t.Fatalf("NewFilesystem: %v", err)
	}
	srv := newServerWithSource(t, src)

	rec := getRoute(t, srv, "/partial/sidebar")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for the sidebar, got %d", rec.Code)
	}

	html := rec.Body.String()
	if !strings.Contains(html, `aria-label="File tree"`) {
		t.Error("expected the local tree to render")
	}
	if strings.Contains(html, `aria-label="Vaults"`) {
		t.Error("a server with no vault mounted must render no vault panel")
	}
	if strings.Contains(html, "/vault/") {
		t.Error("a server with no vault mounted must name no vault route")
	}
}

// npubFixture derives one real npub. No identifier in these tests is written
// by hand: a bech32 string no key ever produced would satisfy a comparison
// here and fail everything that decodes it.
func npubFixture(t *testing.T) string {
	t.Helper()

	pub, err := nostr.GetPublicKey(nostr.GeneratePrivateKey())
	if err != nil {
		t.Fatalf("deriving a vault owner: %v", err)
	}
	npub, err := nip19.EncodePublicKey(pub)
	if err != nil {
		t.Fatalf("encoding a vault owner: %v", err)
	}
	return npub
}

// vaultDescribed is a vault reduced to what the panel reads off it.
func vaultDescribed(name, owner string) *vaultsrc.Vault {
	return &vaultsrc.Vault{Descriptor: vaultsrc.Descriptor{Name: name, Owner: owner}}
}

// TestVaultEntriesSortByNameThenOwner pins the panel's order against
// discovery's: relays answer in whatever order they answer, and the rows
// must not rearrange as they do. The two owners are real npubs put in the
// order they sort, so the expectation names an order rather than assuming
// one of two random keys wins.
func TestVaultEntriesSortByNameThenOwner(t *testing.T) {
	t.Parallel()

	first, second := npubFixture(t), npubFixture(t)
	if first > second {
		first, second = second, first
	}

	srv := &Server{mounts: []*mount{
		{prefix: markdown.FileMount},
		{vault: vaultDescribed("notes", second), prefix: vaultMount(second, "notes")},
		{vault: vaultDescribed("archive", second), prefix: vaultMount(second, "archive")},
		{vault: vaultDescribed("notes", first), prefix: vaultMount(first, "notes")},
	}}

	entries := srv.vaultEntries()
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Name+" "+e.Owner)
	}
	want := []string{"archive " + second, "notes " + first, "notes " + second}
	if !slices.Equal(got, want) {
		t.Errorf("expected the rows in order %v, got %v", want, got)
	}
}

// TestVaultEntriesLeaveThePrimaryOut holds the panel's boundary: it lists
// fetched vaults, and the local tree is the half above it.
func TestVaultEntriesLeaveThePrimaryOut(t *testing.T) {
	t.Parallel()

	srv := &Server{mounts: []*mount{{prefix: markdown.FileMount}}}

	if entries := srv.vaultEntries(); len(entries) != 0 {
		t.Errorf("expected no vault rows for a server with only its primary source, got %v", entries)
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
