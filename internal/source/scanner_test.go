package source

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

const (
	readmeName   = "readme.md"
	helloContent = "hello"
)

func mapFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

func findChild(node *models.Node, name string) *models.Node {
	for _, child := range node.Children {
		if child.Name == name {
			return child
		}
	}
	return nil
}

func TestScanBasic(t *testing.T) {
	t.Parallel()

	fsys := mapFS(map[string]string{
		readmeName: "# hello",
		"notes.md": "notes",
	})

	tree, err := Scan(fsys, false, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if tree.Root == nil {
		t.Fatal("expected root node")
	}
	if tree.Root.Path != "." {
		t.Errorf("expected root path %q, got %q", ".", tree.Root.Path)
	}
	if len(tree.Root.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(tree.Root.Children))
	}
}

func TestScanNested(t *testing.T) {
	t.Parallel()

	fsys := mapFS(map[string]string{
		"index.md":            "index",
		"docs/guide.md":       "guide",
		"docs/deep/nested.md": "nested",
	})

	tree, err := Scan(fsys, false, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	docs := findChild(tree.Root, "docs")
	if docs == nil {
		t.Fatal("expected docs directory in tree")
	}
	if docs.Path != "docs" {
		t.Errorf("expected route-relative path %q, got %q", "docs", docs.Path)
	}

	deep := findChild(docs, "deep")
	if deep == nil {
		t.Fatal("expected deep directory under docs")
	}
	if deep.Path != "docs/deep" {
		t.Errorf("expected forward-slash path %q, got %q", "docs/deep", deep.Path)
	}

	nested := findChild(deep, "nested.md")
	if nested == nil {
		t.Fatal("expected nested.md under docs/deep")
	}
	if nested.Path != "docs/deep/nested.md" {
		t.Errorf("expected path %q, got %q", "docs/deep/nested.md", nested.Path)
	}
}

func TestScanSkipsNonMarkdown(t *testing.T) {
	t.Parallel()

	fsys := mapFS(map[string]string{
		readmeName:  helloContent,
		"image.png": "binary",
		"data.json": "{}",
	})

	tree, err := Scan(fsys, false, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(tree.Root.Children) != 1 {
		t.Fatalf("expected only markdown files, got %d children", len(tree.Root.Children))
	}
	if tree.Root.Children[0].Name != readmeName {
		t.Errorf("expected readme.md, got %s", tree.Root.Children[0].Name)
	}
}

func TestScanPrunesEmptyDirectories(t *testing.T) {
	t.Parallel()

	fsys := mapFS(map[string]string{
		readmeName:       "hello",
		"assets/img.png": "binary",
	})

	tree, err := Scan(fsys, false, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if findChild(tree.Root, "assets") != nil {
		t.Error("expected assets directory without markdown to be pruned")
	}
}

func TestScanIgnorePatterns(t *testing.T) {
	t.Parallel()

	fsys := mapFS(map[string]string{
		readmeName:         "hello",
		"vendor/dep.md":    "dep docs",
		"notes/private.md": "private",
	})

	tree, err := Scan(fsys, false, []string{"vendor"})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if findChild(tree.Root, "vendor") != nil {
		t.Error("expected vendor to be ignored")
	}
	if findChild(tree.Root, "notes") == nil {
		t.Error("expected notes to survive")
	}
}

func TestScanHonorsGitignore(t *testing.T) {
	t.Parallel()

	fsys := mapFS(map[string]string{
		".gitignore":       "secret\n# comment\n",
		"public.md":        "public",
		"secret/hidden.md": "hidden",
	})

	tree, err := Scan(fsys, true, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if findChild(tree.Root, "secret") != nil {
		t.Error("expected gitignored directory to be excluded")
	}

	treeAll, err := Scan(fsys, false, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if findChild(treeAll.Root, "secret") == nil {
		t.Error("expected gitignored directory to be included when not respecting gitignore")
	}
}

func TestScanCapturesModTimes(t *testing.T) {
	t.Parallel()

	stamp := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"stamped.md": &fstest.MapFile{Data: []byte("x"), ModTime: stamp},
	}

	tree, err := Scan(fsys, false, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	node := findChild(tree.Root, "stamped.md")
	if node == nil {
		t.Fatal("expected stamped.md in tree")
	}
	if !node.ModTime.Equal(stamp) {
		t.Errorf("expected mod time %v, got %v", stamp, node.ModTime)
	}
}

func TestScanZeroModTimesAccepted(t *testing.T) {
	t.Parallel()

	fsys := mapFS(map[string]string{"zero.md": "x"})

	tree, err := Scan(fsys, false, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	node := findChild(tree.Root, "zero.md")
	if node == nil {
		t.Fatal("expected zero.md in tree")
	}
	if !node.ModTime.IsZero() {
		t.Errorf("expected zero mod time, got %v", node.ModTime)
	}
}

func TestScanFindRouteRelative(t *testing.T) {
	t.Parallel()

	fsys := mapFS(map[string]string{
		"docs/guide.md": "guide",
	})

	tree, err := Scan(fsys, false, nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if tree.Find("docs") == nil {
		t.Error("expected Find to resolve route-relative directory path")
	}
	if tree.Find("docs/guide.md") == nil {
		t.Error("expected Find to resolve route-relative file path")
	}
	if tree.Find(".") != tree.Root {
		t.Error("expected Find(\".\") to return the root")
	}
	if tree.Find("missing") != nil {
		t.Error("expected Find on unknown path to return nil")
	}
}
