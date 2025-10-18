package server

import (
	"log"
	"net/http"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/embed"
)

func (s *Server) setupRoutes() {
	staticFS, err := embed.StaticFS()
	if err != nil {
		log.Fatalf("failed to load static files: %v", err)
	}
	fs := http.FileServer(http.FS(staticFS))
	s.mux.Handle("/static/", http.StripPrefix("/static/", fs))
}
