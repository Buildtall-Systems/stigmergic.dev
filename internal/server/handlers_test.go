package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
		DefaultFile: testIndexFile,
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
		DefaultFile: testIndexFile,
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
	return startServerWithConfig(t, &config.Config{
		Port:      testutil.FindAvailablePort(t),
		Host:      testHost,
		Theme:     testThemeName,
		WatchPath: watchPath,
	})
}

func startServerWithConfig(t *testing.T, cfg *config.Config) (int, func()) {
	t.Helper()
	port := cfg.Port
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

// TestMarkdownEmbedTranscludesSection drives transclusion through the handler
// rather than the renderer, which is where the embed source and the resolver
// are actually wired together. The poll is on the index: the resolver reads
// the scanned file set, so the target is unresolvable until the background
// scan has seen it.
func TestMarkdownEmbedTranscludesSection(t *testing.T) {
	t.Parallel()

	const (
		transcludedText = "alpha body"
		omittedText     = "beta body"
	)

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "target.md", "# Target\n\n## Alpha\n\nalpha body\n\n## Beta\n\nbeta body\n")
	testutil.CreateTestFile(t, dir, "host.md", "Host text.\n\n![[target#Alpha]]\n")

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/file/host.md", port)

	var html string
	for range 50 {
		resp, err := http.Get(url) //nolint:gosec,noctx
		if err != nil {
			t.Fatalf("failed to get host page: %v", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("failed to close response body: %v", closeErr)
		}
		if readErr != nil {
			t.Fatalf("failed to read body: %v", readErr)
		}
		html = string(body)
		if strings.Contains(html, transcludedText) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(html, transcludedText) {
		t.Fatalf("expected the transcluded section in the response, got: %s", html)
	}
	if !strings.Contains(html, `class="transclusion"`) {
		t.Errorf("expected a transclusion container, got: %s", html)
	}
	if strings.Contains(html, omittedText) {
		t.Errorf("expected only the named section to be transcluded, got: %s", html)
	}

	// The live-reload listener reads this attribute to decide whether an edit
	// to a note the page transcludes should refresh the pane. The quotes are
	// entity-escaped because templ escapes the attribute value.
	if !strings.Contains(html, `data-transcluded="[&#34;target.md&#34;]"`) {
		t.Errorf("expected the host page to carry its transcluded route, got: %s", html)
	}
}

// TestExampleTransclusionCorpusRenders serves the repository's demonstration
// corpus at example/transclusion and confirms the host note renders every
// form: transcluded section content arrives, and the unresolved, unmatched,
// and cycle cases degrade to their markers rather than failing the page.
func TestExampleTransclusionCorpusRenders(t *testing.T) {
	t.Parallel()

	port, cleanup := startServerWithWatchPath(t, filepath.Join("..", "..", "example", "transclusion"))
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/file/host.md", port)

	var html string
	for range 50 {
		resp, err := http.Get(url) //nolint:gosec,noctx
		if err != nil {
			t.Fatalf("failed to get the host note: %v", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("failed to close response body: %v", closeErr)
		}
		if readErr != nil {
			t.Fatalf("failed to read body: %v", readErr)
		}
		html = string(body)
		if strings.Contains(html, "Tide pools") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(html, "Tide pools") {
		t.Fatalf("expected transcluded section content from the corpus, got: %s", html)
	}
	if !strings.Contains(html, `data-embed-error="unresolved"`) {
		t.Errorf("expected the dangling embed's marker, got: %s", html)
	}
	if !strings.Contains(html, `data-embed-error="no-section"`) {
		t.Errorf("expected the unmatched fragment's marker, got: %s", html)
	}
	if !strings.Contains(html, `data-embed-error="cycle"`) {
		t.Errorf("expected the cycle pair to terminate at a marker, got: %s", html)
	}
}

// TestMarkdownEmbedImageResolvesThroughAttachmentRoot follows an image embed
// all the way to its bytes: the probe finds the file under the attachment
// root even though no index entry exists for it, and the route it emits is
// served by the existing non-markdown branch of handleMarkdown.
func TestMarkdownEmbedImageResolvesThroughAttachmentRoot(t *testing.T) {
	t.Parallel()

	const imageSrc = "/file/file/image.png"

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "host.md", "Host text.\n\n![[image.png]]\n")
	if err := os.MkdirAll(filepath.Join(dir, "file"), 0o750); err != nil {
		t.Fatalf("failed to create attachment directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file", "image.png"), minimalPNG, 0644); err != nil {
		t.Fatalf("failed to write PNG: %v", err)
	}

	port, cleanup := startServerWithConfig(t, &config.Config{
		Port:           testutil.FindAvailablePort(t),
		Host:           testHost,
		Theme:          testThemeName,
		WatchPath:      dir,
		AttachmentRoot: "file",
	})
	defer cleanup()

	pageURL := fmt.Sprintf("http://localhost:%d/file/host.md", port)

	var html string
	for range 50 {
		resp, err := http.Get(pageURL) //nolint:gosec,noctx
		if err != nil {
			t.Fatalf("failed to get host page: %v", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("failed to close response body: %v", closeErr)
		}
		if readErr != nil {
			t.Fatalf("failed to read body: %v", readErr)
		}
		html = string(body)
		if strings.Contains(html, imageSrc) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(html, `<img src="`+imageSrc+`">`) {
		t.Fatalf("expected an image element sourced at %s, got: %s", imageSrc, html)
	}

	assetResp, err := http.Get(fmt.Sprintf("http://localhost:%d%s", port, imageSrc)) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to get the embedded image: %v", err)
	}
	defer func() { _ = assetResp.Body.Close() }()

	if assetResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for the embedded image, got %d", assetResp.StatusCode)
	}
	assetBody, err := io.ReadAll(assetResp.Body)
	if err != nil {
		t.Fatalf("failed to read the image body: %v", err)
	}
	if !bytes.Equal(assetBody, minimalPNG) {
		t.Errorf("embedded image bytes do not match the file on disk")
	}
}

func TestHTMXMarkdownIncludesOutlineOOB(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "doc.md", "# Title\n\n## Section One\n\ntext\n")

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/file/doc.md", port)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("HX-Request", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get markdown partial: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("failed to close response body: %v", closeErr)
		}
	}()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	body := string(b)

	if !strings.Contains(body, `id="outline" hx-swap-oob="innerHTML"`) {
		t.Error("expected out-of-band outline fragment in markdown partial")
	}
	if !strings.Contains(body, `data-outline-target="section-one"`) {
		t.Error("expected outline entry for known heading")
	}
}

