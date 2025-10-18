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

func TestWatcherAddFile(t *testing.T) {
	t.Parallel()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := w.Add(filePath); err == nil {
		t.Fatal("expected error when adding file instead of directory, got nil")
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
		timeout := time.After(2 * time.Second)
		for {
			select {
			case event := <-w.Events:
				if event.Type == EventCreate && event.Path == filePath {
					done <- true
					return
				}
			case err := <-w.Errors:
				t.Errorf("unexpected error: %v", err)
				done <- false
				return
			case <-timeout:
				t.Error("timeout waiting for event")
				done <- false
				return
			}
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

func TestWatcherDebouncing(t *testing.T) {
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

	time.Sleep(100 * time.Millisecond)

	eventCount := 0
	done := make(chan bool)

	go func() {
		timeout := time.After(1 * time.Second)
		for {
			select {
			case <-w.Events:
				eventCount++
			case <-timeout:
				done <- true
				return
			}
		}
	}()

	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filePath, []byte("update"), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	<-done

	if eventCount > 2 {
		t.Errorf("expected debouncing to reduce events, got %d events for 5 writes", eventCount)
	}
}

func TestWatcherRecursiveWatch(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	filePath := filepath.Join(subdir, "test.txt")

	done := make(chan bool)
	go func() {
		timeout := time.After(2 * time.Second)
		for {
			select {
			case event := <-w.Events:
				if event.Type == EventCreate && event.Path == filePath {
					done <- true
					return
				}
			case err := <-w.Errors:
				t.Errorf("unexpected error: %v", err)
				done <- false
				return
			case <-timeout:
				t.Error("timeout waiting for event in subdirectory")
				done <- false
				return
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file in subdir: %v", err)
	}

	<-done
}

func TestWatcherNewDirectoryWatch(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	dir := testutil.CreateTempDir(t)

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	newDir := filepath.Join(dir, "newdir")
	if err := os.Mkdir(newDir, 0755); err != nil {
		t.Fatalf("failed to create new directory: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	filePath := filepath.Join(newDir, "test.txt")

	done := make(chan bool)
	go func() {
		timeout := time.After(2 * time.Second)
		for {
			select {
			case event := <-w.Events:
				if event.Type == EventCreate && event.Path == filePath {
					done <- true
					return
				}
			case err := <-w.Errors:
				t.Errorf("unexpected error: %v", err)
				done <- false
				return
			case <-timeout:
				t.Error("timeout waiting for event in new directory")
				done <- false
				return
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file in new dir: %v", err)
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
