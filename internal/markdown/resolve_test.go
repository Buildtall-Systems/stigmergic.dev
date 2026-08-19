package markdown

import (
	"strings"
	"testing"

	"go.abhg.dev/goldmark/wikilink"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

const (
	vaultTestMount = "/vault/npub1example/notes/"
	noteFile       = "note.md"
)

// entriesFor names one source's documents the way the server does before
// building the corpus-wide resolver.
func entriesFor(mount string, files []models.SearchableFile) []RouteEntry {
	entries := make([]RouteEntry, 0, len(files))
	for _, f := range files {
		rel := strings.TrimPrefix(f.Path, "/")
		entries = append(entries, RouteEntry{Path: rel, Route: mount + rel})
	}
	return entries
}

func routedTo(mount string, files []models.SearchableFile) []models.SearchableFile {
	routed := make([]models.SearchableFile, 0, len(files))
	for _, f := range files {
		f.Path = mount + strings.TrimPrefix(f.Path, "/")
		routed = append(routed, f)
	}
	return routed
}

// TestBacklinkIndexSpansMounts is the corpus-wide contract: a document in one
// source naming a document in another is a backlink like any other, because
// the resolver the index runs through knows every mounted source.
func TestBacklinkIndexSpansMounts(t *testing.T) {
	t.Parallel()

	localFiles := []models.SearchableFile{{Name: "local.md", Path: "/local.md"}}
	vaultFiles := []models.SearchableFile{{Name: "remote.md", Path: "/thoughts/remote.md"}}

	routes := NewRouteResolver(append(entriesFor(FileMount, localFiles), entriesFor(vaultTestMount, vaultFiles)...))

	refs := LinkRefs{
		FileMount + "local.md": {{Target: "thoughts/remote"}},
	}
	files := append(routedTo(FileMount, localFiles), routedTo(vaultTestMount, vaultFiles)...)

	index := BuildBacklinkIndex(refs, files, func(string) wikilink.Resolver { return routes }, []string{FileMount, vaultTestMount})

	entries := index[vaultTestMount+"thoughts/remote.md"]
	if len(entries) != 1 {
		t.Fatalf("expected the vault document to carry 1 backlink, got %d: %v", len(entries), index)
	}
	if got, want := entries[0].SourcePath, FileMount+"local.md"; got != want {
		t.Errorf("source path = %q, want %q", got, want)
	}
}

// TestBacklinkIndexSkipsDestinationsOutsideEveryMount pins the other half:
// what no mount serves is not a document this corpus holds.
func TestBacklinkIndexSkipsDestinationsOutsideEveryMount(t *testing.T) {
	t.Parallel()

	files := routedTo(FileMount, []models.SearchableFile{{Name: "local.md", Path: "/local.md"}})
	refs := LinkRefs{FileMount + "local.md": {{Target: "elsewhere"}}}

	stray := resolverFunc(func(*wikilink.Node) ([]byte, error) { return []byte("/elsewhere/doc.md"), nil })

	index := BuildBacklinkIndex(refs, files, func(string) wikilink.Resolver { return stray }, []string{FileMount})
	if len(index) != 0 {
		t.Errorf("expected no backlinks from a destination outside every mount, got %v", index)
	}
}

// resolverFunc adapts a function to wikilink.Resolver for tests that need one
// answer and no index behind it.
type resolverFunc func(*wikilink.Node) ([]byte, error)

func (f resolverFunc) ResolveWikilink(n *wikilink.Node) ([]byte, error) { return f(n) }

// TestChainPrefersTheSourceHoldingTheDocument is why a chain exists: two
// sources holding the same name must not make a document's own neighbour
// ambiguous.
func TestChainPrefersTheSourceHoldingTheDocument(t *testing.T) {
	t.Parallel()

	own := NewRouteResolver([]RouteEntry{{Path: noteFile, Route: vaultTestMount + noteFile}})
	corpus := NewRouteResolver([]RouteEntry{
		{Path: noteFile, Route: FileMount + noteFile},
		{Path: "only-local.md", Route: FileMount + "only-local.md"},
	})

	chain := Chain{own, corpus}

	dest, err := chain.ResolveWikilink(&wikilink.Node{Target: []byte("note")})
	if err != nil {
		t.Fatalf("ResolveWikilink: %v", err)
	}
	if got, want := string(dest), vaultTestMount+noteFile; got != want {
		t.Errorf("destination = %q, want %q: the source holding the document answers first", got, want)
	}

	dest, err = chain.ResolveWikilink(&wikilink.Node{Target: []byte("only-local")})
	if err != nil {
		t.Fatalf("ResolveWikilink: %v", err)
	}
	if got, want := string(dest), FileMount+"only-local.md"; got != want {
		t.Errorf("destination = %q, want %q: a name the source lacks reaches the corpus", got, want)
	}

	if dest, err = chain.ResolveWikilink(&wikilink.Node{Target: []byte("nowhere")}); err != nil || dest != nil {
		t.Errorf("dangling target resolved to %q (err %v), want nil", dest, err)
	}
}

// TestCommonMarkDestinationsResolveThroughTheSeam covers plain markdown links
// and images, which bypass wikilink syntax entirely: what the resolver holds
// is rewritten to its route, and what it does not hold reaches the browser
// exactly as the author wrote it.
func TestCommonMarkDestinationsResolveThroughTheSeam(t *testing.T) {
	t.Parallel()

	resolver := NewRouteResolver([]RouteEntry{
		{Path: "docs/target.md", Route: FileMount + "docs/target.md"},
		{Path: "media/photo.png", Route: FileMount + "media/photo.png"},
	})

	body := "[named](docs/target.md) [anchored](docs/target.md#part) ![shot](media/photo.png)\n\n" +
		"[missing](docs/nowhere.md) ![absent](media/nothing.png) [away](https://example.com/docs/target.md)\n"

	html, _, err := Parse([]byte(body), resolver, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	page := string(html)

	for _, want := range []string{
		`href="` + FileMount + `docs/target.md"`,
		`href="` + FileMount + `docs/target.md#part"`,
		`src="` + FileMount + `media/photo.png"`,
		`href="docs/nowhere.md"`,
		`src="media/nothing.png"`,
		`href="https://example.com/docs/target.md"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the rendered page lacks %s\npage: %s", want, page)
		}
	}
}

// TestCommonMarkDestinationsStayVerbatimWithoutAResolver is the nil-resolver
// idiom: no resolver, no rewriting, byte for byte what goldmark renders.
func TestCommonMarkDestinationsStayVerbatimWithoutAResolver(t *testing.T) {
	t.Parallel()

	body := "[named](docs/target.md) ![shot](media/photo.png)\n"

	html, _, err := Parse([]byte(body), nil, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	page := string(html)

	for _, want := range []string{`href="docs/target.md"`, `src="media/photo.png"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the rendered page lacks %s\npage: %s", want, page)
		}
	}
}

func TestRelativeDestination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		target string
		want   bool
	}{
		{target: "docs/target.md", want: true},
		{target: "./sibling.md", want: true},
		{target: "a-b.md", want: true},
		{target: "", want: false},
		{target: "/rooted.md", want: false},
		{target: "#anchor", want: false},
		{target: "https://example.com/x.md", want: false},
		{target: "mailto:someone@example.com", want: false},
		{target: "nostr:npub1example", want: false},
	}

	for _, tt := range tests {
		if got := relativeDestination(tt.target); got != tt.want {
			t.Errorf("relativeDestination(%q) = %v, want %v", tt.target, got, tt.want)
		}
	}
}
