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

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

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

	url := fmt.Sprintf("http://localhost:%d/", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options header to be nosniff, got %s", resp.Header.Get("X-Content-Type-Options"))
	}

	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("expected X-Frame-Options header to be DENY, got %s", resp.Header.Get("X-Frame-Options"))
	}

	if resp.Header.Get("X-XSS-Protection") != "1; mode=block" {
		t.Errorf("expected X-XSS-Protection header to be '1; mode=block', got %s", resp.Header.Get("X-XSS-Protection"))
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	t.Parallel()

	port := testutil.FindAvailablePort(t)
	cfg := &config.Config{
		Port: port,
		Host: "localhost",
	}

	srv := NewServer(cfg)

	srv.mux.HandleFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	go func() {
		srv.Start()
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/panic", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}
}

func TestLoggingResponseWriter(t *testing.T) {
	t.Parallel()

	w := &loggingResponseWriter{
		ResponseWriter: &mockResponseWriter{},
		statusCode:     http.StatusOK,
	}

	w.WriteHeader(http.StatusNotFound)

	if w.statusCode != http.StatusNotFound {
		t.Errorf("expected statusCode 404, got %d", w.statusCode)
	}
}

type mockResponseWriter struct {
	headers http.Header
}

func (m *mockResponseWriter) Header() http.Header {
	if m.headers == nil {
		m.headers = make(http.Header)
	}
	return m.headers
}

func (m *mockResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {}
