package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/embed"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/web/templates"
)

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

	logger.Log.Info("routes configured")
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	templates.Home(s.tree).Render(r.Context(), w)
}

func (s *Server) handleMarkdown(w http.ResponseWriter, r *http.Request) {
	filePath := strings.TrimPrefix(r.URL.Path, "/file/")
	logger.Log.Info("markdown request", "path", filePath, "htmx", r.Header.Get("HX-Request") == "true")

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

	breadcrumbs := buildBreadcrumbs(filePath)
	title := filepath.Base(filePath)

	isHTMX := r.Header.Get("HX-Request") == "true"
	if isHTMX {
		logger.Log.Debug("rendering HTMX partial")
		templates.MarkdownContent(breadcrumbs, string(html)).Render(r.Context(), w)
	} else {
		logger.Log.Debug("rendering full page")
		templates.Markdown(title, breadcrumbs, string(html)).Render(r.Context(), w)
	}
}

func buildBreadcrumbs(path string) []string {
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	var crumbs []string
	for _, part := range parts {
		if part != "" && part != "." {
			crumbs = append(crumbs, part)
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
	w.Write([]byte(": connected\n\n"))
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
			_, err := w.Write([]byte("data: " + msg + "\n\n"))
			if err != nil {
				logger.Log.Error("failed to write SSE message", "error", err)
				return
			}
			flusher.Flush()
			logger.Log.Debug("SSE message flushed")
		}
	}
}
