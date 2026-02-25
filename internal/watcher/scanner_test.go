package watcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

func TestScanDirectoryBasic(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "file1.md", "content1")
	testutil.CreateTestFile(t, dir, "file2.txt", "content2")

	tree, err := ScanDirectory(dir, false, nil)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	if tree == nil {
		t.Fatal("expected tree to be created, got nil")
	}

	if tree.Root == nil {
		t.Fatal("expected root node, got nil")
	}

	if len(tree.Root.Children) != 1 {
		t.Fatalf("expected 1 child (only .md files), got %d", len(tree.Root.Children))
	}

	if tree.Root.Children[0].Name != "file1.md" {
		t.Errorf("expected child to be file1.md, got %s", tree.Root.Children[0].Name)
	}
}

func TestScanDirectoryNested(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "root.md", "root content")
	testutil.CreateTestFile(t, dir, "subdir/nested.md", "nested content")
	testutil.CreateTestFile(t, dir, "subdir/deep/file.md", "deep content")

	tree, err := ScanDirectory(dir, false, nil)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	if len(tree.Root.Children) != 2 {
		t.Fatalf("expected 2 root children, got %d", len(tree.Root.Children))
	}

	var subdirNode *models.Node
	for _, child := range tree.Root.Children {
		if child.Name == "subdir" {
			subdirNode = child
			break
		}
	}

	if subdirNode == nil {
		t.Fatal("expected to find subdir node")
	}

	if !subdirNode.IsDir() {
		t.Error("expected subdir to be a directory")
	}

	if len(subdirNode.Children) != 2 {
		t.Fatalf("expected 2 children in subdir, got %d", len(subdirNode.Children))
	}

	var deepNode *models.Node
	for _, child := range subdirNode.Children {
		if child.Name == "deep" {
			deepNode = child
			break
		}
	}

	if deepNode == nil {
		t.Fatal("expected to find deep node")
	}

	if len(deepNode.Children) != 1 {
		t.Fatalf("expected 1 child in deep, got %d", len(deepNode.Children))
	}
}

func TestScanDirectoryEmpty(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)

	tree, err := ScanDirectory(dir, false, nil)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	if tree == nil {
		t.Fatal("expected tree to be created, got nil")
	}

	if len(tree.Root.Children) != 0 {
		t.Fatalf("expected 0 children in empty dir, got %d", len(tree.Root.Children))
	}
}

func TestScanDirectoryNonExistent(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	nonExistent := filepath.Join(dir, "nonexistent")

	_, err := ScanDirectory(nonExistent, false, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}

func TestScanDirectoryFile(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := ScanDirectory(filePath, false, nil)
	if err == nil {
		t.Fatal("expected error when scanning file instead of directory, got nil")
	}
}

func TestScanDirectoryModTime(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "file.md", "content")

	tree, err := ScanDirectory(dir, false, nil)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	if tree.Root.ModTime.IsZero() {
		t.Error("expected root ModTime to be set")
	}

	if len(tree.Root.Children) == 0 {
		t.Fatal("expected at least one child")
	}

	child := tree.Root.Children[0]
	if child.ModTime.IsZero() {
		t.Error("expected child ModTime to be set")
	}
}

func TestScanDirectoryNodeTypes(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "file.md", "content")
	testutil.CreateTestFile(t, dir, "subdir/nested.md", "nested")

	tree, err := ScanDirectory(dir, false, nil)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	var fileNode, dirNode *models.Node
	for _, child := range tree.Root.Children {
		if child.Name == "file.md" {
			fileNode = child
		}
		if child.Name == "subdir" {
			dirNode = child
		}
	}

	if fileNode == nil {
		t.Fatal("expected to find file node")
	}
	if !fileNode.IsFile() {
		t.Error("expected file.md to be a file")
	}

	if dirNode == nil {
		t.Fatal("expected to find directory node")
	}
	if !dirNode.IsDir() {
		t.Error("expected subdir to be a directory")
	}
}

func TestScanDirectoryFind(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "root.md", "root")
	testutil.CreateTestFile(t, dir, "subdir/nested.md", "nested")

	tree, err := ScanDirectory(dir, false, nil)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	nestedPath := filepath.Join(dir, "subdir", "nested.md")
	found := tree.Find(nestedPath)
	if found == nil {
		t.Fatal("expected to find nested.md")
	}

	if found.Name != "nested.md" {
		t.Errorf("expected name 'nested.md', got %s", found.Name)
	}
}

func TestScanDirectoryRelativePath(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "file.md", "content")

	tree, err := ScanDirectory(dir, false, nil)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	if tree.Root.Path != "." {
		t.Errorf("expected root path '.', got %s", tree.Root.Path)
	}

	if len(tree.Root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree.Root.Children))
	}

	if tree.Root.Children[0].Path != "file.md" {
		t.Errorf("expected child path 'file.md', got %s", tree.Root.Children[0].Path)
	}
}
