package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/theme"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/watcher"
)

type Server struct {
	httpServer *http.Server
	config     *config.Config
	mux        *http.ServeMux
	tree       *models.Tree
	treeMux    sync.RWMutex
	watcher    *watcher.Watcher
	theme      *theme.Theme
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

	logger.Log.Info("scanning directory", "path", cfg.WatchPath, "respect_gitignore", cfg.RespectGitignore, "ignore_patterns", len(cfg.IgnorePatterns))
	tree, err := watcher.ScanDirectory(cfg.WatchPath, cfg.RespectGitignore, cfg.IgnorePatterns)
	if err != nil {
		logger.Log.Error("failed to scan directory", "error", err)
		tree = &models.Tree{}
	} else {
		logger.Log.Info("directory scanned successfully")
	}

	w, err := watcher.NewWatcher()
	if err != nil {
		logger.Log.Error("failed to create watcher", "error", err)
		panic(fmt.Sprintf("failed to create watcher: %v", err))
	}

	logger.Log.Info("adding watch path", "path", cfg.WatchPath)
	if err := w.Add(cfg.WatchPath, cfg.RespectGitignore, cfg.IgnorePatterns); err != nil {
		logger.Log.Error("failed to watch directory", "error", err)
		panic(fmt.Sprintf("failed to watch directory: %v", err))
	}

	logger.Log.Info("loading theme", "theme", cfg.Theme)
	thm, err := theme.Load(cfg.Theme)
	if err != nil {
		logger.Log.Error("failed to load theme", "error", err, "theme", cfg.Theme)
		panic(fmt.Sprintf("failed to load theme: %v", err))
	}
	logger.Log.Info("theme loaded successfully", "theme", cfg.Theme)

	s := &Server{
		httpServer: srv,
		config:     cfg,
		mux:        mux,
		tree:       tree,
		watcher:    w,
		theme:      thm,
		clients:    make(map[chan string]bool),
	}

	go s.broadcastEvents()

	s.setupRoutes()

	return s
}

func (s *Server) Start() error {
	logger.Log.Info("starting server", "addr", s.httpServer.Addr)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Error("server failed", "error", err)
			errChan <- fmt.Errorf("server failed: %w", err)
		} else {
			errChan <- nil
		}
	}()

	logger.Log.Info("server started successfully", "addr", s.httpServer.Addr)

	select {
	case err := <-errChan:
		return err
	case sig := <-sigChan:
		logger.Log.Info("received shutdown signal", "signal", sig)
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
	logger.Log.Info("broadcast events goroutine started")
	for {
		select {
		case event, ok := <-s.watcher.Events:
			if !ok {
				logger.Log.Info("watcher events channel closed, stopping broadcast")
				return
			}
			logger.Log.Info("broadcasting event to clients", "path", event.Path, "type", event.Type)

			s.updateTree()

			s.clientsMux.RLock()
			clientCount := len(s.clients)
			logger.Log.Debug("active SSE clients", "count", clientCount)
			for client := range s.clients {
				select {
				case client <- filepath.Base(event.Path):
					logger.Log.Debug("sent event to client")
				default:
					logger.Log.Warn("client channel full, skipping")
				}
			}
			s.clientsMux.RUnlock()
		case err, ok := <-s.watcher.Errors:
			if !ok {
				logger.Log.Info("watcher errors channel closed")
				return
			}
			logger.Log.Error("watcher error", "error", err)
		}
	}
}

func (s *Server) addClient(client chan string) {
	s.clientsMux.Lock()
	s.clients[client] = true
	clientCount := len(s.clients)
	s.clientsMux.Unlock()
	logger.Log.Info("SSE client connected", "total_clients", clientCount)
}

func (s *Server) removeClient(client chan string) {
	s.clientsMux.Lock()
	delete(s.clients, client)
	close(client)
	clientCount := len(s.clients)
	s.clientsMux.Unlock()
	logger.Log.Info("SSE client disconnected", "remaining_clients", clientCount)
}

func (s *Server) updateTree() {
	logger.Log.Info("rescanning directory tree", "path", s.config.WatchPath)
	newTree, err := watcher.ScanDirectory(s.config.WatchPath, s.config.RespectGitignore, s.config.IgnorePatterns)
	if err != nil {
		logger.Log.Error("failed to rescan directory", "error", err)
		return
	}

	s.treeMux.Lock()
	s.tree = newTree
	s.treeMux.Unlock()
	logger.Log.Info("directory tree updated successfully")
}
