package markdown

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.abhg.dev/goldmark/wikilink"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

const (
	nameA  = "a.md"
	routeA = "/a.md"
)

func writeTestFile(t *testing.T, dir, route, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, route)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mounted restates a scanned file list the way the server does before
// indexing: each path becomes the route serving it under the single /file/
// mount these tests scan into.
func mounted(files []models.SearchableFile) []models.SearchableFile {
	routed := make([]models.SearchableFile, 0, len(files))
	for _, f := range files {
		f.Path = FileMount + strings.TrimPrefix(f.Path, "/")
		routed = append(routed, f)
	}
	return routed
}

// route is where a source-relative path is served from in these tests.
func route(rel string) string {
	return FileMount + rel
}

// mountedIndex inverts one source's links the way a server holding a single
// mount does: every document answers through the same tree resolver, and the
// only registered route prefix is the one mount.
func mountedIndex(refs LinkRefs, files []models.SearchableFile) models.BacklinkIndex {
	resolver := NewTreeResolver(files)
	return BuildBacklinkIndex(refs, mounted(files), func(string) wikilink.Resolver { return resolver }, []string{FileMount})
}

// coldBacklinkIndex builds the index from nothing, the way a first rebuild
// does: read everything, parse everything, resolve and invert.
func coldBacklinkIndex(dir string, files []models.SearchableFile) models.BacklinkIndex {
	corpus, changed := ReadCorpus(os.DirFS(dir), nil, FileMount, files)
	return mountedIndex(ExtractLinkRefs(nil, corpus, changed), files)
}

// scanFiles lists the markdown files under dir the way the real scanner
// does, carrying the stamps that decide whether a rebuild re-reads a file.
func scanFiles(tb testing.TB, dir string) []models.SearchableFile {
	tb.Helper()

	var files []models.SearchableFile
	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(d.Name()) != models.MarkdownExt {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		files = append(files, models.SearchableFile{
			Name:        d.Name(),
			Path:        "/" + filepath.ToSlash(rel),
			ModTime:     info.ModTime().Unix(),
			ModTimeNano: info.ModTime().UnixNano(),
			Size:        info.Size(),
		})
		return nil
	})
	if walkErr != nil {
		tb.Fatalf("failed to scan test corpus: %v", walkErr)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].ModTime != files[j].ModTime {
			return files[i].ModTime > files[j].ModTime
		}
		return files[i].Path < files[j].Path
	})

	return files
}

// TestIncrementalIndexMatchesFullRebuild is the correctness contract for
// incremental rebuilds: state carried forward must produce exactly the index
// a cold build produces, after any sequence of edits, additions, renames and
// deletions. A weak version of this test would be worse than none, because
// the defect it guards against is a stale backlink appearing intermittently.
//
// Every edit below changes the file's length. The stamp is mod time plus
// size, so an edit preserving both within one mod time tick is by design
// invisible; making the lengths differ keeps the test measuring the
// incremental logic rather than the clock.
func TestIncrementalIndexMatchesFullRebuild(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "Link to [[b]] and [[docs/deep]].")
	writeTestFile(t, dir, "b.md", "Back to [[a]].")
	writeTestFile(t, dir, "docs/deep.md", "Deep note linking [[b]].")

	steps := []struct {
		mutate func()
		name   string
	}{
		{name: "initial build", mutate: func() {}},
		{name: "edit contents, links unchanged", mutate: func() {
			writeTestFile(t, dir, "b.md", "Back to [[a]], with more words than before.")
		}},
		{name: "edit contents, links changed", mutate: func() {
			writeTestFile(t, dir, "b.md", "Now points at [[docs/deep]] instead, plus padding.")
		}},
		{name: "add file", mutate: func() {
			writeTestFile(t, dir, "c.md", "New note pointing at [[a]].")
		}},
		{name: "add same-named file in another directory", mutate: func() {
			writeTestFile(t, dir, "docs/c.md", "Another c, also pointing at [[a]].")
		}},
		// A rename changes no other file's bytes, but every unresolved
		// wikilink naming the old page must stop resolving and every one
		// naming the new page must start. Nothing here would be caught if
		// resolution were cached alongside parsing.
		{name: "rename a link target", mutate: func() {
			if err := os.Rename(filepath.Join(dir, "docs/deep.md"), filepath.Join(dir, "docs/deeper.md")); err != nil {
				t.Fatalf("failed to rename: %v", err)
			}
		}},
		{name: "delete a link source", mutate: func() {
			if err := os.Remove(filepath.Join(dir, "c.md")); err != nil {
				t.Fatalf("failed to remove: %v", err)
			}
		}},
		{name: "restore the deleted name with different contents", mutate: func() {
			writeTestFile(t, dir, "c.md", "Restored c, now linking [[b]] rather than a.")
		}},
		{name: "edit away every link", mutate: func() {
			writeTestFile(t, dir, "a.md", "No links at all in this document any more.")
		}},
	}

	var (
		corpus Corpus
		links  LinkRefs
	)

	for _, step := range steps {
		step.mutate()

		files := scanFiles(t, dir)

		var changed ChangedRoutes
		corpus, changed = ReadCorpus(os.DirFS(dir), corpus, FileMount, files)
		links = ExtractLinkRefs(links, corpus, changed)

		got := mountedIndex(links, files)
		want := coldBacklinkIndex(dir, files)

		if !reflect.DeepEqual(got, want) {
			t.Errorf("after %q: incremental index differs from full rebuild\ngot:  %v\nwant: %v", step.name, got, want)
		}
	}
}

