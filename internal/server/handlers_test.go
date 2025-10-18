package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

func TestStaticFileServing(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	root := filepath.Join(cwd, "../..")
	testFile := filepath.Join(root, "web/static/js/htmx.min.js")
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("htmx.min.js not found, skipping test")
	}

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldDir)

	if err := os.Chdir(root); err != nil {
		t.Fatalf("failed to change to root directory: %v", err)
	}

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port: port,
		Host: "localhost",
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

	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/static/js/htmx.min.js", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to get static file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/javascript; charset=utf-8" && contentType != "application/javascript" {
		t.Errorf("expected JavaScript content type, got %s", contentType)
	}
}

func TestStaticFileMissing(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	root := filepath.Join(cwd, "../..")
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(root)

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port: port,
		Host: "localhost",
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

	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/static/nonexistent.js", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to get static file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestStaticStylesDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	root := filepath.Join(cwd, "../..")
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(root)

	dir := testutil.CreateTempDir(t)
	cssPath := filepath.Join(dir, "test.css")
	if err := os.WriteFile(cssPath, []byte("body { color: red; }"), 0644); err != nil {
		t.Fatalf("failed to create test CSS file: %v", err)
	}

	if err := os.MkdirAll("web/static/styles", 0755); err != nil {
		t.Fatalf("failed to create styles directory: %v", err)
	}

	testCSSPath := "web/static/styles/test.css"
	if err := os.WriteFile(testCSSPath, []byte("body { color: red; }"), 0644); err != nil {
		t.Fatalf("failed to create test CSS: %v", err)
	}
	defer os.Remove(testCSSPath)

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port: port,
		Host: "localhost",
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

	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/static/styles/test.css", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to get CSS file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
