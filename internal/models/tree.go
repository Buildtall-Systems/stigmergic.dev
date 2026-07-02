package models

import (
	"path/filepath"
	"sort"
	"time"
)

type Breadcrumb struct {
	Name string
	Path string
}

type NodeType int

const (
	NodeTypeFile NodeType = iota
	NodeTypeDirectory

	MarkdownExt = ".md"
)

type SearchableFile struct {
	Name         string
	Path         string
	RelativeTime string
	ModTime      int64
}

type Node struct {
	Name     string
	Path     string
	ModTime  time.Time
	Children []*Node
	Type     NodeType
}

type Tree struct {
	Root *Node
}

func NewTree(name string) *Tree {
	return &Tree{
		Root: &Node{
			Name:     name,
			Path:     ".",
			Type:     NodeTypeDirectory,
			Children: make([]*Node, 0),
		},
	}
}

func (n *Node) IsDir() bool {
	return n.Type == NodeTypeDirectory
}

func (n *Node) IsFile() bool {
	return n.Type == NodeTypeFile
}

func (n *Node) ContainsMarkdown() bool {
	if n.IsFile() {
		return filepath.Ext(n.Name) == MarkdownExt
	}

	for _, child := range n.Children {
		if child.ContainsMarkdown() {
			return true
		}
	}

	return false
}

func (n *Node) AddChild(child *Node) {
	if n.Children == nil {
		n.Children = make([]*Node, 0)
	}
	n.Children = append(n.Children, child)
}

// Find looks up a node by route-relative, forward-slash path ("." is the
// root). Paths never carry a leading slash.
func (t *Tree) Find(path string) *Node {
	if t.Root == nil {
		return nil
	}
	if path == "" {
		path = "."
	}
	return t.findNode(t.Root, path)
}

func (t *Tree) findNode(node *Node, path string) *Node {
	if node.Path == path {
		return node
	}

	if node.IsFile() {
		return nil
	}

	for _, child := range node.Children {
		if found := t.findNode(child, path); found != nil {
			return found
		}
	}

	return nil
}

func (t *Tree) FlattenMarkdownFiles() []SearchableFile {
	var files []SearchableFile
	t.flattenMarkdownFilesRecursive(t.Root, &files)
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime > files[j].ModTime
	})
	return files
}

func (t *Tree) flattenMarkdownFilesRecursive(node *Node, files *[]SearchableFile) {
	if node.IsFile() && filepath.Ext(node.Name) == MarkdownExt {
		*files = append(*files, SearchableFile{
			Name:    node.Name,
			Path:    "/" + node.Path,
			ModTime: node.ModTime.Unix(),
		})
		return
	}

	if node.IsDir() {
		for _, child := range node.Children {
			t.flattenMarkdownFilesRecursive(child, files)
		}
	}
}
