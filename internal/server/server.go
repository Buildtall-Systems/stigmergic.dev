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
	index           contentIndex
	serverURL       string
	treeSignature   string
	allowedPubkeys  []string
	treeMux         sync.RWMutex
	clientsMux      sync.RWMutex
	indexMux        sync.Mutex
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
//
// Structural reports whether the content tree's shape changed, which is what
// lets a client refresh the file tree only when there is something new to
// show. It carries no omitempty, so it is present on every envelope: a
// client that finds the field missing entirely is talking to an older server
// and falls back to refreshing everything.
type sseEvent struct {
	Type       string `json:"type"`
	Path       string `json:"path,omitempty"`
	Structural bool   `json:"structural"`
}

const (
	sseTypeReload     = "reload"
	sseTypeIndexReady = "index-ready"
)

func encodeSSEEvent(eventType, path string, structural bool) (string, bool) {
	payload, err := json.Marshal(sseEvent{Type: eventType, Path: path, Structural: structural})
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

func (s *Server) broadcastChange(path string, structural bool) {
	payload, ok := encodeSSEEvent(sseTypeReload, path, structural)
	if !ok {
		return
	}
	s.broadcast(payload)
}

const (
	// coalesceWindow is how long the broadcaster waits for the corpus to
	// fall quiet before rebuilding. Filesystem activity arrives in bursts:
	// an editor writes then renames, an agent writes a handful of
	// documents, a branch checkout rewrites hundreds. Rebuilding once per
	// event repeats a full corpus pass for every file in the burst, so the
	// broadcaster accumulates instead and rebuilds once the writing stops.
	coalesceWindow = 300 * time.Millisecond

	// coalesceMaxDelay bounds how long a sustained writer can defer a
	// rebuild. Without a ceiling, a process touching files faster than the
	// quiet window would postpone the UI update for as long as it ran.
	coalesceMaxDelay = 2 * time.Second
)

// broadcastEvents owns the rebuild loop. It collapses each burst of source
// events into a single rescan and a single client notification: the quiet
// window restarts on every arrival, and the ceiling forces a flush when
// arrivals never stop. Rebuild cost therefore tracks the number of bursts,
// not the number of files touched.
func (s *Server) broadcastEvents() {
	logger.Log.Info("broadcast events goroutine started")
	defer func() {
		s.wg.Done()
		logger.Log.Info("broadcast events goroutine stopped")
	}()

	// pending holds the most recently changed path, which is what follow
	// mode navigates to; count is how many events it stands for. A nil
	// timer channel parks that arm of the select while nothing is pending.
	var (
		pending    string
		count      int
		quietTimer *time.Timer
		maxTimer   *time.Timer
		quietC     <-chan time.Time
		maxC       <-chan time.Time
	)

	disarm := func() {
		if quietTimer != nil {
			quietTimer.Stop()
			quietTimer = nil
			quietC = nil
		}
		if maxTimer != nil {
			maxTimer.Stop()
			maxTimer = nil
			maxC = nil
		}
	}
	defer disarm()

	flush := func() {
		path, events := pending, count
		pending, count = "", 0
		disarm()

		logger.Log.Info("rebuilding after coalesced burst", "events", events, "path", path)

		structural := s.updateTree()

		s.broadcastChange(path, structural)
	}

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

			logger.Log.Debug("coalescing source event", "path", event.Path, "pending", count+1)

			pending = event.Path
			count++

			if quietTimer != nil {
				quietTimer.Stop()
			}
			quietTimer = time.NewTimer(coalesceWindow)
			quietC = quietTimer.C

			if maxTimer == nil {
				maxTimer = time.NewTimer(coalesceMaxDelay)
				maxC = maxTimer.C
			}
		case <-quietC:
			flush()
		case <-maxC:
			logger.Log.Info("coalesce ceiling reached, rebuilding under sustained writes")
			flush()
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

// updateTree rescans the content tree and refreshes every derived index. It
// reports whether the tree's shape changed, which is how clients tell the
// two cases apart: a file whose contents changed, and a corpus that gained,
// lost, or renamed an entry. A failed rescan leaves the previous tree in
// place and is therefore never structural.
func (s *Server) updateTree() bool {
	logger.Log.Info("rescanning content tree", "source", s.source.Name())
	newTree, err := s.scan()
	if err != nil {
		logger.Log.Error("failed to rescan content tree", "error", err)
		return false
	}

	signature := newTree.Signature()

	s.treeMux.Lock()
	structural := signature != s.treeSignature
	s.tree = newTree
	s.treeSignature = signature
	s.treeMux.Unlock()

	s.rebuildIndexes(newTree)
	logger.Log.Info("content tree updated successfully", "structural", structural)
	return structural
}

// contentIndex is the state a rebuild carries forward so the next one can
// skip the work whose inputs did not change: the corpus itself, the
// wikilinks parsed out of each document, and the lowercased search
// documents. Guarded by indexMux.
type contentIndex struct {
	corpus markdown.Corpus
	links  markdown.LinkRefs
	docs   searchDocs
}

// rebuildIndexes refreshes every content-derived cache. Files whose mod time
// and size are unchanged are neither re-read nor re-parsed, so the cost of a
// rebuild tracks what actually changed rather than the size of the corpus.
//
// Resolution and inversion still run in full every time, because a wikilink
// names a page rather than a path: adding or removing any file can change
// what every other file's links resolve to. Those passes are map lookups
// over cached refs, not parsing, so running them unconditionally is cheap
// and removes a whole class of staleness.
//
// Rebuilds are serialized: broadcastEvents and a gitignore toggle can both
// reach here, and the carried-forward state is not safe for concurrent
// mutation.
func (s *Server) rebuildIndexes(newTree *models.Tree) {
	files := newTree.FlattenMarkdownFiles()
	s.cachedFiles.Store(files)

	s.indexMux.Lock()
	defer s.indexMux.Unlock()

	corpus, changed := markdown.ReadCorpus(s.source.FS(), s.index.corpus, files)
	links := markdown.ExtractLinkRefs(s.index.links, corpus, changed)
	docs := updateSearchDocs(s.index.docs, corpus, changed)

	s.index = contentIndex{corpus: corpus, links: links, docs: docs}

	logger.Log.Debug("rebuilt content indexes", "files", len(files), "reread", len(changed))

	s.cachedBacklinks.Store(markdown.BuildBacklinkIndex(links, files))
	s.cachedContent.Store(orderSearchIndex(docs, files))
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
	s.treeSignature = newTree.Signature()
	s.treeMux.Unlock()

	s.rebuildIndexes(newTree)
	s.indexReady.Store(true)
	logger.Log.Info("background scan complete, index ready")

	s.broadcastIndexReady()
}

// broadcastIndexReady tells connected clients the background scan finished.
// It is structural by definition: pages served during indexing hold an empty
// tree and need the real one.
func (s *Server) broadcastIndexReady() {
	payload, ok := encodeSSEEvent(sseTypeIndexReady, "", true)
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
	structural := s.updateTree()
	s.broadcastReload(structural)
	return newVal
}

// broadcastReload asks clients to refresh the reading pane regardless of
// which path changed. Structural carries the same meaning as everywhere
// else: whether the file tree itself needs redrawing.
func (s *Server) broadcastReload(structural bool) {
	s.broadcastChange("", structural)
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
