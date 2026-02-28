package markdown

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

func writeTestFile(t *testing.T, dir, route, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, route)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildBacklinkIndex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "Link to [[b]].")
	writeTestFile(t, dir, "b.md", "No links here.")
	writeTestFile(t, dir, "c.md", "Also links to [[b]].")

	files := []models.SearchableFile{
		{Name: "a.md", Path: "/a.md"},
		{Name: "b.md", Path: "/b.md"},
		{Name: "c.md", Path: "/c.md"},
	}

	index := BuildBacklinkIndex(dir, files)

	entries := index["b.md"]
	if len(entries) != 2 {
		t.Fatalf("expected 2 backlinks to b.md, got %d", len(entries))
	}

	sources := map[string]bool{}
	for _, e := range entries {
		sources[e.SourcePath] = true
	}
	if !sources["a.md"] {
		t.Error("expected a.md in backlinks to b.md")
	}
	if !sources["c.md"] {
		t.Error("expected c.md in backlinks to b.md")
	}
}

func TestBuildBacklinkIndexNoLinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "No wikilinks here.")

	files := []models.SearchableFile{
		{Name: "a.md", Path: "/a.md"},
	}

	index := BuildBacklinkIndex(dir, files)

	if len(index) != 0 {
		t.Errorf("expected empty index, got %d entries", len(index))
	}
}

func TestBuildBacklinkIndexSelfLink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "Self-referencing [[a]].")

	files := []models.SearchableFile{
		{Name: "a.md", Path: "/a.md"},
	}

	index := BuildBacklinkIndex(dir, files)

	if entries := index["a.md"]; len(entries) != 0 {
		t.Errorf("expected self-links to be excluded, got %d entries", len(entries))
	}
}

func TestBuildBacklinkIndexDuplicateLinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "Link to [[b]] and again [[b]].")
	writeTestFile(t, dir, "b.md", "Target.")

	files := []models.SearchableFile{
		{Name: "a.md", Path: "/a.md"},
		{Name: "b.md", Path: "/b.md"},
	}

	index := BuildBacklinkIndex(dir, files)

	entries := index["b.md"]
	if len(entries) != 1 {
		t.Errorf("expected 1 backlink entry (deduped), got %d", len(entries))
	}
}

func TestBuildBacklinkIndexUnresolved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "Link to [[nonexistent]].")

	files := []models.SearchableFile{
		{Name: "a.md", Path: "/a.md"},
	}

	index := BuildBacklinkIndex(dir, files)

	if len(index) != 0 {
		t.Errorf("expected empty index for unresolved links, got %d entries", len(index))
	}
}

func TestTitleFromFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		route string
		want  string
	}{
		{"index.md", "index"},
		{"folder/note.md", "note"},
		{"a/b/deep.md", "deep"},
	}

	for _, tt := range tests {
		if got := titleFromFilename(tt.route); got != tt.want {
			t.Errorf("titleFromFilename(%q) = %q, want %q", tt.route, got, tt.want)
		}
	}
}
