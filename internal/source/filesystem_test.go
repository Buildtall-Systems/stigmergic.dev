package source

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const eventTimeout = 3 * time.Second

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("failed to create parent directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	return path
}

func newTestFilesystem(t *testing.T, respectGitignore bool) (*FilesystemSource, string) {
	t.Helper()
	dir := t.TempDir()
	src, err := NewFilesystem(dir, respectGitignore, nil)
	if err != nil {
		t.Fatalf("NewFilesystem failed: %v", err)
	}
	t.Cleanup(func() {
		if err := src.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})
	return src, dir
}

func TestFilesystemCapabilities(t *testing.T) {
	t.Parallel()

	src, dir := newTestFilesystem(t, true)

	if src.Name() != dir {
		t.Errorf("expected Name %q, got %q", dir, src.Name())
	}
	if src.Root() != dir {
		t.Errorf("expected Root %q, got %q", dir, src.Root())
	}

	var cs ContentSource = src
	if _, ok := cs.(Watchable); !ok {
		t.Error("expected FilesystemSource to be Watchable")
	}
	if _, ok := cs.(GitignoreAware); !ok {
		t.Error("expected FilesystemSource to be GitignoreAware")
	}
	if _, ok := cs.(Timestamped); !ok {
		t.Error("expected FilesystemSource to be Timestamped")
	}
	if _, ok := cs.(Rooted); !ok {
		t.Error("expected FilesystemSource to be Rooted")
	}
}

func TestFilesystemFSReadsContent(t *testing.T) {
	t.Parallel()

	src, dir := newTestFilesystem(t, false)
	writeFile(t, dir, "hello.md", "# hi")

	content, err := fs.ReadFile(src.FS(), "hello.md")
	if err != nil {
		t.Fatalf("failed to read through source FS: %v", err)
	}
	if string(content) != "# hi" {
		t.Errorf("unexpected content: %q", string(content))
	}
}

func TestFilesystemEmitsMarkdownEvents(t *testing.T) {
	t.Parallel()

	src, dir := newTestFilesystem(t, false)

	writeFile(t, dir, "note.md", "content")

	select {
	case ev, ok := <-src.Events():
		if !ok {
			t.Fatal("events channel closed unexpectedly")
		}
		if ev.Path != "note.md" {
			t.Errorf("expected corpus-relative event path note.md, got %s", ev.Path)
		}
	case <-time.After(eventTimeout):
		t.Fatal("timed out waiting for markdown event")
	}
}

func TestFilesystemFiltersIrrelevantEvents(t *testing.T) {
	t.Parallel()

	src, dir := newTestFilesystem(t, false)

	writeFile(t, dir, "image.png", binaryContent)
	writeFile(t, dir, "after.md", "content")

	select {
	case ev, ok := <-src.Events():
		if !ok {
			t.Fatal("events channel closed unexpectedly")
		}
		if filepath.Ext(ev.Path) == ".png" {
			t.Errorf("expected non-markdown file event to be filtered, got %s", ev.Path)
		}
	case <-time.After(eventTimeout):
		t.Fatal("timed out waiting for event")
	}
}

func TestFilesystemGitignoreToggle(t *testing.T) {
	t.Parallel()

	src, _ := newTestFilesystem(t, true)

	if !src.RespectingGitignore() {
		t.Fatal("expected initial respect-gitignore true")
	}
	if got := src.ToggleGitignore(); got {
		t.Error("expected toggle to return false")
	}
	if src.RespectingGitignore() {
		t.Error("expected respect-gitignore false after toggle")
	}
	if got := src.ToggleGitignore(); !got {
		t.Error("expected second toggle to return true")
	}
}

func TestFilesystemCloseIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src, err := NewFilesystem(dir, false, nil)
	if err != nil {
		t.Fatalf("NewFilesystem failed: %v", err)
	}

	if err := src.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := src.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}

	if _, ok := <-src.Events(); ok {
		t.Error("expected events channel to be closed")
	}
	if _, ok := <-src.Errors(); ok {
		t.Error("expected errors channel to be closed")
	}
}

func TestFilesystemNonexistentRoot(t *testing.T) {
	t.Parallel()

	if _, err := NewFilesystem(filepath.Join(t.TempDir(), "missing"), false, nil); err == nil {
		t.Fatal("expected error for nonexistent root")
	}
}
