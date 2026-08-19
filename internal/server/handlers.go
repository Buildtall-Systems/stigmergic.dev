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
		s.mux.HandleFunc(auth.LoginPath, auth.LoginHandler(s.serverURL, s.theme, s.themes))
		s.mux.HandleFunc("/auth/verify", auth.VerifyHandler(s.sessionManager, s.allowedPubkeys, s.serverURL))
		s.mux.HandleFunc("/auth/logout", auth.LogoutHandler(s.sessionManager))
		logger.Log.Info("auth routes registered")
	}

	s.mux.HandleFunc("/", s.handleHome)
	s.mux.HandleFunc(markdown.FileMount, s.handleMarkdown)

	// One registration serves every vault: the mounts are discovered while
	// the server runs, so the handler picks the source by longest matching
	// prefix rather than the mux picking it by pattern.
	s.mux.HandleFunc(vaultRoutePrefix, s.handleMarkdown)
	s.mux.HandleFunc("/events", s.handleSSE)
	s.mux.HandleFunc("/api/files", s.handleFilesAPI)
	s.mux.HandleFunc("/api/search", s.handleSearchAPI)
	s.mux.HandleFunc("/partial/sidebar", s.handleSidebarPartial)
	s.mux.HandleFunc("/partial/recent", s.handleRecentPartial)
	s.mux.HandleFunc("/partial/tree/", s.handleTreePartial)

	if s.primary().caps.GitignoreToggle {
		s.mux.HandleFunc("/api/gitignore", s.handleGitignoreStatus)
		s.mux.HandleFunc("/api/gitignore/toggle", s.handleToggleGitignore)
		logger.Log.Info("gitignore routes registered")
	}

	logger.Log.Info("routes configured")
}

// vaultRoutePrefix is the namespace every vault mounts inside, and the one
// pattern the mux needs for all of them.
const vaultRoutePrefix = "/vault/"

// uiData gathers what a page render needs from the primary source: the tree
// the sidebar draws, how many documents it holds, its recently updated
// documents, and whether the background scan has finished. The sidebar
// describes the primary source alone, so the corpus-wide caches are read
// where they are used rather than here.
func (s *Server) uiData() (*models.Tree, int, []models.SearchableFile, bool) {
	primary := s.primary()

	s.treeMux.RLock()
	tree := primary.tree
	files := primary.files
	s.treeMux.RUnlock()

	// The recent list is routed after it is cut to length rather than
	// before, so a page render restates five paths instead of the whole
	// corpus's.
	var recentFiles []models.SearchableFile
	if primary.caps.RecentlyUpdated {
		recentFiles = routedFiles(primary.prefix, s.computeRecentFiles(files))
	}

	return tree, len(files), recentFiles, s.IsIndexReady()
}

// pageCaps blends the two places a page's affordances come from: those
// acting on the document on screen belong to the source serving it, while
// the sidebar's own belong to the primary source whatever is being read. A
// vault document therefore offers no path to copy and no changes to follow,
// while the tree beside it keeps both.
func (s *Server) pageCaps(m *mount) models.UICapabilities {
	primary := s.primary().caps
	return models.UICapabilities{
		RecentlyUpdated: primary.RecentlyUpdated,
		GitignoreToggle: primary.GitignoreToggle,
		CopyPath:        m.caps.CopyPath,
		FollowMode:      m.caps.FollowMode,
	}
}

// expansionFor is the path the sidebar opens to. The sidebar draws the
// primary source, so a document read from anywhere else leaves it as it is
// rather than opening it to a path it does not hold.
func (s *Server) expansionFor(m *mount, filePath string) string {
	if m != s.primary() {
		return ""
	}
	return filePath
}

// canonicalPath accepts only already-canonical fs paths: anything path.Clean
// would rewrite (empty, trailing slash, ".." or "." elements) is rejected, and
// fs.ValidPath rules out the rest. Shared by every handler that turns a
// request path into a lookup key.
func canonicalPath(p string) (string, bool) {
	cleaned := path.Clean(p)
	if cleaned != p || !fs.ValidPath(cleaned) {
		return "", false
	}
	return cleaned, true
}

