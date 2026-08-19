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
	"go.abhg.dev/goldmark/frontmatter"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/wikilink"
)

// Parse converts markdown source to HTML and extracts any YAML/TOML frontmatter.
// When resolver is non-nil, wiki-style [[links]] are parsed and resolved.
// When embeds is non-nil, an embed link (![[note#section]]) standing alone in a
// block transcludes its target; a nil embeds reproduces the pre-transclusion
// output byte for byte, mirroring the nil-resolver idiom.
// Returns rendered HTML, parsed frontmatter metadata (nil if none), and any error.
func Parse(source []byte, resolver wikilink.Resolver, embeds *EmbedContext) ([]byte, map[string]any, error) {
	extensions := []goldmark.Extender{
		extension.GFM,
		NewWiremdExtension(),
		highlighting.NewHighlighting(
			highlighting.WithFormatOptions(
				html.WithClasses(true),
				html.WithLineNumbers(true),
			),
		),
		&mermaid.Extender{
			RenderMode: mermaid.RenderModeClient,
			NoScript:   true,
		},
		nostr.New(nostr.WithNostrLink("nostr:%s")),
		&frontmatter.Extender{},
	}

	var parserOpts []parser.Option

	// Transcluded content omits AutoHeadingID. Its headings would otherwise
	// collide with the host's ids and capture the outline rail's scrollspy
	// anchors, and the rail lists the host's own headings only.
	if !embeds.nested() {
		parserOpts = append(parserOpts, parser.WithAutoHeadingID())
	}

	rendererOpts := []renderer.Option{
		gmhtml.WithUnsafe(),
		gmhtml.WithHardWraps(),
		renderer.WithNodeRenderers(util.Prioritized(&LinkRenderer{}, 500)),
	}

	if resolver != nil {
		parserOpts = append(parserOpts,
			parser.WithInlineParsers(util.Prioritized(&wikilink.Parser{}, 199)),
		)
		rendererOpts = append(rendererOpts,
			renderer.WithNodeRenderers(util.Prioritized(&WikilinkRenderer{Resolver: resolver}, 199)),
		)

		// Plain CommonMark links and images bypass wikilink syntax
		// entirely, so a resolver that can answer them is consulted
		// through the AST: the destination a node carries is rewritten
		// before rendering, leaving goldmark's own markup rules the only
		// copy of themselves.
		if routes, ok := resolver.(RouteResolver); ok {
			parserOpts = append(parserOpts,
				parser.WithASTTransformers(util.Prioritized(&routeTransformer{routes: routes}, 200)),
			)
		}

		// Transclusion needs the wikilink parser to see an embed at all,
		// so it is registered only alongside the resolver it also uses to
		// turn a target into a route.
		if embeds != nil {
			parserOpts = append(parserOpts,
				parser.WithASTTransformers(util.Prioritized(&embedTransformer{}, 100)),
			)
			rendererOpts = append(rendererOpts,
				renderer.WithNodeRenderers(util.Prioritized(&embedRenderer{ctx: embeds, resolver: resolver}, 100)),
			)
		}
	}

	md := goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(parserOpts...),
		goldmark.WithRendererOptions(rendererOpts...),
	)

	ctx := parser.NewContext()
	var buf bytes.Buffer
	if err := md.Convert(source, &buf, parser.WithContext(ctx)); err != nil {
		return nil, nil, err
	}

	var meta map[string]any
	if fm := frontmatter.Get(ctx); fm != nil {
		if err := fm.Decode(&meta); err != nil {
			return nil, nil, err
		}
	}

	return buf.Bytes(), meta, nil
}
