package server

import (
	"context"
	"fmt"
	"net/http"
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
		Host:  "localhost",
		Theme: "iceberg-dark",
	}

	srv := NewServer(cfg)

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
		Host:  "localhost",
		Theme: "iceberg-dark",
	}

	srv := NewServer(cfg)

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
		Host:  "localhost",
		Theme: "iceberg-dark",
	}

	srv := NewServer(cfg)

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
		Host:        "localhost",
		Theme:       "iceberg-dark",
		DefaultFile: "index.md",
	}

	srv := NewServer(cfg)

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
	resp, err := client.Get(url) //nolint:gosec,noctx
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
		Host:        "localhost",
		Theme:       "iceberg-dark",
		DefaultFile: "index.md",
	}

	srv := NewServer(cfg)

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
		Host:        "localhost",
		Theme:       "iceberg-dark",
		DefaultFile: "",
	}

	srv := NewServer(cfg)

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
	resp, err := client.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to get homepage: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusFound {
		t.Errorf("should not redirect when no default file set, got %d", resp.StatusCode)
	}
}
