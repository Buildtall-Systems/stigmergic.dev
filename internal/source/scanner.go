package source

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

// Scan builds the content tree from any fs.FS. Node paths are route-relative,
// forward-slash, rooted at ".". Mod times are recorded from fs.Stat; zero
// values are acceptable and the UI capability gate decides whether to show
// them. Directories without markdown descendants are pruned.
func Scan(fsys fs.FS, respectGitignore bool, ignorePatterns []string) (*models.Tree, error) {
	rootInfo, err := fs.Stat(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to stat content root: %w", err)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("content root is not a directory")
	}

	gi := compileIgnorePatterns(fsys, respectGitignore, ignorePatterns)

	tree := models.NewTree(rootInfo.Name())
	tree.Root.ModTime = rootInfo.ModTime()

	nodes := map[string]*models.Node{".": tree.Root}

	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("failed to walk %s: %w", p, walkErr)
		}
		if p == "." {
			return nil
		}

		if gi != nil && gi.MatchesPath(p) {
			logger.Log.Debug("ignoring path", "path", p)
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if !d.IsDir() && path.Ext(d.Name()) != models.MarkdownExt {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		nodeType := models.NodeTypeFile
		if d.IsDir() {
			nodeType = models.NodeTypeDirectory
		}

		node := &models.Node{
			Name:     d.Name(),
			Path:     p,
			Type:     nodeType,
			ModTime:  info.ModTime(),
			Children: make([]*models.Node, 0),
		}

		parent, ok := nodes[path.Dir(p)]
		if !ok {
			return nil
		}
		parent.AddChild(node)

		if d.IsDir() {
			nodes[p] = node
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	pruneEmptyDirectories(tree.Root)

	return tree, nil
}

// compileIgnorePatterns merges configured patterns with the content tree's
// .gitignore (when honored) into a single matcher. Returns nil when there is
// nothing to match.
func compileIgnorePatterns(fsys fs.FS, respectGitignore bool, ignorePatterns []string) *gitignore.GitIgnore {
	var allPatterns []string
	allPatterns = append(allPatterns, ignorePatterns...)

	if respectGitignore {
		if content, err := fs.ReadFile(fsys, ".gitignore"); err == nil {
			lineCount := 0
			for line := range strings.SplitSeq(string(content), "\n") {
				line = strings.TrimRight(line, "\r")
				if line != "" && line[0] != '#' {
					allPatterns = append(allPatterns, line)
					lineCount++
				}
			}
			logger.Log.Info("loaded .gitignore", "patterns", lineCount)
		}
	}

	if len(allPatterns) == 0 {
		return nil
	}

	logger.Log.Info("compiled ignore patterns", "total_count", len(allPatterns))
	return gitignore.CompileIgnoreLines(allPatterns...)
}

// pruneEmptyDirectories removes directory nodes with no markdown descendants.
func pruneEmptyDirectories(node *models.Node) {
	kept := node.Children[:0]
	for _, child := range node.Children {
		if child.Type == models.NodeTypeDirectory {
			pruneEmptyDirectories(child)
			if !hasMarkdownDescendants(child) {
				continue
			}
		}
		kept = append(kept, child)
	}
	node.Children = kept
}

func hasMarkdownDescendants(node *models.Node) bool {
	if node.Type == models.NodeTypeFile {
		return true
	}
	return slices.ContainsFunc(node.Children, hasMarkdownDescendants)
}
