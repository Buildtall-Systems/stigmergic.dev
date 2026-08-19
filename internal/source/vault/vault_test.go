package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
	"github.com/buildtall-systems/buildtall/btk/okf"
)

const (
	testVaultName = "notes"
	dirThoughts   = "thoughts"
	cidFirst      = "thoughts/first-note"
	cidSecond     = "thoughts/second-note"
	cidGuide      = "guide"
	nameArt       = "art.png"
	nameCover     = "cover.png"
	docType       = "Document"
)

// bodyFirst carries one link of every class the adapter answers: a basename
// document hit, the same with a fragment, a doc-relative attachment, a
// dangling target, and one standalone note embed.
const bodyFirst = `see [[second-note]] and [[second-note#history]] and [[art.png]] and [[nope]]

![[second-note]]
`

func testPublishTime() time.Time { return time.Unix(1800000000, 0).UTC() }

func artBytes() []byte   { return []byte("art-png-bytes") }
func coverBytes() []byte { return []byte("cover-bytes") }

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// testOwner derives a real key pair at run time and encodes its npub with
// the library, so no fixture carries a fabricated identifier.
func testOwner(t *testing.T) string {
	t.Helper()
	pub, err := nostr.GetPublicKey(nostr.GeneratePrivateKey())
	if err != nil {
		t.Fatalf("deriving the test owner: %v", err)
	}
	npub, err := nip19.EncodePublicKey(pub)
	if err != nil {
		t.Fatalf("encoding the test owner: %v", err)
	}
	return npub
}

func testDomain(t *testing.T) lists.Domain {
	t.Helper()
	domain, err := lists.VaultDomain(testVaultName)
	if err != nil {
		t.Fatalf("VaultDomain(%q): %v", testVaultName, err)
	}
	return domain
}

func mustConcept(t *testing.T, b *okf.Bundle, dir, cid, body string) {
	t.Helper()
	d, err := b.Dir(dir)
	if err != nil {
		t.Fatalf("Dir(%q): %v", dir, err)
	}
	c := &okf.Concept{ConceptID: cid, Body: body, Frontmatter: okf.Frontmatter{Type: docType}}
	if err := d.AddConcept(c); err != nil {
		t.Fatalf("AddConcept(%q): %v", cid, err)
	}
}

// testBundle builds the fixture tree: two documents under thoughts/ with an
// attachment beside them, one root document, and one root attachment.
func testBundle(t *testing.T, domain lists.Domain) *okf.Bundle {
	t.Helper()
	b, err := okf.NewBundle(domain.RootDTag)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	mustConcept(t, b, dirThoughts, cidFirst, bodyFirst)
	mustConcept(t, b, dirThoughts, cidSecond, "the second body\n\n## history\n\nolder words\n")
	mustConcept(t, b, "", cidGuide, "the guide body\n")
	d, err := b.Dir(dirThoughts)
	if err != nil {
		t.Fatalf("Dir(%q): %v", dirThoughts, err)
	}
	if err := d.AddAttachment(okf.Attachment{Name: nameArt, SHA256: hashOf(artBytes())}); err != nil {
		t.Fatalf("AddAttachment(%q): %v", nameArt, err)
	}
	if err := b.Root.AddAttachment(okf.Attachment{Name: nameCover, SHA256: hashOf(coverBytes())}); err != nil {
		t.Fatalf("AddAttachment(%q): %v", nameCover, err)
	}
	return b
}

// testVault publishes the fixture bundle onto an empty wire and assembles
// the Vault a Load would have produced, with base as its one store.
func testVault(t *testing.T, base string) (*Vault, string) {
	t.Helper()
	owner := testOwner(t)
	domain := testDomain(t)
	b := testBundle(t, domain)

	plan, err := okf.ReconcileVault(domain, owner, b, okf.VaultEvents{}, okf.PublishOptions{Now: testPublishTime()})
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
			coord := lists.FormatCoordinate(lists.KindLongFormNote, ev.PubKey, lists.GetDTag(ev))
			events.Documents[coord] = ev
		default:
			t.Fatalf("the plan emitted kind %d, which no vault holds", ev.Kind)
		}
	}
	if events.Root == nil {
		t.Fatal("the plan emitted no root event")
	}

	bundle, err := okf.ExportVault(domain, owner, events, discardLogger())
	if err != nil {
		t.Fatalf("ExportVault: %v", err)
	}

	servers := []string{}
	if base != "" {
		servers = []string{base}
	}
	return &Vault{
		Descriptor: Descriptor{Owner: owner, Name: testVaultName},
		Domain:     domain,
		Events:     events,
		Bundle:     bundle,
		Resolver:   okf.NewResolver(domain, events.Sets, base),
		Servers:    servers,
	}, owner
}

// testStore serves the fixture blobs by bare hash, the way a Blossom store
// answers BUD-01 GETs.
func testStore(t *testing.T) *httptest.Server {
	t.Helper()
	blobs := map[string][]byte{
		"/" + hashOf(artBytes()):   artBytes(),
		"/" + hashOf(coverBytes()): coverBytes(),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := blobs[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write(data); err != nil {
			t.Errorf("writing the blob: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mustHexOf(t *testing.T, npub string) string {
	t.Helper()
	hexKey, err := btknostr.NpubToHex(npub)
	if err != nil {
		t.Fatalf("NpubToHex(%q): %v", npub, err)
	}
	return hexKey
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
