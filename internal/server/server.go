package server

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/a-h/templ"

	"github.com/buildtall-systems/buildtall/btk/auth/identity"
	"github.com/buildtall-systems/buildtall/btk/auth/session"
	"github.com/buildtall-systems/buildtall/btk/auth/site"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
	btkstatic "github.com/buildtall-systems/buildtall/btk/static"
	"github.com/buildtall-systems/buildtall/btk/views/login"

	"go.abhg.dev/goldmark/wikilink"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/auth"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/theme"
	"github.com/Buildtall-Systems/stigmergic.dev/web/templates"
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
	site            *site.Site
	loadVaults      VaultLoader
	owners          chan ownerRequest
	observed        map[string]bool
	signers         map[string]*latestSigner
	primaryMount    *mount
	index           contentIndex
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
// vault the loader finds: at startup for each configured npub, and for each
// npub that signs in, with that reader's signer. A nil loader mounts no
// vaults, which is the whole of today's behavior.
//
// The login is mounted when auth is on, so readers can sign in to pass the
// gate, and when a loader is set, so a reader can sign in to read their own
// private vaults even where no gate stands.
func NewServerWithVaults(cfg *config.Config, src source.ContentSource, load VaultLoader) *Server {
	mux := http.NewServeMux()

	var sm *session.Manager
	if cfg.Auth.Enabled || load != nil {
		var err error
		sm, err = session.NewManager(sessionCookie, cfg.Auth.SessionSecret, cfg.Auth.SessionMaxAge)
		if err != nil {
			logger.Log.Error("failed to create session manager", "error", err)
			panic(fmt.Sprintf("failed to create session manager: %v", err))
		}
	}

	handler := loggingMiddleware(mux)
	handler = recoveryMiddleware(handler)
	handler = securityMiddleware(handler)
	if cfg.Auth.Enabled {
		handler = auth.Gate(handler)
	}
	if sm != nil {
		handler = session.ExtractSession(sm)(handler)
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

	var loginSite *site.Site
	if sm != nil {
		loginSite, err = mountLogin(mux, cfg, sm, thm, themes, newProfileSource(cfg.Profiles.Relays))
		if err != nil {
			logger.Log.Error("failed to mount login", "error", err)
			panic(fmt.Sprintf("failed to mount login: %v", err))
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	primary := newMount(markdown.FileMount, src, cfg.IgnorePatterns)

	s := &Server{
		httpServer:   srv,
		config:       cfg,
		mux:          mux,
		primaryMount: primary,
		mounts:       []*mount{primary},
		theme:        thm,
		themes:       themes,
		clients:      make(map[chan string]bool),
		ctx:          ctx,
		cancel:       cancel,
		site:         loginSite,
		loadVaults:   load,
		owners:       make(chan ownerRequest, ownerQueue),
		observed:     make(map[string]bool),
		signers:      make(map[string]*latestSigner),
	}

	s.cachedFiles.Store([]models.SearchableFile{})
	s.cachedBacklinks.Store(models.BacklinkIndex{})
	s.cachedContent.Store(searchIndex{})
	s.cachedRoutes.Store(newRouteSet(nil, nil))

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

	if s.site != nil {
		s.site.Close()
	}

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
	entries := make([][]markdown.RouteEntry, 0, len(mounts))

	for _, m := range mounts {
		mounted := s.mountFiles(m)

		read, reread := markdown.ReadCorpus(m.src.FS(), prev.corpus, m.prefix, mounted)
		maps.Copy(corpus, read)
		maps.Copy(changed, reread)
		maps.Copy(docs, updateSearchDocs(prev.docs, read, reread, m.src.Name()))

		files = append(files, routedFiles(m.prefix, mounted)...)
		entries = append(entries, routeEntries(m.prefix, mounted))
	}

	links := markdown.ExtractLinkRefs(prev.links, corpus, changed)
	s.index = contentIndex{corpus: corpus, links: links, docs: docs}

	// One order for the whole corpus, most recently modified first, which is
	// what the files API, the recent list, and search results all read as
	// their order.
	sort.SliceStable(files, func(i, j int) bool { return files[i].ModTime > files[j].ModTime })

	routes := newRouteSet(mounts, entries)

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
// wins, and the corpus its owner sees stands behind it so a link reaching
// into another source resolves rather than dangling. Index and render
// agreeing on this is what makes a backlink a claim about the page.
func (s *Server) linkResolvers(mounts []*mount, routes *routeSet) markdown.ResolverFor {
	return func(route string) wikilink.Resolver {
		m, rel, ok := mountOf(mounts, route)
		if !ok {
			return routes.public
		}
		own, _ := m.renderSeams(rel, s.config.AttachmentRoot)
		return markdown.Chain{own, routes.forOwner(m.owner)}
	}
}

// corpusRoutes is the set of corpus-wide resolvers, one of which a page
// render chains behind its own source's.
func (s *Server) corpusRoutes() *routeSet {
	if v, ok := s.cachedRoutes.Load().(*routeSet); ok {
		return v
	}
	return newRouteSet(nil, nil)
}

// routeSet resolves a link against the corpus a document's owner can see. A
// public document resolves only into public sources, whoever reads it, so a
// page and its backlinks never name a private document. A private document
// resolves into the public sources and its own owner's private ones.
type routeSet struct {
	public *markdown.TreeResolver
	owners map[string]*markdown.TreeResolver
}

// newRouteSet builds the resolvers over mounts, where entries holds each
// mount's route entries at the same index. Every resolver keeps the mounts'
// order, so a name two sources hold resolves the same way in each.
func newRouteSet(mounts []*mount, entries [][]markdown.RouteEntry) *routeSet {
	var public []markdown.RouteEntry
	owned := make(map[string][]markdown.RouteEntry)
	for i, m := range mounts {
		if m.owner == "" {
			public = append(public, entries[i]...)
			continue
		}
		owned[m.owner] = nil
	}

	owners := make(map[string]*markdown.TreeResolver, len(owned))
	for owner := range owned {
		var seen []markdown.RouteEntry
		for i, m := range mounts {
			if m.visibleTo(owner) {
				seen = append(seen, entries[i]...)
			}
		}
		owners[owner] = markdown.NewRouteResolver(seen)
	}

	return &routeSet{public: markdown.NewRouteResolver(public), owners: owners}
}

// forOwner is the resolver for a document owned by owner, empty for public.
func (rs *routeSet) forOwner(owner string) *markdown.TreeResolver {
	if r, ok := rs.owners[owner]; ok {
		return r
	}
	return rs.public
}

// sessionCookie names the cookie the login sets.
const sessionCookie = "stigmergic_session"

// mountLogin mounts the btk login: the login page, the extension and
// remote-signer routes, and the sign bridge through which a reader signed
// in with an extension answers a vault relay's challenge. A non-empty
// whitelist, or auth on with an empty one, admits only the npubs it names,
// so an empty whitelist under auth admits no one rather than everyone.
func mountLogin(mux *http.ServeMux, cfg *config.Config, sm *session.Manager, thm *theme.Theme, themes []*theme.Theme, profiles profileSource) (*site.Site, error) {
	opts := site.Options{
		Sessions: sm,
		Logger:   logger.Log,
		Resolve:  resolveUser(profiles),
		LoginPage: func(paths login.Paths, next string) templ.Component {
			return templates.Login(paths, next, thm, themes)
		},
		Name:       "stigmergic",
		BaseURL:    cfg.BaseURL,
		SignBridge: true,
	}

	if cfg.Auth.Enabled || len(cfg.Auth.AllowedNpubs) > 0 {
		whitelist, err := normalizeWhitelist(cfg.Auth.AllowedNpubs)
		if err != nil {
			return nil, err
		}
		opts.Admit = whitelistAdmit(whitelist)
		logger.Log.Info("login whitelist active", "npubs", len(whitelist))
	}

	mux.Handle("GET /static/btk/js/", btkstatic.JSHandler())

	return site.Mount(mux, opts)
}

// Profile lookup bounds. The me route is the only caller and htmx loads it
// after the page, so the deadline delays the nav slot and nothing else. A
// found profile is kept for profileTTL, so a changed picture shows within it.
const (
	profileRelayTimeout = 3 * time.Second
	profileDeadline     = 5 * time.Second
	profileTTL          = time.Hour
)

// profileSource looks up an npub's kind 0 profile. btk's ProfileResolver is
// the production one.
type profileSource interface {
	Resolve(ctx context.Context, npub string) (*btknostr.Profile, error)
}

// newProfileSource reads profiles from relays, the first as home and the rest
// as fallbacks, or returns nil for an empty list, which turns the lookup off.
// The resolver never writes back: these are public relays the server does not
// run, and it has no key to publish with.
func newProfileSource(relays []string) profileSource {
	if len(relays) == 0 {
		return nil
	}
	return btknostr.NewProfileResolver(
		relays[0],
		relays[1:],
		btknostr.WithLogger(logger.Log),
		btknostr.WithCacheToHome(false),
		btknostr.WithTimeout(profileRelayTimeout),
		btknostr.WithMaxDuration(profileDeadline),
		btknostr.WithTTL(profileTTL),
	)
}

// resolveUser builds the nav identity btk's avatar dropdown renders: the
// reader's name and picture from their kind 0 profile, or the npub alone when
// the lookup is off, fails, or finds no profile.
func resolveUser(profiles profileSource) func(context.Context, string) *identity.UserInfo {
	return func(ctx context.Context, npub string) *identity.UserInfo {
		bare := &identity.UserInfo{Npub: npub}
		if profiles == nil {
			return bare
		}
		p, err := profiles.Resolve(ctx, npub)
		if err != nil {
			logger.Log.Warn("profile lookup failed", "npub", npub, "error", err)
			return bare
		}
		if p == nil || p.Event == nil {
			return bare
		}
		user := identity.FromProfile(p, false)
		user.Npub = npub
		return &user
	}
}

// whitelistAdmit admits only the npubs on whitelist.
func whitelistAdmit(whitelist []string) func(context.Context, string) error {
	return func(_ context.Context, npub string) error {
		if !slices.Contains(whitelist, npub) {
			return errNotWhitelisted
		}
		return nil
	}
}

// errNotWhitelisted is the text a reader sees when the whitelist refuses
// their npub.
var errNotWhitelisted = errors.New("this npub may not sign in here")

// normalizeWhitelist turns the configured whitelist into npubs. An entry may
// be an npub or a 64-character hex key, which is encoded to its npub.
func normalizeWhitelist(entries []string) ([]string, error) {
	whitelist := make([]string, 0, len(entries))
	for _, entry := range entries {
		if len(entry) == hexKeyLength {
			npub, err := btknostr.HexToNpub(entry)
			if err != nil {
				return nil, fmt.Errorf("whitelist entry %q: %w", entry, err)
			}
			whitelist = append(whitelist, npub)
			continue
		}
		if _, err := btknostr.NpubToHex(entry); err != nil {
			return nil, fmt.Errorf("whitelist entry %q: %w", entry, err)
		}
		whitelist = append(whitelist, entry)
	}
	return whitelist, nil
}

// hexKeyLength is the length of a public key written as hex.
const hexKeyLength = 64

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
