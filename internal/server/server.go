package server

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/buildtall-systems/buildtall/btk/auth/nip98"
	"github.com/buildtall-systems/buildtall/btk/auth/session"

	"go.abhg.dev/goldmark/wikilink"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/auth"
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
	cachedFiles     atomic.Value
	cachedBacklinks atomic.Value
	cachedContent   atomic.Value
	cachedRoutes    atomic.Value
	theme           *theme.Theme
	themes          []*theme.Theme
	clients         map[chan string]bool
	ctx             context.Context
	cancel          context.CancelFunc
	sessionManager  *session.Manager
	loadVaults      VaultLoader
	owners          chan string
	observed        map[string]bool
	primaryMount    *mount
	index           contentIndex
	serverURL       string
	allowedPubkeys  []string
	mounts          []*mount
	treeMux         sync.RWMutex
	clientsMux      sync.RWMutex
	indexMux        sync.Mutex
	observedMux     sync.Mutex
	wg              sync.WaitGroup
	indexReady      atomic.Bool
}

// ownerQueue bounds the npubs waiting to have their vaults discovered. A
// full queue is not a dropped reader: observe leaves the npub unrecorded, so
// the reader's next request offers it again.
const ownerQueue = 8

// primary is the source the server was started on: the one the sidebar
// draws, the one ignore patterns and the gitignore toggle act on, and the
// one at FileMount. It is held apart from the mounts slice because that
// slice grows as vaults arrive, and every reader of the primary would
// otherwise have to take the lock to read a mount that never changes.
func (s *Server) primary() *mount {
	return s.primaryMount
}

// mountList snapshots the mounted sources. The slice grows as vaults are
// discovered, so every reader outside the mount goroutine takes a copy
// rather than ranging over the field.
func (s *Server) mountList() []*mount {
	s.treeMux.RLock()
	defer s.treeMux.RUnlock()
	return slices.Clone(s.mounts)
}

// NewServer serves one content source at the /file/ mount.
func NewServer(cfg *config.Config, src source.ContentSource) *Server {
	return NewServerWithVaults(cfg, src, nil)
}

