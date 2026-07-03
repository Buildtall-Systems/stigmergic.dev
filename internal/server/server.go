package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/theme"
)

type Server struct {
	httpServer      *http.Server
	config          *config.Config
	mux             *http.ServeMux
	tree            *models.Tree
	cachedFiles     atomic.Value
	cachedBacklinks atomic.Value
	cachedContent   atomic.Value
	source          source.ContentSource
	watchable       source.Watchable
	theme           *theme.Theme
	themes          []*theme.Theme
	clients         map[chan string]bool
	ctx             context.Context
	cancel          context.CancelFunc
	sessionManager  *session.Manager
	serverURL       string
	allowedPubkeys  []string
	treeMux         sync.RWMutex
	clientsMux      sync.RWMutex
	wg              sync.WaitGroup
	uiCaps          models.UICapabilities
	indexReady      atomic.Bool
}

func NewServer(cfg *config.Config, src source.ContentSource) *Server {
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

	logger.Log.Info("loading theme", "theme", cfg.Theme)
	thm, err := theme.Load(cfg.Theme)
	if err != nil {
		logger.Log.Error("failed to load theme", "error", err, "theme", cfg.Theme)
		panic(fmt.Sprintf("failed to load theme: %v", err))
	}
	logger.Log.Info("theme loaded successfully", "theme", cfg.Theme)

	themes, err := theme.LoadEmbedded()
	if err != nil {
		logger.Log.Error("failed to load embedded themes", "error", err)
		panic(fmt.Sprintf("failed to load embedded themes: %v", err))
	}
	bootEmbedded := false
	for _, t := range themes {
		if t.Name == thm.Name {
			bootEmbedded = true
			break
		}
	}
	if !bootEmbedded {
		themes = append([]*theme.Theme{thm}, themes...)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var watchable source.Watchable
	if w, ok := src.(source.Watchable); ok {
		watchable = w
	}
	_, gitignoreAware := src.(source.GitignoreAware)
	_, timestamped := src.(source.Timestamped)
	_, rooted := src.(source.Rooted)

	s := &Server{
		httpServer: srv,
		config:     cfg,
		mux:        mux,
		tree:       tree,
		source:     src,
		watchable:  watchable,
		uiCaps: models.UICapabilities{
			RecentlyUpdated: timestamped,
			GitignoreToggle: gitignoreAware,
			CopyPath:        rooted,
			FollowMode:      watchable != nil,
		},
		theme:          thm,
		themes:         themes,
		clients:        make(map[chan string]bool),
		ctx:            ctx,
		cancel:         cancel,
		sessionManager: sm,
		allowedPubkeys: allowedPubkeys,
		serverURL:      serverURL,
	}

	s.cachedFiles.Store([]models.SearchableFile{})
	s.cachedBacklinks.Store(models.BacklinkIndex{})
	s.cachedContent.Store(searchIndex{})

	s.wg.Add(1)
	go s.initialScan()

	if s.watchable != nil {
		s.wg.Add(1)
		go s.broadcastEvents()
	}

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

	if s.source != nil {
		if err := s.source.Close(); err != nil {
			logger.Log.Error("error closing content source", "error", err)
		}
	}

	s.wg.Wait()

	return s.httpServer.Shutdown(ctx)
}

// sseEvent is the JSON envelope pushed to SSE clients. Type is "reload"
// (Path names the changed file when known; empty means refresh regardless)
// or "index-ready".
type sseEvent struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
}

const (
	sseTypeReload     = "reload"
	sseTypeIndexReady = "index-ready"
)

func encodeSSEEvent(eventType, path string) (string, bool) {
	payload, err := json.Marshal(sseEvent{Type: eventType, Path: path})
	if err != nil {
		logger.Log.Error("failed to marshal SSE event", "error", err, "event_type", eventType, "path", path)
		return "", false
	}
	return string(payload), true
}

func (s *Server) broadcast(payload string) {
	s.clientsMux.RLock()
	defer s.clientsMux.RUnlock()
	logger.Log.Debug("broadcasting to SSE clients", "count", len(s.clients), "payload", payload)
	for client := range s.clients {
		select {
		case client <- payload:
		default:
			logger.Log.Warn("client channel full, skipping")
		}
	}
}

func (s *Server) broadcastChange(path string) {
	payload, ok := encodeSSEEvent(sseTypeReload, path)
	if !ok {
		return
	}
	s.broadcast(payload)
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
		case event, ok := <-s.watchable.Events():
			if !ok {
				logger.Log.Info("source events channel closed, stopping broadcast")
				return
			}

			logger.Log.Info("broadcasting event to clients", "path", event.Path)

			s.updateTree()

			s.broadcastChange(event.Path)
		case err, ok := <-s.watchable.Errors():
			if !ok {
				logger.Log.Info("source errors channel closed")
				return
			}
			logger.Log.Error("source error", "error", err)
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

func (s *Server) scan() (*models.Tree, error) {
	respectGitignore := false
	if ga, ok := s.source.(source.GitignoreAware); ok {
		respectGitignore = ga.RespectingGitignore()
	}
	return source.Scan(s.source.FS(), respectGitignore, s.config.IgnorePatterns)
}

func (s *Server) updateTree() {
	logger.Log.Info("rescanning content tree", "source", s.source.Name())
	newTree, err := s.scan()
	if err != nil {
		logger.Log.Error("failed to rescan content tree", "error", err)
		return
	}

	s.treeMux.Lock()
	s.tree = newTree
	s.treeMux.Unlock()

	s.rebuildIndexes(newTree)
	logger.Log.Info("content tree updated successfully")
}

// rebuildIndexes refreshes every content-derived cache from a single read
// of the corpus: the searchable file list, the backlink index, and the
// full-text search index.
func (s *Server) rebuildIndexes(newTree *models.Tree) {
	files := newTree.FlattenMarkdownFiles()
	s.cachedFiles.Store(files)
	contents := markdown.ReadCorpus(s.source.FS(), files)
	s.cachedBacklinks.Store(markdown.BuildBacklinkIndex(contents, files))
	s.cachedContent.Store(buildSearchIndex(contents, files))
}

func (s *Server) initialScan() {
	defer s.wg.Done()
	logger.Log.Info("starting background content scan", "source", s.source.Name(), "ignore_patterns", len(s.config.IgnorePatterns))

	newTree, err := s.scan()
	if err != nil {
		logger.Log.Error("background scan failed", "error", err)
		s.indexReady.Store(true) // Mark ready even on failure so UI doesn't hang
		return
	}

	s.treeMux.Lock()
	s.tree = newTree
	s.treeMux.Unlock()

	s.rebuildIndexes(newTree)
	s.indexReady.Store(true)
	logger.Log.Info("background scan complete, index ready")

	s.broadcastIndexReady()
}

func (s *Server) broadcastIndexReady() {
	payload, ok := encodeSSEEvent(sseTypeIndexReady, "")
	if !ok {
		return
	}
	logger.Log.Info("broadcasting index-ready to clients")
	s.broadcast(payload)
}

func (s *Server) IsIndexReady() bool {
	return s.indexReady.Load()
}

func (s *Server) IsRespectingGitignore() bool {
	if ga, ok := s.source.(source.GitignoreAware); ok {
		return ga.RespectingGitignore()
	}
	return false
}

func (s *Server) ToggleRespectGitignore() bool {
	ga, ok := s.source.(source.GitignoreAware)
	if !ok {
		return false
	}
	newVal := ga.ToggleGitignore()
	s.updateTree()
	s.broadcastReload()
	return newVal
}

func (s *Server) broadcastReload() {
	s.broadcastChange("")
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
