package markdown

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

const (
	embedContainer   = `class="transclusion"`
	embedAlphaBody   = "alpha body"
	embedBetaBody    = "beta body"
	embedChildBody   = "alpha child body"
	embedBetaSource  = "![[target#Beta]]\n"
	embedImageSource = "![[dependency.png]]\n"
	embedImageTag    = "<img"
	embedTargetRoute = "target.md"
	embedChainSource = "![[d1]]\n"
)

// embedTargetDoc carries a two-level heading structure so section slicing,
// descendant inclusion, and section termination are all observable.
const embedTargetDoc = "# Target\n\n" +
	"## Alpha\n\nalpha body\n\n" +
	"### Alpha Child\n\nalpha child body\n\n" +
	"## Beta\n\nbeta body\n"

// testEmbedVault returns a corpus and the searchable file set a scan would
// produce from it. Only markdown enters the file set, because source.Scan
// admits nothing else; an attachment that renders therefore proves the probe
// found it without any index entry to help.
func testEmbedVault() (fstest.MapFS, []models.SearchableFile) {
	fsys := fstest.MapFS{
		embedTargetRoute:           {Data: []byte(embedTargetDoc)},
		"d1.md":                    {Data: []byte("level one\n\n![[d2]]\n")},
		"d2.md":                    {Data: []byte("level two\n\n![[d3]]\n")},
		"d3.md":                    {Data: []byte("level three\n\n![[d4]]\n")},
		"d4.md":                    {Data: []byte("level four\n")},
		"cycle-a.md":               {Data: []byte("side a\n\n![[cycle-b]]\n")},
		"cycle-b.md":               {Data: []byte("side b\n\n![[cycle-a]]\n")},
		"file/dependency.png":      {Data: []byte("\x89PNG")},
		"diagrams/flow.svg":        {Data: []byte("<svg/>")},
		"file/The Kekule Left.pdf": {Data: []byte("%PDF")},
		"file/episode.mp3":         {Data: []byte("ID3")},
		"file/clip.mp4":            {Data: []byte("ftyp")},
	}

	files := make([]models.SearchableFile, 0, len(fsys))
	for path := range fsys {
		if !strings.HasSuffix(path, models.MarkdownExt) {
			continue
		}
		files = append(files, models.SearchableFile{Name: path, Path: "/" + path})
	}
	return fsys, files
}

func renderEmbedded(t *testing.T, source string) string {
	t.Helper()
	return renderEmbeddedWithRoot(t, source, "")
}

func renderEmbeddedWithRoot(t *testing.T, source, attachmentRoot string) string {
	t.Helper()

	fsys, files := testEmbedVault()
	html, _, err := Parse([]byte(source), NewTreeResolver(files), NewEmbedContext(FileMount, NewFSEmbedSource(fsys, attachmentRoot)))
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
			source:   embedChainSource,
			contains: []string{"level one", "level two", "level three"},
		},
		{
			name:     "depth cap yields a marker instead of recursing",
			source:   embedChainSource,
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

func TestEmbedAttachments(t *testing.T) {
	t.Parallel()

	const attachmentRoot = "file"

	tests := []struct {
		name           string
		attachmentRoot string
		source         string
		contains       []string
		excludes       []string
	}{
		{
			name:           "bare image filename resolved through the attachment root",
			attachmentRoot: attachmentRoot,
			source:         embedImageSource,
			contains:       []string{`<img src="/file/file/dependency.png">`},
		},
		{
			name:     "image with an explicit path resolved at the content root",
			source:   "![[diagrams/flow.svg]]\n",
			contains: []string{`<img src="/file/diagrams/flow.svg">`},
		},
		{
			name:           "image label becomes alt text",
			attachmentRoot: attachmentRoot,
			source:         "![[dependency.png|a dependency diagram]]\n",
			contains:       []string{`alt="a dependency diagram"`},
		},
		{
			name:           "image label equal to the target sets no alt text",
			attachmentRoot: attachmentRoot,
			source:         embedImageSource,
			excludes:       []string{"alt="},
		},
		{
			name:           "image with no matching file yields a marker",
			attachmentRoot: attachmentRoot,
			source:         "![[absent.png]]\n",
			contains:       []string{embedErrorAttr(embedErrUnresolved)},
			excludes:       []string{embedImageTag},
		},
		{
			name:           "pdf renders an anchor",
			attachmentRoot: attachmentRoot,
			source:         "![[The Kekule Left.pdf]]\n",
			contains:       []string{`href="/file/file/The%20Kekule%20Left.pdf"`, "The Kekule Left.pdf"},
			excludes:       []string{embedImageTag},
		},
		{
			name:           "audio renders an anchor",
			attachmentRoot: attachmentRoot,
			source:         "![[episode.mp3]]\n",
			contains:       []string{`href="/file/file/episode.mp3"`},
			excludes:       []string{embedImageTag},
		},
		{
			name:           "video renders an anchor",
			attachmentRoot: attachmentRoot,
			source:         "![[clip.mp4]]\n",
			contains:       []string{`href="/file/file/clip.mp4"`},
			excludes:       []string{embedImageTag},
		},
		{
			name:     "bare filename misses without an attachment root",
			source:   embedImageSource,
			contains: []string{embedErrorAttr(embedErrUnresolved)},
			excludes: []string{embedImageTag},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			html := renderEmbeddedWithRoot(t, tt.source, tt.attachmentRoot)
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

// transcludedRoutes parses source and returns the dependency list the render
// recorded, which is what the live-reload client is handed.
func transcludedRoutes(t *testing.T, source string) []string {
	t.Helper()

	fsys, files := testEmbedVault()
	embeds := NewEmbedContext(FileMount, NewFSEmbedSource(fsys, ""))
	if _, _, err := Parse([]byte(source), NewTreeResolver(files), embeds); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	return embeds.Transcluded()
}

func TestEmbedTranscludedRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "no embeds",
			source: "just text\n",
			want:   nil,
		},
		{
			name:   "one note",
			source: "![[target#Alpha]]\n",
			want:   []string{embedTargetRoute},
		},
		{
			// Two sections of one note are one dependency: an edit to that
			// note refreshes the page once, not twice.
			name:   "repeated target collapses",
			source: "![[target#Alpha]]\n\n" + embedBetaSource,
			want:   []string{embedTargetRoute},
		},
		{
			// A nested render shares the host's context, so a note reached
			// only through another transclusion is a dependency too. d4 is
			// absent because the depth cap stops the chain before it.
			name:   "nested routes recorded in document order",
			source: embedChainSource,
			want:   []string{"d1.md", "d2.md", "d3.md"},
		},
		{
			name:   "unresolved records nothing",
			source: "![[nowhere]]\n",
			want:   nil,
		},
		{
			name:   "unmatched fragment records nothing",
			source: "![[target#Missing]]\n",
			want:   nil,
		},
		{
			// An attachment is found by filesystem probe rather than by the
			// scan, so the watcher never reports a change to it and it is not
			// a dependency the client can act on.
			name:   "attachment records nothing",
			source: embedImageSource,
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := transcludedRoutes(t, tt.source)
			if len(got) != len(tt.want) {
				t.Fatalf("expected routes %v, got %v", tt.want, got)
			}
			for i, route := range tt.want {
				if got[i] != route {
					t.Errorf("expected route %d to be %q, got %q", i, route, got[i])
				}
			}
		})
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
