package templates

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/theme"
)

const (
	themeTestName = "test"
	bgAltHex      = "#1e2132"
	testTreeRoot  = "/test"
)

func testTheme() *theme.Theme {
	return &theme.Theme{
		Name: themeTestName,
		Colors: theme.Colors{
			Background:       "#161821",
			Foreground:       "#c6c8d1",
			BackgroundAlt:    bgAltHex,
			Comment:          "#6b7089",
			CurrentLine:      bgAltHex,
			Selection:        "#272c42",
			LineNumber:       "#444b71",
			LineNumberActive: "#cdd1e6",
			Red:              "#e27878",
			Orange:           "#e2a478",
			Yellow:           "#e4aa80",
			Green:            "#b4be82",
			Cyan:             "#89b8c2",
			Blue:             "#84a0c6",
			Purple:           "#a093c7",
			Link:             "#89b8c2",
			LinkHover:        "#84a0c6",
			CodeBg:           bgAltHex,
			CodeFg:           "#c6c8d1",
			BorderColor:      "#0f1117",
		},
	}
}

func testThemes() []*theme.Theme {
	dark := testTheme()
	dark.MermaidTheme = "dark"
	dark.ChromaCSS = `[data-theme="test"] .chroma { color: var(--code-fg-color); }` + "\n"
	light := testTheme()
	light.Name = "test-light"
	light.Colors.Background = "#e8e9ec"
	light.MermaidTheme = "default"
	light.ChromaCSS = `[data-theme="test-light"] .chroma { color: var(--code-fg-color); }` + "\n"
	return []*theme.Theme{dark, light}
}

func TestHomeRendersWithoutTree(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := Home(localPanel(models.TreeView{}), "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, 0, 0, true, models.UICapabilities{RecentlyUpdated: true, GitignoreToggle: true, CopyPath: true}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	if !strings.Contains(html, "No markdown files found") {
		t.Error("expected 'No markdown files found' message")
	}
	if !strings.Contains(html, "stigmergic") {
		t.Error("expected title to contain 'stigmergic'")
	}
}

func TestHomeRendersWithEmptyTree(t *testing.T) {
	t.Parallel()

	tree := &models.Tree{}
	var sb strings.Builder
	err := Home(localPanel(models.TreeView{Tree: tree}), "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, 1, 1, true, models.UICapabilities{RecentlyUpdated: true, GitignoreToggle: true, CopyPath: true}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	if !strings.Contains(html, "No markdown files found") {
		t.Error("expected 'No markdown files found' message")
	}
}

func TestHomeRendersWithTree(t *testing.T) {
	t.Parallel()

	tree := &models.Tree{
		Root: &models.Node{
			Path: testTreeRoot,
			Name: themeTestName,
			Type: models.NodeTypeDirectory,
			Children: []*models.Node{
				{
					Path: "/test/file.md",
					Name: "file.md",
					Type: models.NodeTypeFile,
				},
			},
		},
	}

	var sb strings.Builder
	err := Home(localPanel(models.TreeView{Tree: tree}), "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, 1, 1, true, models.UICapabilities{RecentlyUpdated: true, GitignoreToggle: true, CopyPath: true}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	if !strings.Contains(html, "file.md") {
		t.Error("expected filename in output")
	}
	if !strings.Contains(html, "/test/path") {
		t.Error("expected rootPath in output")
	}
}

func nestedTestTree() *models.Tree {
	return &models.Tree{
		Root: &models.Node{
			Path: testTreeRoot,
			Name: themeTestName,
			Type: models.NodeTypeDirectory,
			Children: []*models.Node{
				{
					Path: "/test/dir",
					Name: "dir",
					Type: models.NodeTypeDirectory,
					Children: []*models.Node{
						{
							Path: "/test/dir/nested.md",
							Name: "nested.md",
							Type: models.NodeTypeFile,
						},
					},
				},
			},
		},
	}
}

// testMount is the route prefix a tree's rows link through, the one the
// primary source answers at.
const testMount = "/file/"

