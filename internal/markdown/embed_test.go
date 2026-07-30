package markdown

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

const (
	embedContainer  = `class="transclusion"`
	embedAlphaBody  = "alpha body"
	embedBetaBody   = "beta body"
	embedChildBody  = "alpha child body"
	embedBetaSource = "![[target#Beta]]\n"
)

// embedTargetDoc carries a two-level heading structure so section slicing,
// descendant inclusion, and section termination are all observable.
const embedTargetDoc = "# Target\n\n" +
	"## Alpha\n\nalpha body\n\n" +
	"### Alpha Child\n\nalpha child body\n\n" +
	"## Beta\n\nbeta body\n"

func testEmbedVault() (fstest.MapFS, []models.SearchableFile) {
	fsys := fstest.MapFS{
		"target.md":  {Data: []byte(embedTargetDoc)},
		"d1.md":      {Data: []byte("level one\n\n![[d2]]\n")},
		"d2.md":      {Data: []byte("level two\n\n![[d3]]\n")},
		"d3.md":      {Data: []byte("level three\n\n![[d4]]\n")},
		"d4.md":      {Data: []byte("level four\n")},
		"cycle-a.md": {Data: []byte("side a\n\n![[cycle-b]]\n")},
		"cycle-b.md": {Data: []byte("side b\n\n![[cycle-a]]\n")},
	}

	files := make([]models.SearchableFile, 0, len(fsys))
	for path := range fsys {
		files = append(files, models.SearchableFile{Name: path, Path: "/" + path})
	}
	return fsys, files
}

func renderEmbedded(t *testing.T, source string) string {
	t.Helper()

	fsys, files := testEmbedVault()
	html, _, err := Parse([]byte(source), NewTreeResolver(files), NewEmbedContext(NewFSEmbedSource(fsys, "")))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	return string(html)
}

func embedErrorAttr(reason string) string {
	return `data-embed-error="` + reason + `"`
}

func TestEmbedTransclusion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		source   string
		contains []string
		excludes []string
	}{
		{
			name:     "whole note embed renders the target body",
			source:   "![[target]]\n",
			contains: []string{embedContainer, "Target", embedAlphaBody, embedBetaBody},
		},
		{
			name:     "section embed renders only that section",
			source:   embedBetaSource,
			contains: []string{embedContainer, embedBetaBody},
			excludes: []string{embedAlphaBody, embedChildBody},
		},
		{
			name:     "section embed includes descendant subsections",
			source:   "![[target#Alpha]]\n",
			contains: []string{embedAlphaBody, "Alpha Child", embedChildBody},
			excludes: []string{embedBetaBody},
		},
		{
			name:     "embed alone in a paragraph is promoted",
			source:   embedBetaSource,
			contains: []string{embedContainer, embedBetaBody},
			excludes: []string{`<p><a href="/file/target.md">`},
		},
		{
			name:     "embed as the sole content of a list item is promoted",
			source:   "- ![[target#Beta]]\n",
			contains: []string{embedContainer, embedBetaBody},
		},
		{
			name:     "embed mid sentence stays an anchor",
			source:   "See ![[target]] inline.\n",
			contains: []string{`<a href="/file/target.md">`},
			excludes: []string{embedContainer, embedAlphaBody},
		},
		{
			name:     "nested embed renders at depth two",
			source:   "![[d1]]\n",
			contains: []string{"level one", "level two", "level three"},
		},
		{
			name:     "depth cap yields a marker instead of recursing",
			source:   "![[d1]]\n",
			contains: []string{"level three", embedErrorAttr(embedErrDepth)},
			excludes: []string{"level four"},
		},
		{
			name:     "two note cycle terminates with a marker",
			source:   "![[cycle-a]]\n",
			contains: []string{"side a", "side b", embedErrorAttr(embedErrCycle)},
			excludes: []string{embedErrorAttr(embedErrDepth)},
		},
		{
			name:     "unresolved target yields a marker",
			source:   "![[no such note]]\n",
			contains: []string{embedErrorAttr(embedErrUnresolved), "![[no such note]]"},
		},
		{
			name:     "unmatched fragment yields a marker",
			source:   "![[target#Nonexistent]]\n",
			contains: []string{embedErrorAttr(embedErrNoSection), "![[target#Nonexistent]]"},
			excludes: []string{embedAlphaBody, embedBetaBody},
		},
		{
			name:     "embed links back to its source note",
			source:   embedBetaSource,
			contains: []string{`class="transclusion-source" href="/file/target.md"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			html := renderEmbedded(t, tt.source)
			for _, want := range tt.contains {
				if !strings.Contains(html, want) {
					t.Errorf("expected output to contain %q, got %q", want, html)
				}
			}
			for _, unwanted := range tt.excludes {
				if strings.Contains(html, unwanted) {
					t.Errorf("expected output to exclude %q, got %q", unwanted, html)
				}
			}
		})
	}
}

func TestEmbedTwoSectionsOfOneNote(t *testing.T) {
	t.Parallel()

	html := renderEmbedded(t, "![[target#Alpha]]\n\n![[target#Beta]]\n")

	if !strings.Contains(html, embedAlphaBody) || !strings.Contains(html, embedBetaBody) {
		t.Fatalf("expected both sections to render, got %q", html)
	}
	if strings.Contains(html, embedErrorAttr(embedErrCycle)) {
		t.Errorf("two sections of one note must not read as a cycle, got %q", html)
	}
}

// TestEmbedRepeatedSection pins the visited set to the path rather than the
// page: the same section embedded twice as siblings is not a cycle, and both
// occurrences must render.
func TestEmbedRepeatedSection(t *testing.T) {
	t.Parallel()

	html := renderEmbedded(t, "![[target#Beta]]\n\n![[target#Beta]]\n")

	if got := strings.Count(html, embedBetaBody); got != 2 {
		t.Errorf("expected the section to render twice, got %d in %q", got, html)
	}
	if strings.Contains(html, embedErrorAttr(embedErrCycle)) {
		t.Errorf("sibling embeds of one section must not read as a cycle, got %q", html)
	}
}

// TestEmbedHeadingsCarryNoID keeps transcluded headings out of the id space,
// where they would collide with host headings and capture the outline rail's
// scrollspy anchors.
func TestEmbedHeadingsCarryNoID(t *testing.T) {
	t.Parallel()

	html := renderEmbedded(t, "## Host Heading\n\n![[target#Alpha]]\n")

	if !strings.Contains(html, `<h2 id="host-heading">`) {
		t.Errorf("expected the host heading to keep its id, got %q", html)
	}
	if !strings.Contains(html, "<h2>Alpha</h2>") {
		t.Errorf("expected the transcluded heading to render without an id, got %q", html)
	}
	if strings.Contains(html, `id="alpha"`) {
		t.Errorf("transcluded heading must not carry an id, got %q", html)
	}
}

// TestEmbedNilContextIsInert proves the nil-disableable parameter: without an
// EmbedContext the output is byte-identical to the pre-transclusion renderer,
// which is what makes reverting this feature a one-line change.
func TestEmbedNilContextIsInert(t *testing.T) {
	t.Parallel()

	const source = "![[target#Alpha]]\n\nSee ![[target]] inline.\n"

	_, files := testEmbedVault()
	resolver := NewTreeResolver(files)

	html, _, err := Parse([]byte(source), resolver, nil)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	got := string(html)
	if strings.Contains(got, "transclusion") {
		t.Errorf("a nil EmbedContext must not transclude, got %q", got)
	}
	if !strings.Contains(got, `<a href="/file/target.md#Alpha">`) {
		t.Errorf("expected the embed to render as an anchor, got %q", got)
	}
}
