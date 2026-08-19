package server

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

const (
	testHost      = "localhost"
	testThemeName = "iceberg-dark"
	testIndexFile = "index.md"

	// indexReadyPayload is the exact envelope the background scan broadcasts
	// when it completes. Tests that wait for a content broadcast filter it
	// out, since the scan may race one in.
	indexReadyPayload = `{"type":"index-ready","structural":true}`
)

// newTestServer builds a server over a FilesystemSource. Tests without an
// explicit WatchPath get a dedicated temp dir; the old empty-path behavior
// (resolving to the working directory) was accidental, never specified.
// fakeWatchableSource feeds the broadcast loop directly, so coalescing is
// measured without a real filesystem watcher and its own per-path debounce
// in the path. It implements only the interfaces the server asserts on.
type fakeWatchableSource struct {
	fsys      fs.FS
	events    chan source.Event
	errs      chan error
	closeOnce sync.Once
}

var (
	_ source.ContentSource = (*fakeWatchableSource)(nil)
	_ source.Watchable     = (*fakeWatchableSource)(nil)
)

func newFakeWatchableSource() *fakeWatchableSource {
	return &fakeWatchableSource{
		fsys: fstest.MapFS{
			"initial.md":  &fstest.MapFile{Data: []byte("# initial\n")},
			"docs/api.md": &fstest.MapFile{Data: []byte("# api\n")},
		},
		events: make(chan source.Event, 256),
		errs:   make(chan error, 1),
	}
}

func (f *fakeWatchableSource) FS() fs.FS                   { return f.fsys }
func (f *fakeWatchableSource) Name() string                { return "fake" }
func (f *fakeWatchableSource) Events() <-chan source.Event { return f.events }
func (f *fakeWatchableSource) Errors() <-chan error        { return f.errs }

func (f *fakeWatchableSource) Close() error {
	f.closeOnce.Do(func() {
		close(f.events)
		close(f.errs)
	})
	return nil
}

// emit queues a change event. The buffer is sized well above any burst these
// tests produce, so a full channel means the test is wrong rather than the
// server being slow.
func (f *fakeWatchableSource) emit(t *testing.T, path string) {
	t.Helper()
	select {
	case f.events <- source.Event{Path: path}:
	default:
		t.Fatal("fake source event buffer full")
	}
}

// newServerWithSource builds a server over an arbitrary source and blocks
// until its background scan has finished, so tests start from a known index.
func newServerWithSource(t *testing.T, src source.ContentSource) *Server {
	t.Helper()

	cfg := &config.Config{
		Port:             8080,
		Host:             testHost,
		Theme:            testThemeName,
		RecentFilesCount: 5,
	}
	srv := NewServer(cfg, src)

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			t.Errorf("failed to shut down server: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.WaitForIndexReady(ctx); err != nil {
		t.Fatalf("timed out waiting for index: %v", err)
	}

	return srv
}

func newTestServer(t *testing.T, cfg *config.Config) *Server {
	t.Helper()
	if cfg.WatchPath == "" {
		cfg.WatchPath = testutil.CreateTempDir(t)
	}
	src, err := source.NewFilesystem(cfg.WatchPath, cfg.RespectGitignore, cfg.IgnorePatterns)
	if err != nil {
		t.Fatalf("failed to create filesystem source: %v", err)
	}
	return NewServer(cfg, src)
}

func TestNewServer(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  testHost,
		Theme: testThemeName,
	}

	srv := newTestServer(t, cfg)
	if srv == nil {
		t.Fatal("expected server to be created, got nil")
	}

	if srv.httpServer == nil {
		t.Error("expected httpServer to be initialized")
	}

	if srv.config != cfg {
		t.Error("expected config to be set")
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	t.Parallel()

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:  port,
		Host:  testHost,
		Theme: testThemeName,
	}

	srv := newTestServer(t, cfg)

	done := make(chan error, 1)
	go func() {
		done <- srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	_ = resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("server Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for server to stop")
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	t.Parallel()

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:  port,
		Host:  testHost,
		Theme: testThemeName,
	}

	srv := newTestServer(t, cfg)

	go func() {
		_ = srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("graceful shutdown failed: %v", err)
	}
}

func TestServerShutdownTimeout(t *testing.T) {
	t.Parallel()

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:  port,
		Host:  testHost,
		Theme: testThemeName,
	}

	srv := newTestServer(t, cfg)

	go func() {
		_ = srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond)

	_ = srv.Shutdown(ctx)
}