// localPanel is the sidebar a test renders when it cares about the local
// tree alone: the primary source's view with no vault mounted beneath it.
func localPanel(view models.TreeView) models.SidebarView {
	return models.SidebarView{Primary: view}
}

func renderHomeTree(t *testing.T, view models.TreeView) string {
	t.Helper()

	var sb strings.Builder
	err := Home(localPanel(view), "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, 1, 1, true, models.UICapabilities{RecentlyUpdated: true, GitignoreToggle: true, CopyPath: true}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}
	return sb.String()
}

// A directory ships its children only when the render expands it. Everything
// else is a placeholder the client fetches on demand, which is what keeps the
// cold payload proportional to what is visible rather than to the corpus.
func TestHomeRendersNestedDirectoriesOnlyWhenExpanded(t *testing.T) {
	t.Parallel()

	tree := nestedTestTree()

	collapsed := renderHomeTree(t, models.TreeView{Tree: tree, Mount: testMount})
	if !strings.Contains(collapsed, ">dir</span>") {
		t.Error("expected the directory row to render")
	}
	if strings.Contains(collapsed, "nested.md") {
		t.Error("a collapsed directory must not ship its children")
	}
	if !strings.Contains(collapsed, `data-children-path="/test/dir" data-mount="`+testMount+`" data-loaded="false"`) {
		t.Error("expected an unloaded placeholder carrying the directory path")
	}
	if !strings.Contains(collapsed, `data-expanded="false"`) {
		t.Error("expected the collapsed directory row to report its state")
	}

	expanded := renderHomeTree(t, models.TreeView{Tree: tree, Mount: testMount, Expanded: models.AncestorDirs("/test/dir/nested.md")})
	if !strings.Contains(expanded, "nested.md") {
		t.Error("expected an expanded directory to ship its children")
	}
	if !strings.Contains(expanded, `data-children-path="/test/dir" data-mount="`+testMount+`" data-loaded="true"`) {
		t.Error("expected the expanded container to be marked loaded")
	}
	if !strings.Contains(expanded, `data-expanded="true"`) {
		t.Error("expected the expanded directory row to report its state")
	}
}

// The payload a cold page carries is bounded by the top level's width, not by
// the corpus behind it. Growing the hidden depth must not grow the page.
func TestHomeTreePayloadIsProportionalToVisibleRows(t *testing.T) {
	t.Parallel()

	shallow := renderHomeTree(t, models.TreeView{Tree: wideTestTree(t, 20, 1)})
	deep := renderHomeTree(t, models.TreeView{Tree: wideTestTree(t, 20, 6)})

	if len(shallow) != len(deep) {
		t.Errorf("hidden depth must not reach the payload: %d bytes at depth 1, %d at depth 6", len(shallow), len(deep))
	}
	if strings.Count(shallow, `<use href="#tree-icon-folder"></use>`) != 20 {
		t.Error("expected every directory row to reference the sprite rather than inline its glyph")
	}
	if n := strings.Count(shallow, `d="M3 7v10a2 2 0 002 2h14`); n != 1 {
		t.Errorf("the folder glyph's path data belongs in the sprite alone, found it %d times", n)
	}
}

// wideTestTree builds a root of width directories, each a chain depth levels
// deep ending in a markdown file.
func wideTestTree(t *testing.T, width int, depth int) *models.Tree {
	t.Helper()

	root := &models.Node{Path: ".", Name: themeTestName, Type: models.NodeTypeDirectory}
	for i := range width {
		dirPath := fmt.Sprintf("dir%d", i)
		top := &models.Node{Path: dirPath, Name: dirPath, Type: models.NodeTypeDirectory}
		node := top
		for d := range depth {
			childPath := fmt.Sprintf("%s/level%d", node.Path, d)
			child := &models.Node{Path: childPath, Name: fmt.Sprintf("level%d", d), Type: models.NodeTypeDirectory}
			node.Children = []*models.Node{child}
			node = child
		}
		node.Children = []*models.Node{{Path: node.Path + "/leaf.md", Name: "leaf.md", Type: models.NodeTypeFile}}
		root.Children = append(root.Children, top)
	}
	return &models.Tree{Root: root}
}

func TestLayoutRendersThreePaneLandmarks(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := Home(localPanel(models.TreeView{}), "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, 0, 0, true, models.UICapabilities{}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	for _, landmark := range []string{`id="sidebar"`, `id="content"`, `id="outline"`} {
		if !strings.Contains(html, landmark) {
			t.Errorf("expected %s landmark in layout", landmark)
		}
	}
	if strings.Contains(html, `id="main"`) {
		t.Error("legacy #main swap target must be absent")
	}
	if strings.Contains(html, "indicator-") {
		t.Error("live indicator markup and CSS must be absent")
	}
}

func TestTreeLinksCarryDataPathAndContentTarget(t *testing.T) {
	t.Parallel()

	tree := &models.Tree{
		Root: &models.Node{
			Path: testTreeRoot,
			Name: themeTestName,
			Type: models.NodeTypeDirectory,
			Children: []*models.Node{
				{
					Path: "/test/file.md",
					Name: "file.md",
					Type: models.NodeTypeFile,
				},
			},
		},
	}

	var sb strings.Builder
	err := Home(localPanel(models.TreeView{Tree: tree}), "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, 1, 0, true, models.UICapabilities{}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	if !strings.Contains(html, `data-path="/test/file.md"`) {
		t.Error("expected tree file link to carry data-path attribute")
	}
	if !strings.Contains(html, `hx-target="#content"`) {
		t.Error("expected tree file link to target #content")
	}
	if strings.Contains(html, `hx-target="main"`) {
		t.Error("legacy main target must be absent from tree links")
	}
}

func TestLayoutFollowToggleGatedOnCapability(t *testing.T) {
	t.Parallel()

	renderHome := func(caps models.UICapabilities) string {
		var sb strings.Builder
		err := Home(localPanel(models.TreeView{}), testTreeRoot, testTheme(), testThemes(), []models.SearchableFile{}, 0, 0, true, caps).Render(context.Background(), &sb)
		if err != nil {
			t.Fatalf("failed to render: %v", err)
		}
		return sb.String()
	}

	if !strings.Contains(renderHome(models.UICapabilities{FollowMode: true}), "data-follow-toggle") {
		t.Error("expected follow toggle when source is watchable")
	}
	if strings.Contains(renderHome(models.UICapabilities{}), "data-follow-toggle") {
		t.Error("follow toggle must be absent without the FollowMode capability")
	}
}

func TestMarkdownFullPageRendersOutlineRail(t *testing.T) {
	t.Parallel()

	outline := []models.OutlineEntry{
		{Level: 2, Text: "Section One", ID: "section-one"},
		{Level: 3, Text: "Detail", ID: "detail"},
	}

	var sb strings.Builder
	err := Markdown("doc.md", nil, "<p>hi</p>", "hi", testTreeRoot, "", testTheme(), testThemes(), localPanel(models.TreeView{}), []models.SearchableFile{}, true, nil, nil, models.UICapabilities{}, outline, nil).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	if !strings.Contains(html, `href="#section-one"`) {
		t.Error("expected outline link anchored to heading id")
	}
	if !strings.Contains(html, `data-outline-target="detail"`) {
		t.Error("expected outline link to carry data-outline-target")
	}
	if !strings.Contains(html, "On this page") {
		t.Error("expected outline rail heading")
	}
}

func TestLayoutOutlineRailEmptyWithoutEntries(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := Home(localPanel(models.TreeView{}), testTreeRoot, testTheme(), testThemes(), []models.SearchableFile{}, 0, 0, true, models.UICapabilities{}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if strings.Contains(sb.String(), "On this page") {
		t.Error("outline rail must render empty on pages without a document outline")
	}
}

func TestMarkdownBreadcrumbsUseHTMXSwaps(t *testing.T) {
	t.Parallel()

	crumbs := []models.Breadcrumb{
		{Name: "dir", Path: "/file/dir"},
		{Name: "doc.md", Path: "/file/dir/doc.md"},
	}

	var sb strings.Builder
	err := MarkdownContent(crumbs, "<p>hello</p>", "hello", "/root", "", nil, nil, models.UICapabilities{}, nil).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	if !strings.Contains(html, `hx-get="/file/dir"`) {
		t.Error("expected breadcrumb link to carry hx-get")
	}
	if !strings.Contains(html, `hx-target="#content"`) {
		t.Error("expected breadcrumb link to target #content")
	}
	if !strings.Contains(html, `hx-push-url="true"`) {
		t.Error("expected breadcrumb link to push URL")
	}
}

func TestMarkdownContentCarriesTranscludedRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		want        string
		transcluded []string
	}{
		{
			name:        "none",
			want:        `data-transcluded="[]"`,
			transcluded: nil,
		},
		{
			name:        "one route",
			want:        `data-transcluded="[&#34;notes/target.md&#34;]"`,
			transcluded: []string{"notes/target.md"},
		},
		{
			// A vault route carries spaces, which is why the attribute is
			// JSON rather than a separator-joined list.
			name:        "route with spaces",
			want:        `data-transcluded="[&#34;reading/papers/lifes irreducible structure.md&#34;]"`,
			transcluded: []string{"reading/papers/lifes irreducible structure.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sb strings.Builder
			err := MarkdownContent(nil, "<p>hello</p>", "hello", "/root", "", nil, nil, models.UICapabilities{}, tt.transcluded).Render(context.Background(), &sb)
			if err != nil {
				t.Fatalf("failed to render: %v", err)
			}

			if !strings.Contains(sb.String(), tt.want) {
				t.Errorf("expected rendered content to contain %s", tt.want)
			}
		})
	}
}

