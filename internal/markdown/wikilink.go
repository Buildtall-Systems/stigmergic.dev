package markdown

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/wikilink"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

// FileMount is the route prefix the filesystem and embedded sources serve
// under. A resolver built from a flat file list names its destinations here.
const FileMount = "/file/"

// RouteEntry is one document as a resolver knows it: the path a link writes
// to reach it, relative to the source holding it, and the route that serves
// it. The two differ once more than one source is mounted, and keeping both
// is what lets a corpus-wide resolver match a name written in one source
// against a document held by another.
type RouteEntry struct {
	Path  string
	Route string
}

// TreeResolver resolves wikilinks against a set of route entries.
// It implements the wikilink.Resolver interface.
type TreeResolver struct {
	pathIndex map[string]RouteEntry   // normalized source path → entry
	nameIndex map[string][]RouteEntry // normalized filename → entries
}

// NewTreeResolver builds a resolver from the flat list of searchable files,
// whose paths are relative to a single source mounted at FileMount.
func NewTreeResolver(files []models.SearchableFile) *TreeResolver {
	entries := make([]RouteEntry, 0, len(files))
	for _, f := range files {
		p := strings.TrimPrefix(f.Path, "/")
		entries = append(entries, RouteEntry{Path: p, Route: FileMount + p})
	}
	return NewRouteResolver(entries)
}

// NewRouteResolver builds a resolver over documents that may sit in
// different sources. Matching runs against each entry's source-relative
// path, so a link writes the name it would write inside its own source, and
// the destination is the entry's route, wherever that source is mounted.
func NewRouteResolver(entries []RouteEntry) *TreeResolver {
	r := &TreeResolver{
		pathIndex: make(map[string]RouteEntry, len(entries)),
		nameIndex: make(map[string][]RouteEntry),
	}
	for _, e := range entries {
		normPath := normalize(e.Path)
		r.pathIndex[normPath] = e

		normName := normalize(filepath.Base(e.Path))
		r.nameIndex[normName] = append(r.nameIndex[normName], e)
	}
	return r
}

// ResolveWikilink resolves a wikilink node to a /file/ URL.
// Returns nil destination for unresolved links.
func (r *TreeResolver) ResolveWikilink(n *wikilink.Node) ([]byte, error) {
	target := string(n.Target)
	if target == "" {
		if len(n.Fragment) > 0 {
			return []byte("#" + string(n.Fragment)), nil
		}
		return nil, nil
	}

	entry, ok := r.match(target)
	if !ok {
		return nil, nil
	}
	return r.buildURL(entry, n.Fragment), nil
}

// ResolveRoute answers a CommonMark destination against the same index,
// which is what keeps a plain markdown link and a wikilink to the same
// document pointing at one route. The fragment travels with the route so an
// anchored link keeps its anchor.
func (r *TreeResolver) ResolveRoute(target string) (string, bool) {
	name, fragment, _ := strings.Cut(target, "#")
	entry, ok := r.match(name)
	if !ok {
		return "", false
	}
	return string(r.buildURL(entry, []byte(fragment))), true
}

// match finds the entry a target names: an exact path first, then a
// filename. Ambiguity resolves rather than refuses, the shortest source path
// winning, so one corpus yields one graph however the ambiguity arose.
func (r *TreeResolver) match(target string) (RouteEntry, bool) {
	if target == "" {
		return RouteEntry{}, false
	}

	if entry, ok := r.pathIndex[normalize(target)]; ok {
		return entry, true
	}

	entries, ok := r.nameIndex[normalize(filepath.Base(target))]
	if !ok || len(entries) == 0 {
		return RouteEntry{}, false
	}
	best := entries[0]
	for _, e := range entries[1:] {
		if len(e.Path) < len(best.Path) {
			best = e
		}
	}
	return best, true
}

func (r *TreeResolver) buildURL(entry RouteEntry, fragment []byte) []byte {
	url := entry.Route
	if len(fragment) > 0 {
		url += "#" + string(fragment)
	}
	return []byte(url)
}

// normalize lowercases, strips .md suffix, and normalizes separators
// (spaces, underscores → hyphens).
func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSuffix(s, ".md")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

// WikilinkRenderer renders wikilink nodes as HTML,
// using a TreeResolver to determine link targets.
// Resolved links become <a href="...">, unresolved become
// <span class="wikilink-unresolved">.
type WikilinkRenderer struct {
	Resolver wikilink.Resolver
	hasDest  sync.Map // *wikilink.Node → struct{}

	once sync.Once
}

func (r *WikilinkRenderer) init() {
	r.once.Do(func() {
		if r.Resolver == nil {
			r.Resolver = wikilink.DefaultResolver
		}
	})
}

// RegisterFuncs registers the renderer for wikilink nodes.
func (r *WikilinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(wikilink.Kind, r.Render)
}

// Render renders a wikilink node as HTML.
func (r *WikilinkRenderer) Render(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	r.init()

	n, ok := node.(*wikilink.Node)
	if !ok {
		return ast.WalkStop, fmt.Errorf("unexpected node %T, expected *wikilink.Node", node)
	}

	if entering {
		return r.enter(w, n)
	}

	if err := r.exit(w, n); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkContinue, nil
}

// enter opens the element for one wikilink.
//
// An embed link (![[...]]) reaches here only when it did not stand alone in
// its block, because embedTransformer promotes the standalone ones out of the
// inline stream before rendering. What is left is a mid-sentence embed, and it
// renders as an ordinary anchor on purpose: transcluded content is block
// content, and emitting it inside a <p> would be repaired by the browser in a
// way that corrupts the surrounding document.
func (r *WikilinkRenderer) enter(w util.BufWriter, n *wikilink.Node) (ast.WalkStatus, error) {
	dest, err := r.Resolver.ResolveWikilink(n)
	if err != nil {
		return ast.WalkStop, fmt.Errorf("resolve %q: %w", n.Target, err)
	}

	if len(dest) > 0 {
		r.hasDest.Store(n, struct{}{})
		if _, err := w.WriteString(`<a href="`); err != nil {
			return ast.WalkStop, err
		}
		if _, err := w.Write(util.URLEscape(dest, true)); err != nil {
			return ast.WalkStop, err
		}
		if _, err := w.WriteString(`">`); err != nil {
			return ast.WalkStop, err
		}
	} else {
		if _, err := w.WriteString(`<span class="wikilink-unresolved">`); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

func (r *WikilinkRenderer) exit(w util.BufWriter, n *wikilink.Node) error {
	if _, ok := r.hasDest.LoadAndDelete(n); ok {
		if _, err := w.WriteString("</a>"); err != nil {
			return err
		}
	} else {
		if _, err := w.WriteString("</span>"); err != nil {
			return err
		}
	}
	return nil
}
