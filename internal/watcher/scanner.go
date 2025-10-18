package watcher

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

func ScanDirectory(rootPath string) (*models.Tree, error) {
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", absPath)
	}

	tree := models.NewTree(absPath)
	tree.Root.Path = "."
	tree.Root.ModTime = info.ModTime()

	if err := scanNode(tree.Root, absPath); err != nil {
		return nil, err
	}

	return tree, nil
}

func scanNode(node *models.Node, rootPath string) error {
	absNodePath := filepath.Join(rootPath, node.Path)
	entries, err := os.ReadDir(absNodePath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", absNodePath, err)
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		nodeType := models.NodeTypeFile
		if entry.IsDir() {
			nodeType = models.NodeTypeDirectory
		}

		childRelPath := filepath.Join(node.Path, entry.Name())

		child := &models.Node{
			Name:     entry.Name(),
			Path:     childRelPath,
			Type:     nodeType,
			ModTime:  info.ModTime(),
			Children: make([]*models.Node, 0),
		}

		node.AddChild(child)

		if entry.IsDir() {
			if err := scanNode(child, rootPath); err != nil {
				return err
			}
		}
	}

	return nil
}
