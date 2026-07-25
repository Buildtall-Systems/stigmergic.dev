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

	// ModTimeNano and Size together decide whether a rebuild must re-read a
	// file. ModTime is not usable for this: at second granularity, an edit
	// that changes one character within the same second looks unchanged,
	// which would leave the search and backlink indexes silently stale.
	// Both are excluded from the JSON payload because no client needs them
	// and /api/files already ships megabytes.
	ModTimeNano int64 `json:"-"`
	Size        int64 `json:"-"`
}

type Node struct {
	Name     string
	Path     string
	ModTime  time.Time
	Children []*Node
	Type     NodeType
	Size     int64
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

// Signature describes the tree's shape: every node's path and type, in
// traversal order. Mod times are deliberately excluded, so the signature
// changes when a file or directory is created, removed, or renamed, and
// never when a file's contents change.
//
// The signature is the shape material itself rather than a hash of it, so
// comparing two signatures is an exact equality test. A hash would trade a
// few hundred kilobytes for the possibility of a collision silently
// suppressing a sidebar refresh, which is the wrong trade for a value held
// once per tree.
func (t *Tree) Signature() string {
	if t.Root == nil {
		return ""
	}
	return string(appendSignature(nil, t.Root))
}

func appendSignature(buf []byte, node *Node) []byte {
	buf = append(buf, node.Path...)
	if node.IsDir() {
		buf = append(buf, '/')
	}
	buf = append(buf, '\n')
	for _, child := range node.Children {
		buf = appendSignature(buf, child)
	}
	return buf
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
			Name:        node.Name,
			Path:        "/" + node.Path,
			ModTime:     node.ModTime.Unix(),
			ModTimeNano: node.ModTime.UnixNano(),
			Size:        node.Size,
		})
		return
	}

	if node.IsDir() {
		for _, child := range node.Children {
			t.flattenMarkdownFilesRecursive(child, files)
		}
	}
}
