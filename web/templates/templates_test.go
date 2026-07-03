package templates

import (
	"context"
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
	err := Home(nil, "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, []models.SearchableFile{}, 0, 0, true, models.UICapabilities{RecentlyUpdated: true, GitignoreToggle: true, CopyPath: true}).Render(context.Background(), &sb)
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
	err := Home(tree, "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, []models.SearchableFile{}, 1, 1, true, models.UICapabilities{RecentlyUpdated: true, GitignoreToggle: true, CopyPath: true}).Render(context.Background(), &sb)
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
	err := Home(tree, "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, []models.SearchableFile{}, 1, 1, true, models.UICapabilities{RecentlyUpdated: true, GitignoreToggle: true, CopyPath: true}).Render(context.Background(), &sb)
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

func TestHomeRendersNestedDirectories(t *testing.T) {
	t.Parallel()

	tree := &models.Tree{
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

	var sb strings.Builder
	err := Home(tree, "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, []models.SearchableFile{}, 1, 1, true, models.UICapabilities{RecentlyUpdated: true, GitignoreToggle: true, CopyPath: true}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	if !strings.Contains(html, "dir") {
		t.Error("expected directory name in output")
	}
	if !strings.Contains(html, "nested.md") {
		t.Error("expected nested file in output")
	}
}

func TestLayoutRendersThreePaneLandmarks(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := Home(nil, "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, []models.SearchableFile{}, 0, 0, true, models.UICapabilities{}).Render(context.Background(), &sb)
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
	err := Home(tree, "/test/path", testTheme(), testThemes(), []models.SearchableFile{}, []models.SearchableFile{}, 1, 0, true, models.UICapabilities{}).Render(context.Background(), &sb)
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
		err := Home(nil, testTreeRoot, testTheme(), testThemes(), []models.SearchableFile{}, []models.SearchableFile{}, 0, 0, true, caps).Render(context.Background(), &sb)
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
	err := Markdown("doc.md", nil, "<p>hi</p>", "hi", testTreeRoot, "", testTheme(), testThemes(), []models.SearchableFile{}, nil, []models.SearchableFile{}, true, nil, nil, models.UICapabilities{}, outline).Render(context.Background(), &sb)
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
	err := Home(nil, testTreeRoot, testTheme(), testThemes(), []models.SearchableFile{}, []models.SearchableFile{}, 0, 0, true, models.UICapabilities{}).Render(context.Background(), &sb)
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
	err := MarkdownContent(crumbs, "<p>hello</p>", "hello", "/root", "", nil, nil, models.UICapabilities{}).Render(context.Background(), &sb)
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

func TestLayoutEmitsThemeSwitchingScaffolding(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := Home(nil, testTreeRoot, testTheme(), testThemes(), []models.SearchableFile{}, []models.SearchableFile{}, 0, 0, true, models.UICapabilities{}).Render(context.Background(), &sb)
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