func TestServerRespondsToRequests(t *testing.T) {
	t.Parallel()

	tmpDir := testutil.CreateTempDir(t)
	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:      port,
		Host:      testHost,
		WatchPath: tmpDir,
		Theme:     testThemeName,
	}

	srv := newTestServer(t, cfg)

	go func() {
		_ = srv.Start()
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	waitForServer(t, port)

	rootURL := fmt.Sprintf("http://localhost:%d/", port)
	resp, err := http.Get(rootURL) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for /, got %d", resp.StatusCode)
	}

	notFoundURL := fmt.Sprintf("http://localhost:%d/nonexistent", port)
	resp2, err := http.Get(notFoundURL) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for /nonexistent, got %d", resp2.StatusCode)
	}
}

func TestServerAddress(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  9000,
		Host:  "0.0.0.0",
		Theme: testThemeName,
	}

	srv := newTestServer(t, cfg)

	expectedAddr := "0.0.0.0:9000"
	if srv.httpServer.Addr != expectedAddr {
		t.Errorf("expected address %s, got %s", expectedAddr, srv.httpServer.Addr)
	}
}

func TestServerTimeouts(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  testHost,
		Theme: testThemeName,
	}

	srv := newTestServer(t, cfg)

	if srv.httpServer.ReadTimeout != 15*time.Second {
		t.Errorf("expected ReadTimeout 15s, got %v", srv.httpServer.ReadTimeout)
	}

	if srv.httpServer.IdleTimeout != 60*time.Second {
		t.Errorf("expected IdleTimeout 60s, got %v", srv.httpServer.IdleTimeout)
	}
}

func TestServerUpdateTree(t *testing.T) {
	t.Parallel()

	tmpDir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, tmpDir, "initial.md", "initial content")

	cfg := &config.Config{
		Port:             8080,
		Host:             testHost,
		WatchPath:        tmpDir,
		Theme:            testThemeName,
		RespectGitignore: false,
		IgnorePatterns:   []string{},
	}

	srv := newTestServer(t, cfg)

	// Wait for background scan to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.WaitForIndexReady(ctx); err != nil {
		t.Fatalf("timed out waiting for index: %v", err)
	}

	srv.treeMux.RLock()
	initialNode := srv.primary().tree.Find("initial.md")
	srv.treeMux.RUnlock()

	if initialNode == nil {
		t.Fatal("expected initial.md to be in tree")
	}

	testutil.CreateTestFile(t, tmpDir, "new.md", "new content")

	srv.updateTree()

	srv.treeMux.RLock()
	newNode := srv.primary().tree.Find("new.md")
	srv.treeMux.RUnlock()

	if newNode == nil {
		t.Error("expected new.md to be in tree after updateTree")
	}
}

func TestServerUpdateTreeConcurrent(t *testing.T) {
	t.Parallel()

	tmpDir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, tmpDir, "test.md", "test content")

	cfg := &config.Config{
		Port:             8080,
		Host:             testHost,
		WatchPath:        tmpDir,
		Theme:            testThemeName,
		RespectGitignore: false,
		IgnorePatterns:   []string{},
	}

	srv := newTestServer(t, cfg)

	// Wait for background scan to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.WaitForIndexReady(ctx); err != nil {
		t.Fatalf("timed out waiting for index: %v", err)
	}

	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			srv.treeMux.RLock()
			_ = srv.primary().tree
			srv.treeMux.RUnlock()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			srv.updateTree()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	<-done
	<-done
}

func TestServerUpdateTreeOnScanFailure(t *testing.T) {
	t.Parallel()

	tmpDir := testutil.CreateTempDir(t)

	cfg := &config.Config{
		Port:             8080,
		Host:             testHost,
		WatchPath:        tmpDir,
		Theme:            testThemeName,
		RespectGitignore: false,
		IgnorePatterns:   []string{},
	}

	srv := newTestServer(t, cfg)

	// Wait for background scan to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.WaitForIndexReady(ctx); err != nil {
		t.Fatalf("timed out waiting for index: %v", err)
	}

	srv.treeMux.RLock()
	oldTree := srv.primary().tree
	srv.treeMux.RUnlock()

	// Removing the content root makes the next scan fail.
	if err := os.RemoveAll(tmpDir); err != nil {
		t.Fatalf("failed to remove content root: %v", err)
	}

	srv.updateTree()

	srv.treeMux.RLock()
	newTree := srv.primary().tree
	srv.treeMux.RUnlock()

	if newTree != oldTree {
		t.Error("expected tree to remain unchanged when updateTree fails")
	}

	if srv.updateTree() {
		t.Error("expected a failed rescan to report no structural change")
	}
}

