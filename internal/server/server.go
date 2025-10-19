package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/watcher"
)

type Server struct {
	httpServer *http.Server
	config     *config.Config
	mux        *http.ServeMux
	tree       *models.Tree
	watcher    *watcher.Watcher
	clients    map[chan string]bool
	clientsMux sync.RWMutex
}

func NewServer(cfg *config.Config) *Server {
	mux := http.NewServeMux()

	handler := loggingMiddleware(mux)
	handler = recoveryMiddleware(handler)
	handler = securityMiddleware(handler)

	srv := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	tree, err := watcher.ScanDirectory(cfg.WatchPath)
	if err != nil {
		log.Printf("failed to scan directory: %v", err)
		tree = &models.Tree{}
	}

	w, err := watcher.NewWatcher()
	if err != nil {
		log.Fatalf("failed to create watcher: %v", err)
	}

	if err := w.Add(cfg.WatchPath); err != nil {
		log.Fatalf("failed to watch directory: %v", err)
	}

	s := &Server{
		httpServer: srv,
		config:     cfg,
		mux:        mux,
		tree:       tree,
		watcher:    w,
		clients:    make(map[chan string]bool),
	}

	go s.broadcastEvents()

	s.setupRoutes()

	return s
}

func (s *Server) Start() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("server failed: %w", err)
		} else {
			errChan <- nil
		}
	}()

	select {
	case err := <-errChan:
		return err
	case sig := <-sigChan:
		log.Printf("Received signal: %v, shutting down gracefully...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.Shutdown(ctx)
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.watcher != nil {
		s.watcher.Close()
	}
	s.clientsMux.Lock()
	for client := range s.clients {
		close(client)
	}
	s.clients = make(map[chan string]bool)
	s.clientsMux.Unlock()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) broadcastEvents() {
	for {
		select {
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			s.clientsMux.RLock()
			for client := range s.clients {
				select {
				case client <- filepath.Base(event.Path):
				default:
				}
			}
			s.clientsMux.RUnlock()
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

func (s *Server) addClient(client chan string) {
	s.clientsMux.Lock()
	s.clients[client] = true
	s.clientsMux.Unlock()
}

func (s *Server) removeClient(client chan string) {
	s.clientsMux.Lock()
	delete(s.clients, client)
	close(client)
	s.clientsMux.Unlock()
}
