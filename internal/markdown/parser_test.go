package markdown

import (
	"strings"
	"testing"
)

const (
	boldHTML          = "<strong>bold</strong>"
	preClassFragment  = `class="chroma"`
	spanClassFragment = `<span class="`
	spanStyleFragment = `<span style=`
)

func TestParseBasicMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "heading level 1",
			input:    "# Hello World",
			expected: "<h1 id=\"hello-world\">Hello World</h1>",
		},
		{
			name:     "heading level 2",
			input:    "## Subheading",
			expected: "<h2 id=\"subheading\">Subheading</h2>",
		},
		{
			name:     "paragraph",
			input:    "This is a paragraph.",
			expected: "<p>This is a paragraph.</p>",
		},
		{
			name:     "bold text",
			input:    "**bold**",
			expected: boldHTML,
		},
		{
			name:     "italic text",
			input:    "*italic*",
			expected: "<em>italic</em>",
		},
		{
			name:     "link",
			input:    "[text](https://example.com)",
			expected: "<a href=\"https://example.com\" target=\"_blank\" rel=\"noopener noreferrer\">text</a>",
		},
		{
			name:     "unordered list",
			input:    "- Item 1\n- Item 2",
			expected: "<ul>\n<li>Item 1</li>\n<li>Item 2</li>\n</ul>",
		},
		{
			name:     "ordered list",
			input:    "1. First\n2. Second",
			expected: "<ol>\n<li>First</li>\n<li>Second</li>\n</ol>",
		},
		{
			name:     "code block",
			input:    "```\ncode\n```",
			expected: "<pre><code>code\n</code></pre>",
		},
		{
			name:     "inline code",
			input:    "`code`",
			expected: "<code>code</code>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, _, err := Parse([]byte(tt.input), nil, nil)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			output := strings.TrimSpace(string(result))
			if !strings.Contains(output, tt.expected) {
				t.Errorf("\nInput:    %q\nExpected: %q\nGot:      %q", tt.input, tt.expected, output)
			}
		})
	}
}

func TestParseComplexMarkdown(t *testing.T) {
	t.Parallel()

	input := `# Main Title

This is a paragraph with **bold** and *italic* text.

## Section 1

- Item 1
- Item 2
  - Nested item

### Subsection

Link to [example](https://example.com).

` + "```go\nfunc main() {\n}\n```"

	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)

	expectations := []string{
		"<h1 id=\"main-title\">Main Title</h1>",
		"<h2 id=\"section-1\">Section 1</h2>",
		"<h3 id=\"subsection\">Subsection</h3>",
		boldHTML,
		"<em>italic</em>",
		"<a href=\"https://example.com\" target=\"_blank\" rel=\"noopener noreferrer\">example</a>",
		"<ul>",
		"<li>Item 1</li>",
		preClassFragment,
	}

	for _, expected := range expectations {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q", expected)
		}
	}
}

func TestParseEmptyInput(t *testing.T) {
	t.Parallel()

	result, _, err := Parse([]byte(""), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed on empty input: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty output, got: %q", string(result))
	}
}

func TestParseHTMLPassthrough(t *testing.T) {
	t.Parallel()

	input := `<div class="test">HTML content</div>`
	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if !strings.Contains(output, `<div class="test">`) {
		t.Error("Expected HTML to pass through with unsafe rendering enabled")
	}
}

func TestParseAutoHeadingIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple heading",
			input:    "# Hello World",
			expected: `id="hello-world"`,
		},
		{
			name:     "heading with special chars",
			input:    "## Test & Demo",
			expected: `id="test--demo"`,
		},
		{
			name:     "heading with numbers",
			input:    "### Step 123",
			expected: `id="step-123"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, _, err := Parse([]byte(tt.input), nil, nil)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			output := string(result)
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Expected ID %q in output:\n%s", tt.expected, output)
			}
		})
	}
}

func TestParseBlockquote(t *testing.T) {
	t.Parallel()

	input := "> This is a quote"
	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if !strings.Contains(output, "<blockquote>") {
		t.Error("Expected blockquote in output")
	}
	if !strings.Contains(output, "This is a quote") {
		t.Error("Expected quote text in output")
	}
}

func TestParseSyntaxHighlighting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "go code block",
			input: "```go\nfunc main() {\n}\n```",
			expected: []string{
				preClassFragment,
				spanClassFragment,
				">func</span>",
				">main</span>",
			},
		},
		{
			name:  "python code block",
			input: "```python\ndef hello():\n    pass\n```",
			expected: []string{
				preClassFragment,
				spanClassFragment,
				">def</span>",
				">hello</span>",
			},
		},
		{
			name:  "javascript code block",
			input: "```javascript\nconst x = 42;\n```",
			expected: []string{
				preClassFragment,
				spanClassFragment,
				">const</span>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, _, err := Parse([]byte(tt.input), nil, nil)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			output := string(result)
			if strings.Contains(output, spanStyleFragment) {
				t.Errorf("Expected class-based highlight output, found inline style %q", spanStyleFragment)
			}
			for _, expected := range tt.expected {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q\nGot:\n%s", expected, output)
				}
			}
		})
	}
}

func TestParseLineNumbers(t *testing.T) {
	t.Parallel()

	input := "```go\nline1\nline2\nline3\n```"
	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if !strings.Contains(output, `<span class="ln">`) {
		t.Error("Expected line number spans in output")
	}
}

func TestParseGFMTable(t *testing.T) {
	t.Parallel()

	input := `| Header 1 | Header 2 |
|----------|----------|
| Cell 1   | Cell 2   |`

	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	expectations := []string{
		"<table>",
		"<thead>",
		"<th>Header 1</th>",
		"<th>Header 2</th>",
		"<tbody>",
		"<td>Cell 1</td>",
		"<td>Cell 2</td>",
	}

	for _, expected := range expectations {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q\nGot:\n%s", expected, output)
		}
	}
}

func TestParseGFMStrikethrough(t *testing.T) {
	t.Parallel()

	input := "~~strikethrough~~"
	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if !strings.Contains(output, "<del>strikethrough</del>") {
		t.Errorf("Expected strikethrough in output\nGot:\n%s", output)
	}
}

func TestParseGFMTaskList(t *testing.T) {
	t.Parallel()

	input := `- [ ] Unchecked
- [x] Checked`

	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	expectations := []string{
		"<input",
		"type=\"checkbox\"",
		"disabled",
		"Unchecked",
		"Checked",
	}

	for _, expected := range expectations {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q\nGot:\n%s", expected, output)
		}
	}

	if !strings.Contains(output, "checked") {
		t.Error("Expected checked attribute for checked task")
	}
}

func TestParseGFMLinkify(t *testing.T) {
	t.Parallel()

	input := "Check out https://example.com for more info."
	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if !strings.Contains(output, "<a href=\"https://example.com\" target=\"_blank\" rel=\"noopener noreferrer\">https://example.com</a>") {
		t.Errorf("Expected auto-linked URL in output\nGot:\n%s", output)
	}
}

func TestParseExternalLinksNewTab(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "https link opens in new tab",
			input:    "[Go](https://go.dev/)",
			expected: `<a href="https://go.dev/" target="_blank" rel="noopener noreferrer">Go</a>`,
		},
		{
			name:     "http link opens in new tab",
			input:    "[old](http://example.com)",
			expected: `<a href="http://example.com" target="_blank" rel="noopener noreferrer">old</a>`,
		},
		{
			name:     "relative link stays in same tab",
			input:    "[readme](/file/readme.md)",
			expected: `<a href="/file/readme.md">readme</a>`,
		},
		{
			name:     "anchor link stays in same tab",
			input:    "[section](#overview)",
			expected: `<a href="#overview">section</a>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, _, err := Parse([]byte(tt.input), nil, nil)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			output := strings.TrimSpace(string(result))
			if !strings.Contains(output, tt.expected) {
				t.Errorf("\nInput:    %q\nExpected: %q\nGot:      %q", tt.input, tt.expected, output)
			}
		})
	}
}

func TestParseMermaidFlowchart(t *testing.T) {
	t.Parallel()

	input := "```mermaid\ngraph TD\nA-->B\n```"
	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	expectations := []string{
		"<pre class=\"mermaid\">",
		"graph TD",
	}

	for _, expected := range expectations {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q\nGot:\n%s", expected, output)
		}
	}
}