func TestLayoutEmitsThemeSwitchingScaffolding(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := Home(localPanel(models.TreeView{}), testTreeRoot, testTheme(), testThemes(), []models.SearchableFile{}, 0, 0, true, models.UICapabilities{}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	for _, want := range []string{
		":root {",
		`[data-theme="test"] {`,
		`[data-theme="test-light"] {`,
		`[data-theme="test"] .chroma`,
		`[data-theme="test-light"] .chroma`,
		"localStorage.getItem('stigmergic-theme')",
		`id="theme-config"`,
		`"boot":"test"`,
		"Cycle theme (T)",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected layout to contain %q", want)
		}
	}
	if strings.Contains(html, "startOnLoad: true") {
		t.Error("mermaid must not auto-render; theme-aware render owns it")
	}
}

func TestLoginPageUsesThemeVariables(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := Login("http://localhost:8080", testTheme(), testThemes()).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	for _, want := range []string{
		":root {",
		`[data-theme="test-light"] {`,
		"localStorage.getItem('stigmergic-theme')",
		"border: 1px solid var(--cyan-color)",
		"color: var(--red-color)",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected login page to contain %q", want)
		}
	}
	if strings.Contains(html, "background: #161821") {
		t.Error("login page must not hardcode palette colors")
	}
}

