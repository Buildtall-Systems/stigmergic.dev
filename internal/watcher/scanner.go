package watcher

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	gitignore "github.com/sabhiram/go-gitignore"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

func ScanDirectory(rootPath string, respectGitignore bool, ignorePatterns []string) (*models.Tree, error) {
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

	var allPatterns []string
	allPatterns = append(allPatterns, ignorePatterns...)

	if respectGitignore {
		gitignorePath := filepath.Join(absPath, ".gitignore")
		if file, err := os.Open(gitignorePath); err == nil { //nolint:gosec
			defer func() { _ = file.Close() }()
			scanner := bufio.NewScanner(file)
			lineCount := 0
			for scanner.Scan() {
				line := scanner.Text()
				if line != "" && line[0] != '#' {
					allPatterns = append(allPatterns, line)
					lineCount++
				}
			}
			logger.Log.Info("loaded .gitignore", "path", gitignorePath, "patterns", lineCount)
		}
	}

	var gi *gitignore.GitIgnore
	if len(allPatterns) > 0 {
		gi = gitignore.CompileIgnoreLines(allPatterns...)
		logger.Log.Info("compiled ignore patterns", "total_count", len(allPatterns))
	}

	tree := models.NewTree(absPath)
	tree.Root.Path = "."
	tree.Root.ModTime = info.ModTime()

	if err := scanNode(tree.Root, absPath, gi); err != nil {
		return nil, err
	}

	return tree, nil
}

func hasMarkdownDescendants(node *models.Node) bool {
	if node.Type == models.NodeTypeFile {
		return true
	}

	for _, child := range node.Children {
		if hasMarkdownDescendants(child) {
			return true
		}
	}

	return false
}

func scanNode(node *models.Node, rootPath string, gi *gitignore.GitIgnore) error {
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

		childRelPath := filepath.Join(node.Path, entry.Name())

		if gi != nil && gi.MatchesPath(childRelPath) {
			logger.Log.Debug("ignoring path from .gitignore", "path", childRelPath)
			continue
		}

		nodeType := models.NodeTypeFile
		if entry.IsDir() {
			nodeType = models.NodeTypeDirectory
		} else if filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		child := &models.Node{
			Name:     entry.Name(),
			Path:     childRelPath,
			Type:     nodeType,
			ModTime:  info.ModTime(),
			Children: make([]*models.Node, 0),
		}

		if entry.IsDir() {
			if err := scanNode(child, rootPath, gi); err != nil {
				return err
			}

			if !hasMarkdownDescendants(child) {
				continue
			}
		}

		node.AddChild(child)
	}

	return nil
}