func TestHTMXHomeClearsOutlineOOB(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "doc.md", "# Title\n")

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/", port)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("HX-Request", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get home partial: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("failed to close response body: %v", closeErr)
		}
	}()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	body := string(b)

	if !strings.Contains(body, `id="outline" hx-swap-oob="innerHTML"`) {
		t.Error("expected out-of-band outline fragment in home partial")
	}
	if strings.Contains(body, "data-outline-target") {
		t.Error("home partial must clear the outline rail, not populate it")
	}
}

func TestSSEStreamEnvelopeFraming(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://localhost:%d/events", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to SSE endpoint: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("failed to close response body: %v", closeErr)
		}
	}()

	reader := bufio.NewReader(resp.Body)
	connected, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read connection confirmation: %v", err)
	}
	if !strings.HasPrefix(connected, ": connected") {
		t.Fatalf("expected connection comment, got %q", connected)
	}

	// The registered client now receives watcher broadcasts; a file write
	// must arrive as a framed JSON envelope naming the changed path.
	testutil.CreateTestFile(t, dir, "changed.md", "# Changed\n")

	// Scan frames until the change envelope arrives; the index-ready
	// broadcast may legitimately precede it. The request context deadline
	// bounds the read loop.
	var eventLine string
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatalf("SSE stream ended before change envelope arrived: %v", readErr)
		}
		if strings.HasPrefix(line, "event: ") {
			eventLine = strings.TrimSpace(line)
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		var envelope struct {
			Type string `json:"type"`
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			t.Fatalf("SSE data is not valid JSON: %v (payload %q)", err, payload)
		}
		if envelope.Type != "reload" || envelope.Path != "changed.md" {
			continue
		}
		if eventLine != "event: message" {
			t.Errorf("expected 'event: message' framing, got %q", eventLine)
		}
		return
	}
}

