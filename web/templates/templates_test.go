package templates

import (
	"context"
	"strings"
	"testing"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/theme"
)

func testTheme() *theme.Theme {
	return &theme.Theme{
		Name: "test",
		Colors: theme.Colors{
			Background:       "#161821",
			Foreground:       "#c6c8d1",
			BackgroundAlt:    "#1e2132",
			Comment:          "#6b7089",
			CurrentLine:      "#1e2132",
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
			CodeBg:           "#1e2132",
			CodeFg:           "#c6c8d1",
			BorderColor:      "#0f1117",
		},
	}
}

func TestHomeRendersWithoutTree(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := Home(nil, testTheme()).Render(context.Background(), &sb)
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
	err := Home(tree, testTheme()).Render(context.Background(), &sb)
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
			Path: "/test",
			Name: "test",
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
	err := Home(tree, testTheme()).Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	html := sb.String()
	if !strings.Contains(html, "file.md") {
		t.Error("expected filename in output")
	}
	if !strings.Contains(html, "Markdown Files") {
		t.Error("expected heading in output")
	}
}

func TestHomeRendersNestedDirectories(t *testing.T) {
	t.Parallel()

	tree := &models.Tree{
		Root: &models.Node{
			Path: "/test",
			Name: "test",
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
	err := Home(tree, testTheme()).Render(context.Background(), &sb)
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
