package okf

import (
	"path"
	"sort"
	"strings"
)

// FileEntry is one file the link matcher can resolve to: a concept document
// or an attachment. SHA256 is the attachment's stated hash, empty exactly
// when the entry is a concept document, because an attachment is stated as
// name plus hash and a concept never carries one.
type FileEntry struct {
	Path   string
	SHA256 string
}

// LinkEdge is one resolved link: the bundle-relative file path of the
// referencing document, the reference as written, and the entry the matcher
// joined it to.
type LinkEdge struct {
	Source string
	Target FileEntry
	// Ref sits last for field alignment: its non-pointer tail keeps the GC's
	// pointer-scan region off the end of the struct.
	Ref LinkRef
}

// DanglingLink records a reference that resolved to nothing: the referencing
// document's file path and the target as written. Dangling is a report,
// never a refusal: a broken link is a fact about the vault, not a reason the
// vault cannot publish.
type DanglingLink struct {
	Source string
	Target string
}

// LinkGraph is a bundle's resolved link structure: every edge, references in
// document order within each document and documents in lexical path order,
// plus every reference that resolved to nothing, in the same order.
type LinkGraph struct {
	Edges    []LinkEdge
	Dangling []DanglingLink
}

// BuildLinkGraph extracts and resolves every link reference in the bundle's
// concept documents against the bundle's own file set.
func BuildLinkGraph(b *Bundle) LinkGraph {
	if b == nil || b.Root == nil {
		return LinkGraph{}
	}
	return graphOver(bundleConcepts(b), indexBundle(b))
}

// graphOver resolves every reference the concepts carry against idx: the one
// resolution pass both the bundle graph and ReadBundle's tier 1 post-pass
// run, over whichever index their file set built. Concepts arrive in lexical
// ConceptID order and the graph preserves it.
func graphOver(concepts []*Concept, idx *fileIndex) LinkGraph {
	var g LinkGraph
	for _, c := range concepts {
		source := c.ConceptID + conceptFileExt
		refDir, _ := splitPath(c.ConceptID)
		for _, ref := range ExtractLinkRefs(c.Body) {
			entry, ok := idx.resolve(refDir, ref)
			if !ok {
				g.Dangling = append(g.Dangling, DanglingLink{Source: source, Target: ref.Target})
				continue
			}
			g.Edges = append(g.Edges, LinkEdge{Source: source, Ref: ref, Target: entry})
		}
	}
	return g
}

// bundleConcepts returns every concept in the bundle in lexical ConceptID
// order, so one bundle yields one graph however its tree was assembled.
func bundleConcepts(b *Bundle) []*Concept {
	concepts := collectConcepts(b.Root)
	sort.Slice(concepts, func(i, j int) bool {
		return concepts[i].ConceptID < concepts[j].ConceptID
	})
	return concepts
}

func collectConcepts(d *Directory) []*Concept {
	concepts := append([]*Concept{}, d.Concepts...)
	for _, child := range d.Children() {
		concepts = append(concepts, collectConcepts(child)...)
	}
	return concepts
}

// fileIndex maps a file set for the matcher: exact paths and final segments.
// One index shape serves both constructions, from a Bundle on the publish
// path and from set events at render time, which is what keeps the two
// consumers resolving one link one way.
type fileIndex struct {
	byPath map[string]FileEntry
	byBase map[string][]FileEntry
}

func newFileIndex() *fileIndex {
	return &fileIndex{byPath: map[string]FileEntry{}, byBase: map[string][]FileEntry{}}
}

func (idx *fileIndex) add(e FileEntry) {
	idx.byPath[e.Path] = e
	_, base := splitPath(e.Path)
	idx.byBase[base] = append(idx.byBase[base], e)
}

// indexBundle builds the matcher's index from a bundle's tree: every concept
// at its on-disk path, every attachment with its stated hash. Reserved
// sidecars and directory indexes are not files a link can claim.
func indexBundle(b *Bundle) *fileIndex {
	idx := newFileIndex()
	indexDirectory(idx, b.Root)
	return idx
}

func indexDirectory(idx *fileIndex, d *Directory) {
	for _, c := range d.Concepts {
		idx.add(FileEntry{Path: c.ConceptID + conceptFileExt})
	}
	for _, a := range d.Attachments {
		idx.add(FileEntry{Path: joinPath(d.Path, a.Name), SHA256: a.SHA256})
	}
	for _, child := range d.Children() {
		indexDirectory(idx, child)
	}
}

// resolve joins one reference to the entry it names. Matching runs in tiers,
// location outranking name derivation: an exact path relative to the
// referencing document's directory, then an exact path from the bundle root,
// then a basename match for single-segment targets. Within a tier the target
// matches as written before the wikilink document form, the target plus
// ".md", since Obsidian strips that extension from document links;
// attachment targets carry their extension and match as written. Matching is
// case-sensitive throughout: it runs against real stated names, never
// normalized guesses.
func (idx *fileIndex) resolve(refDir string, ref LinkRef) (FileEntry, bool) {
	candidates := []string{ref.Target}
	if ref.Syntax == SyntaxWikilink {
		candidates = append(candidates, ref.Target+conceptFileExt)
	}

	for _, name := range candidates {
		if strings.HasPrefix(name, bundlePathSeparator) {
			continue
		}
		if p, ok := relativePath(path.Join(refDir, name)); ok {
			if e, found := idx.byPath[p]; found {
				return e, true
			}
		}
	}
	for _, name := range candidates {
		if p, ok := relativePath(name); ok {
			if e, found := idx.byPath[p]; found {
				return e, true
			}
		}
	}
	for _, name := range candidates {
		if strings.Contains(name, bundlePathSeparator) {
			continue
		}
		if e, found := idx.baseMatch(name); found {
			return e, true
		}
	}
	return FileEntry{}, false
}

// relativePath cleans a candidate path and reports whether it stays inside
// the bundle: a reference climbing past the root or naming an absolute path
// names nothing the bundle holds.
func relativePath(p string) (string, bool) {
	cleaned := path.Clean(p)
	if cleaned == "." || cleaned == ".." ||
		strings.HasPrefix(cleaned, ".."+bundlePathSeparator) ||
		strings.HasPrefix(cleaned, bundlePathSeparator) {
		return "", false
	}
	return cleaned, true
}

// baseMatch resolves a single-segment name against every file bearing it as
// its final segment. Ambiguity resolves rather than refuses: the shallowest
// path wins and lexicographic order breaks equal depth, so one corpus yields
// one graph no matter how the ambiguity arose.
func (idx *fileIndex) baseMatch(name string) (FileEntry, bool) {
	entries := idx.byBase[name]
	if len(entries) == 0 {
		return FileEntry{}, false
	}
	best := entries[0]
	for _, e := range entries[1:] {
		bestDepth, depth := pathDepth(best.Path), pathDepth(e.Path)
		if depth < bestDepth || (depth == bestDepth && e.Path < best.Path) {
			best = e
		}
	}
	return best, true
}