func TestSidebarPartialRendersTree(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "test.md", "# Hello World\n")

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/partial/sidebar", port)
	deadline := time.Now().Add(5 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to get sidebar partial: %v", err)
		}
		b, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("failed to close response body: %v", closeErr)
		}
		if readErr != nil {
			t.Fatalf("failed to read response body: %v", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}
		body = string(b)
		if strings.Contains(body, "test.md") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !strings.Contains(body, "test.md") {
		t.Errorf("expected sidebar partial to contain tree entry, got: %s", body)
	}
	if !strings.Contains(body, `hx-target="#content"`) {
		t.Error("expected sidebar links to target #content")
	}
	if strings.Contains(body, "<html") {
		t.Error("sidebar partial must not be a full page")
	}
}

// TestRecentPartialMatchesSidebarRecentBlock keeps the cheap refresh target
// honest. The recent partial must render exactly the block the full sidebar
// renders, and nothing else: not the tree, and not the standing container,
// which stays in the page so it remains a valid innerHTML swap target.
func TestRecentPartialMatchesSidebarRecentBlock(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "test.md", "# Hello World\n")

	cfg := &config.Config{
		Port:             8080,
		Host:             testHost,
		WatchPath:        dir,
		Theme:            testThemeName,
		RespectGitignore: false,
		IgnorePatterns:   []string{},
		RecentFilesCount: 5,
	}
	srv := newTestServer(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.WaitForIndexReady(ctx); err != nil {
		t.Fatalf("timed out waiting for index: %v", err)
	}

	recentRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(recentRec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/partial/recent", nil))

	if recentRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from recent partial, got %d", recentRec.Code)
	}

	recentBody := recentRec.Body.String()
	if !strings.Contains(recentBody, "test.md") {
		t.Errorf("expected recent partial to list the file, got: %s", recentBody)
	}
	if strings.Contains(recentBody, `id="recent"`) {
		t.Error("recent partial must render the block body only; the container stays in the sidebar")
	}
	if strings.Contains(recentBody, `aria-label="File tree"`) {
		t.Error("recent partial must not carry the file tree")
	}

	sidebarRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(sidebarRec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/partial/sidebar", nil))

	sidebarBody := sidebarRec.Body.String()
	if !strings.Contains(sidebarBody, `id="recent"`) {
		t.Error("expected the sidebar to carry the standing #recent container")
	}
	if !strings.Contains(sidebarBody, recentBody) {
		t.Errorf("expected the sidebar's recent block to match the recent partial exactly, got sidebar: %s", sidebarBody)
	}
}

func TestHTMXFileRequestReturnsContentPartial(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "test.md", "# Hello World\n")

	port, cleanup := startServerWithWatchPath(t, dir)
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%d/file/test.md", port)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("HX-Request", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get file partial: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	html := string(b)
	if !strings.Contains(html, "<h1") {
		t.Errorf("expected rendered markdown in partial, got: %s", html[:min(200, len(html))])
	}
	if strings.Contains(html, `id="sidebar"`) {
		t.Error("content partial must not carry sidebar markup")
	}
	if strings.Contains(html, "<html") {
		t.Error("content partial must not be a full page")
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

// treePartialServer builds a server over a corpus with a nested directory and
// blocks until its index is ready, so tree lookups hit a populated tree.
func treePartialServer(t *testing.T) *Server {
	t.Helper()

	dir := testutil.CreateTempDir(t)
	testutil.CreateTestFile(t, dir, "top.md", "# top\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs", "deep"), 0o750); err != nil {
		t.Fatalf("failed to create nested directories: %v", err)
	}
	testutil.CreateTestFile(t, dir, filepath.Join("docs", "guide.md"), "# guide\n")
	testutil.CreateTestFile(t, dir, filepath.Join("docs", "deep", "buried.md"), "# buried\n")

	cfg := &config.Config{
		Port:             8080,
		Host:             testHost,
		WatchPath:        dir,
		Theme:            testThemeName,
		RecentFilesCount: 5,
	}
	srv := newTestServer(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.WaitForIndexReady(ctx); err != nil {
		t.Fatalf("timed out waiting for index: %v", err)
	}
	return srv
}

func TestTreePartialRendersOneDirectory(t *testing.T) {
	t.Parallel()

	srv := treePartialServer(t)

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/partial/tree/docs", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "guide.md") {
		t.Errorf("expected the directory's own file, got: %s", body)
	}
	if !strings.Contains(body, `data-children-path="docs/deep" data-mount="/file/" data-loaded="false"`) {
		t.Errorf("expected a nested directory to render as an unloaded placeholder, got: %s", body)
	}
	if strings.Contains(body, "buried.md") {
		t.Error("the partial must render one level; a nested file means it recursed")
	}
	if strings.Contains(body, "top.md") {
		t.Error("the partial must render the requested directory, not the root")
	}
	if strings.Contains(body, `aria-label="File tree"`) {
		t.Error("the partial must render tree rows only, without the sidebar frame")
	}
}

func TestTreePartialRejectsNonCanonicalAndUnknownPaths(t *testing.T) {
	t.Parallel()

	srv := treePartialServer(t)

	for _, path := range []string{
		"/partial/tree/docs/",
		"/partial/tree/",
		"/partial/tree/nosuchdir",
		"/partial/tree/top.md",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))

			if rec.Code != http.StatusNotFound {
				t.Errorf("expected status 404 for %q, got %d", path, rec.Code)
			}
		})
	}
}