// treeViewFor pairs the tree with the mount its rows link through and the
// directories that render expanded, which are those containing filePath.
// Every other directory ships as a placeholder, so the cold sidebar is
// proportional to what is visible rather than to the corpus. An empty
// filePath collapses everything below the root.
func treeViewFor(tree *models.Tree, mount, filePath string) models.TreeView {
	return models.TreeView{Tree: tree, Mount: mount, Expanded: models.AncestorDirs(filePath)}
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

	s.observeSession(r)

	if s.config.DefaultFile != "" && !isHTMXRequest(r) {
		http.Redirect(w, r, markdown.FileMount+s.config.DefaultFile, http.StatusFound)
		return
	}

	tree, fileCount, recentFiles, indexReady := s.uiData()

	var dirCount int
	if tree != nil {
		dirCount = countDirs(tree.Root)
	}

	primary := s.primary()

	if isHTMXRequest(r) {
		logger.Log.Debug("rendering HTMX home partial")
		if err := templates.HomeContent(primary.src.Name(), recentFiles, fileCount, dirCount, indexReady, primary.caps).Render(r.Context(), w); err != nil {
			logger.Log.Error("failed to render home content template", "error", err)
		}
		s.renderOutlineOOB(w, r, nil)
	} else {
		logger.Log.Debug("rendering full home page")
		if err := templates.Home(treeViewFor(tree, s.primary().prefix, ""), primary.src.Name(), s.theme, s.themes, recentFiles, fileCount, dirCount, indexReady, primary.caps).Render(r.Context(), w); err != nil {
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

// handleRecentPartial renders the sidebar's recently-updated list alone.
// Clients target it when a file's contents changed but the tree's shape did
// not, which is the overwhelmingly common case and costs a few hundred bytes
// instead of the whole tree.
func (s *Server) handleRecentPartial(w http.ResponseWriter, r *http.Request) {
	_, _, recentFiles, _ := s.uiData()
	if err := components.SidebarRecent(recentFiles, s.primary().caps).Render(r.Context(), w); err != nil {
		logger.Log.Error("failed to render recent partial", "error", err)
	}
}

// handleSidebarPartial re-renders the whole sidebar after a structural change.
// The client passes the file it is displaying so the tree comes back with that
// row already visible; the path only seeds the expansion and touches no
// filesystem, so a non-canonical one is dropped and the tree renders collapsed
// rather than the request failing.
func (s *Server) handleSidebarPartial(w http.ResponseWriter, r *http.Request) {
	tree, _, recentFiles, indexReady := s.uiData()

	var current string
	if raw := r.URL.Query().Get("path"); raw != "" {
		cleaned, ok := canonicalPath(raw)
		if !ok {
			logger.Log.Warn("ignoring non-canonical sidebar path", "path", raw)
		} else {
			current = cleaned
		}
	}

	if err := components.Sidebar(treeViewFor(tree, s.primary().prefix, current), recentFiles, indexReady, s.primary().caps).Render(r.Context(), w); err != nil {
		logger.Log.Error("failed to render sidebar partial", "error", err)
	}
}

// handleTreePartial renders one directory's rows. The sidebar ships collapsed
// below the current file's ancestor chain, so this is where the rest of the
// tree arrives: one directory at a time, on first expand, rather than all of
// it on every page load.
func (s *Server) handleTreePartial(w http.ResponseWriter, r *http.Request) {
	dirPath := strings.TrimPrefix(r.URL.Path, "/partial/tree/")

	cleaned, ok := canonicalPath(dirPath)
	if !ok {
		logger.Log.Warn("invalid tree path", "path", dirPath)
		http.NotFound(w, r)
		return
	}

	// The directory path alone no longer says which tree it belongs to, so
	// the row that asked names its own source; naming none means the
	// primary, which is what every row the sidebar ships today names.
	m, ok := s.mountAt(r.URL.Query().Get("mount"))
	if !ok {
		logger.Log.Warn("tree partial named no mounted source", "mount", r.URL.Query().Get("mount"))
		http.NotFound(w, r)
		return
	}

	s.treeMux.RLock()
	var node *models.Node
	if m.tree != nil {
		node = m.tree.Find(cleaned)
	}
	s.treeMux.RUnlock()

	if node == nil || !node.IsDir() {
		logger.Log.Warn("tree directory not found", "path", cleaned, "source", m.src.Name())
		http.NotFound(w, r)
		return
	}

	// Nil expansion: the requested level renders collapsed, and each nested
	// directory becomes a placeholder resolved by its own request.
	if err := components.Tree(node, nil, m.prefix).Render(r.Context(), w); err != nil {
		logger.Log.Error("failed to render tree partial", "error", err)
	}
}

// mountAt finds a mounted source by its route prefix, answering with the
// primary when none is named.
func (s *Server) mountAt(prefix string) (*mount, bool) {
	if prefix == "" {
		return s.primary(), true
	}
	for _, m := range s.mountList() {
		if m.prefix == prefix {
			return m, true
		}
	}
	return nil, false
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
	s.observeSession(r)

	m, filePath, mounted := mountOf(s.mountList(), r.URL.Path)
	if !mounted {
		logger.Log.Warn("no source mounted for route", "route", r.URL.Path)
		http.NotFound(w, r)
		return
	}

	logger.Log.Info("file request", "route", r.URL.Path, "source", m.src.Name(), "htmx", isHTMXRequest(r))

	// Only already-canonical fs paths are served: anything path.Clean would
	// rewrite (empty, trailing slash, ".." or "." elements) is rejected, and
	// fs.ValidPath rules out the rest. The source FS enforces the same
	// semantics on every operation, so traversal is impossible by
	// construction. Written out here rather than delegated to canonicalPath
	// because gosec's taint analysis follows the sanitizer inline and cannot
	// see one behind a call, and this path reaches http.ServeFileFS.
	cleaned := path.Clean(filePath)
	if cleaned != filePath || !fs.ValidPath(cleaned) {
		logger.Log.Warn("invalid file path", "path", filePath, "source", m.src.Name())
		http.NotFound(w, r)
		return
	}
	filePath = cleaned

	contentFS := m.src.FS()

	info, err := fs.Stat(contentFS, filePath)
	if err != nil {
		logger.Log.Warn("path not found", "path", filePath, "source", m.src.Name(), "error", err)
		http.NotFound(w, r)
		return
	}

	breadcrumbs := buildBreadcrumbs(m.prefix, filePath)
	title := filepath.Base(filePath)
	isHTMX := isHTMXRequest(r)
	caps := s.pageCaps(m)

	tree, _, recentFiles, indexReady := s.uiData()

	if info.IsDir() {
		s.treeMux.RLock()
		node := m.tree.Find(filePath)
		s.treeMux.RUnlock()

		if node == nil {
			logger.Log.Warn("directory node not found in tree", "path", filePath, "source", m.src.Name())
			http.NotFound(w, r)
			return
		}

		if isHTMX {
			logger.Log.Debug("rendering HTMX directory partial")
			if renderErr := templates.DirectoryContent(breadcrumbs, node, m.src.Name(), m.prefix).Render(r.Context(), w); renderErr != nil {
				logger.Log.Error("failed to render directory content template", "error", renderErr)
			}
			s.renderOutlineOOB(w, r, nil)
		} else {
			logger.Log.Debug("rendering full directory page")
			// A directory page shows its own contents, so the directory
			// itself joins its ancestors in the expansion.
			view := treeViewFor(tree, s.primary().prefix, s.expansionFor(m, filePath))
			if m == s.primary() {
				view.Expanded.Add(filePath)
			}
			if renderErr := templates.Directory(title, breadcrumbs, node, m.src.Name(), m.prefix, s.theme, s.themes, view, recentFiles, indexReady, caps).Render(r.Context(), w); renderErr != nil {
				logger.Log.Error("failed to render directory template", "error", renderErr)
			}
		}
		return
	}

	// Serve non-markdown files directly (images, PDFs, etc.)
	if filepath.Ext(filePath) != models.MarkdownExt {
		logger.Log.Debug("serving static asset", "path", filePath, "source", m.src.Name())
		http.ServeFileFS(w, r, contentFS, filePath)
		return
	}

	content, err := fs.ReadFile(contentFS, filePath)
	if err != nil {
		logger.Log.Warn("file read failed", "path", filePath, "source", m.src.Name(), "error", err)
		http.NotFound(w, r)
		return
	}

	logger.Log.Debug("file read successfully", "path", filePath, "size", len(content))

	// The embed source reads through the same filesystem this handler just
	// read the host file from, rather than through the search corpus, which
	// sits behind a mutex and can lag during a rebuild. A context is built
	// per request because rendering mutates its depth and visited set.
	//
	// The source holding the document answers its links first and the
	// corpus-wide resolver stands behind it, so a name the source holds
	// always wins and a link reaching into another source still resolves.
	own, embedSource := m.renderSeams(filePath, s.config.AttachmentRoot)
	resolver := markdown.Chain{own, s.corpusRoutes()}
	embeds := markdown.NewEmbedContext(m.prefix, embedSource)
	html, meta, err := markdown.Parse(content, resolver, embeds)
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
	fileBacklinks := backlinks[m.prefix+filePath]

	var relativePath string
	if rooted, ok := m.src.(source.Rooted); ok {
		relativePath = computeBuildtallRelativePath(rooted.Root(), filePath)
	}

	outline := markdown.ExtractOutline(content)

	// The routes the render pulled in travel to the client, where the
	// live-reload listener refreshes the pane when one of them changes.
	transcluded := embeds.Transcluded()

	if isHTMX {
		logger.Log.Debug("rendering HTMX partial")
		if renderErr := templates.MarkdownContent(breadcrumbs, string(html), string(content), m.src.Name(), relativePath, fileBacklinks, meta, caps, transcluded).Render(r.Context(), w); renderErr != nil {
			logger.Log.Error("failed to render markdown content template", "error", renderErr)
		}
		s.renderOutlineOOB(w, r, outline)
	} else {
		logger.Log.Debug("rendering full page")
		if renderErr := templates.Markdown(title, breadcrumbs, string(html), string(content), m.src.Name(), relativePath, s.theme, s.themes, treeViewFor(tree, s.primary().prefix, s.expansionFor(m, filePath)), recentFiles, indexReady, fileBacklinks, meta, caps, outline, transcluded).Render(r.Context(), w); renderErr != nil {
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

// buildBreadcrumbs names each ancestor of a document, every crumb carrying
// the route that serves it inside mount.
func buildBreadcrumbs(mount, path string) []models.Breadcrumb {
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
				Path: mount + currentPath,
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
