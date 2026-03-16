package markdown

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type mathBlock struct {
	ast.BaseBlock
}

func (n *mathBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

var KindMathBlock = ast.NewNodeKind("MathBlock")

func (n *mathBlock) Kind() ast.NodeKind {
	return KindMathBlock
}

func NewMathBlock() *mathBlock {
	return &mathBlock{}
}

type mathInline struct {
	ast.BaseInline
}

func (n *mathInline) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

var KindMathInline = ast.NewNodeKind("MathInline")

func (n *mathInline) Kind() ast.NodeKind {
	return KindMathInline
}

func NewMathInline() *mathInline {
	return &mathInline{}
}

type mathBlockParser struct{}

var defaultMathBlockParser = &mathBlockParser{}

func NewMathBlockParser() parser.BlockParser {
	return defaultMathBlockParser
}

func (b *mathBlockParser) Trigger() []byte {
	return []byte{'$'}
}

func (b *mathBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	if len(line) < 2 || line[0] != '$' || line[1] != '$' {
		return nil, parser.NoChildren
	}

	trimmed := line
	if len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		trimmed = line[:len(line)-1]
		if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\r' {
			trimmed = trimmed[:len(trimmed)-1]
		}
	}

	if len(trimmed) >= 4 && trimmed[len(trimmed)-1] == '$' && trimmed[len(trimmed)-2] == '$' {
		closingPos := len(trimmed) - 2
		node := NewMathBlock()
		contentSegment := segment.WithStop(segment.Start + closingPos)
		contentSegment = contentSegment.WithStart(segment.Start + 2)
		node.Lines().Append(contentSegment)
		reader.AdvanceLine()
		return node, parser.Close
	}

	reader.Advance(2)
	return NewMathBlock(), parser.NoChildren
}

func (b *mathBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, segment := reader.PeekLine()
	if len(line) >= 2 && line[0] == '$' && line[1] == '$' {
		reader.Advance(segment.Len())
		return parser.Close
	}
	node.Lines().Append(segment)
	return parser.Continue | parser.NoChildren
}

func (b *mathBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
}

func (b *mathBlockParser) CanInterruptParagraph() bool {
	return true
}

func (b *mathBlockParser) CanAcceptIndentedLine() bool {
	return false
}

type mathInlineParser struct{}

var defaultMathInlineParser = &mathInlineParser{}

func NewMathInlineParser() parser.InlineParser {
	return defaultMathInlineParser
}

func (p *mathInlineParser) Trigger() []byte {
	return []byte{'$'}
}

func (p *mathInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, startSegment := block.PeekLine()
	if len(line) < 1 || line[0] != '$' {
		return nil
	}

	if len(line) >= 2 && line[1] == '$' {
		return nil
	}

	block.Advance(1)

	pos := 1
	for pos < len(line) {
		if line[pos] == '$' && (pos == 1 || line[pos-1] != '\\') {
			node := NewMathInline()
			segment := startSegment.WithStop(startSegment.Start + pos)
			segment = segment.WithStart(segment.Start + 1)
			node.AppendChild(node, ast.NewTextSegment(segment))
			block.Advance(pos)
			return node
		}
		pos++
	}

	return nil
}

type mathRenderer struct {
	html.Config
}

func NewMathRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &mathRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindMathBlock, r.renderMathBlock)
	reg.Register(KindMathInline, r.renderMathInline)
}

func (r *mathRenderer) renderMathBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		if _, err := w.WriteString("<div class=\"math-block\">\n$$\n"); err != nil {
			return ast.WalkStop, err
		}
		for i := 0; i < node.Lines().Len(); i++ {
			line := node.Lines().At(i)
			if _, err := w.Write(line.Value(source)); err != nil {
				return ast.WalkStop, err
			}
		}
		if _, err := w.WriteString("\n$$\n</div>\n"); err != nil {
			return ast.WalkStop, err
		}
	}
	return ast.WalkContinue, nil
}

func (r *mathRenderer) renderMathInline(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		if _, err := w.WriteString("<span class=\"math-inline\">$"); err != nil {
			return ast.WalkStop, err
		}
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			textNode, ok := c.(*ast.Text)
			if !ok {
				continue
			}
			if _, err := w.Write(textNode.Segment.Value(source)); err != nil {
				return ast.WalkStop, err
			}
		}
		if _, err := w.WriteString("$</span>"); err != nil {
			return ast.WalkStop, err
		}
	}
	return ast.WalkContinue, nil
}

type mathExtension struct{}

func NewMathExtension() goldmark.Extender {
	return &mathExtension{}
}

func (e *mathExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(NewMathBlockParser(), 100),
		),
		parser.WithInlineParsers(
			util.Prioritized(NewMathInlineParser(), 100),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewMathRenderer(), 100),
		),
	)
}
