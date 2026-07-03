package markdown

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/frontmatter"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

// ExtractOutline parses source and collects its headings in document order.
// The parser mirrors Parse's heading-affecting configuration — AutoHeadingID
// for anchor ids and the frontmatter extender so a metadata block is not
// misread as a setext heading — which keeps ids identical to the rendered
// HTML. Returns nil for documents without headings.
func ExtractOutline(source []byte) []models.OutlineEntry {
	md := goldmark.New(
		goldmark.WithExtensions(&frontmatter.Extender{}),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)

	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	var outline []models.OutlineEntry
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		id, ok := heading.AttributeString("id")
		if !ok {
			return ast.WalkContinue, nil
		}
		idBytes, ok := id.([]byte)
		if !ok {
			return ast.WalkContinue, nil
		}
		outline = append(outline, models.OutlineEntry{
			Level: heading.Level,
			Text:  string(appendInlineText(nil, heading, source)),
			ID:    string(idBytes),
		})
		return ast.WalkContinue, nil
	}); err != nil {
		return nil
	}
	return outline
}

// appendInlineText flattens a node's inline descendants to plain text,
// dropping formatting (emphasis, code spans, links) but keeping their
// textual content.
func appendInlineText(buf []byte, n ast.Node, source []byte) []byte {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			buf = append(buf, t.Segment.Value(source)...)
		case *ast.String:
			buf = append(buf, t.Value...)
		default:
			buf = appendInlineText(buf, c, source)
		}
	}
	return buf
}
