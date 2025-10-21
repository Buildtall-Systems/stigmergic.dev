package models

import (
	"path/filepath"
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
)

type Node struct {
	Name     string
	Path     string
	Type     NodeType
	ModTime  time.Time
	Children []*Node
}

type Tree struct {
	Root     *Node
	RootPath string
}

func NewTree(rootPath string) *Tree {
	absPath, _ := filepath.Abs(rootPath)
	return &Tree{
		RootPath: absPath,
		Root: &Node{
			Name:     filepath.Base(absPath),
			Path:     absPath,
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
		return filepath.Ext(n.Name) == ".md"
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

func (t *Tree) Find(path string) *Node {
	absPath, _ := filepath.Abs(path)
	relPath, err := filepath.Rel(t.RootPath, absPath)
	if err != nil {
		return nil
	}
	return t.findNode(t.Root, relPath)
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
