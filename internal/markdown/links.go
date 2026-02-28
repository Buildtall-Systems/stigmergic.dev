package markdown

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// LinkRenderer adds target="_blank" and rel="noopener noreferrer"
// to external links (http:// and https://) in rendered markdown.
// Internal and relative links render without these attributes.
type LinkRenderer struct{}

// RegisterFuncs registers the renderer for link and autolink nodes.
func (r *LinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
}

func isExternalURL(dest []byte) bool {
	return bytes.HasPrefix(dest, []byte("http://")) ||
		bytes.HasPrefix(dest, []byte("https://"))
}

func (r *LinkRenderer) renderLink(
	w util.BufWriter, source []byte, node ast.Node, entering bool,
) (ast.WalkStatus, error) {
	n, ok := node.(*ast.Link)
	if !ok {
		return ast.WalkContinue, nil
	}
	if !entering {
		_, _ = w.WriteString("</a>")
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(`<a href="`)
	_, _ = w.Write(util.URLEscape(n.Destination, true))
	_, _ = w.WriteString(`"`)
	if n.Title != nil {
		_, _ = w.WriteString(` title="`)
		_, _ = w.Write(util.EscapeHTML(n.Title))
		_, _ = w.WriteString(`"`)
	}
	if isExternalURL(n.Destination) {
		_, _ = w.WriteString(` target="_blank" rel="noopener noreferrer"`)
	}
	_, _ = w.WriteString(`>`)
	return ast.WalkContinue, nil
}

func (r *LinkRenderer) renderAutoLink(
	w util.BufWriter, source []byte, node ast.Node, entering bool,
) (ast.WalkStatus, error) {
	n, ok := node.(*ast.AutoLink)
	if !ok {
		return ast.WalkContinue, nil
	}
	if !entering {
		_, _ = w.WriteString("</a>")
		return ast.WalkContinue, nil
	}

	url := n.URL(source)
	label := n.Label(source)

	_, _ = w.WriteString(`<a href="`)
	if n.AutoLinkType == ast.AutoLinkEmail &&
		!bytes.HasPrefix(bytes.ToLower(url), []byte("mailto:")) {
		_, _ = w.WriteString("mailto:")
	}
	_, _ = w.Write(util.URLEscape(url, false))
	_, _ = w.WriteString(`"`)
	if isExternalURL(url) {
		_, _ = w.WriteString(` target="_blank" rel="noopener noreferrer"`)
	}
	_, _ = w.WriteString(`>`)
	_, _ = w.Write(util.EscapeHTML(label))
	return ast.WalkContinue, nil
}
