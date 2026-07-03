package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

const (
	testHost      = "localhost"
	testThemeName = "iceberg-dark"
	testIndexFile = "index.md"
)

// newTestServer builds a server over a FilesystemSource. Tests without an
// explicit WatchPath get a dedicated temp dir; the old empty-path behavior
// (resolving to the working directory) was accidental, never specified.
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
	initialNode := srv.tree.Find("initial.md")
	srv.treeMux.RUnlock()

	if initialNode == nil {
		t.Fatal("expected initial.md to be in tree")
	}

	testutil.CreateTestFile(t, tmpDir, "new.md", "new content")

	srv.updateTree()

	srv.treeMux.RLock()
	newNode := srv.tree.Find("new.md")
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
			_ = srv.tree
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
	oldTree := srv.tree
	srv.treeMux.RUnlock()

	// Removing the content root makes the next scan fail.
	if err := os.RemoveAll(tmpDir); err != nil {
		t.Fatalf("failed to remove content root: %v", err)
	}

	srv.updateTree()

	srv.treeMux.RLock()
	newTree := srv.tree
	srv.treeMux.RUnlock()

	if newTree != oldTree {
		t.Error("expected tree to remain unchanged when updateTree fails")
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
	initialNode := srv.tree.Find("new-file.md")
	srv.treeMux.RUnlock()

	if initialNode != nil {
		t.Fatal("expected new-file.md to not be in tree initially")
	}

	testutil.CreateTestFile(t, tmpDir, "new-file.md", "new content")

	time.Sleep(500 * time.Millisecond)

	srv.treeMux.RLock()
	newNode := srv.tree.Find("new-file.md")
	srv.treeMux.RUnlock()

	if newNode == nil {
		t.Error("expected new-file.md to be in tree after file creation event")
	}
}

// readBroadcast drains the client channel until a payload other than the
// index-ready envelope arrives; the background scan may race one in.
func readBroadcast(t *testing.T, client chan string) string {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case payload := <-client:
			if payload != `{"type":"index-ready"}` {
				return payload
			}
		case <-deadline:
			t.Fatal("timed out waiting for broadcast payload")
		}
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

	srv.broadcastChange("docs/example.md")

	want := `{"type":"reload","path":"docs/example.md"}`
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

	srv.broadcastReload()

	want := `{"type":"reload"}`
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

	want := `{"type":"index-ready"}`
	select {
	case got := <-client:
		if got != want {
			t.Errorf("expected payload %s, got %s", want, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for index-ready payload")
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

	if !srv.uiCaps.FollowMode {
		t.Error("expected FollowMode capability for watchable filesystem source")
	}
}
