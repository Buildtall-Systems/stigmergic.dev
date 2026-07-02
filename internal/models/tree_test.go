package models

import (
	"path/filepath"
	"testing"
	"time"
)

const (
	rootNodeName = "root"
	fileOneName  = "file1.md"
	readmeName   = "readme.md"
	guideName    = "guide.md"
	docsName     = "docs"
	apiName      = "api.md"
)

func TestNewTree(t *testing.T) {
	t.Parallel()

	tree := NewTree("path")
	if tree == nil {
		t.Fatal("expected tree to be created, got nil")
	}

	if tree.Root == nil {
		t.Fatal("expected root node to be created, got nil")
	}

	if tree.Root.Type != NodeTypeDirectory {
		t.Errorf("expected root type to be directory, got %d", tree.Root.Type)
	}

	if tree.Root.Name != "path" {
		t.Errorf("expected root name to be 'path', got %s", tree.Root.Name)
	}

	if tree.Root.Path != "." {
		t.Errorf("expected root path to be '.', got %s", tree.Root.Path)
	}

	if tree.Root.Children == nil {
		t.Error("expected root children to be initialized, got nil")
	}
}

func TestNodeIsDir(t *testing.T) {
	t.Parallel()

	dirNode := &Node{Type: NodeTypeDirectory}
	if !dirNode.IsDir() {
		t.Error("expected IsDir to return true for directory node")
	}

	fileNode := &Node{Type: NodeTypeFile}
	if fileNode.IsDir() {
		t.Error("expected IsDir to return false for file node")
	}
}

func TestNodeIsFile(t *testing.T) {
	t.Parallel()

	fileNode := &Node{Type: NodeTypeFile}
	if !fileNode.IsFile() {
		t.Error("expected IsFile to return true for file node")
	}

	dirNode := &Node{Type: NodeTypeDirectory}
	if dirNode.IsFile() {
		t.Error("expected IsFile to return false for directory node")
	}
}

func TestNodeAddChild(t *testing.T) {
	t.Parallel()

	parent := &Node{
		Name:     "parent",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	child := &Node{
		Name: "child",
		Type: NodeTypeFile,
	}

	parent.AddChild(child)

	if len(parent.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(parent.Children))
	}

	if parent.Children[0] != child {
		t.Error("expected child to be added to parent")
	}
}

func TestNodeAddChildWithNilChildren(t *testing.T) {
	t.Parallel()

	parent := &Node{
		Name:     "parent",
		Type:     NodeTypeDirectory,
		Children: nil,
	}

	child := &Node{
		Name: "child",
		Type: NodeTypeFile,
	}

	parent.AddChild(child)

	if parent.Children == nil {
		t.Fatal("expected Children to be initialized, got nil")
	}

	if len(parent.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(parent.Children))
	}
}

