package theme

import (
	"bytes"
	"fmt"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
)

// scopedChromaCSS renders the chroma stylesheet for the named style with
// every rule's selector prefixed by scope. WriteCSS emits one rule per line,
// each selector rooted at a class, so prefixing at the first '.' past any
// leading comment covers the whole sheet. A trailing override pins code
// block colors to the theme palette instead of the chroma style's own.
func scopedChromaCSS(styleName, scope string) (string, error) {
	style, ok := styles.Registry[styleName]
	if !ok {
		return "", fmt.Errorf("unknown chroma style %q", styleName)
	}

	formatter := chromahtml.New(chromahtml.WithClasses(true), chromahtml.WithLineNumbers(true))
	var buf bytes.Buffer
	if err := formatter.WriteCSS(&buf, style); err != nil {
		return "", fmt.Errorf("failed to generate chroma CSS for style %q: %w", styleName, err)
	}

	lines := strings.Split(buf.String(), "\n")
	for i, line := range lines {
		start := 0
		if end := strings.Index(line, "*/"); end != -1 {
			start = end + 2
		}
		dot := strings.Index(line[start:], ".")
		if dot == -1 {
			continue
		}
		pos := start + dot
		lines[i] = line[:pos] + scope + " " + line[pos:]
	}

	override := fmt.Sprintf("%s .chroma { color: var(--code-fg-color); background-color: var(--code-bg-color); }", scope)
	return strings.Join(lines, "\n") + "\n" + override + "\n", nil
}
