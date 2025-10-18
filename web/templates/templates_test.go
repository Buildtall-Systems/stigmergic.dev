package templates

import (
	"context"
	"strings"
	"testing"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

func TestHomeRendersWithoutTree(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := Home(nil).Render(context.Background(), &sb)
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
	err := Home(tree).Render(context.Background(), &sb)
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
	err := Home(tree).Render(context.Background(), &sb)
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
	err := Home(tree).Render(context.Background(), &sb)
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
