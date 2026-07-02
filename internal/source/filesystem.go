package source

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/watcher"
)

// FilesystemSource serves content from a live directory on the local
// filesystem. It watches for changes, honors .gitignore filtering with a
// runtime toggle, and carries meaningful mod times.
type FilesystemSource struct {
	fsys             fs.FS
	watcher          *watcher.Watcher
	events           chan Event
	errs             chan error
	done             chan struct{}
	root             string
	wg               sync.WaitGroup
	closeOnce        sync.Once
	respectGitignore atomic.Bool
}

var (
	_ ContentSource  = (*FilesystemSource)(nil)
	_ Watchable      = (*FilesystemSource)(nil)
	_ GitignoreAware = (*FilesystemSource)(nil)
	_ Timestamped    = (*FilesystemSource)(nil)
	_ Rooted         = (*FilesystemSource)(nil)
)

// NewFilesystem creates a source over root, an existing directory. The
// returned source owns a filesystem watcher registered on the whole tree.
func NewFilesystem(root string, respectGitignore bool, ignorePatterns []string) (*FilesystemSource, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	w, err := watcher.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	if err := w.Add(absRoot, respectGitignore, ignorePatterns); err != nil {
		if closeErr := w.Close(); closeErr != nil {
			logger.Log.Error("error closing watcher after failed add", "error", closeErr)
		}
		return nil, fmt.Errorf("failed to watch directory: %w", err)
	}

	f := &FilesystemSource{
		root:    absRoot,
		fsys:    os.DirFS(absRoot),
		watcher: w,
		events:  make(chan Event, 100),
		errs:    make(chan error, 10),
		done:    make(chan struct{}),
	}
	f.respectGitignore.Store(respectGitignore)

	f.wg.Add(1)
	go f.pump()

	return f, nil
}

func (f *FilesystemSource) FS() fs.FS {
	return f.fsys
}

func (f *FilesystemSource) Name() string {
	return f.root
}

func (f *FilesystemSource) Root() string {
	return f.root
}

func (f *FilesystemSource) Events() <-chan Event {
	return f.events
}

func (f *FilesystemSource) Errors() <-chan error {
	return f.errs
}

func (f *FilesystemSource) RespectingGitignore() bool {
	return f.respectGitignore.Load()
}

func (f *FilesystemSource) ToggleGitignore() bool {
	for {
		current := f.respectGitignore.Load()
		newVal := !current
		if f.respectGitignore.CompareAndSwap(current, newVal) {
			logger.Log.Info("toggled respect gitignore", "new_value", newVal)
			return newVal
		}
	}
}

// ModTimesMeaningful marks the source as Timestamped.
func (f *FilesystemSource) ModTimesMeaningful() {}

// Close is idempotent: it stops event forwarding, closes the underlying
// watcher, and closes the Events/Errors channels.
func (f *FilesystemSource) Close() error {
	var err error
	f.closeOnce.Do(func() {
		close(f.done)
		err = f.watcher.Close()
		f.wg.Wait()
	})
	return err
}

// pump forwards watcher events to consumers, classifying each so only
// relevant changes (markdown files and directories) pass through.
func (f *FilesystemSource) pump() {
	defer func() {
		close(f.events)
		close(f.errs)
		f.wg.Done()
	}()

	for {
		select {
		case <-f.done:
			return
		case ev, ok := <-f.watcher.Events:
			if !ok {
				return
			}
			if !f.relevant(ev) {
				logger.Log.Debug("ignoring non-markdown file event", "path", ev.Path)
				continue
			}
			select {
			case f.events <- Event{Path: ev.Path}:
			case <-f.done:
				return
			}
		case err, ok := <-f.watcher.Errors:
			if !ok {
				return
			}
			select {
			case f.errs <- err:
			case <-f.done:
				return
			}
		}
	}
}

// relevant reports whether a raw watcher event concerns rendered content:
// an existing directory, an existing markdown file, or a removed markdown
// file.
func (f *FilesystemSource) relevant(ev watcher.Event) bool {
	ext := filepath.Ext(ev.Path)
	info, err := os.Stat(ev.Path)
	if err == nil {
		return info.IsDir() || ext == ".md"
	}
	return ev.Type == watcher.EventRemove && ext == ".md"
}
