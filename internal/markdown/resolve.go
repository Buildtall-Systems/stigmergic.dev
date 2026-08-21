package markdown

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/wikilink"
)

// RouteResolver answers a CommonMark destination. A relative link or image
// target naming something the source holds resolves to the route serving it;
// anything else reports false and the destination renders exactly as
// written. It is the seam wikilink.Resolver does not cover: plain markdown
// links bypass wikilink syntax entirely, and inside a vault they must resolve
// through the same matcher or the reader and the link graph disagree about
// what the document points at.
type RouteResolver interface {
	ResolveRoute(target string) (string, bool)
}

// AbsenceReporter declares a target absent from what a source holds. Only a
// source whose index covers assets as well as documents may implement it:
// the corpus-wide tree resolver indexes .md files alone, so a destination it
// declines may still name a file the server serves, and it stays silent
// rather than break every working relative asset link.
type AbsenceReporter interface {
	RouteAbsent(target string) bool
}

// Chain resolves through its members in order, taking the first destination
// offered. It is what lets a document link out of its own source: the source
// answers first, so a name it holds always wins, and the corpus-wide
// resolver answers what the source does not hold.
type Chain []wikilink.Resolver

var (
	_ wikilink.Resolver = Chain(nil)
	_ RouteResolver     = Chain(nil)
	_ AbsenceReporter   = Chain(nil)
)

// ResolveWikilink answers with the first member that offers a destination. A
// member's error stops the chain: a resolver that failed has no opinion to
// pass over.
func (c Chain) ResolveWikilink(n *wikilink.Node) ([]byte, error) {
	for _, r := range c {
		if r == nil {
			continue
		}
		dest, err := r.ResolveWikilink(n)
		if err != nil {
			return nil, err
		}
		if len(dest) > 0 {
			return dest, nil
		}
	}
	return nil, nil
}

// ResolveRoute answers with the first member that both resolves CommonMark
// destinations and holds the target. A member that does not implement
// RouteResolver has nothing to say about plain markdown links and is passed
// over rather than treated as a miss.
func (c Chain) ResolveRoute(target string) (string, bool) {
	for _, r := range c {
		rr, ok := r.(RouteResolver)
		if !ok {
			continue
		}
		if route, found := rr.ResolveRoute(target); found {
			return route, true
		}
	}
	return "", false
}

// RouteAbsent forwards to the first member able to declare absence, which
// mirrors resolution order: the source serving the document speaks first. A
// chain with no reporting member has no authority to declare anything absent
// and reports false, leaving the destination verbatim.
func (c Chain) RouteAbsent(target string) bool {
	for _, r := range c {
		if reporter, ok := r.(AbsenceReporter); ok {
			return reporter.RouteAbsent(target)
		}
	}
	return false
}

// unresolvedAttr marks a link node whose relative destination the serving
// source declared absent. The link renderer reads the mark and emits the
// span wikilinks already use for a dangling target, instead of an anchor the
// browser would resolve against the page URL into a 404.
const unresolvedAttr = "okf-unresolved"

// routeTransformer rewrites relative CommonMark link and image destinations
// to the routes the corpus serves them at.
//
// Rewriting in the AST rather than in a renderer keeps one copy of
// goldmark's markup rules: the stock image renderer and this package's link
// renderer both emit whatever destination the node carries, so neither has
// to be reimplemented to consult a resolver.
type routeTransformer struct {
	routes RouteResolver
}

// Transform rewrites every destination the resolver claims. A destination it
// does not claim is left exactly as the author wrote it, with one exception:
// a relative link destination the resolver declares absent is marked
// unresolved. Images are never marked: a broken image already fails visibly,
// and replacing <img> would take a custom image renderer.
func (t *routeTransformer) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Link:
			if route, ok := t.resolve(node.Destination); ok {
				node.Destination = route
			} else if t.absent(node.Destination) {
				node.SetAttributeString(unresolvedAttr, true)
			}
		case *ast.Image:
			if route, ok := t.resolve(node.Destination); ok {
				node.Destination = route
			}
		}
		return ast.WalkContinue, nil
	}); err != nil {
		return
	}
}

// resolve answers one destination, declining anything that is already a
// route: an absolute path, a protocol-relative or scheme'd URL, and a bare
// fragment all name their target without the corpus's help.
func (t *routeTransformer) resolve(destination []byte) ([]byte, bool) {
	target := string(destination)
	if !relativeDestination(target) {
		return nil, false
	}
	route, ok := t.routes.ResolveRoute(target)
	if !ok {
		return nil, false
	}
	return []byte(route), true
}

// absent reports whether a declined destination names something the serving
// source has declared it does not hold. Only a relative destination can be
// judged: an absolute path, a scheme'd URL, and a bare fragment all name
// their target without the corpus's help and are never marked.
func (t *routeTransformer) absent(destination []byte) bool {
	target := string(destination)
	if !relativeDestination(target) {
		return false
	}
	reporter, ok := t.routes.(AbsenceReporter)
	if !ok {
		return false
	}
	return reporter.RouteAbsent(target)
}

// relativeDestination reports whether a destination is a corpus-relative
// reference. Scheme detection follows RFC 3986: a leading letter, then
// letters, digits, "+", "-", or ".", up to a ":".
func relativeDestination(target string) bool {
	if target == "" || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "#") {
		return false
	}
	idx := strings.IndexByte(target, ':')
	if idx <= 0 {
		return true
	}
	if !isAlpha(target[0]) {
		return true
	}
	for i := 1; i < idx; i++ {
		if !isSchemeByte(target[i]) {
			return true
		}
	}
	return false
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSchemeByte(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'
}