// TestUpdateTreeStructuralVerdict pins the contract the sidebar gate rests
// on. If the verdict inverts, either the file tree stops updating when files
// appear, or it resumes redrawing on every save, which is the defect this
// work exists to remove.
func TestUpdateTreeStructuralVerdict(t *testing.T) {
	t.Parallel()

	tmpDir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, tmpDir, "initial.md", "initial content")

	cfg := &config.Config{
		Port:             8080,
		Host:             testHost,
		WatchPath:        tmpDir,
		Theme:            testThemeName,
		RespectGitignore: false,
		IgnorePatterns:   []string{},
	}

	srv := newTestServer(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.WaitForIndexReady(ctx); err != nil {
		t.Fatalf("timed out waiting for index: %v", err)
	}

	testutil.CreateTestFile(t, tmpDir, "initial.md", "rewritten content")
	if srv.updateTree() {
		t.Error("expected rewriting an existing file to be non-structural")
	}

	if srv.updateTree() {
		t.Error("expected a rescan with no change at all to be non-structural")
	}

	testutil.CreateTestFile(t, tmpDir, "added.md", "added content")
	if !srv.updateTree() {
		t.Error("expected file creation to be structural")
	}

	if err := os.Remove(filepath.Join(tmpDir, "added.md")); err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}
	if !srv.updateTree() {
		t.Error("expected file removal to be structural")
	}

	testutil.CreateTestFile(t, tmpDir, "nested/deep.md", "nested content")
	if !srv.updateTree() {
		t.Error("expected directory creation to be structural")
	}
}

func TestServerTreeUpdateOnFileEvent(t *testing.T) {
	t.Parallel()

	tmpDir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, tmpDir, "initial.md", "initial content")

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:             port,
		Host:             testHost,
		WatchPath:        tmpDir,
		Theme:            testThemeName,
		RespectGitignore: false,
		IgnorePatterns:   []string{},
	}

	srv := newTestServer(t, cfg)

	go func() {
		_ = srv.Start()
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	// Wait for background scan to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.WaitForIndexReady(ctx); err != nil {
		t.Fatalf("timed out waiting for index: %v", err)
	}

	srv.treeMux.RLock()
	initialNode := srv.primary().tree.Find("new-file.md")
	srv.treeMux.RUnlock()

	if initialNode != nil {
		t.Fatal("expected new-file.md to not be in tree initially")
	}

	// Block on the broadcast rather than a fixed wait: it is sent only after
	// the rebuild completes, so the tree is settled when it arrives. The
	// budget covers the watcher debounce plus a full coalescing ceiling.
	client := make(chan string, 10)
	srv.addClient(client)
	defer srv.removeClient(client)

	testutil.CreateTestFile(t, tmpDir, "new-file.md", "new content")

	readBroadcastWithin(t, client, coalesceMaxDelay+5*time.Second)

	srv.treeMux.RLock()
	newNode := srv.primary().tree.Find("new-file.md")
	srv.treeMux.RUnlock()

	if newNode == nil {
		t.Error("expected new-file.md to be in tree after file creation event")
	}
}

// readBroadcast drains the client channel until a payload other than the
// index-ready envelope arrives; the background scan may race one in.
func readBroadcast(t *testing.T, client chan string) string {
	t.Helper()
	return readBroadcastWithin(t, client, time.Second)
}

// readBroadcastWithin returns the next content broadcast, skipping the
// index-ready envelope the background scan may race in. Waits that span a
// rebuild need a budget derived from the coalescing constants, since the
// rebuild is deliberately deferred.
func readBroadcastWithin(t *testing.T, client chan string, within time.Duration) string {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case payload := <-client:
			if payload != indexReadyPayload {
				return payload
			}
		case <-deadline:
			t.Fatal("timed out waiting for broadcast payload")
			return ""
		}
	}
}

// assertNoFurtherBroadcast asserts the burst produced nothing more. The
// absence of a signal cannot be blocked on, so this is necessarily a bounded
// wait; it is sized well beyond the coalescing window.
func assertNoFurtherBroadcast(t *testing.T, client chan string) {
	t.Helper()
	select {
	case extra := <-client:
		if extra != indexReadyPayload {
			t.Errorf("expected the burst to produce exactly one broadcast, also got %s", extra)
		}
	case <-time.After(3 * coalesceWindow):
	}
}

func TestBroadcastChangePayloadShape(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  testHost,
		Theme: testThemeName,
	}
	srv := newTestServer(t, cfg)

	client := make(chan string, 10)
	srv.addClient(client)
	defer srv.removeClient(client)

	srv.broadcastChange("docs/example.md", false)

	want := `{"type":"reload","path":"docs/example.md","structural":false}`
	if got := readBroadcast(t, client); got != want {
		t.Errorf("expected payload %s, got %s", want, got)
	}
}

// TestBroadcastChangeCarriesStructuralFlag pins the field the SSE client
// switches on. It is always present, never omitted, so a client can tell a
// structural false apart from an older server that sends no flag at all.
func TestBroadcastChangeCarriesStructuralFlag(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  testHost,
		Theme: testThemeName,
	}
	srv := newTestServer(t, cfg)

	client := make(chan string, 10)
	srv.addClient(client)
	defer srv.removeClient(client)

	srv.broadcastChange("docs/example.md", true)

	want := `{"type":"reload","path":"docs/example.md","structural":true}`
	if got := readBroadcast(t, client); got != want {
		t.Errorf("expected payload %s, got %s", want, got)
	}
}

