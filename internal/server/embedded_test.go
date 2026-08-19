package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

const embeddedSourceName = "embedded-test"

func embeddedTestFS() fstest.MapFS {
	return fstest.MapFS{
		testIndexFile:   &fstest.MapFile{Data: []byte("# Embedded Home\n\nWelcome.\n")},
		"docs/guide.md": &fstest.MapFile{Data: []byte("# Guide\n")},
		"img/logo.png":  &fstest.MapFile{Data: minimalPNG},
	}
}

func startEmbeddedServer(t *testing.T, fsys fs.FS) (int, func()) {
	t.Helper()
	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:  port,
		Host:  testHost,
		Theme: testThemeName,
	}
	srv := NewServer(cfg, source.NewEmbedded(fsys, embeddedSourceName))
	go func() {
		if err := srv.Start(); err != nil {
			t.Errorf("embedded server exited with error: %v", err)
		}
	}()
	waitForServer(t, port)

	readyCtx, readyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readyCancel()
	if err := srv.WaitForIndexReady(readyCtx); err != nil {
		t.Fatalf("index never became ready for embedded source: %v", err)
	}

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("embedded server shutdown failed: %v", err)
		}
	}
	return port, cleanup
}

func getWithContext(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if err := resp.Body.Close(); err != nil {
		t.Errorf("failed to close response body: %v", err)
	}
}

func TestEmbeddedFollowModeAbsent(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  testHost,
		Theme: testThemeName,
	}
	srv := NewServer(cfg, source.NewEmbedded(embeddedTestFS(), embeddedSourceName))

	if srv.primary().caps.FollowMode {
		t.Error("embedded source must not advertise the FollowMode capability")
	}
}

func TestEmbeddedMarkdownRenders(t *testing.T) {
	t.Parallel()

	port, cleanup := startEmbeddedServer(t, embeddedTestFS())
	defer cleanup()

	resp := getWithContext(t, fmt.Sprintf("http://localhost:%d/file/index.md", port))
	defer closeBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if !strings.Contains(string(body), "<h1") {
		t.Errorf("expected rendered HTML with <h1>, got: %s", string(body)[:min(200, len(body))])
	}
}

func TestEmbeddedImageServing(t *testing.T) {
	t.Parallel()

	port, cleanup := startEmbeddedServer(t, embeddedTestFS())
	defer cleanup()

	resp := getWithContext(t, fmt.Sprintf("http://localhost:%d/file/img/logo.png", port))
	defer closeBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/png") {
		t.Errorf("expected image/png content type, got %s", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if !bytes.Equal(body, minimalPNG) {
		t.Error("response body does not match embedded PNG bytes")
	}
}

func TestEmbeddedDirectoryListing(t *testing.T) {
	t.Parallel()

	port, cleanup := startEmbeddedServer(t, embeddedTestFS())
	defer cleanup()

	resp := getWithContext(t, fmt.Sprintf("http://localhost:%d/file/docs", port))
	defer closeBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if !strings.Contains(string(body), "guide.md") {
		t.Error("expected directory listing to include guide.md")
	}
}

func TestEmbeddedFilesAPI(t *testing.T) {
	t.Parallel()

	port, cleanup := startEmbeddedServer(t, embeddedTestFS())
	defer cleanup()

	resp := getWithContext(t, fmt.Sprintf("http://localhost:%d/api/files", port))
	defer closeBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var files []models.SearchableFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		t.Fatalf("failed to decode files JSON: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 markdown files in index, got %d", len(files))
	}
}

func TestEmbeddedGitignoreEndpointsAbsent(t *testing.T) {
	t.Parallel()

	port, cleanup := startEmbeddedServer(t, embeddedTestFS())
	defer cleanup()

	statusResp := getWithContext(t, fmt.Sprintf("http://localhost:%d/api/gitignore", port))
	defer closeBody(t, statusResp)
	if statusResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for gitignore status on embedded source, got %d", statusResp.StatusCode)
	}

	toggleReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		fmt.Sprintf("http://localhost:%d/api/gitignore/toggle", port), nil)
	if err != nil {
		t.Fatalf("failed to build toggle request: %v", err)
	}
	toggleResp, err := http.DefaultClient.Do(toggleReq)
	if err != nil {
		t.Fatalf("toggle request failed: %v", err)
	}
	defer closeBody(t, toggleResp)
	if toggleResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for gitignore toggle on embedded source, got %d", toggleResp.StatusCode)
	}
}

func TestEmbeddedTraversalRejected(t *testing.T) {
	t.Parallel()

	port, cleanup := startEmbeddedServer(t, embeddedTestFS())
	defer cleanup()

	for _, tc := range []struct {
		name string
		path string
	}{
		{"dotdot", "/file/../secret.md"},
		{"nested dotdot", "/file/docs/../../secret.md"},
		{"encoded dotdot", "/file/%2e%2e/secret.md"},
		{"absolute", "/file//etc/passwd"},
		{"trailing slash", "/file/index.md/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := getWithContext(t, fmt.Sprintf("http://localhost:%d%s", port, tc.path))
			defer closeBody(t, resp)
			if resp.StatusCode == http.StatusOK {
				t.Errorf("expected traversal path %q to be rejected, got 200", tc.path)
			}
		})
	}
}

func TestEmbeddedLifecycleShutdown(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  testutil.FindAvailablePort(t),
		Host:  testHost,
		Theme: testThemeName,
	}
	srv := NewServer(cfg, source.NewEmbedded(embeddedTestFS(), embeddedSourceName))

	readyCtx, readyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readyCancel()
	if err := srv.WaitForIndexReady(readyCtx); err != nil {
		t.Fatalf("index never became ready: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}