// The command palette reads its own icon markup out of <template> elements by
// id. The tree sprite is emitted earlier in the body, so a shared id would
// hand the palette a <symbol> instead. The two sets must stay disjoint.
func TestTreeSpriteIDsDoNotCollideWithPaletteIcons(t *testing.T) {
	t.Parallel()

	html := renderHomeTree(t, models.TreeView{Tree: nestedTestTree()})

	for _, id := range []string{"icon-file", "icon-command", "icon-content", "icon-return"} {
		if strings.Contains(html, `<symbol id="`+id+`"`) {
			t.Errorf("sprite symbol %q collides with a palette template id", id)
		}
	}
	for _, id := range []string{"tree-icon-chevron", "tree-icon-folder", "tree-icon-file"} {
		if !strings.Contains(html, `<symbol id="`+id+`"`) {
			t.Errorf("expected the sprite to define %q", id)
		}
	}
}

// The file list is fetched, never inlined. Shipping it in the page cost more
// than the entire tree does.
func TestHomeDoesNotInlineTheFileList(t *testing.T) {
	t.Parallel()

	html := renderHomeTree(t, models.TreeView{Tree: nestedTestTree()})

	if strings.Contains(html, `id="markdown-files"`) {
		t.Error("the file list must be fetched from /api/files, not inlined in the page")
	}
}
