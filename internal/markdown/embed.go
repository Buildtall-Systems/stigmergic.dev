package markdown

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/wikilink"
)

// MaxEmbedDepth bounds how many levels of transcluded content a single page
// renders. The host document is depth zero, so the cap admits three nested
// layers before a marker stands in for further recursion. It exists as a
// backstop for depth alone: a genuine cycle is caught by the visited set,
// which terminates it sooner and reports it as a cycle rather than as depth.
const MaxEmbedDepth = 3

// KindEmbedBlock is the node kind for a wiki-link embed promoted out of its
// containing block.
var KindEmbedBlock = ast.NewNodeKind("EmbedBlock")

// embedBlock is a block-level wiki-link embed.
//
// Promotion is mandatory rather than stylistic: wikilink.Node embeds
// ast.BaseInline, while the content it transcludes is block content. Rendering
// a paragraph inside a <p> would be repaired by the browser in a way that
// corrupts the surrounding document.
type embedBlock struct {
	target   string
	fragment string
	label    string

	ast.BaseBlock
}

func (n *embedBlock) Kind() ast.NodeKind {
	return KindEmbedBlock
}

func (n *embedBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Target":   n.target,
		"Fragment": n.fragment,
	}, nil)
}

// EmbedContext carries the state one page render needs to transclude: where to
// read content from, how deep the current render sits, and which targets are
// already open on the path to here.
//
// It is mutated during rendering and is not safe for concurrent use. Construct
// one per request.
type EmbedContext struct {
	source  EmbedSource
	visited map[string]struct{}
	depth   int
}

// NewEmbedContext builds a context reading through source.
func NewEmbedContext(source EmbedSource) *EmbedContext {
	return &EmbedContext{
		source:  source,
		visited: make(map[string]struct{}),
	}
}

// nested reports whether this render sits inside a transclusion. Nested
// renders omit AutoHeadingID, which is what keeps transcluded headings from
// colliding with host heading ids or capturing the outline rail's scrollspy
// anchors.
func (c *EmbedContext) nested() bool {
	return c != nil && c.depth > 0
}

// embedTransformer replaces a block whose sole content is an embed with an
// embedBlock, following the same shape as wiremdTransformer.
type embedTransformer struct{}

// Transform promotes standalone embeds.
//
// Both ast.Paragraph and ast.TextBlock are accepted as containers. A tight
// list item wraps its content in a TextBlock rather than a Paragraph, and an
// embed that is the sole content of a list item is an ordinary form in a
// vault; handling only Paragraph drops it silently.
//
// An embed with text beside it is left alone and reaches WikilinkRenderer as
// an inline anchor.
func (t *embedTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var containers []ast.Node

	if err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node.(type) {
		case *ast.Paragraph, *ast.TextBlock:
		default:
			return ast.WalkContinue, nil
		}
		if soleEmbed(node) == nil {
			return ast.WalkContinue, nil
		}
		containers = append(containers, node)
		return ast.WalkContinue, nil
	}); err != nil {
		return
	}

	for _, container := range containers {
		wn := soleEmbed(container)
		if wn == nil {
			continue
		}
		block := &embedBlock{
			target:   string(wn.Target),
			fragment: string(wn.Fragment),
			label:    string(appendInlineText(nil, wn, source)),
		}
		container.Parent().ReplaceChild(container.Parent(), container, block)
	}
}

// soleEmbed returns the embed node when it is the only thing in container,
// and nil otherwise.
func soleEmbed(container ast.Node) *wikilink.Node {
	if container.ChildCount() != 1 {
		return nil
	}
	wn, ok := container.FirstChild().(*wikilink.Node)
	if !ok || !wn.Embed {
		return nil
	}
	return wn
}

// embedRenderer renders promoted embeds by resolving the target, slicing the
// requested section out of its raw markdown, and rendering that slice
// recursively.
type embedRenderer struct {
	ctx      *EmbedContext
	resolver wikilink.Resolver
}

// RegisterFuncs registers the renderer for embed blocks.
func (r *embedRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindEmbedBlock, r.render)
}

func (r *embedRenderer) render(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n, ok := node.(*embedBlock)
	if !ok {
		return ast.WalkStop, fmt.Errorf("unexpected node %T, expected *embedBlock", node)
	}
	if err := r.renderEmbed(w, n); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}

