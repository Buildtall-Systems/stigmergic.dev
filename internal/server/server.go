package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
)

type Server struct {
	httpServer *http.Server
	config     *config.Config
	mux        *http.ServeMux
}

func NewServer(cfg *config.Config) *Server {
	mux := http.NewServeMux()

	handler := loggingMiddleware(mux)
	handler = recoveryMiddleware(handler)
	handler = securityMiddleware(handler)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s := &Server{
		httpServer: srv,
		config:     cfg,
		mux:        mux,
	}

	s.setupRoutes()

	return s
}

func (s *Server) Start() error {
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
