package okf

import (
	"net/url"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/wikilink"
)

// wikilinkPriority registers the wikilink inline parser ahead of goldmark's
// built-in link parser; at the built-in's priority or above, "[[...]]" parses
// as plain text and the wikilink node never exists.
const wikilinkPriority = 199

// LinkSyntax identifies the markdown syntax a link reference was written in.
type LinkSyntax int

const (
	SyntaxWikilink LinkSyntax = iota
	SyntaxLink
	SyntaxImage
)

// LinkRef is one link reference as written in a markdown body, before any
// resolution. The target is recorded without judgment: whether it names a
// document, an attachment, or nothing at all is resolution's business, never
// extraction's, because a name like "claude.ai" defeats any extension
// heuristic. Wikilink targets are stored verbatim; CommonMark destinations
// are percent-decoded, since the on-disk name is the reference and foreign
// corpora write "my%20file.md" for "my file.md".
type LinkRef struct {
	Target   string
	Fragment string
	Embed    bool
	Syntax   LinkSyntax
}

// linkRefMarkdown is the extraction parser, built once: bare goldmark plus
// the wikilink inline parser, no renderer. Extraction depends only on the
// document's own bytes, so one shared instance serves every parse.
var linkRefMarkdown = goldmark.New(
	goldmark.WithParserOptions(
		parser.WithInlineParsers(util.Prioritized(&wikilink.Parser{}, wikilinkPriority)),
	),
)

// ExtractLinkRefs parses one markdown body and returns its link references in
// document order. Wikilinks, CommonMark links, and images are collected.
// Structural non-edges never appear: autolinks and raw inline HTML are not
// walked, and targets that are empty or anchor-only, scheme'd or
// protocol-relative, or trailing-slash directories are dropped here.
func ExtractLinkRefs(body string) []LinkRef {
	doc := linkRefMarkdown.Parser().Parse(text.NewReader([]byte(body)))

	var refs []LinkRef
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		ref, ok := refOf(n)
		if !ok || excludedTarget(ref.Target) {
			return ast.WalkContinue, nil
		}
		refs = append(refs, ref)
		return ast.WalkContinue, nil
	}); err != nil {
		return nil
	}

	return refs
}

// refOf turns an AST node into its reference when the node is one of the
// three collected syntaxes.
func refOf(n ast.Node) (LinkRef, bool) {
	switch node := n.(type) {
	case *wikilink.Node:
		return LinkRef{
			Target:   string(node.Target),
			Fragment: string(node.Fragment),
			Embed:    node.Embed,
			Syntax:   SyntaxWikilink,
		}, true
	case *ast.Link:
		return destinationRef(node.Destination, SyntaxLink, false), true
	case *ast.Image:
		return destinationRef(node.Destination, SyntaxImage, true), true
	}
	return LinkRef{}, false
}

// destinationRef builds the reference for a CommonMark destination. The
// fragment splits on the first "#" before decoding, so an escaped "%23" in a
// path never masquerades as an anchor.
func destinationRef(destination []byte, syntax LinkSyntax, embed bool) LinkRef {
	target := string(destination)
	var fragment string
	if idx := strings.IndexByte(target, '#'); idx >= 0 {
		fragment = pathUnescaped(target[idx+1:])
		target = target[:idx]
	}
	return LinkRef{
		Target:   pathUnescaped(target),
		Fragment: fragment,
		Embed:    embed,
		Syntax:   syntax,
	}
}

// pathUnescaped percent-decodes s; bytes that are not valid percent-encoding
// stay literal, because an author who wrote a stray "%" wrote a "%".
func pathUnescaped(s string) string {
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// excludedTarget reports whether target is structurally not an edge: empty or
// anchor-only, protocol-relative, a trailing-slash directory, or carrying a
// URI scheme. Scheme detection follows RFC 3986: a leading letter, then
// letters, digits, "+", "-", or ".", up to a ":".
func excludedTarget(target string) bool {
	if target == "" || strings.HasSuffix(target, "/") || strings.HasPrefix(target, "//") {
		return true
	}
	return hasURIScheme(target)
}

func hasURIScheme(target string) bool {
	idx := strings.IndexByte(target, ':')
	if idx <= 0 {
		return false
	}
	if !isAlpha(target[0]) {
		return false
	}
	for i := 1; i < idx; i++ {
		if !isSchemeByte(target[i]) {
			return false
		}
	}
	return true
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSchemeByte(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'
}