// TestIncrementalCorpusRereadsOnlyChangedFiles pins the saving itself. If
// this regresses, correctness survives but the phase has no point.
func TestIncrementalCorpusRereadsOnlyChangedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "first")
	writeTestFile(t, dir, "b.md", "second")
	writeTestFile(t, dir, "c.md", "third")

	files := scanFiles(t, dir)
	corpus, changed := ReadCorpus(os.DirFS(dir), nil, FileMount, files)
	if len(changed) != 3 {
		t.Fatalf("expected a cold build to read all 3 files, read %d", len(changed))
	}

	corpus, changed = ReadCorpus(os.DirFS(dir), corpus, FileMount, scanFiles(t, dir))
	if len(changed) != 0 {
		t.Errorf("expected an unchanged corpus to read nothing, read %d", len(changed))
	}

	writeTestFile(t, dir, "b.md", "second, edited to a different length")

	_, changed = ReadCorpus(os.DirFS(dir), corpus, FileMount, scanFiles(t, dir))
	if len(changed) != 1 {
		t.Fatalf("expected exactly 1 re-read, got %d", len(changed))
	}
	if _, ok := changed[route("b.md")]; !ok {
		t.Errorf("expected b.md to be the re-read file, got %v", changed)
	}
}

// TestIncrementalCorpusDropsRemovedFiles guards the other direction: state
// carried forward must not resurrect a file that no longer exists.
func TestIncrementalCorpusDropsRemovedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "first")
	writeTestFile(t, dir, "b.md", "second")

	corpus, _ := ReadCorpus(os.DirFS(dir), nil, FileMount, scanFiles(t, dir))

	if err := os.Remove(filepath.Join(dir, "b.md")); err != nil {
		t.Fatalf("failed to remove: %v", err)
	}

	corpus, _ = ReadCorpus(os.DirFS(dir), corpus, FileMount, scanFiles(t, dir))

	if _, ok := corpus[route("b.md")]; ok {
		t.Error("expected the removed file to be dropped from the corpus")
	}
	if _, ok := corpus[route("a.md")]; !ok {
		t.Error("expected the surviving file to remain in the corpus")
	}
}

func TestBuildBacklinkIndex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "Link to [[b]].")
	writeTestFile(t, dir, "b.md", "No links here.")
	writeTestFile(t, dir, "c.md", "Also links to [[b]].")

	files := []models.SearchableFile{
		{Name: nameA, Path: routeA},
		{Name: "b.md", Path: "/b.md"},
		{Name: "c.md", Path: "/c.md"},
	}

	index := coldBacklinkIndex(dir, files)

	entries := index[route("b.md")]
	if len(entries) != 2 {
		t.Fatalf("expected 2 backlinks to b.md, got %d", len(entries))
	}

	sources := map[string]bool{}
	for _, e := range entries {
		sources[e.SourcePath] = true
	}
	if !sources[route("a.md")] {
		t.Error("expected a.md in backlinks to b.md")
	}
	if !sources[route("c.md")] {
		t.Error("expected c.md in backlinks to b.md")
	}
}

func TestBuildBacklinkIndexNoLinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "No wikilinks here.")

	files := []models.SearchableFile{
		{Name: nameA, Path: routeA},
	}

	index := coldBacklinkIndex(dir, files)

	if len(index) != 0 {
		t.Errorf("expected empty index, got %d entries", len(index))
	}
}

func TestBuildBacklinkIndexSelfLink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "Self-referencing [[a]].")

	files := []models.SearchableFile{
		{Name: nameA, Path: routeA},
	}

	index := coldBacklinkIndex(dir, files)

	if entries := index[route("a.md")]; len(entries) != 0 {
		t.Errorf("expected self-links to be excluded, got %d entries", len(entries))
	}
}

func TestBuildBacklinkIndexDuplicateLinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "Link to [[b]] and again [[b]].")
	writeTestFile(t, dir, "b.md", "Target.")

	files := []models.SearchableFile{
		{Name: nameA, Path: routeA},
		{Name: "b.md", Path: "/b.md"},
	}

	index := coldBacklinkIndex(dir, files)

	entries := index[route("b.md")]
	if len(entries) != 1 {
		t.Errorf("expected 1 backlink entry (deduped), got %d", len(entries))
	}
}

func TestBuildBacklinkIndexUnresolved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.md", "Link to [[nonexistent]].")

	files := []models.SearchableFile{
		{Name: nameA, Path: routeA},
	}

	index := coldBacklinkIndex(dir, files)

	if len(index) != 0 {
		t.Errorf("expected empty index for unresolved links, got %d entries", len(index))
	}
}

func TestTitleFromFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		route string
		want  string
	}{
		{"index.md", "index"},
		{"folder/note.md", "note"},
		{"a/b/deep.md", "deep"},
	}

	for _, tt := range tests {
		if got := titleFromFilename(tt.route); got != tt.want {
			t.Errorf("titleFromFilename(%q) = %q, want %q", tt.route, got, tt.want)
		}
	}
}
