package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

func TestSecurityHeaders(t *testing.T) {
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

	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

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
		Port:  port,
		Host:  testHost,
		Theme: testThemeName,
	}

	srv := newTestServer(t, cfg)

	srv.mux.HandleFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	go func() {
		_ = srv.Start()
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/panic", port)
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

// TestLoggingMiddlewareClearsWriteDeadline covers the sign bridge stream,
// which clears its write deadline through http.ResponseController and fails
// with 500 when the logging writer hides the server's writer.
func TestLoggingMiddlewareClearsWriteDeadline(t *testing.T) {
	t.Parallel()

	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL) //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
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
