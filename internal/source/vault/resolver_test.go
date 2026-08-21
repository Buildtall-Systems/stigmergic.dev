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

func TestEmbedSourceProbesAssetsInsideTheVault(t *testing.T) {
	r, v, _ := testAdapter(t, "")
	fsys, err := NewFS(context.Background(), v.Bundle, v.Name, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	es := NewEmbedSource(fsys, r)

	route, ok := es.ProbeAsset(nameArt)
	if !ok {
		t.Fatal("the stated attachment did not probe")
	}
	if want := dirThoughts + "/" + nameArt; route != want {
		t.Errorf("route = %q, want %q: the seam speaks paths inside the vault, and the renderer prefixes the mount", route, want)
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
// visibly, and the standalone note embed transcludes the note it names,
// because the embed renderer now strips the mount the page is served under
// rather than a literal /file/.
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

	html, _, err := markdown.Parse(source, r, markdown.NewEmbedContext(mountPrefix(owner), NewEmbedSource(fsys, r)))
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
		`class="transclusion" data-embed-route="` + cidSecond + conceptExt + `"`,
		"the second body",
		`href="` + mount + "/" + cidSecond + conceptExt + `">second-note</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the rendered page lacks %s\npage: %s", want, page)
		}
	}
	if strings.Contains(page, `data-embed-error=`) {
		t.Errorf("an embed failed to render\npage: %s", page)
	}
}

// mountPrefix is the route prefix a vault page is served under, the trailing
// slash included: it is what the embed renderer strips off a resolved
// destination to reach a path inside the vault.
func mountPrefix(owner string) string {
	return testMount(owner) + "/"
}

func TestAdapterResolvesACommonMarkDestination(t *testing.T) {
	r, _, owner := testAdapter(t, "")

	route, ok := r.ResolveRoute("second-note.md")
	if !ok {
		t.Fatal("a plain markdown link to a stated concept did not resolve")
	}
	if want := testMount(owner) + "/" + cidSecond + conceptExt; route != want {
		t.Errorf("route = %q, want %q", route, want)
	}
}

// TestAdapterKeepsACommonMarkFragment pins the anchor travelling with the
// route, and TestAdapterDecodesACommonMarkDestination the percent-decoding a
// foreign corpus needs: the name in the vault is the reference.
func TestAdapterKeepsACommonMarkFragment(t *testing.T) {
	r, _, owner := testAdapter(t, "")

	route, ok := r.ResolveRoute("second-note.md#history")
	if !ok {
		t.Fatal("an anchored markdown link did not resolve")
	}
	if want := testMount(owner) + "/" + cidSecond + conceptExt + "#history"; route != want {
		t.Errorf("route = %q, want %q", route, want)
	}
}

func TestAdapterDecodesACommonMarkDestination(t *testing.T) {
	r, _, owner := testAdapter(t, "")

	route, ok := r.ResolveRoute("thoughts%2Fsecond-note.md")
	if !ok {
		t.Fatal("a percent-encoded destination did not resolve")
	}
	if want := testMount(owner) + "/" + cidSecond + conceptExt; route != want {
		t.Errorf("route = %q, want %q", route, want)
	}
}

// TestAdapterDeclinesAnUnstatedDestination is the verbatim fallback: what the
// vault does not hold keeps the destination its author wrote.
func TestAdapterDeclinesAnUnstatedDestination(t *testing.T) {
	r, _, _ := testAdapter(t, "")

	if route, ok := r.ResolveRoute("nowhere.md"); ok {
		t.Errorf("route = %q, want a miss for a target the vault does not hold", route)
	}
}

// TestAdapterResolvesAnImageDestination walks the whole render: a plain
// markdown image naming a stated attachment reaches the browser pointing
// inside the mount, where the filesystem serves the blob.
func TestAdapterResolvesAnImageDestination(t *testing.T) {
	r, v, owner := testAdapter(t, "")
	fsys, err := NewFS(context.Background(), v.Bundle, v.Name, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}

	body := "![the art](art.png) and [the guide](" + cidGuide + ".md) and [away](https://example.com/art.png)\n"
	html, _, err := markdown.Parse([]byte(body), r, markdown.NewEmbedContext(mountPrefix(owner), NewEmbedSource(fsys, r)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	page := string(html)

	mount := testMount(owner)
	for _, want := range []string{
		`src="` + mount + "/" + dirThoughts + "/" + nameArt + `"`,
		`href="` + mount + "/" + cidGuide + conceptExt + `"`,
		`href="https://example.com/art.png"`,
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

// TestAdapterDeclaresAbsenceOnlyForWhatItDoesNotHold is the vault's side of
// the absence seam: its index covers attachments beside documents, so a name
// resolving to nothing names nothing the vault holds.
func TestAdapterDeclaresAbsenceOnlyForWhatItDoesNotHold(t *testing.T) {
	r, _, _ := testAdapter(t, "")
	if r.RouteAbsent(cidSecond + conceptExt) {
		t.Errorf("%s is a held document and must not be declared absent", cidSecond+conceptExt)
	}
	if r.RouteAbsent(nameArt) {
		t.Errorf("%s is a held attachment and must not be declared absent", nameArt)
	}
	if !r.RouteAbsent("nowhere.md") {
		t.Error("a target the vault does not hold must be declared absent")
	}
}
