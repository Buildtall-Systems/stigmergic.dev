package models

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewTree(t *testing.T) {
	t.Parallel()

	tree := NewTree("/test/path")
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
		Name:     "root",
		Path:     "/test/root",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	child1 := &Node{
		Name: "file1.md",
		Path: "/test/root/file1.md",
		Type: NodeTypeFile,
	}

	subdir := &Node{
		Name:     "subdir",
		Path:     "/test/root/subdir",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	child2 := &Node{
		Name: "file2.md",
		Path: "/test/root/subdir/file2.md",
		Type: NodeTypeFile,
	}

	root.AddChild(child1)
	root.AddChild(subdir)
	subdir.AddChild(child2)

	tree := &Tree{Root: root}

	found := tree.Find("/test/root/file1.md")
	if found == nil {
		t.Fatal("expected to find file1.md, got nil")
	}
	if found.Name != "file1.md" {
		t.Errorf("expected to find file1.md, got %s", found.Name)
	}

	found = tree.Find("/test/root/subdir/file2.md")
	if found == nil {
		t.Fatal("expected to find file2.md, got nil")
	}
	if found.Name != "file2.md" {
		t.Errorf("expected to find file2.md, got %s", found.Name)
	}

	found = tree.Find("/test/root/nonexistent.md")
	if found != nil {
		t.Errorf("expected nil for nonexistent file, got %v", found)
	}
}

func TestTreeFindRoot(t *testing.T) {
	t.Parallel()

	tree := NewTree("/test/path")
	absPath, _ := filepath.Abs("/test/path")
	tree.Root.Path = absPath

	found := tree.Find(absPath)
	if found == nil {
		t.Fatal("expected to find root node, got nil")
	}
	if found != tree.Root {
		t.Error("expected to find root node")
	}
}

func TestNodeModTime(t *testing.T) {
	t.Parallel()

	now := time.Now()
	node := &Node{
		Name:    "test.md",
		Path:    "/test/test.md",
		Type:    NodeTypeFile,
		ModTime: now,
	}

	if !node.ModTime.Equal(now) {
		t.Errorf("expected ModTime to be %v, got %v", now, node.ModTime)
	}
}

func TestTreeFindInFileNode(t *testing.T) {
	t.Parallel()

	root := &Node{
		Name:     "root",
		Path:     "/test/root",
		Type:     NodeTypeDirectory,
		Children: make([]*Node, 0),
	}

	fileNode := &Node{
		Name:     "file.md",
		Path:     "/test/root/file.md",
		Type:     NodeTypeFile,
		Children: nil,
	}

	root.AddChild(fileNode)
	tree := &Tree{Root: root}

	found := tree.Find("/test/root/file.md/child")
	if found != nil {
		t.Error("expected nil when searching in file node, got a result")
	}
}
