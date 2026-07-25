package markdown

import (
	"io/fs"
	"strings"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

// CorpusEntry is one file's bytes together with the stamp that decides
// whether a later rebuild has to read it again.
type CorpusEntry struct {
	Data    []byte
	ModTime int64
	Size    int64
}

// Corpus holds every markdown file's contents, keyed by route path (no
// leading slash).
type Corpus map[string]CorpusEntry

// ChangedRoutes is the set of routes a rebuild actually read. Whatever is
// derived from a single file's bytes needs recomputing exactly for these;
// whatever is derived from the corpus as a whole needs recomputing
// regardless.
type ChangedRoutes map[string]struct{}

// ReadCorpus returns the corpus for files, carrying entries over from prev
// whenever a file's mod time and size are both unchanged. Files absent from
// files are dropped, so the result always describes the current tree
// exactly. Unreadable files are skipped, matching the behavior every
// consumer already tolerates; leaving such a route out of both maps means a
// later rebuild retries it.
//
// The returned set names the routes actually read. A first build, where prev
// is nil, reports every route as changed.
//
// The stamp is nanosecond mod time plus size. That pair can in principle
// miss an edit preserving both, which needs a write landing inside the
// filesystem's mod time granularity without changing the file's length. The
// alternative, hashing contents, would mean reading every file on every
// rebuild, which is the cost this exists to avoid.
func ReadCorpus(contentFS fs.FS, prev Corpus, files []models.SearchableFile) (Corpus, ChangedRoutes) {
	corpus := make(Corpus, len(files))
	changed := make(ChangedRoutes)

	for _, f := range files {
		route := strings.TrimPrefix(f.Path, "/")

		if cached, ok := prev[route]; ok && cached.ModTime == f.ModTimeNano && cached.Size == f.Size {
			corpus[route] = cached
			continue
		}

		data, err := fs.ReadFile(contentFS, route)
		if err != nil {
			continue
		}

		corpus[route] = CorpusEntry{Data: data, ModTime: f.ModTimeNano, Size: f.Size}
		changed[route] = struct{}{}
	}

	return corpus, changed
}
