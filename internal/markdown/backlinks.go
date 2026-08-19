package markdown

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/wikilink"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

// LinkRef is one wikilink as written, before resolution.
//
// Splitting extraction from resolution is what makes rebuilds incremental.
// Parsing a document depends only on its own bytes, so it can be cached per
// file. Resolution depends on the whole file set, since a wikilink names a
// page rather than a path, so it has to be redone on every rebuild no matter
// what changed. Caching refs rather than resolved routes keeps the expensive
// half cacheable without ever serving a stale resolution.
type LinkRef struct {
	Target   string
	Fragment string
}

// LinkRefs maps a source route to the wikilinks it contains, in document
// order.
type LinkRefs map[string][]LinkRef

// ExtractLinkRefs returns the wikilinks in every route of corpus, parsing
// only the routes named in changed and carrying the rest over from prev.
// Routes absent from corpus are dropped.
func ExtractLinkRefs(prev LinkRefs, corpus Corpus, changed ChangedRoutes) LinkRefs {
	md := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithInlineParsers(util.Prioritized(&wikilink.Parser{}, 199)),
		),
	)

	refs := make(LinkRefs, len(corpus))

	for route, entry := range corpus {
		if _, dirty := changed[route]; !dirty {
			if cached, ok := prev[route]; ok {
				refs[route] = cached
				continue
			}
		}
		refs[route] = parseLinkRefs(md, entry.Data)
	}

	return refs
}

// parseLinkRefs walks one document and collects its wikilinks in order. A
// walk error discards the document's links, matching the behavior of the
// single-pass builder this replaced.
func parseLinkRefs(md goldmark.Markdown, source []byte) []LinkRef {
	doc := md.Parser().Parse(text.NewReader(source))

	var refs []LinkRef
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		wn, ok := n.(*wikilink.Node)
		if !ok {
			return ast.WalkContinue, nil
		}
		refs = append(refs, LinkRef{Target: string(wn.Target), Fragment: string(wn.Fragment)})
		return ast.WalkContinue, nil
	}); err != nil {
		return nil
	}

	return refs
}

// ResolverFor supplies the resolver that answers one document's links. It is
// per document rather than per corpus because a source can match
// doc-relative: the same target written in two documents of one vault can
// name two different files, and an index that resolved them any other way
// than the page render does would disagree with the page it describes.
type ResolverFor func(route string) wikilink.Resolver

// BuildBacklinkIndex resolves every extracted wikilink and inverts the
// result, mapping each target route to the files that link to it. Self-links
// and repeats within one source are skipped.
//
// files and the keys of refs are routes, mount prefix included, so one index
// spans every mounted source: a document in one source linking to a document
// in another is a backlink like any other. mounts are the registered route
// prefixes; a destination lying outside all of them names nothing the corpus
// holds and is passed over.
func BuildBacklinkIndex(refs LinkRefs, files []models.SearchableFile, resolveFor ResolverFor, mounts []string) models.BacklinkIndex {
	index := make(models.BacklinkIndex)

	for _, f := range files {
		route := f.Path

		fileRefs, ok := refs[route]
		if !ok {
			continue
		}

		resolver := resolveFor(route)
		if resolver == nil {
			continue
		}

		seen := make(map[string]bool)
		for _, ref := range fileRefs {
			node := &wikilink.Node{Target: []byte(ref.Target), Fragment: []byte(ref.Fragment)}

			dest, err := resolver.ResolveWikilink(node)
			if err != nil || len(dest) == 0 {
				continue
			}

			targetRoute, inCorpus := mountedRoute(string(dest), mounts)
			if !inCorpus {
				continue
			}

			if targetRoute == route || seen[targetRoute] {
				continue
			}
			seen[targetRoute] = true

			index[targetRoute] = append(index[targetRoute], models.BacklinkEntry{
				SourcePath:  route,
				SourceTitle: titleFromFilename(route),
			})
		}
	}

	// Ordering by title alone leaves same-named sources in different
	// directories unordered, which would make an incremental index differ
	// from a full one for no reason. Path breaks the tie.
	for target, entries := range index {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].SourceTitle != entries[j].SourceTitle {
				return entries[i].SourceTitle < entries[j].SourceTitle
			}
			return entries[i].SourcePath < entries[j].SourcePath
		})
		index[target] = entries
	}

	return index
}

// mountedRoute drops a resolved destination's anchor and reports what is
// left as a corpus route when it lies inside one of the registered mounts.
// A destination naming no mount is not a document this corpus serves, which
// is how an external or malformed target stays out of the index.
func mountedRoute(dest string, mounts []string) (string, bool) {
	if idx := strings.Index(dest, "#"); idx >= 0 {
		dest = dest[:idx]
	}
	for _, mount := range mounts {
		if len(dest) > len(mount) && strings.HasPrefix(dest, mount) {
			return dest, true
		}
	}
	return "", false
}

// titleFromFilename extracts a display name from a route path.
func titleFromFilename(route string) string {
	return strings.TrimSuffix(filepath.Base(route), ".md")
}
