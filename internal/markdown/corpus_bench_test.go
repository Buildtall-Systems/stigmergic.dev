package markdown

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// benchCorpusFiles is small next to a real corpus but large enough that the
// difference between reading everything and reading one file is unambiguous.
const benchCorpusFiles = 500

// benchCorpusDir writes a synthetic corpus whose documents carry wikilinks,
// so extraction has real parsing to do rather than walking empty documents.
func benchCorpusDir(b *testing.B) string {
	b.Helper()

	dir := b.TempDir()
	for i := range benchCorpusFiles {
		body := fmt.Sprintf(
			"# Note %d\n\nSome prose about the subject at hand, long enough to parse.\n\nSee also [[note-%d]] and [[note-%d]].\n",
			i, (i+1)%benchCorpusFiles, (i+7)%benchCorpusFiles,
		)
		path := filepath.Join(dir, fmt.Sprintf("note-%d.md", i))
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			b.Fatalf("failed to write bench corpus: %v", err)
		}
	}

	return dir
}

// These benchmarks exist to keep the incremental path honest: cold measures
// the work a first build does, warm measures a rebuild after a single file
// changed. `make test` does not run them; invoke them directly when changing
// anything in the rebuild path.

func BenchmarkReadCorpusCold(b *testing.B) {
	dir := benchCorpusDir(b)
	fsys := os.DirFS(dir)
	files := scanFiles(b, dir)

	for b.Loop() {
		ReadCorpus(fsys, nil, files)
	}
}

func BenchmarkReadCorpusWarm(b *testing.B) {
	dir := benchCorpusDir(b)
	fsys := os.DirFS(dir)
	files := scanFiles(b, dir)
	warm, _ := ReadCorpus(fsys, nil, files)

	for b.Loop() {
		ReadCorpus(fsys, warm, files)
	}
}

func BenchmarkExtractLinkRefsCold(b *testing.B) {
	dir := benchCorpusDir(b)
	files := scanFiles(b, dir)
	corpus, changed := ReadCorpus(os.DirFS(dir), nil, files)

	for b.Loop() {
		ExtractLinkRefs(nil, corpus, changed)
	}
}

func BenchmarkExtractLinkRefsWarm(b *testing.B) {
	dir := benchCorpusDir(b)
	files := scanFiles(b, dir)
	corpus, changed := ReadCorpus(os.DirFS(dir), nil, files)
	warm := ExtractLinkRefs(nil, corpus, changed)

	oneChanged := ChangedRoutes{"note-0.md": {}}

	for b.Loop() {
		ExtractLinkRefs(warm, corpus, oneChanged)
	}
}

// BenchmarkBuildBacklinkIndex measures the pass that still runs in full on
// every rebuild, since resolution depends on the whole file set. It should
// stay cheap enough that running it unconditionally is the right trade.
func BenchmarkBuildBacklinkIndex(b *testing.B) {
	dir := benchCorpusDir(b)
	files := scanFiles(b, dir)
	corpus, changed := ReadCorpus(os.DirFS(dir), nil, files)
	refs := ExtractLinkRefs(nil, corpus, changed)

	for b.Loop() {
		BuildBacklinkIndex(refs, files)
	}
}
