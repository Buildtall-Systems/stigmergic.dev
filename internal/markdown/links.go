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
		if _, err := w.WriteString("</a>"); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	if _, err := w.WriteString(`<a href="`); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.Write(util.URLEscape(n.Destination, true)); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.WriteString(`"`); err != nil {
		return ast.WalkStop, err
	}
	if n.Title != nil {
		if _, err := w.WriteString(` title="`); err != nil {
			return ast.WalkStop, err
		}
		if _, err := w.Write(util.EscapeHTML(n.Title)); err != nil {
			return ast.WalkStop, err
		}
		if _, err := w.WriteString(`"`); err != nil {
			return ast.WalkStop, err
		}
	}
	if isExternalURL(n.Destination) {
		if _, err := w.WriteString(` target="_blank" rel="noopener noreferrer"`); err != nil {
			return ast.WalkStop, err
		}
	}
	if _, err := w.WriteString(`>`); err != nil {
		return ast.WalkStop, err
	}
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
		if _, err := w.WriteString("</a>"); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	url := n.URL(source)
	label := n.Label(source)

	if _, err := w.WriteString(`<a href="`); err != nil {
		return ast.WalkStop, err
	}
	if n.AutoLinkType == ast.AutoLinkEmail &&
		!bytes.HasPrefix(bytes.ToLower(url), []byte("mailto:")) {
		if _, err := w.WriteString("mailto:"); err != nil {
			return ast.WalkStop, err
		}
	}
	if _, err := w.Write(util.URLEscape(url, false)); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.WriteString(`"`); err != nil {
		return ast.WalkStop, err
	}
	if isExternalURL(url) {
		if _, err := w.WriteString(` target="_blank" rel="noopener noreferrer"`); err != nil {
			return ast.WalkStop, err
		}
	}
	if _, err := w.WriteString(`>`); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.Write(util.EscapeHTML(label)); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkContinue, nil
}
