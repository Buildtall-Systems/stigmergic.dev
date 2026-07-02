package source

import (
	"io/fs"
	"testing"

	"github.com/Buildtall-Systems/stigmergic.dev/site"
)

const embeddedTestName = "embedded-test"

func TestEmbeddedCoreInterface(t *testing.T) {
	t.Parallel()

	fsys := mapFS(map[string]string{readmeName: helloContent})
	src := NewEmbedded(fsys, embeddedTestName)

	if src.Name() != embeddedTestName {
		t.Errorf("expected name %q, got %q", embeddedTestName, src.Name())
	}

	data, err := fs.ReadFile(src.FS(), readmeName)
	if err != nil {
		t.Fatalf("failed to read through source FS: %v", err)
	}
	if string(data) != helloContent {
		t.Errorf("expected content %q, got %q", helloContent, string(data))
	}

	if err := src.Close(); err != nil {
		t.Errorf("expected no-op close to succeed, got %v", err)
	}
	if err := src.Close(); err != nil {
		t.Errorf("expected repeated close to succeed, got %v", err)
	}
}

func TestEmbeddedAssertsNoCapabilities(t *testing.T) {
	t.Parallel()

	var src ContentSource = NewEmbedded(mapFS(nil), embeddedTestName)

	if _, ok := src.(Watchable); ok {
		t.Error("EmbeddedSource must not assert Watchable")
	}
	if _, ok := src.(GitignoreAware); ok {
		t.Error("EmbeddedSource must not assert GitignoreAware")
	}
	if _, ok := src.(Timestamped); ok {
		t.Error("EmbeddedSource must not assert Timestamped")
	}
	if _, ok := src.(Rooted); ok {
		t.Error("EmbeddedSource must not assert Rooted")
	}
}

func TestEmbeddedScan(t *testing.T) {
	t.Parallel()

	src := NewEmbedded(mapFS(map[string]string{
		indexName:      "index",
		guidePath:      guideContent,
		"img/logo.png": binaryContent,
	}), embeddedTestName)

	tree, err := Scan(src.FS(), false, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if findChild(tree.Root, indexName) == nil {
		t.Error("expected index.md in tree")
	}
	if findChild(tree.Root, "docs") == nil {
		t.Error("expected docs directory in tree")
	}
	if findChild(tree.Root, "img") != nil {
		t.Error("expected markdown-less img directory to be pruned")
	}
}

func TestEmbeddedRealSiteFS(t *testing.T) {
	t.Parallel()

	fsys, err := site.Content()
	if err != nil {
		t.Fatalf("failed to open embedded site content: %v", err)
	}
	src := NewEmbedded(fsys, embeddedTestName)

	tree, err := Scan(src.FS(), false, nil)
	if err != nil {
		t.Fatalf("Scan failed over real site FS: %v", err)
	}

	for _, page := range []string{indexName, "installation.md", "features.md", "architecture.md", "demo.md"} {
		if findChild(tree.Root, page) == nil {
			t.Errorf("expected %s in embedded site tree", page)
		}
	}

	data, err := fs.ReadFile(src.FS(), "img/stigmergic.png")
	if err != nil {
		t.Fatalf("failed to read embedded image: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty embedded image")
	}
}
