package markdown

import (
	"html"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type wiremdBlock struct {
	ast.BaseBlock
}

var KindWiremdBlock = ast.NewNodeKind("WiremdBlock")

func (n *wiremdBlock) Kind() ast.NodeKind {
	return KindWiremdBlock
}

func (n *wiremdBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type wiremdTransformer struct{}

func (t *wiremdTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	var toReplace []*ast.FencedCodeBlock

	if err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := node.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		lang := fcb.Language(source)
		if string(lang) == "wiremd" {
			toReplace = append(toReplace, fcb)
		}
		return ast.WalkContinue, nil
	}); err != nil {
		return
	}

	for _, fcb := range toReplace {
		wmd := &wiremdBlock{}
		for i := 0; i < fcb.Lines().Len(); i++ {
			wmd.Lines().Append(fcb.Lines().At(i))
		}
		fcb.Parent().ReplaceChild(fcb.Parent(), fcb, wmd)
	}
}

type wiremdRenderer struct {
	gmhtml.Config
}

func (r *wiremdRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindWiremdBlock, r.renderWiremdBlock)
}

func (r *wiremdRenderer) renderWiremdBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		if _, err := w.WriteString("<pre class=\"wiremd\">"); err != nil {
			return ast.WalkStop, err
		}
		for i := 0; i < node.Lines().Len(); i++ {
			line := node.Lines().At(i)
			escaped := html.EscapeString(string(line.Value(source)))
			if _, err := w.WriteString(escaped); err != nil {
				return ast.WalkStop, err
			}
		}
		if _, err := w.WriteString("</pre>\n"); err != nil {
			return ast.WalkStop, err
		}
	}
	return ast.WalkContinue, nil
}

type wiremdExtension struct{}

func NewWiremdExtension() goldmark.Extender {
	return &wiremdExtension{}
}

func (e *wiremdExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&wiremdTransformer{}, 100),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&wiremdRenderer{}, 100),
		),
	)
}
