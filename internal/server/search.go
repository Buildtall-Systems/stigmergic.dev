package server

import (
	"strings"
	"unicode/utf8"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

const (
	searchResultLimit   = 20
	searchSnippetRadius = 40
)

type searchDoc struct {
	Path  string // route path with leading slash, as served by /api/files
	Title string
	Text  string
	Lower string
}

// searchIndex holds every document's content twice — original for snippet
// extraction, lowercased for case-insensitive matching — in files-slice
// order (most recently modified first), which is also result order.
type searchIndex []searchDoc

type searchMatch struct {
	Path       string `json:"path"`
	Title      string `json:"title"`
	Snippet    string `json:"snippet"`
	MatchStart int    `json:"matchStart"`
	MatchEnd   int    `json:"matchEnd"`
}

type searchResponse struct {
	Query     string        `json:"query"`
	Results   []searchMatch `json:"results"`
	Truncated bool          `json:"truncated"`
}

func buildSearchIndex(contents map[string][]byte, files []models.SearchableFile) searchIndex {
	idx := make(searchIndex, 0, len(files))
	for _, f := range files {
		route := strings.TrimPrefix(f.Path, "/")
		data, ok := contents[route]
		if !ok {
			continue
		}
		text := string(data)
		idx = append(idx, searchDoc{
			Path:  f.Path,
			Title: f.Name,
			Text:  text,
			Lower: strings.ToLower(text),
		})
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
