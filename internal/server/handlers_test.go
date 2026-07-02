package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

func waitForServer(t *testing.T, port int) {
	t.Helper()
	url := fmt.Sprintf("http://localhost:%d/", port)
	for i := 0; i < 50; i++ {
		resp, err := http.Get(url) //nolint:gosec,noctx
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server failed to start within timeout")
}

func TestStaticFileServing(t *testing.T) {
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
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	waitForServer(t, port)

	url := fmt.Sprintf("http://localhost:%d/static/js/htmx.min.js", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to get static file: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/javascript; charset=utf-8" && contentType != "application/javascript" {
		t.Errorf("expected JavaScript content type, got %s", contentType)
	}
}

func TestStaticFileMissing(t *testing.T) {
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
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	waitForServer(t, port)

	url := fmt.Sprintf("http://localhost:%d/static/nonexistent.js", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to get static file: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestStaticStylesDirectory(t *testing.T) {
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
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	waitForServer(t, port)

	url := fmt.Sprintf("http://localhost:%d/static/styles/output.css", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to get CSS file: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for CSS file, got %d", resp.StatusCode)
	}
}

func TestDefaultFileRedirect(t *testing.T) {
	t.Parallel()

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:        port,
		Host:        testHost,
		Theme:       testThemeName,
		DefaultFile: "index.md",
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

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	url := fmt.Sprintf("http://localhost:%d/", port)
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("failed to get homepage: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected redirect status 302, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/file/index.md" {
		t.Errorf("expected redirect to /file/index.md, got %s", location)
	}
}

func TestDefaultFileNoRedirectForHTMX(t *testing.T) {
	t.Parallel()

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:        port,
		Host:        testHost,
		Theme:       testThemeName,
		DefaultFile: "index.md",
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

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	url := fmt.Sprintf("http://localhost:%d/", port)
	req, _ := http.NewRequest("GET", url, nil) //nolint:noctx
	req.Header.Set("HX-Request", "true")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to get homepage with HTMX: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusFound {
		t.Errorf("HTMX request should not redirect, got %d", resp.StatusCode)
	}
}

func TestNoDefaultFileNoRedirect(t *testing.T) {
	t.Parallel()

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:        port,
		Host:        testHost,
		Theme:       testThemeName,
		DefaultFile: "",
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

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	url := fmt.Sprintf("http://localhost:%d/", port)
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("failed to get homepage: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusFound {
		t.Errorf("should not redirect when no default file set, got %d", resp.StatusCode)
	}
}

// Minimal valid 1x1 transparent PNG.
var minimalPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x62, 0x00, 0x00, 0x00, 0x02,
	0x00, 0x01, 0xe5, 0x27, 0xde, 0xfc, 0x00, 0x00,
	0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42,
	0x60, 0x82,
}

func startServerWithWatchPath(t *testing.T, watchPath string) (int, func()) {
	t.Helper()
	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:      port,
		Host:      testHost,
		Theme:     testThemeName,
		WatchPath: watchPath,
	}
	srv := newTestServer(t, cfg)
	go func() { _ = srv.Start() }()
	waitForServer(t, port)
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	return port, cleanup
}

func TestWatchDirImageServing(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "image.png"), minimalPNG, 0644); err != nil {
		t.Fatalf("failed to write PNG: %v", err)
	}

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/file/image.png", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to get image: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/png") {
		t.Errorf("expected image/png content type, got %s", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if !bytes.Equal(body, minimalPNG) {
		t.Errorf("response body does not match PNG bytes")
	}
}

func TestWatchDirMarkdownStillRenders(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "test.md", "# Hello World\n")

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/file/test.md", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to get markdown: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	html := string(body)
	if !strings.Contains(html, "<h1") {
		t.Errorf("expected rendered HTML with <h1>, got: %s", html[:min(200, len(html))])
	}
}

func TestWatchDirAssetNotFound(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/file/nonexistent.png", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestWatchDirAssetInSubdirectory(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil { //nolint:gosec
		t.Fatalf("failed to create subdir: %v", err)
	}
	svgContent := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`)
	if err := os.WriteFile(filepath.Join(subDir, "diagram.svg"), svgContent, 0644); err != nil {
		t.Fatalf("failed to write SVG: %v", err)
	}

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/file/sub/diagram.svg", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to get SVG: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "svg") {
		t.Errorf("expected SVG content type, got %s", ct)
	}
}
