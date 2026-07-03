package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Buildtall-Systems/btk/auth/session"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/auth"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/embed"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/timeutil"
	"github.com/Buildtall-Systems/stigmergic.dev/web/templates"
	"github.com/Buildtall-Systems/stigmergic.dev/web/templates/components"
)

func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func (s *Server) setupRoutes() {
	staticFS, err := embed.StaticFS()
	if err != nil {
		logger.Log.Error("failed to load static files", "error", err)
		panic("failed to load static files")
	}
	fs := http.FileServer(http.FS(staticFS))
	s.mux.Handle("/static/", http.StripPrefix("/static/", fs))

	if s.config.Auth.Enabled {
		s.mux.HandleFunc(session.LoginPath, auth.LoginHandler(s.serverURL))
		s.mux.HandleFunc("/auth/verify", auth.VerifyHandler(s.sessionManager, s.allowedPubkeys, s.serverURL))
		s.mux.HandleFunc("/auth/logout", auth.LogoutHandler(s.sessionManager))
		logger.Log.Info("auth routes registered")
	}

	s.mux.HandleFunc("/", s.handleHome)
	s.mux.HandleFunc("/file/", s.handleMarkdown)
	s.mux.HandleFunc("/events", s.handleSSE)
	s.mux.HandleFunc("/api/files", s.handleFilesAPI)
	s.mux.HandleFunc("/api/search", s.handleSearchAPI)
	s.mux.HandleFunc("/partial/sidebar", s.handleSidebarPartial)

	if s.uiCaps.GitignoreToggle {
		s.mux.HandleFunc("/api/gitignore", s.handleGitignoreStatus)
		s.mux.HandleFunc("/api/gitignore/toggle", s.handleToggleGitignore)
		logger.Log.Info("gitignore routes registered")
	}

	logger.Log.Info("routes configured")
}

func (s *Server) uiData() (*models.Tree, []models.SearchableFile, []models.SearchableFile, bool) {
	s.treeMux.RLock()
	tree := s.tree
	s.treeMux.RUnlock()

	var files []models.SearchableFile
	if v, ok := s.cachedFiles.Load().([]models.SearchableFile); ok {
		files = v
	}

	var recentFiles []models.SearchableFile
	if s.uiCaps.RecentlyUpdated {
		recentFiles = s.computeRecentFiles(files)
	}

	return tree, files, recentFiles, s.IsIndexReady()
}

func countDirs(node *models.Node) int {
	if node == nil {
		return 0
	}
	count := 0
	for _, child := range node.Children {
		if child.IsDir() {
			count += 1 + countDirs(child)
		}
	}
	return count
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if s.config.DefaultFile != "" && !isHTMXRequest(r) {
		http.Redirect(w, r, "/file/"+s.config.DefaultFile, http.StatusFound)
		return
	}

	tree, files, recentFiles, indexReady := s.uiData()

	var dirCount int
	if tree != nil {
		dirCount = countDirs(tree.Root)
	}

	if isHTMXRequest(r) {
		logger.Log.Debug("rendering HTMX home partial")
		if err := templates.HomeContent(s.source.Name(), recentFiles, len(files), dirCount, indexReady, s.uiCaps).Render(r.Context(), w); err != nil {
			logger.Log.Error("failed to render home content template", "error", err)
		}
		s.renderOutlineOOB(w, r, nil)
	} else {
		logger.Log.Debug("rendering full home page")
		if err := templates.Home(tree, s.source.Name(), s.theme, files, recentFiles, len(files), dirCount, indexReady, s.uiCaps).Render(r.Context(), w); err != nil {
			logger.Log.Error("failed to render home template", "error", err)
		}
	}
}

// renderOutlineOOB appends the out-of-band outline-rail fragment to an htmx
// partial response: entries for markdown documents, nil to clear the rail.
func (s *Server) renderOutlineOOB(w http.ResponseWriter, r *http.Request, outline []models.OutlineEntry) {
	if err := components.OutlineOOB(outline).Render(r.Context(), w); err != nil {
		logger.Log.Error("failed to render outline OOB fragment", "error", err)
	}
}

