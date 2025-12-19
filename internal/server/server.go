package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/theme"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/watcher"
)

type Server struct {
	httpServer  *http.Server
	config      *config.Config
	mux         *http.ServeMux
	tree        *models.Tree
	treeMux     sync.RWMutex
	cachedFiles atomic.Value
	watcher     *watcher.Watcher
	theme       *theme.Theme
	clients     map[chan string]bool
	clientsMux  sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
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

	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		httpServer: srv,
		config:     cfg,
		mux:        mux,
		tree:       tree,
		watcher:    w,
		theme:      thm,
		clients:    make(map[chan string]bool),
		ctx:        ctx,
		cancel:     cancel,
	}

	s.cachedFiles.Store(tree.FlattenMarkdownFiles())

	s.wg.Add(1)
	go s.broadcastEvents()

	s.setupRoutes()

	return s
}

func (s *Server) Start() error {
	logger.Log.Info("starting server", "addr", s.httpServer.Addr)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	errChan := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Error("server failed", "error", err)
			errChan <- fmt.Errorf("server failed: %w", err)
		}
	}()

	logger.Log.Info("server started successfully", "addr", s.httpServer.Addr)

	select {
	case err := <-errChan:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if shutdownErr := s.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.Log.Error("shutdown error after server failure", "error", shutdownErr)
		}
		return err
	case sig := <-sigChan:
		logger.Log.Info("received shutdown signal", "signal", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.Shutdown(ctx)
	case <-s.ctx.Done():
		logger.Log.Info("server context cancelled")
		return nil
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	logger.Log.Info("shutting down server")

	s.cancel()

	s.clientsMux.Lock()
	for client := range s.clients {
		close(client)
	}
	s.clients = make(map[chan string]bool)
	s.clientsMux.Unlock()

	if s.watcher != nil {
		if err := s.watcher.Close(); err != nil {
			logger.Log.Error("error closing watcher", "error", err)
		}
	}

	s.wg.Wait()

	return s.httpServer.Shutdown(ctx)
}

func (s *Server) broadcastEvents() {
	logger.Log.Info("broadcast events goroutine started")
	defer func() {
		s.wg.Done()
		logger.Log.Info("broadcast events goroutine stopped")
	}()

	for {
		select {
		case <-s.ctx.Done():
			logger.Log.Info("broadcast context cancelled, stopping")
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				logger.Log.Info("watcher events channel closed, stopping broadcast")
				return
			}

			shouldBroadcast := false
			ext := filepath.Ext(event.Path)

			info, err := os.Stat(event.Path)
			if err == nil {
				if info.IsDir() {
					shouldBroadcast = true
				} else if ext == ".md" {
					shouldBroadcast = true
				}
			} else if event.Type == watcher.EventRemove && ext == ".md" {
				shouldBroadcast = true
			}

			if !shouldBroadcast {
				logger.Log.Debug("ignoring non-markdown file event", "path", event.Path, "ext", ext)
				continue
			}

			logger.Log.Info("broadcasting event to clients", "path", event.Path, "type", event.Type)

			s.updateTree()

			s.clientsMux.RLock()
			clientCount := len(s.clients)
			logger.Log.Debug("active SSE clients", "count", clientCount)
			for client := range s.clients {
				select {
				case client <- "reload":
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
	_, exists := s.clients[client]
	delete(s.clients, client)
	clientCount := len(s.clients)
	s.clientsMux.Unlock()

	if exists {
		close(client)
	}
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

	s.cachedFiles.Store(newTree.FlattenMarkdownFiles())
	logger.Log.Info("directory tree updated successfully")
}
