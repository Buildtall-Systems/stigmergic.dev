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

	"github.com/Buildtall-Systems/btk/auth/nip98"
	"github.com/Buildtall-Systems/btk/auth/session"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/theme"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/watcher"
)

type Server struct {
	httpServer       *http.Server
	config           *config.Config
	mux              *http.ServeMux
	tree             *models.Tree
	cachedFiles      atomic.Value
	cachedBacklinks  atomic.Value
	watcher          *watcher.Watcher
	theme            *theme.Theme
	clients          map[chan string]bool
	ctx              context.Context
	cancel           context.CancelFunc
	sessionManager   *session.Manager
	serverURL        string
	allowedPubkeys   []string
	treeMux          sync.RWMutex
	clientsMux       sync.RWMutex
	wg               sync.WaitGroup
	indexReady       atomic.Bool
	respectGitignore atomic.Bool
}

func NewServer(cfg *config.Config) *Server {
	mux := http.NewServeMux()

	handler := loggingMiddleware(mux)
	handler = recoveryMiddleware(handler)
	handler = securityMiddleware(handler)

	var sm *session.Manager
	var allowedPubkeys []string
	var serverURL string

	if cfg.Auth.Enabled {
		var err error
		allowedPubkeys, err = nip98.NormalizePubkeys(cfg.Auth.AllowedNpubs)
		if err != nil {
			logger.Log.Error("invalid pubkey in allowlist", "error", err)
			panic(fmt.Sprintf("invalid pubkey in auth allowlist: %v", err))
		}

		sm, err = session.NewManager("stigmergic_session", cfg.Auth.SessionSecret, cfg.Auth.SessionMaxAge)
		if err != nil {
			logger.Log.Error("failed to create session manager", "error", err)
			panic(fmt.Sprintf("failed to create session manager: %v", err))
		}

		if cfg.BaseURL != "" {
			serverURL = cfg.BaseURL
		} else {
			serverURL = fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
		}
		handler = session.Middleware(sm)(handler)
		logger.Log.Info("auth enabled", "allowed_pubkeys", len(allowedPubkeys))
	}

	srv := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	// Initialize with empty tree - background scan will populate it
	tree := &models.Tree{}

	w, err := watcher.NewWatcher()
	if err != nil {
		logger.Log.Error("failed to create watcher", "error", err)
		panic(fmt.Sprintf("failed to create watcher: %v", err))
	}

	logger.Log.Info("adding watch path", "path", cfg.WatchPath)
	if addErr := w.Add(cfg.WatchPath, cfg.RespectGitignore, cfg.IgnorePatterns); addErr != nil {
		logger.Log.Error("failed to watch directory", "error", addErr)
		panic(fmt.Sprintf("failed to watch directory: %v", addErr))
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
		httpServer:     srv,
		config:         cfg,
		mux:            mux,
		tree:           tree,
		watcher:        w,
		theme:          thm,
		clients:        make(map[chan string]bool),
		ctx:            ctx,
		cancel:         cancel,
		sessionManager: sm,
		allowedPubkeys: allowedPubkeys,
		serverURL:      serverURL,
	}

	s.cachedFiles.Store([]models.SearchableFile{})
	s.cachedBacklinks.Store(models.BacklinkIndex{})
	s.respectGitignore.Store(cfg.RespectGitignore)

	s.wg.Add(2)
	go s.broadcastEvents()
	go s.initialScan()

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
	respectGitignore := s.respectGitignore.Load()
	logger.Log.Info("rescanning directory tree", "path", s.config.WatchPath, "respect_gitignore", respectGitignore)
	newTree, err := watcher.ScanDirectory(s.config.WatchPath, respectGitignore, s.config.IgnorePatterns)
	if err != nil {
		logger.Log.Error("failed to rescan directory", "error", err)
		return
	}

	s.treeMux.Lock()
	s.tree = newTree
	s.treeMux.Unlock()

	files := newTree.FlattenMarkdownFiles()
	s.cachedFiles.Store(files)
	s.cachedBacklinks.Store(markdown.BuildBacklinkIndex(s.config.WatchPath, files))
	logger.Log.Info("directory tree updated successfully")
}

func (s *Server) initialScan() {
	defer s.wg.Done()
	respectGitignore := s.respectGitignore.Load()
	logger.Log.Info("starting background directory scan", "path", s.config.WatchPath, "respect_gitignore", respectGitignore, "ignore_patterns", len(s.config.IgnorePatterns))

	newTree, err := watcher.ScanDirectory(s.config.WatchPath, respectGitignore, s.config.IgnorePatterns)
	if err != nil {
		logger.Log.Error("background scan failed", "error", err)
		s.indexReady.Store(true) // Mark ready even on failure so UI doesn't hang
		return
	}

	s.treeMux.Lock()
	s.tree = newTree
	s.treeMux.Unlock()

	files := newTree.FlattenMarkdownFiles()
	s.cachedFiles.Store(files)
	s.cachedBacklinks.Store(markdown.BuildBacklinkIndex(s.config.WatchPath, files))
	s.indexReady.Store(true)
	logger.Log.Info("background scan complete, index ready")

	s.broadcastIndexReady()
}

func (s *Server) broadcastIndexReady() {
	s.clientsMux.RLock()
	clientCount := len(s.clients)
	logger.Log.Info("broadcasting index-ready to clients", "count", clientCount)
	for client := range s.clients {
		select {
		case client <- "index-ready":
			logger.Log.Debug("sent index-ready to client")
		default:
			logger.Log.Warn("client channel full, skipping index-ready")
		}
	}
	s.clientsMux.RUnlock()
}

func (s *Server) IsIndexReady() bool {
	return s.indexReady.Load()
}

func (s *Server) IsRespectingGitignore() bool {
	return s.respectGitignore.Load()
}

func (s *Server) ToggleRespectGitignore() bool {
	for {
		current := s.respectGitignore.Load()
		newVal := !current
		if s.respectGitignore.CompareAndSwap(current, newVal) {
			logger.Log.Info("toggled respect gitignore", "new_value", newVal)
			s.updateTree()
			s.broadcastReload()
			return newVal
		}
	}
}

func (s *Server) broadcastReload() {
	s.clientsMux.RLock()
	for client := range s.clients {
		select {
		case client <- "reload":
		default:
		}
	}
	s.clientsMux.RUnlock()
}

// WaitForIndexReady blocks until the background index scan completes or ctx is cancelled.
// Primarily intended for tests that need synchronous behavior.
func (s *Server) WaitForIndexReady(ctx context.Context) error {
	for !s.indexReady.Load() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return nil
}
