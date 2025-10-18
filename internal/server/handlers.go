package server

import (
	"net/http"
)

func (s *Server) setupRoutes() {
	fs := http.FileServer(http.Dir("web/static"))
	s.mux.Handle("/static/", http.StripPrefix("/static/", fs))
}
