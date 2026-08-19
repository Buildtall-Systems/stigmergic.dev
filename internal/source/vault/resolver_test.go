package vault

import (
	"context"
	"strings"
	"testing"

	"go.abhg.dev/goldmark/wikilink"

	"github.com/buildtall-systems/buildtall/btk/lists"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
)

func testMount(owner string) string {
	return "/vault/" + owner + "/" + testVaultName
}

func testAdapter(t *testing.T, base string) (*Resolver, *Vault, string) {
	t.Helper()
	v, owner := testVault(t, base)
	dTag, err := v.DocDTag(cidFirst + conceptExt)
	if err != nil {
		t.Fatalf("DocDTag: %v", err)
	}
	return NewResolver(v, testMount(owner), dTag), v, owner
}

func resolve(t *testing.T, r *Resolver, target, fragment string) string {
	t.Helper()
	dest, err := r.ResolveWikilink(&wikilink.Node{Target: []byte(target), Fragment: []byte(fragment)})
	if err != nil {
		t.Fatalf("ResolveWikilink(%q): %v", target, err)
	}
	return string(dest)
}

func TestAdapterResolvesByBasename(t *testing.T) {
	r, _, owner := testAdapter(t, "")
	if got, want := resolve(t, r, "second-note", ""), testMount(owner)+"/"+cidSecond+conceptExt; got != want {
		t.Errorf("destination = %q, want %q", got, want)
	}
}

func TestAdapterKeepsTheFragment(t *testing.T) {
	r, _, owner := testAdapter(t, "")
	if got, want := resolve(t, r, "second-note", "history"), testMount(owner)+"/"+cidSecond+conceptExt+"#history"; got != want {
		t.Errorf("destination = %q, want %q", got, want)
	}
}

func TestAdapterResolvesTheMdCandidate(t *testing.T) {
	r, _, owner := testAdapter(t, "")
	if got, want := resolve(t, r, cidSecond+conceptExt, ""), testMount(owner)+"/"+cidSecond+conceptExt; got != want {
		t.Errorf("destination = %q, want %q", got, want)
	}
}

func TestAdapterResolvesFromTheRoot(t *testing.T) {
	r, _, owner := testAdapter(t, "")
	if got, want := resolve(t, r, cidGuide, ""), testMount(owner)+"/"+cidGuide+conceptExt; got != want {
		t.Errorf("destination = %q, want %q", got, want)
	}
}

func TestAdapterRoutesAnAttachmentIntoTheMount(t *testing.T) {
	r, _, owner := testAdapter(t, "")
	if got, want := resolve(t, r, nameArt, ""), testMount(owner)+"/"+dirThoughts+"/"+nameArt; got != want {
		t.Errorf("destination = %q, want %q: bytes serve through the filesystem", got, want)
	}
}

func TestAdapterReportsDanglingAsNil(t *testing.T) {
	if got := resolve(t, testAdapterOnly(t), "nope", ""); got != "" {
		t.Errorf("destination = %q, want nil for a dangling target", got)
	}
}

func testAdapterOnly(t *testing.T) *Resolver {
	t.Helper()
	r, _, _ := testAdapter(t, "")
	return r
}

func TestEmbedSourceProbesAssetsIntoTheMount(t *testing.T) {
	r, v, owner := testAdapter(t, "")
	fsys, err := NewFS(context.Background(), v.Bundle, v.Name, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	es := NewEmbedSource(fsys, r)

	route, ok := es.ProbeAsset(nameArt)
	if !ok {
		t.Fatal("the stated attachment did not probe")
	}
	if want := testMount(owner) + "/" + dirThoughts + "/" + nameArt; route != want {
		t.Errorf("route = %q, want %q", route, want)
	}
	if _, ok := es.ProbeAsset("missing.png"); ok {
		t.Error("a missing asset probed, want a miss")
	}
}

func TestEmbedSourceReadsNotesThroughTheFS(t *testing.T) {
	r, v, owner := testAdapter(t, "")
	fsys, err := NewFS(context.Background(), v.Bundle, v.Name, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	es := NewEmbedSource(fsys, r)

	for _, route := range []string{
		cidSecond + conceptExt,
		testMount(owner) + "/" + cidSecond + conceptExt,
	} {
		data, ok := es.NoteSource(route)
		if !ok {
			t.Fatalf("NoteSource(%q) missed", route)
		}
		if !strings.Contains(string(data), "the second body") {
			t.Errorf("NoteSource(%q) = %q, want the note's body", route, data)
		}
	}
	if _, ok := es.NoteSource("thoughts/nope.md"); ok {
		t.Error("a missing note read, want a miss")
	}
}

// TestBundleToRenderWalk is the integration walk: synthetic vault events in,
// the first document's bytes read through the filesystem and rendered with
// the adapter. Wikilink hrefs land inside the mount, a dangling link renders
// visibly, and the standalone note embed renders the unresolved marker for
// now: the embed renderer still strips a literal /file/ prefix, the seam the
// multi-source server phase generalizes, at which point this assertion flips
// to transcluded content.
func TestBundleToRenderWalk(t *testing.T) {
	r, v, owner := testAdapter(t, "")
	fsys, err := NewFS(context.Background(), v.Bundle, v.Name, v.docModTimes(), nil, nil)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}

	source, ok := NewEmbedSource(fsys, r).NoteSource(cidFirst + conceptExt)
	if !ok {
		t.Fatal("the first document did not read through the filesystem")
	}

	html, _, err := markdown.Parse(source, r, markdown.NewEmbedContext(NewEmbedSource(fsys, r)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	page := string(html)

	mount := testMount(owner)
	for _, want := range []string{
		`href="` + mount + "/" + cidSecond + conceptExt + `"`,
		`href="` + mount + "/" + cidSecond + conceptExt + `#history"`,
		`href="` + mount + "/" + dirThoughts + "/" + nameArt + `"`,
		`class="wikilink-unresolved"`,
		`data-embed-error="unresolved"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the rendered page lacks %s\npage: %s", want, page)
		}
	}
}

// TestVaultDomainMatchesTheFixture pins the fixture to the real family
// grammar: the bundle name is the domain's root d-tag.
func TestVaultDomainMatchesTheFixture(t *testing.T) {
	domain := testDomain(t)
	if domain.RootDTag != "vault-"+testVaultName {
		t.Errorf("root d-tag = %q, want the family form", domain.RootDTag)
	}
	if domain.LeafKind != lists.KindCurationSet {
		t.Errorf("leaf kind = %d, want %d", domain.LeafKind, lists.KindCurationSet)
	}
}
