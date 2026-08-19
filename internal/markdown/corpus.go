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

// Corpus holds every markdown file's contents, keyed by the route serving
// it: the mount prefix of the source holding the file, followed by the
// file's path within that source. One corpus therefore spans every mounted
// source without its keys ever colliding.
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
// The files are one source's own, their paths relative to contentFS; mount
// is the route prefix that source answers at, and prefixing it is what makes
// the returned keys routes. prev is the whole corpus, every source included,
// so a rebuild carries entries over no matter which source they came from.
func ReadCorpus(contentFS fs.FS, prev Corpus, mount string, files []models.SearchableFile) (Corpus, ChangedRoutes) {
	corpus := make(Corpus, len(files))
	changed := make(ChangedRoutes)

	for _, f := range files {
		route := mount + strings.TrimPrefix(f.Path, "/")

		if cached, ok := prev[route]; ok && cached.ModTime == f.ModTimeNano && cached.Size == f.Size {
			corpus[route] = cached
			continue
		}

		data, err := fs.ReadFile(contentFS, strings.TrimPrefix(f.Path, "/"))
		if err != nil {
			continue
		}

		corpus[route] = CorpusEntry{Data: data, ModTime: f.ModTimeNano, Size: f.Size}
		changed[route] = struct{}{}
	}

	return corpus, changed
}
