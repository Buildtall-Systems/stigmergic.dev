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

// TreeResolver resolves wikilinks against the file tree index.
// It implements the wikilink.Resolver interface.
type TreeResolver struct {
	pathIndex map[string]string   // normalized full path → route path
	nameIndex map[string][]string // normalized filename → []route paths
}

// NewTreeResolver builds a resolver from the flat list of searchable files.
func NewTreeResolver(files []models.SearchableFile) *TreeResolver {
	r := &TreeResolver{
		pathIndex: make(map[string]string, len(files)),
		nameIndex: make(map[string][]string),
	}
	for _, f := range files {
		route := strings.TrimPrefix(f.Path, "/")
		normPath := normalize(route)
		r.pathIndex[normPath] = route

		name := filepath.Base(route)
		normName := normalize(name)
		r.nameIndex[normName] = append(r.nameIndex[normName], route)
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

	norm := normalize(target)

	// Try exact path match first.
	if route, ok := r.pathIndex[norm]; ok {
		return r.buildURL(route, n.Fragment), nil
	}

	// Try filename match.
	normName := normalize(filepath.Base(target))
	if routes, ok := r.nameIndex[normName]; ok && len(routes) > 0 {
		best := routes[0]
		for _, route := range routes[1:] {
			if len(route) < len(best) {
				best = route
			}
		}
		return r.buildURL(best, n.Fragment), nil
	}

	return nil, nil
}

func (r *TreeResolver) buildURL(route string, fragment []byte) []byte {
	url := "/file/" + route
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

	once    sync.Once
	hasDest sync.Map // *wikilink.Node → struct{}
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

	r.exit(w, n)
	return ast.WalkContinue, nil
}

func (r *WikilinkRenderer) enter(w util.BufWriter, n *wikilink.Node) (ast.WalkStatus, error) {
	dest, err := r.Resolver.ResolveWikilink(n)
	if err != nil {
		return ast.WalkStop, fmt.Errorf("resolve %q: %w", n.Target, err)
	}

	if len(dest) > 0 {
		r.hasDest.Store(n, struct{}{})
		_, _ = w.WriteString(`<a href="`)
		_, _ = w.Write(util.URLEscape(dest, true))
		_, _ = w.WriteString(`">`)
	} else {
		_, _ = w.WriteString(`<span class="wikilink-unresolved">`)
	}

	return ast.WalkContinue, nil
}

func (r *WikilinkRenderer) exit(w util.BufWriter, n *wikilink.Node) {
	if _, ok := r.hasDest.LoadAndDelete(n); ok {
		_, _ = w.WriteString("</a>")
	} else {
		_, _ = w.WriteString("</span>")
	}
}