// renderEmbed writes one embed. Every failure path writes a visible marker
// rather than returning an error: roughly half the link targets in a real
// vault are dangling, so an unresolved embed is an ordinary outcome and not a
// reason to fail a page.
func (r *embedRenderer) renderEmbed(w util.BufWriter, n *embedBlock) error {
	switch ClassifyEmbedTarget(n.target) {
	case EmbedTargetImage:
		return r.renderAsset(w, n, true)
	case EmbedTargetAttachment:
		return r.renderAsset(w, n, false)
	case EmbedTargetNote:
	}

	// An empty target names a section of the host document, as in
	// ![[#Overview]]. Resolution has no route to offer for it, so it lands
	// on the unresolved marker deliberately rather than by oversight.
	route, ok := r.resolveRoute(n.target)
	if !ok {
		return writeEmbedMarker(w, n, embedErrUnresolved)
	}

	key := route + "#" + n.fragment
	if _, open := r.ctx.visited[key]; open {
		return writeEmbedMarker(w, n, embedErrCycle)
	}
	if r.ctx.depth >= MaxEmbedDepth {
		return writeEmbedMarker(w, n, embedErrDepth)
	}

	content, ok := r.ctx.source.NoteSource(route)
	if !ok {
		return writeEmbedMarker(w, n, embedErrUnresolved)
	}

	if n.fragment != "" {
		section, found := ExtractSection(content, n.fragment)
		if !found {
			return writeEmbedMarker(w, n, embedErrNoSection)
		}
		content = section
	}

	// The visited key is popped on the way out so the guard tracks the path
	// to here rather than the whole page. Two sibling embeds of the same
	// section are not a cycle and both render.
	r.ctx.visited[key] = struct{}{}
	r.ctx.depth++
	inner, _, err := Parse(content, r.resolver, r.ctx)
	r.ctx.depth--
	delete(r.ctx.visited, key)
	if err != nil {
		return writeEmbedMarker(w, n, embedErrUnresolved)
	}

	h := &embedWriter{w: w}
	h.str(`<div class="transclusion" data-embed-route="`)
	h.escaped(route)
	h.str(`">`)
	h.str(`<div class="transclusion-body">`)
	h.bytes(inner)
	h.str(`</div>`)
	h.str(`<a class="transclusion-source" href="/file/`)
	h.bytes(util.URLEscape([]byte(route), true))
	h.str(`">`)
	h.escaped(embedDisplayLabel(n))
	h.str(`</a></div>` + "\n")
	return h.err
}

// renderAsset writes an embed whose target is not markdown. The asset is
// found by probing the filesystem rather than by lookup, which is what keeps
// the scanner, the tree, the sidebar, and the search corpus free of binary
// entries. Its bytes are already served by the non-markdown branch of
// handleMarkdown, so the route is all that is needed here.
func (r *embedRenderer) renderAsset(w util.BufWriter, n *embedBlock, asImage bool) error {
	route, ok := r.ctx.source.ProbeAsset(n.target)
	if !ok {
		return writeEmbedMarker(w, n, embedErrUnresolved)
	}

	h := &embedWriter{w: w}
	if !asImage {
		h.str(`<a class="attachment" href="/file/`)
		h.bytes(util.URLEscape([]byte(route), true))
		h.str(`">`)
		h.escaped(embedDisplayLabel(n))
		h.str(`</a>` + "\n")
		return h.err
	}

	h.str(`<img src="/file/`)
	h.bytes(util.URLEscape([]byte(route), true))
	// The label becomes alt text only when the author wrote one, matching
	// the rule the wikilink package's own image renderer follows: ![[x.png]]
	// must not become alt="x.png", while ![[x.png|a diagram]] does.
	if n.label != "" && n.label != n.target {
		h.str(`" alt="`)
		h.escaped(n.label)
	}
	h.str(`">` + "\n")
	return h.err
}

// resolveRoute turns an embed target into a content-root relative path by
// asking the same resolver ordinary wikilinks use. The node carries no
// fragment, so the resolver returns a bare route and no anchor has to be
// trimmed back off.
func (r *embedRenderer) resolveRoute(target string) (string, bool) {
	if r.resolver == nil || target == "" {
		return "", false
	}
	dest, err := r.resolver.ResolveWikilink(&wikilink.Node{Target: []byte(target), Embed: true})
	if err != nil || len(dest) == 0 {
		return "", false
	}
	route, found := strings.CutPrefix(string(dest), "/file/")
	if !found {
		return "", false
	}
	return route, true
}

// Reasons an embed rendered a marker instead of content. They reach the
// browser as a data attribute so styling and tests can tell them apart while
// the visible text stays the same in every case.
const (
	embedErrUnresolved = "unresolved"
	embedErrNoSection  = "no-section"
	embedErrCycle      = "cycle"
	embedErrDepth      = "depth"
)

// writeEmbedMarker renders an embed that produced no content, echoing the
// link as written and reusing the wikilink-unresolved styling vocabulary.
func writeEmbedMarker(w util.BufWriter, n *embedBlock, reason string) error {
	h := &embedWriter{w: w}
	h.str(`<div class="transclusion transclusion-unresolved" data-embed-error="`)
	h.escaped(reason)
	h.str(`"><span class="wikilink-unresolved">![[`)
	h.escaped(n.target)
	if n.fragment != "" {
		h.str("#")
		h.escaped(n.fragment)
	}
	h.str(`]]</span></div>` + "\n")
	return h.err
}

// embedDisplayLabel is the text of the link back to the transcluded note. A
// label the author did not write is the target itself, which reads better than
// repeating a path.
func embedDisplayLabel(n *embedBlock) string {
	if n.label != "" {
		return n.label
	}
	return n.target
}

// embedWriter accumulates the first write error so a long run of markup does
// not need a check between every fragment.
type embedWriter struct {
	w   util.BufWriter
	err error
}

func (h *embedWriter) str(s string) {
	if h.err == nil {
		_, h.err = h.w.WriteString(s)
	}
}

func (h *embedWriter) bytes(b []byte) {
	if h.err == nil {
		_, h.err = h.w.Write(b)
	}
}

func (h *embedWriter) escaped(s string) {
	h.bytes(util.EscapeHTML([]byte(s)))
}