// A ".." never reaches the handler: ServeMux cleans the request path and
// redirects first, exactly as it does for /file/. The handler's own canonical
// check still stands behind it, and neither layer serves content for a
// traversal attempt.
func TestTreePartialTraversalNeverServesContent(t *testing.T) {
	t.Parallel()

	srv := treePartialServer(t)

	for _, path := range []string{
		"/partial/tree/../../etc/passwd",
		"/partial/tree/docs/../docs",
		"/partial/tree/docs/deep/../../top.md",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))

			if rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusTemporaryRedirect {
				t.Errorf("expected %q to be redirected by path cleaning, got %d", path, rec.Code)
			}
			if strings.Contains(rec.Body.String(), "data-nav-item") {
				t.Errorf("expected no tree rows for %q, got: %s", path, rec.Body.String())
			}
		})
	}
}

// The sidebar partial ships collapsed unless the client says what it is
// displaying, which is what spares it a walk back down the ancestor chain.
func TestSidebarPartialExpandsToCurrentPath(t *testing.T) {
	t.Parallel()

	srv := treePartialServer(t)

	// data-path is the tree's own marker for a file row. The Recent list names
	// the same files, so matching on the filename alone would not distinguish
	// a materialized row from a recently-updated entry.
	const buriedRow = `data-path="docs/deep/buried.md"`

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/partial/sidebar", nil))
	if strings.Contains(rec.Body.String(), buriedRow) {
		t.Error("a sidebar with no current path must ship every directory collapsed")
	}

	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/partial/sidebar?path=docs%2Fdeep%2Fburied.md", nil))
	if !strings.Contains(rec.Body.String(), buriedRow) {
		t.Errorf("expected the current file's row to ship with the sidebar, got: %s", rec.Body.String())
	}

	// A path the server would not serve only seeds the expansion, so it
	// degrades to a collapsed tree rather than failing the request.
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/partial/sidebar?path=../escape/buried.md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected a non-canonical path to render a collapsed sidebar, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), buriedRow) {
		t.Error("a non-canonical path must not expand anything")
	}
}
