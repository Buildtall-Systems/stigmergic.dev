package server

import (
	"path"
	"strings"
	"unicode/utf8"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

const (
	searchResultLimit   = 20
	searchSnippetRadius = 40
)

type searchDoc struct {
	Path   string // the route serving the document, mount prefix included
	Title  string
	Source string // the name of the source holding it
	Text   string
	Lower  string
}

// searchIndex holds every document's content twice — original for snippet
// extraction, lowercased for case-insensitive matching — in files-slice
// order (most recently modified first), which is also result order.
type searchIndex []searchDoc

type searchMatch struct {
	Path       string `json:"path"`
	Title      string `json:"title"`
	Source     string `json:"source"`
	Snippet    string `json:"snippet"`
	MatchStart int    `json:"matchStart"`
	MatchEnd   int    `json:"matchEnd"`
}

type searchResponse struct {
	Query     string        `json:"query"`
	Results   []searchMatch `json:"results"`
	Truncated bool          `json:"truncated"`
}

// searchDocs holds every document keyed by route, so a rebuild can replace
// the entries whose contents changed and leave the rest alone. Lowercasing
// the whole corpus is the expensive part and is what this avoids repeating.
type searchDocs map[string]searchDoc

// updateSearchDocs returns the document set for corpus, recomputing only the
// routes named in changed and carrying the rest over from prev. Routes
// absent from corpus are dropped. sourceName travels onto every document so
// a result from a corpus spanning several sources says which one holds it.
func updateSearchDocs(prev searchDocs, corpus markdown.Corpus, changed markdown.ChangedRoutes, sourceName string) searchDocs {
	docs := make(searchDocs, len(corpus))

	for route, entry := range corpus {
		if _, dirty := changed[route]; !dirty {
			if cached, ok := prev[route]; ok {
				docs[route] = cached
				continue
			}
		}

		body := string(entry.Data)
		docs[route] = searchDoc{
			Path:   route,
			Title:  path.Base(route),
			Source: sourceName,
			Text:   body,
			Lower:  strings.ToLower(body),
		}
	}

	return docs
}

// orderSearchIndex materializes docs in files order, which is most recently
// modified first and therefore also result order.
func orderSearchIndex(docs searchDocs, files []models.SearchableFile) searchIndex {
	idx := make(searchIndex, 0, len(files))
	for _, f := range files {
		if doc, ok := docs[f.Path]; ok {
			idx = append(idx, doc)
		}
	}
	return idx
}

// search scans every document for the first case-insensitive occurrence of
// query, returning at most limit matches, each with a context snippet and
// the match offsets within it. An empty or whitespace query matches nothing.
func (idx searchIndex) search(query string, limit int) searchResponse {
	resp := searchResponse{Query: query, Results: []searchMatch{}}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return resp
	}

	for _, doc := range idx {
		pos := strings.Index(doc.Lower, q)
		if pos < 0 {
			continue
		}
		if len(resp.Results) >= limit {
			resp.Truncated = true
			break
		}
		// Lowercasing preserves byte offsets for ASCII text. When a
		// document contains case pairs that change encoded width, offsets
		// into the original would drift, so the snippet comes from the
		// lowered copy — cosmetically lowercased but correctly aligned.
		text := doc.Text
		if len(doc.Lower) != len(doc.Text) {
			text = doc.Lower
		}
		snippet, start, end := snippetAround(text, pos, pos+len(q))
		resp.Results = append(resp.Results, searchMatch{
			Path:       doc.Path,
			Title:      doc.Title,
			Source:     doc.Source,
			Snippet:    snippet,
			MatchStart: start,
			MatchEnd:   end,
		})
	}
	return resp
}

// snippetAround windows searchSnippetRadius bytes each side of the match,
// widened outward to rune boundaries, with line breaks flattened to spaces
// (byte-preserving, so offsets stay valid). Returns the snippet and the
// match offsets within it.
func snippetAround(text string, matchStart, matchEnd int) (string, int, int) {
	start := matchStart - searchSnippetRadius
	if start < 0 {
		start = 0
	}
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	end := matchEnd + searchSnippetRadius
	if end > len(text) {
		end = len(text)
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	snippet := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(text[start:end])
	return snippet, matchStart - start, matchEnd - start
}
