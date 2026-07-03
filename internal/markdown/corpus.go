package markdown

import (
	"io/fs"
	"strings"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

// ReadCorpus reads every markdown file once, keyed by route path (no leading
// slash). Both the backlink index and the search index build from this map,
// so a rescan costs a single I/O pass over the corpus. Unreadable files are
// skipped, matching the previous per-consumer behavior.
func ReadCorpus(contentFS fs.FS, files []models.SearchableFile) map[string][]byte {
	contents := make(map[string][]byte, len(files))
	for _, f := range files {
		route := strings.TrimPrefix(f.Path, "/")
		data, err := fs.ReadFile(contentFS, route)
		if err != nil {
			continue
		}
		contents[route] = data
	}
	return contents
}
