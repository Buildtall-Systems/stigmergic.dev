package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/embed"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/timeutil"
	"github.com/Buildtall-Systems/stigmergic.dev/web/templates"
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

	s.mux.HandleFunc("/", s.handleHome)
	s.mux.HandleFunc("/file/", s.handleMarkdown)
	s.mux.HandleFunc("/events", s.handleSSE)
	s.mux.HandleFunc("/api/files", s.handleFilesAPI)

	logger.Log.Info("routes configured")
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s.treeMux.RLock()
	tree := s.tree
	s.treeMux.RUnlock()

	files := tree.FlattenMarkdownFiles()
	recentFiles := s.computeRecentFiles(files)

	if isHTMXRequest(r) {
		logger.Log.Debug("rendering HTMX home partial")
		if err := templates.HomeContent(tree, s.config.WatchPath, recentFiles).Render(r.Context(), w); err != nil {
			logger.Log.Error("failed to render home content template", "error", err)
		}
	} else {
		logger.Log.Debug("rendering full home page")
		if err := templates.Home(tree, s.config.WatchPath, s.theme, files, recentFiles).Render(r.Context(), w); err != nil {
			logger.Log.Error("failed to render home template", "error", err)
		}
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
	logger.Log.Info("markdown request", "path", filePath, "htmx", isHTMXRequest(r))

	if filePath == "" {
		logger.Log.Warn("empty file path")
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(s.config.WatchPath, filePath)

	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, s.config.WatchPath) {
		logger.Log.Warn("path traversal attempt", "requested", filePath, "clean", cleanPath)
		http.NotFound(w, r)
		return
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		logger.Log.Warn("path not found", "path", cleanPath, "error", err)
		http.NotFound(w, r)
		return
	}

	breadcrumbs := buildBreadcrumbs(filePath)
	title := filepath.Base(filePath)
	isHTMX := isHTMXRequest(r)

	s.treeMux.RLock()
	files := s.tree.FlattenMarkdownFiles()
	s.treeMux.RUnlock()

	if info.IsDir() {
		s.treeMux.RLock()
		node := s.tree.Find(cleanPath)
		s.treeMux.RUnlock()

		if node == nil {
			logger.Log.Warn("directory node not found in tree", "path", cleanPath)
			http.NotFound(w, r)
			return
		}

		if isHTMX {
			logger.Log.Debug("rendering HTMX directory partial")
			if err := templates.DirectoryContent(breadcrumbs, node, s.config.WatchPath).Render(r.Context(), w); err != nil {
				logger.Log.Error("failed to render directory content template", "error", err)
			}
		} else {
			logger.Log.Debug("rendering full directory page")
			if err := templates.Directory(title, breadcrumbs, node, s.config.WatchPath, s.theme, files).Render(r.Context(), w); err != nil {
				logger.Log.Error("failed to render directory template", "error", err)
			}
		}
		return
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		logger.Log.Warn("file not found", "path", cleanPath, "error", err)
		http.NotFound(w, r)
		return
	}

	logger.Log.Debug("file read successfully", "path", cleanPath, "size", len(content))

	html, err := markdown.Parse(content)
	if err != nil {
		logger.Log.Error("markdown parse failed", "error", err)
		http.Error(w, "Failed to parse markdown", http.StatusInternalServerError)
		return
	}

	if isHTMX {
		logger.Log.Debug("rendering HTMX partial")
		if err := templates.MarkdownContent(breadcrumbs, string(html), string(content), s.config.WatchPath).Render(r.Context(), w); err != nil {
			logger.Log.Error("failed to render markdown content template", "error", err)
		}
	} else {
		logger.Log.Debug("rendering full page")
		if err := templates.Markdown(title, breadcrumbs, string(html), string(content), s.config.WatchPath, s.theme, files).Render(r.Context(), w); err != nil {
			logger.Log.Error("failed to render markdown template", "error", err)
		}
	}
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

func (s *Server) handleFilesAPI(w http.ResponseWriter, r *http.Request) {
	s.treeMux.RLock()
	files := s.tree.FlattenMarkdownFiles()
	s.treeMux.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		logger.Log.Error("failed to encode files JSON", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