// NewServerWithVaults serves the primary source at /file/ and mounts every
// vault the loader finds: at startup for each configured npub and, when auth
// is on, for each npub that signs in. A nil loader mounts no vaults, which
// is the whole of today's behavior.
func NewServerWithVaults(cfg *config.Config, src source.ContentSource, load VaultLoader) *Server {
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
		handler = auth.Middleware(sm)(handler)
		logger.Log.Info("auth enabled", "allowed_pubkeys", len(allowedPubkeys))
	}

	srv := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

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

	primary := newMount(markdown.FileMount, src, cfg.IgnorePatterns)

	s := &Server{
		httpServer:     srv,
		config:         cfg,
		mux:            mux,
		primaryMount:   primary,
		mounts:         []*mount{primary},
		theme:          thm,
		themes:         themes,
		clients:        make(map[chan string]bool),
		ctx:            ctx,
		cancel:         cancel,
		sessionManager: sm,
		loadVaults:     load,
		owners:         make(chan string, ownerQueue),
		observed:       make(map[string]bool),
		allowedPubkeys: allowedPubkeys,
		serverURL:      serverURL,
	}

	s.cachedFiles.Store([]models.SearchableFile{})
	s.cachedBacklinks.Store(models.BacklinkIndex{})
	s.cachedContent.Store(searchIndex{})
	s.cachedRoutes.Store(markdown.NewRouteResolver(nil))

	s.wg.Add(1)
	go s.initialScan()

	if w := primary.watchable; w != nil {
		s.wg.Add(1)
		go s.broadcastEvents(w)
	}

	if s.loadVaults != nil {
		s.wg.Add(1)
		go s.mountOwners()
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

	for _, m := range s.mountList() {
		if err := m.src.Close(); err != nil {
			logger.Log.Error("error closing content source", "source", m.src.Name(), "error", err)
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
func (s *Server) broadcastEvents(w source.Watchable) {
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
		case event, ok := <-w.Events():
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
		case err, ok := <-w.Errors():
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

// scanMount rescans one source and stores everything its shape decides: the
// tree the sidebar draws, the flat file list the corpus reads, and the
// resolver its own documents' links answer through. It reports whether the
// tree's shape changed. A failed rescan leaves the previous tree in place
// and is therefore never structural.
func (s *Server) scanMount(m *mount) bool {
	respectGitignore := false
	if ga, ok := m.src.(source.GitignoreAware); ok {
		respectGitignore = ga.RespectingGitignore()
	}

	tree, err := source.Scan(m.src.FS(), respectGitignore, m.ignore)
	if err != nil {
		logger.Log.Error("failed to scan content source", "source", m.src.Name(), "error", err)
		return false
	}

	files := tree.FlattenMarkdownFiles()
	signature := tree.Signature()

	m.resolver.Store(markdown.NewRouteResolver(routeEntries(m.prefix, files)))

	s.treeMux.Lock()
	structural := signature != m.signature
	m.tree = tree
	m.signature = signature
	m.files = files
	s.treeMux.Unlock()

	return structural
}

// updateTree rescans every source whose tree can have changed and refreshes
// the corpus-wide indexes. It reports whether any tree's shape changed,
// which is how clients tell the two cases apart: a file whose contents
// changed, and a corpus that gained, lost, or renamed an entry.
//
// A fetched vault and an embedded site are scanned once, when they are
// mounted, and skipped here: their bytes are fixed for the life of the
// serve, so rescanning them would walk a tree that cannot have moved.
func (s *Server) updateTree() bool {
	structural := false
	for _, m := range s.mountList() {
		if !m.mutable() {
			continue
		}
		logger.Log.Info("rescanning content tree", "source", m.src.Name())
		if s.scanMount(m) {
			structural = true
		}
	}

	s.rebuildIndexes()
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
func (s *Server) rebuildIndexes() {
	mounts := s.mountList()

	s.indexMux.Lock()
	defer s.indexMux.Unlock()

	prev := s.index
	corpus := make(markdown.Corpus, len(prev.corpus))
	changed := make(markdown.ChangedRoutes)
	docs := make(searchDocs, len(prev.docs))
	files := make([]models.SearchableFile, 0, len(prev.corpus))
	entries := make([]markdown.RouteEntry, 0, len(prev.corpus))

	for _, m := range mounts {
		mounted := s.mountFiles(m)

		read, reread := markdown.ReadCorpus(m.src.FS(), prev.corpus, m.prefix, mounted)
		maps.Copy(corpus, read)
		maps.Copy(changed, reread)
		maps.Copy(docs, updateSearchDocs(prev.docs, read, reread, m.src.Name()))

		files = append(files, routedFiles(m.prefix, mounted)...)
		entries = append(entries, routeEntries(m.prefix, mounted)...)
	}

	links := markdown.ExtractLinkRefs(prev.links, corpus, changed)
	s.index = contentIndex{corpus: corpus, links: links, docs: docs}

	// One order for the whole corpus, most recently modified first, which is
	// what the files API, the recent list, and search results all read as
	// their order.
	sort.SliceStable(files, func(i, j int) bool { return files[i].ModTime > files[j].ModTime })

	routes := markdown.NewRouteResolver(entries)

	logger.Log.Debug("rebuilt content indexes", "files", len(files), "reread", len(changed), "sources", len(mounts))

	s.cachedRoutes.Store(routes)
	s.cachedFiles.Store(files)
	s.cachedBacklinks.Store(markdown.BuildBacklinkIndex(links, files, s.linkResolvers(mounts, routes), mountPrefixes(mounts)))
	s.cachedContent.Store(orderSearchIndex(docs, files))
}

// mountFiles reads one mount's current file list.
func (s *Server) mountFiles(m *mount) []models.SearchableFile {
	s.treeMux.RLock()
	defer s.treeMux.RUnlock()
	return m.files
}

// linkResolvers answers one document's links exactly as its own page render
// would: the source holding it answers first, so a name it holds always
// wins, and the corpus-wide resolver stands behind it so a link reaching
// into another source resolves rather than dangling. Index and render
// agreeing on this is what makes a backlink a claim about the page.
func (s *Server) linkResolvers(mounts []*mount, routes *markdown.TreeResolver) markdown.ResolverFor {
	return func(route string) wikilink.Resolver {
		m, rel, ok := mountOf(mounts, route)
		if !ok {
			return routes
		}
		own, _ := m.renderSeams(rel, s.config.AttachmentRoot)
		return markdown.Chain{own, routes}
	}
}

// corpusRoutes is the resolver over every mounted source, which a page
// render chains behind its own source's.
func (s *Server) corpusRoutes() *markdown.TreeResolver {
	if v, ok := s.cachedRoutes.Load().(*markdown.TreeResolver); ok {
		return v
	}
	return markdown.NewRouteResolver(nil)
}

func (s *Server) initialScan() {
	defer s.wg.Done()
	logger.Log.Info("starting background content scan", "source", s.primary().src.Name(), "ignore_patterns", len(s.config.IgnorePatterns))

	for _, m := range s.mountList() {
		s.scanMount(m)
	}

	s.rebuildIndexes()
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

// IsRespectingGitignore reports the primary source's filtering. Gitignore is
// a working copy's own rule, so it is the primary source's alone: a fetched
// vault carries no such file and never answers here.
func (s *Server) IsRespectingGitignore() bool {
	if ga, ok := s.primary().src.(source.GitignoreAware); ok {
		return ga.RespectingGitignore()
	}
	return false
}

func (s *Server) ToggleRespectGitignore() bool {
	ga, ok := s.primary().src.(source.GitignoreAware)
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
