package markdown

import (
	"bytes"

	"github.com/alecthomas/chroma/v2/formatters/html"
	nostr "github.com/github-tijlxyz/goldmark-nostr"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/mermaid"
)

func Parse(source []byte) ([]byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				highlighting.WithStyle("nord"),
				highlighting.WithFormatOptions(
					html.WithLineNumbers(true),
				),
			),
			&mermaid.Extender{
				RenderMode: mermaid.RenderModeClient,
			},
			nostr.New(nostr.WithNostrLink("nostr:%s")),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			gmhtml.WithUnsafe(),
			gmhtml.WithHardWraps(),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert(source, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