func TestTreeFind(t *testing.T) {
	t.Parallel()

	root := &Node{
		Name:     rootNodeName,
		Path:     ".",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	child1 := &Node{
		Name: fileOneName,
		Path: fileOneName,
		Type: NodeTypeFile,
	}

	subdir := &Node{
		Name:     "subdir",
		Path:     "subdir",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	child2 := &Node{
		Name: "file2.md",
		Path: "subdir/file2.md",
		Type: NodeTypeFile,
	}

	root.AddChild(child1)
	root.AddChild(subdir)
	subdir.AddChild(child2)

	tree := &Tree{Root: root}

	found := tree.Find(fileOneName)
	if found == nil {
		t.Fatal("expected to find file1.md, got nil")
	}
	if found.Name != fileOneName {
		t.Errorf("expected to find file1.md, got %s", found.Name)
	}

	found = tree.Find("subdir/file2.md")
	if found == nil {
		t.Fatal("expected to find file2.md, got nil")
	}
	if found.Name != "file2.md" {
		t.Errorf("expected to find file2.md, got %s", found.Name)
	}

	found = tree.Find("nonexistent.md")
	if found != nil {
		t.Errorf("expected nil for nonexistent file, got %v", found)
	}
}

func TestTreeFindRoot(t *testing.T) {
	t.Parallel()

	tree := NewTree("path")

	found := tree.Find(".")
	if found == nil {
		t.Fatal("expected to find root node, got nil")
	}
	if found != tree.Root {
		t.Error("expected to find root node")
	}

	if empty := tree.Find(""); empty != tree.Root {
		t.Error("expected empty path to resolve to root")
	}
}

func TestTreeFindNilRoot(t *testing.T) {
	t.Parallel()

	tree := &Tree{}
	if found := tree.Find("anything"); found != nil {
		t.Errorf("expected nil for tree without root, got %v", found)
	}
}

func TestNodeModTime(t *testing.T) {
	t.Parallel()

	now := time.Now()
	node := &Node{
		ModTime: now,
	}

	if !node.ModTime.Equal(now) {
		t.Errorf("expected ModTime to be %v, got %v", now, node.ModTime)
	}
}

func TestTreeFindInFileNode(t *testing.T) {
	t.Parallel()

	root := &Node{
		Name:     rootNodeName,
		Path:     ".",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	fileNode := &Node{
		Name:     "file.md",
		Path:     "file.md",
		Type:     NodeTypeFile,
		Children: nil,
	}

	root.AddChild(fileNode)
	tree := &Tree{Root: root}

	found := tree.Find("file.md/child")
	if found != nil {
		t.Error("expected nil when searching in file node, got a result")
	}
}

func TestFlattenMarkdownFiles(t *testing.T) {
	t.Parallel()

	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now()

	root := &Node{
		Name:     rootNodeName,
		Path:     ".",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	file1 := &Node{
		Name:    readmeName,
		Path:    readmeName,
		Type:    NodeTypeFile,
		ModTime: older,
	}

	file2 := &Node{
		Name:    guideName,
		Path:    guideName,
		Type:    NodeTypeFile,
		ModTime: newer,
	}

	textFile := &Node{
		Name:    "notes.txt",
		Path:    "notes.txt",
		Type:    NodeTypeFile,
		ModTime: newer,
	}

	subdir := &Node{
		Name:     docsName,
		Path:     docsName,
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	file3 := &Node{
		Name:    apiName,
		Path:    "docs/api.md",
		Type:    NodeTypeFile,
		ModTime: older,
	}

	root.AddChild(file1)
	root.AddChild(file2)
	root.AddChild(textFile)
	root.AddChild(subdir)
	subdir.AddChild(file3)

	tree := &Tree{Root: root}

	files := tree.FlattenMarkdownFiles()

	if len(files) != 3 {
		t.Fatalf("expected 3 markdown files, got %d", len(files))
	}

	if files[0].Name != guideName {
		t.Errorf("expected first file to be guide.md (newest), got %s", files[0].Name)
	}

	if files[1].Name != readmeName && files[1].Name != apiName {
		t.Errorf("expected second file to be readme.md or api.md, got %s", files[1].Name)
	}

	if files[2].Name != apiName && files[2].Name != readmeName {
		t.Errorf("expected third file to be api.md or readme.md, got %s", files[2].Name)
	}

	for _, f := range files {
		if filepath.Ext(f.Name) != MarkdownExt {
			t.Errorf("expected only .md files, got %s", f.Name)
		}
	}
}

func TestFlattenMarkdownFilesEmpty(t *testing.T) {
	t.Parallel()

	root := &Node{
		Name:     rootNodeName,
		Path:     ".",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	tree := &Tree{Root: root}

	files := tree.FlattenMarkdownFiles()

	if len(files) != 0 {
		t.Fatalf("expected 0 markdown files, got %d", len(files))
	}
}

func TestFlattenMarkdownFilesPaths(t *testing.T) {
	t.Parallel()

	root := &Node{
		Name:     rootNodeName,
		Path:     ".",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	subdir := &Node{
		Name:     docsName,
		Path:     docsName,
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	file := &Node{
		Name:    guideName,
		Path:    "docs/guide.md",
		Type:    NodeTypeFile,
		ModTime: time.Now(),
	}

	root.AddChild(subdir)
	subdir.AddChild(file)

	tree := &Tree{Root: root}

	files := tree.FlattenMarkdownFiles()

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	expectedPath := "/docs/guide.md"
	if files[0].Path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, files[0].Path)
	}
}