func (s *Server) handleSidebarPartial(w http.ResponseWriter, r *http.Request) {
	tree, _, recentFiles, indexReady := s.uiData()
	if err := components.Sidebar(tree, recentFiles, indexReady, s.uiCaps).Render(r.Context(), w); err != nil {
		logger.Log.Error("failed to render sidebar partial", "error", err)
	}
}

func (s *Server) computeRecentFiles(files []models.SearchableFile) []models.SearchableFile {
	count := s.config.RecentFilesCount
	if count <= 0 {
		return nil
	}
	if len(files) < count {
		count = len(files)
	}

	recentFiles := make([]models.SearchableFile, count)
	copy(recentFiles, files[:count])

	for i := range recentFiles {
		recentFiles[i].RelativeTime = timeutil.RelativeTime(time.Unix(recentFiles[i].ModTime, 0))
	}

	return recentFiles
}

func (s *Server) handleMarkdown(w http.ResponseWriter, r *http.Request) {
	filePath := strings.TrimPrefix(r.URL.Path, "/file/")
	logger.Log.Info("file request", "path", filePath, "htmx", isHTMXRequest(r))

	// Only already-canonical fs paths are served: anything path.Clean would
	// rewrite (empty, trailing slash, ".." or "." elements) is rejected, and
	// fs.ValidPath rules out the rest. The source FS enforces the same
	// semantics on every operation, so traversal is impossible by
	// construction.
	cleaned := path.Clean(filePath)
	if cleaned != filePath || !fs.ValidPath(cleaned) {
		logger.Log.Warn("invalid file path", "path", filePath)
		http.NotFound(w, r)
		return
	}
	filePath = cleaned

	contentFS := s.source.FS()

	info, err := fs.Stat(contentFS, filePath)
	if err != nil {
		logger.Log.Warn("path not found", "path", filePath, "error", err)
		http.NotFound(w, r)
		return
	}

	breadcrumbs := buildBreadcrumbs(filePath)
	title := filepath.Base(filePath)
	isHTMX := isHTMXRequest(r)

	tree, files, recentFiles, indexReady := s.uiData()

	if info.IsDir() {
		s.treeMux.RLock()
		node := s.tree.Find(filePath)
		s.treeMux.RUnlock()

		if node == nil {
			logger.Log.Warn("directory node not found in tree", "path", filePath)
			http.NotFound(w, r)
			return
		}

		if isHTMX {
			logger.Log.Debug("rendering HTMX directory partial")
			if renderErr := templates.DirectoryContent(breadcrumbs, node, s.source.Name()).Render(r.Context(), w); renderErr != nil {
				logger.Log.Error("failed to render directory content template", "error", renderErr)
			}
			s.renderOutlineOOB(w, r, nil)
		} else {
			logger.Log.Debug("rendering full directory page")
			if renderErr := templates.Directory(title, breadcrumbs, node, s.source.Name(), s.theme, files, tree, recentFiles, indexReady, s.uiCaps).Render(r.Context(), w); renderErr != nil {
				logger.Log.Error("failed to render directory template", "error", renderErr)
			}
		}
		return
	}

	// Serve non-markdown files directly (images, PDFs, etc.)
	if filepath.Ext(filePath) != models.MarkdownExt {
		logger.Log.Debug("serving static asset", "path", filePath)
		http.ServeFileFS(w, r, contentFS, filePath)
		return
	}

	content, err := fs.ReadFile(contentFS, filePath)
	if err != nil {
		logger.Log.Warn("file read failed", "path", filePath, "error", err)
		http.NotFound(w, r)
		return
	}

	logger.Log.Debug("file read successfully", "path", filePath, "size", len(content))

	resolver := markdown.NewTreeResolver(files)
	html, meta, err := markdown.Parse(content, resolver)
	if err != nil {
		logger.Log.Error("markdown parse failed", "error", err)
		http.Error(w, "Failed to parse markdown", http.StatusInternalServerError)
		return
	}

	// Prefer frontmatter title for the HTML <title> when present.
	if metaTitle, ok := meta["title"].(string); ok && metaTitle != "" {
		title = metaTitle
	}

	var backlinks models.BacklinkIndex
	if v, ok := s.cachedBacklinks.Load().(models.BacklinkIndex); ok {
		backlinks = v
	}
	fileBacklinks := backlinks[filePath]

	var relativePath string
	if rooted, ok := s.source.(source.Rooted); ok {
		relativePath = computeBuildtallRelativePath(rooted.Root(), filePath)
	}

	outline := markdown.ExtractOutline(content)

	if isHTMX {
		logger.Log.Debug("rendering HTMX partial")
		if renderErr := templates.MarkdownContent(breadcrumbs, string(html), string(content), s.source.Name(), relativePath, fileBacklinks, meta, s.uiCaps).Render(r.Context(), w); renderErr != nil {
			logger.Log.Error("failed to render markdown content template", "error", renderErr)
		}
		s.renderOutlineOOB(w, r, outline)
	} else {
		logger.Log.Debug("rendering full page")
		if renderErr := templates.Markdown(title, breadcrumbs, string(html), string(content), s.source.Name(), relativePath, s.theme, files, tree, recentFiles, indexReady, fileBacklinks, meta, s.uiCaps, outline).Render(r.Context(), w); renderErr != nil {
			logger.Log.Error("failed to render markdown template", "error", renderErr)
		}
	}
}