func TestBroadcastReloadOmitsPath(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  testHost,
		Theme: testThemeName,
	}
	srv := newTestServer(t, cfg)

	client := make(chan string, 10)
	srv.addClient(client)
	defer srv.removeClient(client)

	srv.broadcastReload(true)

	want := `{"type":"reload","structural":true}`
	if got := readBroadcast(t, client); got != want {
		t.Errorf("expected payload %s, got %s", want, got)
	}
}

func TestBroadcastIndexReadyPayloadShape(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  testHost,
		Theme: testThemeName,
	}
	srv := newTestServer(t, cfg)

	client := make(chan string, 10)
	srv.addClient(client)
	defer srv.removeClient(client)

	srv.broadcastIndexReady()

	want := indexReadyPayload
	select {
	case got := <-client:
		if got != want {
			t.Errorf("expected payload %s, got %s", want, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for index-ready payload")
	}
}

// TestBroadcastCoalescesBurst is the contract this coalescing exists for: a
// burst of writes costs one rescan and one notification, not one per file.
// The reported path is the newest in the burst, which is what follow mode
// navigates to.
func TestBroadcastCoalescesBurst(t *testing.T) {
	t.Parallel()

	src := newFakeWatchableSource()
	srv := newServerWithSource(t, src)

	client := make(chan string, 64)
	srv.addClient(client)
	defer srv.removeClient(client)

	const burst = 20
	for i := range burst {
		src.emit(t, fmt.Sprintf("burst%d.md", i))
	}

	want := fmt.Sprintf(`{"type":"reload","path":"burst%d.md","structural":false}`, burst-1)
	if got := readBroadcastWithin(t, client, coalesceMaxDelay+time.Second); got != want {
		t.Errorf("expected a single coalesced broadcast %s, got %s", want, got)
	}

	assertNoFurtherBroadcast(t, client)
}

// TestBroadcastSeparatedBurstsRebuildEach confirms coalescing defers work
// rather than discarding it: once a burst has flushed, the next change gets
// its own rebuild.
func TestBroadcastSeparatedBurstsRebuildEach(t *testing.T) {
	t.Parallel()

	src := newFakeWatchableSource()
	srv := newServerWithSource(t, src)

	client := make(chan string, 64)
	srv.addClient(client)
	defer srv.removeClient(client)

	src.emit(t, "first.md")
	first := readBroadcastWithin(t, client, coalesceMaxDelay+time.Second)
	if want := `{"type":"reload","path":"first.md","structural":false}`; first != want {
		t.Errorf("expected first broadcast %s, got %s", want, first)
	}

	// The read above returned only after the first flush, so this event
	// begins a genuinely separate burst without any timing assumption.
	src.emit(t, "second.md")
	second := readBroadcastWithin(t, client, coalesceMaxDelay+time.Second)
	if want := `{"type":"reload","path":"second.md","structural":false}`; second != want {
		t.Errorf("expected second broadcast %s, got %s", want, second)
	}
}

// TestBroadcastFlushesUnderSustainedWrites exercises the ceiling. Events
// arrive faster than the quiet window can ever elapse, so the only thing
// that can produce a broadcast is coalesceMaxDelay.
func TestBroadcastFlushesUnderSustainedWrites(t *testing.T) {
	t.Parallel()

	src := newFakeWatchableSource()
	srv := newServerWithSource(t, src)

	client := make(chan string, 256)
	srv.addClient(client)
	defer srv.removeClient(client)

	var emitted atomic.Int64
	stop := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(coalesceWindow / 3)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-ticker.C:
				select {
				case src.events <- source.Event{Path: fmt.Sprintf("sustained%d.md", i)}:
					emitted.Add(1)
				case <-stop:
					return
				}
			}
		}
	}()

	// Stop the writer and let it drain before the cleanup closes the source,
	// so no send can race the channel close.
	defer func() {
		close(stop)
		<-stopped
	}()

	readBroadcastWithin(t, client, coalesceMaxDelay+3*time.Second)

	// A quiet-window flush is impossible at this arrival rate, so more than
	// a couple of events in flight proves the ceiling did the work.
	if n := emitted.Load(); n < 4 {
		t.Errorf("expected the ceiling to flush while writes continued, only %d events emitted", n)
	}
}

func TestFollowModeCapabilityForFilesystemSource(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  testHost,
		Theme: testThemeName,
	}
	srv := newTestServer(t, cfg)

	if !srv.primary().caps.FollowMode {
		t.Error("expected FollowMode capability for watchable filesystem source")
	}
}
