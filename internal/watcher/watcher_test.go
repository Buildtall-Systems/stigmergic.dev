package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

func TestNewWatcher(t *testing.T) {
	t.Parallel()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	if w == nil {
		t.Fatal("expected watcher to be created, got nil")
	}

	if w.Events == nil {
		t.Error("expected Events channel to be initialized")
	}

	if w.Errors == nil {
		t.Error("expected Errors channel to be initialized")
	}
}

func TestWatcherAdd(t *testing.T) {
	t.Parallel()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
}

func TestWatcherAddNonExistent(t *testing.T) {
	t.Parallel()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)
	nonExistent := filepath.Join(dir, "nonexistent")

	if err := w.Add(nonExistent); err == nil {
		t.Fatal("expected error when adding nonexistent path, got nil")
	}
}

func TestWatcherRemove(t *testing.T) {
	t.Parallel()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if err := w.Remove(dir); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
}

func TestWatcherRemoveNotWatched(t *testing.T) {
	t.Parallel()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)

	if err := w.Remove(dir); err == nil {
		t.Fatal("expected error when removing unwatched path, got nil")
	}
}

func TestWatcherCreateEvent(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	filePath := filepath.Join(dir, "test.txt")

	done := make(chan bool)
	go func() {
		select {
		case event := <-w.Events:
			if event.Type != EventCreate {
				t.Errorf("expected EventCreate, got %v", event.Type)
			}
			if event.Path != filePath {
				t.Errorf("expected path %s, got %s", filePath, event.Path)
			}
			done <- true
		case err := <-w.Errors:
			t.Errorf("unexpected error: %v", err)
			done <- false
		case <-time.After(2 * time.Second):
			t.Error("timeout waiting for event")
			done <- false
		}
	}()

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	<-done
}

func TestWatcherWriteEvent(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)
	filePath := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(filePath, []byte("initial"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	done := make(chan bool)
	go func() {
		select {
		case event := <-w.Events:
			if event.Type != EventWrite {
				t.Errorf("expected EventWrite, got %v", event.Type)
			}
			done <- true
		case err := <-w.Errors:
			t.Errorf("unexpected error: %v", err)
			done <- false
		case <-time.After(2 * time.Second):
			t.Error("timeout waiting for event")
			done <- false
		}
	}()

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filePath, []byte("updated"), 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	<-done
}

func TestWatcherRemoveEvent(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)
	filePath := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	done := make(chan bool)
	go func() {
		select {
		case event := <-w.Events:
			if event.Type != EventRemove {
				t.Errorf("expected EventRemove, got %v", event.Type)
			}
			done <- true
		case err := <-w.Errors:
			t.Errorf("unexpected error: %v", err)
			done <- false
		case <-time.After(2 * time.Second):
			t.Error("timeout waiting for event")
			done <- false
		}
	}()

	time.Sleep(100 * time.Millisecond)

	if err := os.Remove(filePath); err != nil {
		t.Fatalf("failed to remove test file: %v", err)
	}

	<-done
}

func TestWatcherClose(t *testing.T) {
	t.Parallel()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestWatcherCloseChannels(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	select {
	case _, ok := <-w.Events:
		if ok {
			t.Error("expected Events channel to be closed")
		}
	default:
		t.Error("Events channel should be readable (closed)")
	}
}

func TestWatcherRelativePath(t *testing.T) {
	t.Parallel()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add with absolute path failed: %v", err)
	}
}