func computeBuildtallRelativePath(watchPath, filePath string) string {
	buildtallRoot := filepath.Join(os.Getenv("HOME"), "git", "buildtall.systems")
	fullPath := filepath.Join(watchPath, filePath)
	relPath, err := filepath.Rel(buildtallRoot, fullPath)
	if err != nil {
		return filePath
	}
	return relPath
}

func buildBreadcrumbs(path string) []models.Breadcrumb {
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	var crumbs []models.Breadcrumb
	var currentPath string
	for _, part := range parts {
		if part != "" && part != "." {
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = filepath.Join(currentPath, part)
			}
			crumbs = append(crumbs, models.Breadcrumb{
				Name: part,
				Path: "/file/" + currentPath,
			})
		}
	}
	return crumbs
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	logger.Log.Info("SSE connection request", "remote_addr", r.RemoteAddr)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Log.Error("streaming unsupported for this client")
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	clientChan := make(chan string, 10)
	s.addClient(clientChan)
	defer s.removeClient(clientChan)

	logger.Log.Debug("sending SSE connection confirmation")
	if _, err := w.Write([]byte(": connected\n\n")); err != nil {
		logger.Log.Error("failed to write SSE connection confirmation", "error", err)
		return
	}
	flusher.Flush()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("SSE client context done", "reason", ctx.Err())
			return
		case msg, ok := <-clientChan:
			if !ok {
				logger.Log.Info("SSE client channel closed")
				return
			}
			logger.Log.Info("sending SSE message to client", "message", msg)
			_, err := w.Write([]byte("event: message\ndata: " + msg + "\n\n"))
			if err != nil {
				logger.Log.Error("failed to write SSE message", "error", err)
				return
			}
			flusher.Flush()
			logger.Log.Debug("SSE message flushed")
		}
	}
}

func (s *Server) handleSearchAPI(w http.ResponseWriter, r *http.Request) {
	var idx searchIndex
	if v, ok := s.cachedContent.Load().(searchIndex); ok {
		idx = v
	}

	resp := idx.search(r.URL.Query().Get("q"), searchResultLimit)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Log.Error("failed to encode search results", "error", err)
	}
}

func (s *Server) handleFilesAPI(w http.ResponseWriter, r *http.Request) {
	var files []models.SearchableFile
	if v, ok := s.cachedFiles.Load().([]models.SearchableFile); ok {
		files = v
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		logger.Log.Error("failed to encode files JSON", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleToggleGitignore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	newValue := s.ToggleRespectGitignore()

	w.Header().Set("Content-Type", "application/json")
	response := map[string]bool{"respectGitignore": newValue}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Log.Error("failed to encode toggle response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleGitignoreStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]bool{"respectGitignore": s.IsRespectingGitignore()}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Log.Error("failed to encode gitignore status", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
