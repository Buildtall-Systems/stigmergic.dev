package watcher

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	gitignore "github.com/sabhiram/go-gitignore"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
)

type EventType int

const (
	EventCreate EventType = iota
	EventWrite
	EventRemove
	EventRename
	EventChmod
)

type Event struct {
	Type EventType
	Path string
}

type Watcher struct {
	watcher        *fsnotify.Watcher
	Events         chan Event
	Errors         chan error
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	debounceMap    map[string]*time.Timer
	debounceMutex  sync.Mutex
	watchedDirs    map[string]bool
	watchMutex     sync.RWMutex
	gitignore      *gitignore.GitIgnore
	rootPath       string
	debounceWindow time.Duration
}

const DefaultDebounceWindow = 200 * time.Millisecond

func NewWatcher() (*Watcher, error) {
	return NewWatcherWithDebounce(DefaultDebounceWindow)
}

func NewWatcherWithDebounce(debounceWindow time.Duration) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	logger.Log.Info("watcher created")

	ctx, cancel := context.WithCancel(context.Background())

	w := &Watcher{
		watcher:        fw,
		Events:         make(chan Event, 100),
		Errors:         make(chan error, 10),
		ctx:            ctx,
		cancel:         cancel,
		debounceMap:    make(map[string]*time.Timer),
		watchedDirs:    make(map[string]bool),
		debounceWindow: debounceWindow,
	}

	w.wg.Add(1)
	go w.eventLoop()

	return w, nil
}

func (w *Watcher) Add(path string, respectGitignore bool, ignorePatterns []string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	if w.rootPath == "" {
		w.rootPath = absPath

		var allPatterns []string
		allPatterns = append(allPatterns, ignorePatterns...)

		if respectGitignore {
			gitignorePath := filepath.Join(absPath, ".gitignore")
			if file, err := os.Open(gitignorePath); err == nil { //nolint:gosec
				defer func() { _ = file.Close() }()
				scanner := bufio.NewScanner(file)
				lineCount := 0
				for scanner.Scan() {
					line := scanner.Text()
					if line != "" && line[0] != '#' {
						allPatterns = append(allPatterns, line)
						lineCount++
					}
				}
				logger.Log.Info("watcher loaded .gitignore", "path", gitignorePath, "patterns", lineCount)
			}
		}

		if len(allPatterns) > 0 {
			w.gitignore = gitignore.CompileIgnoreLines(allPatterns...)
			logger.Log.Info("watcher compiled ignore patterns", "total_count", len(allPatterns))
		}
	}

	return w.addRecursive(absPath)
}

func (w *Watcher) addRecursive(path string) error {
	relPath, _ := filepath.Rel(w.rootPath, path)
	if w.gitignore != nil && w.gitignore.MatchesPath(relPath) {
		logger.Log.Debug("skipping ignored path", "path", relPath)
		return nil
	}

	w.watchMutex.Lock()
	if w.watchedDirs[path] {
		w.watchMutex.Unlock()
		logger.Log.Debug("path already watched", "path", path)
		return nil
	}
	w.watchedDirs[path] = true
	w.watchMutex.Unlock()

	if err := w.watcher.Add(path); err != nil {
		w.watchMutex.Lock()
		delete(w.watchedDirs, path)
		w.watchMutex.Unlock()
		logger.Log.Error("failed to add watch", "path", path, "error", err)
		return fmt.Errorf("failed to watch path: %w", err)
	}

	logger.Log.Info("watching path", "path", path)

	entries, err := os.ReadDir(path)
	if err != nil {
		logger.Log.Error("failed to read directory", "path", path, "error", err)
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			childPath := filepath.Join(path, entry.Name())
			if err := w.addRecursive(childPath); err != nil {
				logger.Log.Warn("failed to watch subdirectory", "path", childPath, "error", err)
				continue
			}
		}
	}

	return nil
}

func (w *Watcher) Remove(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	w.watchMutex.Lock()
	delete(w.watchedDirs, absPath)
	w.watchMutex.Unlock()

	if err := w.watcher.Remove(absPath); err != nil {
		return fmt.Errorf("failed to unwatch path: %w", err)
	}

	return nil
}

func (w *Watcher) Close() error {
	w.cancel()

	w.debounceMutex.Lock()
	for _, timer := range w.debounceMap {
		timer.Stop()
	}
	w.debounceMutex.Unlock()

	w.wg.Wait()

	err := w.watcher.Close()

	close(w.Events)
	close(w.Errors)

	return err
}

func (w *Watcher) eventLoop() {
	logger.Log.Debug("event loop started")
	defer func() {
		w.wg.Done()
		logger.Log.Debug("event loop stopped")
	}()

	for {
		select {
		case <-w.ctx.Done():
			logger.Log.Debug("event loop stopping")
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				logger.Log.Debug("watcher events channel closed")
				return
			}
			logger.Log.Debug("received fsnotify event", "event", event.String())
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				logger.Log.Debug("watcher errors channel closed")
				return
			}
			logger.Log.Error("watcher error from fsnotify", "error", err)
			select {
			case w.Errors <- err:
			case <-w.ctx.Done():
				return
			}
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	convertedEvent := w.convertEvent(event)
	logger.Log.Info("handling event", "type", convertedEvent.Type, "path", convertedEvent.Path)

	if convertedEvent.Type == EventCreate {
		info, err := os.Stat(event.Name)
		if err == nil && info.IsDir() {
			logger.Log.Info("new directory created, adding watch", "path", event.Name)
			if err := w.addRecursive(event.Name); err != nil {
				logger.Log.Error("failed to add watch for new directory", "path", event.Name, "error", err)
			}
		}
	}

	w.debounceEvent(convertedEvent)
}

func (w *Watcher) debounceEvent(event Event) {
	w.debounceMutex.Lock()
	defer w.debounceMutex.Unlock()

	key := fmt.Sprintf("%d:%s", event.Type, event.Path)

	if timer, exists := w.debounceMap[key]; exists {
		logger.Log.Debug("debouncing repeated event", "key", key)
		timer.Stop()
	}

	w.debounceMap[key] = time.AfterFunc(w.debounceWindow, func() {
		logger.Log.Info("sending debounced event", "type", event.Type, "path", event.Path)
		select {
		case w.Events <- event:
		case <-w.ctx.Done():
			logger.Log.Debug("watcher closed, discarding debounced event")
		}

		w.debounceMutex.Lock()
		delete(w.debounceMap, key)
		w.debounceMutex.Unlock()
	})
}

func (w *Watcher) convertEvent(event fsnotify.Event) Event {
	var eventType EventType

	if event.Has(fsnotify.Create) {
		eventType = EventCreate
	} else if event.Has(fsnotify.Remove) {
		eventType = EventRemove
	} else if event.Has(fsnotify.Rename) {
		eventType = EventRename
	} else if event.Has(fsnotify.Write) {
		_, err := os.Stat(event.Name)
		if err == nil {
			eventType = EventWrite
		} else if os.IsNotExist(err) {
			eventType = EventCreate
		} else {
			eventType = EventWrite
		}
	} else if event.Has(fsnotify.Chmod) {
		eventType = EventChmod
	}

	return Event{
		Type: eventType,
		Path: event.Name,
	}
}