func TestParseMermaidSequenceDiagram(t *testing.T) {
	t.Parallel()

	input := "```mermaid\nsequenceDiagram\nAlice->>Bob: Hello\nBob->>Alice: Hi\n```"

	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	expectations := []string{
		"<pre class=\"mermaid\">",
		"sequenceDiagram",
	}

	for _, expected := range expectations {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q\nGot:\n%s", expected, output)
		}
	}
}

func TestParseNostrNpub(t *testing.T) {
	t.Parallel()

	input := "Follow me at nostr:npub1prtsd0e39unnacud7vzxwxec49xxau33xyq2lzuj3xpzfxg0z9wqjn0v8q for updates."
	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	expectations := []string{
		"<a href=\"nostr:npub1prtsd0e39unnacud7vzxwxec49xxau33xyq2lzuj3xpzfxg0z9wqjn0v8q\"",
		"nostr:npub1prtsd0e39unnacud7vzxwxec49xxau33xyq2lzuj3xpzfxg0z9wqjn0v8q",
	}

	for _, expected := range expectations {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q\nGot:\n%s", expected, output)
		}
	}
}

func TestParseNostrMultipleNpubs(t *testing.T) {
	t.Parallel()

	input := "Follow nostr:npub1r45pcpwtqsnp5t0pj8x7y95tse7k3t47pp4az889aqutwlh7dcsql04zht and nostr:npub139dkdt7rtcqn8wrxtemfjz8ah47l5c7fuxxqhfsepzcwvaqwqy9s34w8ju"
	result, _, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if !strings.Contains(output, "nostr:npub1r45pcpwtqsnp5t0pj8x7y95tse7k3t47pp4az889aqutwlh7dcsql04zht") {
		t.Error("Expected first nostr link in output")
	}
	if !strings.Contains(output, "nostr:npub139dkdt7rtcqn8wrxtemfjz8ah47l5c7fuxxqhfsepzcwvaqwqy9s34w8ju") {
		t.Error("Expected second nostr link in output")
	}
	if strings.Count(output, "<a href=\"nostr:") != 2 {
		t.Errorf("Expected exactly 2 nostr links, got %d", strings.Count(output, "<a href=\"nostr:"))
	}
}

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()

	input := `---
title: Test Note
tags: [nostr, go, lightning]
date: 2026-02-28
draft: false
---
# Test Note

Content here.`

	result, meta, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)

	// Frontmatter should be stripped from HTML output.
	if strings.Contains(output, "<hr>") {
		t.Error("Expected no <hr> from frontmatter delimiters in output")
	}
	if strings.Contains(output, "title: Test Note") {
		t.Error("Expected raw frontmatter text stripped from output")
	}

	// HTML should still have the content.
	if !strings.Contains(output, "<h1") {
		t.Error("Expected heading in output")
	}
	if !strings.Contains(output, "Content here.") {
		t.Error("Expected content in output")
	}

	// Metadata should be populated.
	if meta == nil {
		t.Fatal("Expected non-nil metadata")
	}
	if meta["title"] != "Test Note" {
		t.Errorf("Expected title 'Test Note', got %v", meta["title"])
	}
	if meta["draft"] != false {
		t.Errorf("Expected draft false, got %v", meta["draft"])
	}

	tags, ok := meta["tags"].([]any)
	if !ok {
		t.Fatalf("Expected tags to be []any, got %T", meta["tags"])
	}
	if len(tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(tags))
	}
}

func TestParseFrontmatterEmpty(t *testing.T) {
	t.Parallel()

	input := "# No Frontmatter\n\nJust content."

	result, meta, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if meta != nil {
		t.Errorf("Expected nil metadata for content without frontmatter, got %v", meta)
	}

	output := string(result)
	if !strings.Contains(output, "No Frontmatter") {
		t.Error("Expected content to render normally")
	}
}

func TestParseFrontmatterArrayValues(t *testing.T) {
	t.Parallel()

	input := `---
tags:
  - alpha
  - beta
  - gamma
---
Content.`

	_, meta, err := Parse([]byte(input), nil, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if meta == nil {
		t.Fatal("Expected non-nil metadata")
	}

	tags, ok := meta["tags"].([]any)
	if !ok {
		t.Fatalf("Expected tags to be []any, got %T", meta["tags"])
	}
	if len(tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(tags))
	}
	if tags[0] != "alpha" {
		t.Errorf("Expected first tag 'alpha', got %v", tags[0])
	}
}
