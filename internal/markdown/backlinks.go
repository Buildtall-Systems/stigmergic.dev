package markdown

import (
	"os"
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

// BuildBacklinkIndex scans all files for wikilinks and builds an inverse
// index mapping each target route to the files that link to it.
func BuildBacklinkIndex(rootPath string, files []models.SearchableFile) models.BacklinkIndex {
	resolver := NewTreeResolver(files)

	md := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithInlineParsers(util.Prioritized(&wikilink.Parser{}, 199)),
		),
	)

	index := make(models.BacklinkIndex)

	for _, f := range files {
		route := strings.TrimPrefix(f.Path, "/")

		source, err := os.ReadFile(filepath.Join(rootPath, route)) //nolint:gosec // G304: paths come from our scanned file tree, not user input
		if err != nil {
			continue
		}

		reader := text.NewReader(source)
		doc := md.Parser().Parse(reader)

		seen := make(map[string]bool)
		_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			wn, ok := n.(*wikilink.Node)
			if !ok {
				return ast.WalkContinue, nil
			}

			dest, err := resolver.ResolveWikilink(wn)
			if err != nil || len(dest) == 0 {
				return ast.WalkContinue, nil
			}

			targetRoute := strings.TrimPrefix(string(dest), "/file/")
			// Strip fragment anchors.
			if idx := strings.Index(targetRoute, "#"); idx >= 0 {
				targetRoute = targetRoute[:idx]
			}

			// Skip self-links and duplicates within the same source.
			if targetRoute == route || seen[targetRoute] {
				return ast.WalkContinue, nil
			}
			seen[targetRoute] = true

			index[targetRoute] = append(index[targetRoute], models.BacklinkEntry{
				SourcePath:  route,
				SourceTitle: titleFromFilename(route),
			})

			return ast.WalkContinue, nil
		})
	}

	for target, entries := range index {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].SourceTitle < entries[j].SourceTitle
		})
		index[target] = entries
	}

	return index
}

// titleFromFilename extracts a display name from a route path.
func titleFromFilename(route string) string {
	return strings.TrimSuffix(filepath.Base(route), ".md")
}
