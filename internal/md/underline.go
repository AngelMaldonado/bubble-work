package md

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// ++text++ is a limited inline extension, not permission to render raw HTML.
var kindUnderline = ast.NewNodeKind("Underline")

type underlineNode struct{ ast.BaseInline }

func (n *underlineNode) Kind() ast.NodeKind            { return kindUnderline }
func (n *underlineNode) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type underlineProcessor struct{}

func (*underlineProcessor) IsDelimiter(b byte) bool                   { return b == '+' }
func (*underlineProcessor) CanOpenCloser(a, b *parser.Delimiter) bool { return a.Char == b.Char }
func (*underlineProcessor) OnMatch(int) ast.Node                      { return &underlineNode{} }

type underlineParser struct{}

func (*underlineParser) Trigger() []byte { return []byte{'+'} }
func (*underlineParser) Parse(parent ast.Node, r text.Reader, pc parser.Context) ast.Node {
	before := r.PrecendingCharacter()
	line, segment := r.PeekLine()
	n := parser.ScanDelimiter(line, before, 2, &underlineProcessor{})
	if n == nil || n.OriginalLength != 2 || before == '+' {
		return nil
	}
	n.Segment = segment.WithStop(segment.Start + 2)
	r.Advance(2)
	pc.PushDelimiter(n)
	return n
}
func (*underlineParser) CloseBlock(ast.Node, parser.Context) {}

type underlineRenderer struct{}

func (*underlineRenderer) RegisterFuncs(r renderer.NodeRendererFuncRegisterer) {
	r.Register(kindUnderline, func(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		tag := "</u>"
		if entering {
			tag = "<u>"
		}
		_, err := w.WriteString(tag)
		return ast.WalkContinue, err
	})
}

type underlineExtension struct{}

func (underlineExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(util.Prioritized(&underlineParser{}, 500)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(&underlineRenderer{}, 500)))
}
