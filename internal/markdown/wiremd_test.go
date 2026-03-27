package markdown

import (
	"strings"
	"testing"
)

func TestWiremdFencedCodeBlock(t *testing.T) {
	t.Parallel()

	input := "```wiremd\n## Contact Form\n[Submit]{.primary}\n```"
	result, _, err := Parse([]byte(input), nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if !strings.Contains(output, "<pre class=\"wiremd\">") {
		t.Errorf("Expected <pre class=\"wiremd\">, got:\n%s", output)
	}
	if !strings.Contains(output, "## Contact Form") {
		t.Errorf("Expected wiremd source content preserved, got:\n%s", output)
	}
	if !strings.Contains(output, "[Submit]{.primary}") {
		t.Errorf("Expected wiremd source content preserved, got:\n%s", output)
	}
}

func TestWiremdHTMLEscaping(t *testing.T) {
	t.Parallel()

	input := "```wiremd\n<script>alert('xss')</script>\n```"
	result, _, err := Parse([]byte(input), nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if strings.Contains(output, "<script>") {
		t.Errorf("Expected HTML special characters to be escaped, got:\n%s", output)
	}
	if !strings.Contains(output, "&lt;script&gt;") {
		t.Errorf("Expected escaped <script> tag, got:\n%s", output)
	}
}

func TestWiremdDoesNotAffectOtherCodeBlocks(t *testing.T) {
	t.Parallel()

	input := "```go\nfunc main() {}\n```\n\n```wiremd\n## Title\n```\n\n```mermaid\ngraph TD\nA-->B\n```"
	result, _, err := Parse([]byte(input), nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if !strings.Contains(output, "<pre style=") {
		t.Error("Expected syntax-highlighted Go code block")
	}
	if !strings.Contains(output, "<pre class=\"wiremd\">") {
		t.Error("Expected wiremd block")
	}
	if !strings.Contains(output, "<pre class=\"mermaid\">") {
		t.Error("Expected mermaid block")
	}
}

func TestWiremdPreservesSourceContent(t *testing.T) {
	t.Parallel()

	input := "```wiremd\n## Login Page\n[_____] Username\n[_____] Password\n[Login]{.primary}\n```"
	result, _, err := Parse([]byte(input), nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	output := string(result)
	if !strings.Contains(output, "## Login Page") {
		t.Error("Expected wiremd source preserved in output")
	}
	if !strings.Contains(output, "[Login]{.primary}") {
		t.Error("Expected wiremd source preserved in output")
	}
}
