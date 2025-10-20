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

func TestNewServer(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  "localhost",
		Theme: "iceberg-dark",
	}

	srv := NewServer(cfg)
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
		Host:  "localhost",
		Theme: "iceberg-dark",
	}

	srv := NewServer(cfg)

	done := make(chan error, 1)
	go func() {
		done <- srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	resp.Body.Close()

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
		Host:  "localhost",
		Theme: "iceberg-dark",
	}

	srv := NewServer(cfg)

	go func() {
		srv.Start()
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
		Host:  "localhost",
		Theme: "iceberg-dark",
	}

	srv := NewServer(cfg)

	go func() {
		srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond)

	srv.Shutdown(ctx)
}

func TestServerRespondsToRequests(t *testing.T) {
	t.Parallel()

	tmpDir := testutil.CreateTempDir(t)
	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port:      port,
		Host:      "localhost",
		WatchPath: tmpDir,
		Theme:     "iceberg-dark",
	}

	srv := NewServer(cfg)

	go func() {
		srv.Start()
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	waitForServer(t, port)

	rootURL := fmt.Sprintf("http://localhost:%d/", port)
	resp, err := http.Get(rootURL)
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for /, got %d", resp.StatusCode)
	}

	notFoundURL := fmt.Sprintf("http://localhost:%d/nonexistent", port)
	resp2, err := http.Get(notFoundURL)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for /nonexistent, got %d", resp2.StatusCode)
	}
}

func TestServerAddress(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  9000,
		Host:  "0.0.0.0",
		Theme: "iceberg-dark",
	}

	srv := NewServer(cfg)

	expectedAddr := "0.0.0.0:9000"
	if srv.httpServer.Addr != expectedAddr {
		t.Errorf("expected address %s, got %s", expectedAddr, srv.httpServer.Addr)
	}
}

func TestServerTimeouts(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:  8080,
		Host:  "localhost",
		Theme: "iceberg-dark",
	}

	srv := NewServer(cfg)

	if srv.httpServer.ReadTimeout != 15*time.Second {
		t.Errorf("expected ReadTimeout 15s, got %v", srv.httpServer.ReadTimeout)
	}

	if srv.httpServer.IdleTimeout != 60*time.Second {
		t.Errorf("expected IdleTimeout 60s, got %v", srv.httpServer.IdleTimeout)
	}
}
