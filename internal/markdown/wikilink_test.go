package markdown

import (
	"strings"
	"testing"

	"go.abhg.dev/goldmark/wikilink"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

func testFiles() []models.SearchableFile {
	return []models.SearchableFile{
		{Name: "Simple Note.md", Path: "/Simple Note.md"},
		{Name: "note.md", Path: "/folder/note.md"},
		{Name: "Deep Note.md", Path: "/a/b/c/Deep Note.md"},
		{Name: "Deep Note.md", Path: "/x/Deep Note.md"},
		{Name: "kebab-case.md", Path: "/kebab-case.md"},
		{Name: "under_score.md", Path: "/under_score.md"},
		{Name: "UPPER.md", Path: "/docs/UPPER.md"},
	}
}

func TestTreeResolverPathMatch(t *testing.T) {
	t.Parallel()

	r := NewTreeResolver(testFiles())

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "exact path with extension",
			target: "Simple Note.md",
			want:   "/file/Simple Note.md",
		},
		{
			name:   "exact path without extension",
			target: "Simple Note",
			want:   "/file/Simple Note.md",
		},
		{
			name:   "full path match",
			target: "folder/note",
			want:   "/file/folder/note.md",
		},
		{
			name:   "case insensitive path",
			target: "docs/upper",
			want:   "/file/docs/UPPER.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dest := resolveTarget(r, tt.target)
			if dest != tt.want {
				t.Errorf("got %q, want %q", dest, tt.want)
			}
		})
	}
}

func TestTreeResolverNameMatch(t *testing.T) {
	t.Parallel()

	r := NewTreeResolver(testFiles())

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "filename only",
			target: "note",
			want:   "/file/folder/note.md",
		},
		{
			name:   "case insensitive name",
			target: "UPPER",
			want:   "/file/docs/UPPER.md",
		},
		{
			name:   "separator normalization spaces to hyphens",
			target: "kebab case",
			want:   "/file/kebab-case.md",
		},
		{
			name:   "separator normalization underscores to hyphens",
			target: "under score",
			want:   "/file/under_score.md",
		},
		{
			name:   "duplicate filenames shortest path wins",
			target: "Deep Note",
			want:   "/file/x/Deep Note.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dest := resolveTarget(r, tt.target)
			if dest != tt.want {
				t.Errorf("got %q, want %q", dest, tt.want)
			}
		})
	}
}

func TestTreeResolverUnresolved(t *testing.T) {
	t.Parallel()

	r := NewTreeResolver(testFiles())
	dest := resolveTarget(r, "Nonexistent")
	if dest != "" {
		t.Errorf("expected empty destination for unresolved link, got %q", dest)
	}
}

func TestTreeResolverFragment(t *testing.T) {
	t.Parallel()

	r := NewTreeResolver(testFiles())

	t.Run("self-link fragment only", func(t *testing.T) {
		t.Parallel()
		dest := resolveWithFragment(r, "", "Section")
		if dest != "#Section" {
			t.Errorf("got %q, want %q", dest, "#Section")
		}
	})

	t.Run("page with fragment", func(t *testing.T) {
		t.Parallel()
		dest := resolveWithFragment(r, "Simple Note", "heading")
		if dest != "/file/Simple Note.md#heading" {
			t.Errorf("got %q, want %q", dest, "/file/Simple Note.md#heading")
		}
	})
}

// Integration tests: full markdown render pipeline.

func TestWikilinkIntegrationResolved(t *testing.T) {
	t.Parallel()

	r := NewTreeResolver(testFiles())
	html, _, err := Parse([]byte("See [[Simple Note]] for details."), r, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(html)
	if !strings.Contains(output, `<a href="/file/Simple%20Note.md">`) {
		t.Errorf("expected resolved link in output:\n%s", output)
	}
	if !strings.Contains(output, "Simple Note</a>") {
		t.Errorf("expected link text in output:\n%s", output)
	}
}

func TestWikilinkIntegrationUnresolved(t *testing.T) {
	t.Parallel()

	r := NewTreeResolver(testFiles())
	html, _, err := Parse([]byte("See [[Missing Page]] for details."), r, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(html)
	if !strings.Contains(output, `<span class="wikilink-unresolved">`) {
		t.Errorf("expected unresolved span in output:\n%s", output)
	}
	if !strings.Contains(output, "Missing Page</span>") {
		t.Errorf("expected unresolved text in output:\n%s", output)
	}
}

func TestWikilinkIntegrationFragment(t *testing.T) {
	t.Parallel()

	r := NewTreeResolver(testFiles())

	t.Run("page with heading fragment", func(t *testing.T) {
		t.Parallel()
		html, _, err := Parse([]byte("See [[Simple Note#Overview]] for details."), r, nil)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		output := string(html)
		if !strings.Contains(output, `/file/Simple%20Note.md#Overview">`) {
			t.Errorf("expected href with fragment in output:\n%s", output)
		}
		if !strings.Contains(output, "</a>") {
			t.Errorf("expected closing anchor tag in output:\n%s", output)
		}
	})

	t.Run("self-link fragment", func(t *testing.T) {
		t.Parallel()
		html, _, err := Parse([]byte("Jump to [[#Section]] below."), r, nil)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		output := string(html)
		if !strings.Contains(output, `href="#Section"`) {
			t.Errorf("expected self-link fragment href in output:\n%s", output)
		}
	})
}

func TestWikilinkIntegrationAlias(t *testing.T) {
	t.Parallel()

	r := NewTreeResolver(testFiles())
	html, _, err := Parse([]byte("See [[Simple Note|my note]] for details."), r, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(html)
	if !strings.Contains(output, `<a href="/file/Simple%20Note.md">`) {
		t.Errorf("expected resolved link in output:\n%s", output)
	}
	if !strings.Contains(output, "my note</a>") {
		t.Errorf("expected alias text in output:\n%s", output)
	}
}

func TestWikilinkIntegrationMixed(t *testing.T) {
	t.Parallel()

	r := NewTreeResolver(testFiles())
	input := `# Heading

Some text with [[Simple Note]] and **bold**.

- List item with [[Missing]]
- Another item

> Blockquote with [[folder/note]]`

	html, _, err := Parse([]byte(input), r, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(html)
	expectations := []string{
		`<a href="/file/Simple%20Note.md">`,
		`<span class="wikilink-unresolved">`,
		`<a href="/file/folder/note.md">`,
		"<h1",
		boldHTML,
		"<li>",
		"<blockquote>",
	}

	for _, expected := range expectations {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q\nGot:\n%s", expected, output)
		}
	}
}

func TestWikilinkIntegrationNilResolver(t *testing.T) {
	t.Parallel()

	html, _, err := Parse([]byte("Text with [[brackets]] here."), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(html)
	if strings.Contains(output, "<a href") {
		t.Errorf("expected no link with nil resolver:\n%s", output)
	}
	if !strings.Contains(output, "[[brackets]]") {
		t.Errorf("expected literal brackets with nil resolver:\n%s", output)
	}
}

// Test helpers

func resolveTarget(r *TreeResolver, target string) string {
	return resolveWithFragment(r, target, "")
}

func resolveWithFragment(r *TreeResolver, target, fragment string) string {
	n := &wikilink.Node{
		Target:   []byte(target),
		Fragment: []byte(fragment),
	}
	dest, err := r.ResolveWikilink(n)
	if err != nil {
		panic(err)
	}
	return string(dest)
}
