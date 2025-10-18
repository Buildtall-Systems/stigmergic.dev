package server

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/embed"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/web/templates"
)

func (s *Server) setupRoutes() {
	staticFS, err := embed.StaticFS()
	if err != nil {
		log.Fatalf("failed to load static files: %v", err)
	}
	fs := http.FileServer(http.FS(staticFS))
	s.mux.Handle("/static/", http.StripPrefix("/static/", fs))

	s.mux.HandleFunc("/", s.handleHome)
	s.mux.HandleFunc("/file/", s.handleMarkdown)
	s.mux.HandleFunc("/events", s.handleSSE)
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
	if filePath == "" {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(s.config.WatchPath, filePath)

	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, s.config.WatchPath) {
		http.NotFound(w, r)
		return
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	html, err := markdown.Parse(content)
	if err != nil {
		http.Error(w, "Failed to parse markdown", http.StatusInternalServerError)
		return
	}

	breadcrumbs := buildBreadcrumbs(filePath)
	title := filepath.Base(filePath)

	isHTMX := r.Header.Get("HX-Request") == "true"
	if isHTMX {
		templates.MarkdownContent(breadcrumbs, string(html)).Render(r.Context(), w)
	} else {
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
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	clientEvents := make(chan string, 10)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-s.events:
				if !ok {
					return
				}
				clientEvents <- filepath.Base(event.Path)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-clientEvents:
			_, err := w.Write([]byte("data: " + msg + "\n\n"))
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
