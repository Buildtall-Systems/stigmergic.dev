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

	templates.Markdown(title, breadcrumbs, string(html)).Render(r.Context(), w)
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
