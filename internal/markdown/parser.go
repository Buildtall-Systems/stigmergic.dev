package markdown

import (
	"bytes"

	"github.com/alecthomas/chroma/v2/formatters/html"
	nostr "github.com/github-tijlxyz/goldmark-nostr"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/wikilink"
)

// Parse converts markdown source to HTML.
// When resolver is non-nil, wiki-style [[links]] are parsed and resolved.
func Parse(source []byte, resolver wikilink.Resolver) ([]byte, error) {
	extensions := []goldmark.Extender{
		extension.GFM,
		highlighting.NewHighlighting(
			highlighting.WithStyle("nord"),
			highlighting.WithFormatOptions(
				html.WithLineNumbers(true),
			),
		),
		&mermaid.Extender{
			RenderMode: mermaid.RenderModeClient,
			NoScript:   true,
		},
		nostr.New(nostr.WithNostrLink("nostr:%s")),
	}

	parserOpts := []parser.Option{
		parser.WithAutoHeadingID(),
	}

	rendererOpts := []renderer.Option{
		gmhtml.WithUnsafe(),
		gmhtml.WithHardWraps(),
	}

	if resolver != nil {
		parserOpts = append(parserOpts,
			parser.WithInlineParsers(util.Prioritized(&wikilink.Parser{}, 199)),
		)
		rendererOpts = append(rendererOpts,
			renderer.WithNodeRenderers(util.Prioritized(&WikilinkRenderer{Resolver: resolver}, 199)),
		)
	}

	md := goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(parserOpts...),
		goldmark.WithRendererOptions(rendererOpts...),
	)

	var buf bytes.Buffer
	if err := md.Convert(source, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
