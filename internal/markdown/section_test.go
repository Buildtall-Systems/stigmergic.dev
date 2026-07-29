package markdown

import (
	"strings"
	"testing"
)

const (
	overviewDoc  = "## Overview\n\nsummary\n"
	overviewBody = "summary"
	keepMarker   = "keep"
	dropMarker   = "drop"
)

func TestExtractSection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		source    string
		fragment  string
		contains  []string
		excludes  []string
		wantFound bool
	}{
		{
			name:      "atx heading matched exactly keeps its marker",
			source:    "# Title\n\nintro\n\n### DNA information generates mechanism\n\nbody text\n\n### Next\n\nother\n",
			fragment:  "DNA information generates mechanism",
			wantFound: true,
			contains:  []string{"### DNA information generates mechanism", "body text"},
			excludes:  []string{"intro", "### Next", "other"},
		},
		{
			name:      "heading wholly wrapped in a wikilink",
			source:    "## [[03-02-23]]\n\njournal entry\n\n## [[03-03-23]]\n\nnext entry\n",
			fragment:  "03-02-23",
			wantFound: true,
			contains:  []string{"journal entry"},
			excludes:  []string{"next entry"},
		},
		{
			name:      "wikilink mid heading",
			source:    "### EP 168 [[Nate Hagens]] on Collective Futures\n\nshow notes\n\n### EP 169\n\nlater\n",
			fragment:  "EP 168 Nate Hagens on Collective Futures",
			wantFound: true,
			contains:  []string{"show notes"},
			excludes:  []string{"later"},
		},
		{
			name:      "fragment with surrounding whitespace",
			source:    overviewDoc,
			fragment:  "  Overview  ",
			wantFound: true,
			contains:  []string{overviewBody},
		},
		{
			name:      "heading with trailing whitespace",
			source:    "## Overview   \n\nsummary\n",
			fragment:  "Overview",
			wantFound: true,
			contains:  []string{overviewBody},
		},
		{
			name:      "descendant subsections are included",
			source:    "## Parent\n\nlead\n\n### Child\n\nnested\n\n#### Grandchild\n\ndeeper\n\n## Sibling\n\nout\n",
			fragment:  "Parent",
			wantFound: true,
			contains:  []string{"lead", "### Child", "nested", "#### Grandchild", "deeper"},
			excludes:  []string{"## Sibling", "out"},
		},
		{
			name:      "terminated by a same level heading",
			source:    "## First\n\nkeep\n\n## Second\n\ndrop\n",
			fragment:  "First",
			wantFound: true,
			contains:  []string{keepMarker},
			excludes:  []string{dropMarker},
		},
		{
			name:      "terminated by a higher level heading",
			source:    "# Doc\n\n### Deep\n\nkeep\n\n## Shallow\n\ndrop\n",
			fragment:  "Deep",
			wantFound: true,
			contains:  []string{keepMarker},
			excludes:  []string{dropMarker},
		},
		{
			name:      "last section runs to end of input",
			source:    "## First\n\ndrop\n\n## Last\n\nkeep to the end\n",
			fragment:  "Last",
			wantFound: true,
			contains:  []string{"keep to the end"},
			excludes:  []string{dropMarker},
		},
		{
			name:      "exact match wins over a longer sibling",
			source:    "#### floodlights\n\nthe short one\n\n#### on the floodlights\n\nthe long one\n",
			fragment:  "floodlights",
			wantFound: true,
			contains:  []string{"the short one"},
			excludes:  []string{"the long one"},
		},
		{
			name:      "longer sibling is reachable by its own text",
			source:    "#### floodlights\n\nthe short one\n\n#### on the floodlights\n\nthe long one\n",
			fragment:  "on the floodlights",
			wantFound: true,
			contains:  []string{"the long one"},
			excludes:  []string{"the short one"},
		},
		{
			name:      "case insensitive fallback",
			source:    overviewDoc,
			fragment:  "overview",
			wantFound: true,
			contains:  []string{overviewBody},
		},
		{
			name:      "setext heading keeps its underline",
			source:    "Chapter One\n===========\n\nkeep\n\nChapter Two\n-----------\n\nalso kept\n",
			fragment:  "Chapter One",
			wantFound: true,
			contains:  []string{"Chapter One", "===========", keepMarker, "Chapter Two", "also kept"},
		},
		{
			name:      "obsidian tag line is not a heading",
			source:    "---\ntitle: Paper\n---\n\n#paper #complexity\n\n## Real Heading\n\nbody\n",
			fragment:  "paper #complexity",
			wantFound: false,
		},
		{
			name:      "heading after an obsidian tag line still matches",
			source:    "---\ntitle: Paper\n---\n\n#paper #complexity\n\n## Real Heading\n\nbody\n",
			fragment:  "Real Heading",
			wantFound: true,
			contains:  []string{"## Real Heading", "body"},
			excludes:  []string{"#paper"},
		},
		{
			name:      "no such heading",
			source:    overviewDoc,
			fragment:  "Nonexistent",
			wantFound: false,
		},
		{
			name:      "empty fragment",
			source:    overviewDoc,
			fragment:  "",
			wantFound: false,
		},
		{
			name:      "document without headings",
			source:    "just a paragraph\n\nand another\n",
			fragment:  "Overview",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, found := ExtractSection([]byte(tt.source), tt.fragment)
			if found != tt.wantFound {
				t.Fatalf("expected found %v, got %v (section %q)", tt.wantFound, found, got)
			}
			if !found {
				return
			}
			section := string(got)
			for _, want := range tt.contains {
				if !strings.Contains(section, want) {
					t.Errorf("expected section to contain %q, got %q", want, section)
				}
			}
			for _, unwanted := range tt.excludes {
				if strings.Contains(section, unwanted) {
					t.Errorf("expected section to exclude %q, got %q", unwanted, section)
				}
			}
		})
	}
}

func TestClassifyEmbedTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   EmbedTargetKind
	}{
		{name: "bare note name", target: "lifes irreducible structure", want: EmbedTargetNote},
		{name: "explicit markdown extension", target: "reading/papers/note.md", want: EmbedTargetNote},
		{name: "png image", target: testImageName, want: EmbedTargetImage},
		{name: "uppercase extension", target: "IMG_0001.PNG", want: EmbedTargetImage},
		{name: "svg image", target: "diagram.svg", want: EmbedTargetImage},
		{name: "pdf attachment", target: "The Kekule Problem - Nautilus.pdf", want: EmbedTargetAttachment},
		{name: "audio attachment", target: "episode.mp3", want: EmbedTargetAttachment},
		{name: "video attachment", target: "clip.mp4", want: EmbedTargetAttachment},
		{name: "dotted note title is not an extension", target: "v1.2 release", want: EmbedTargetNote},
		{name: "abbreviated note title is not an extension", target: "Ch. 3 notes", want: EmbedTargetNote},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ClassifyEmbedTarget(tt.target); got != tt.want {
				t.Errorf("expected kind %d, got %d", tt.want, got)
			}
		})
	}
}
