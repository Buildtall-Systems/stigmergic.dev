package markdown

import (
	"testing"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

func TestExtractOutline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		want   []models.OutlineEntry
	}{
		{
			name:   "nested heading levels",
			source: "# Title\n\n## Section\n\n### Detail\n\n## Another\n",
			want: []models.OutlineEntry{
				{Level: 1, Text: "Title", ID: "title"},
				{Level: 2, Text: "Section", ID: "section"},
				{Level: 3, Text: "Detail", ID: "detail"},
				{Level: 2, Text: "Another", ID: "another"},
			},
		},
		{
			name:   "duplicate heading slugs get suffixed",
			source: "## Setup\n\ntext\n\n## Setup\n",
			want: []models.OutlineEntry{
				{Level: 2, Text: "Setup", ID: "setup"},
				{Level: 2, Text: "Setup", ID: "setup-1"},
			},
		},
		{
			name:   "setext headings",
			source: "Top Level\n=========\n\nSecond Level\n------------\n",
			want: []models.OutlineEntry{
				{Level: 1, Text: "Top Level", ID: "top-level"},
				{Level: 2, Text: "Second Level", ID: "second-level"},
			},
		},
		{
			name:   "empty document",
			source: "",
			want:   nil,
		},
		{
			name:   "no headings",
			source: "just a paragraph\n\nand another\n",
			want:   nil,
		},
		{
			name:   "inline formatting flattened",
			source: "## Using `htmx.ajax` with *care* and [links](https://example.com)\n",
			want: []models.OutlineEntry{
				// The id slug includes the link destination — goldmark
				// AutoHeadingID behavior, matched by the rendered HTML.
				{Level: 2, Text: "Using htmx.ajax with care and links", ID: "using-htmxajax-with-care-and-linkshttpsexamplecom"},
			},
		},
		{
			name:   "frontmatter is not a heading",
			source: "---\ntitle: Document\n---\n\n## Real Heading\n",
			want: []models.OutlineEntry{
				{Level: 2, Text: "Real Heading", ID: "real-heading"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ExtractOutline([]byte(tt.source))
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d entries, got %d: %+v", len(tt.want), len(got), got)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("entry %d: expected %+v, got %+v", i, want, got[i])
				}
			}
		})
	}
}
