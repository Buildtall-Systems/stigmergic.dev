package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/fsnotify/fsnotify"
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
	watcher       *fsnotify.Watcher
	Events        chan Event
	Errors        chan error
	done          chan struct{}
	debounceMap   map[string]*time.Timer
	debounceMutex sync.Mutex
	watchedDirs   map[string]bool
	watchMutex    sync.RWMutex
}

const debounceWindow = 200 * time.Millisecond

func NewWatcher() (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	logger.Log.Info("watcher created")

	w := &Watcher{
		watcher:     fw,
		Events:      make(chan Event, 100),
		Errors:      make(chan error, 10),
		done:        make(chan struct{}),
		debounceMap: make(map[string]*time.Timer),
		watchedDirs: make(map[string]bool),
	}

	go w.eventLoop()

	return w, nil
}

func (w *Watcher) Add(path string) error {
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

	return w.addRecursive(absPath)
}

func (w *Watcher) addRecursive(path string) error {
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
	close(w.done)

	w.debounceMutex.Lock()
	for _, timer := range w.debounceMap {
		timer.Stop()
	}
	w.debounceMutex.Unlock()

	return w.watcher.Close()
}

func (w *Watcher) eventLoop() {
	logger.Log.Debug("event loop started")
	for {
		select {
		case <-w.done:
			logger.Log.Debug("event loop stopping")
			close(w.Events)
			close(w.Errors)
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
			w.Errors <- err
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
			w.addRecursive(event.Name)
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

	w.debounceMap[key] = time.AfterFunc(debounceWindow, func() {
		logger.Log.Info("sending debounced event", "type", event.Type, "path", event.Path)
		w.Events <- event

		w.debounceMutex.Lock()
		delete(w.debounceMap, key)
		w.debounceMutex.Unlock()
	})
}

func (w *Watcher) convertEvent(event fsnotify.Event) Event {
	var eventType EventType

	if event.Has(fsnotify.Create) {
		eventType = EventCreate
	} else if event.Has(fsnotify.Write) {
		eventType = EventWrite
	} else if event.Has(fsnotify.Remove) {
		eventType = EventRemove
	} else if event.Has(fsnotify.Rename) {
		eventType = EventRename
	} else if event.Has(fsnotify.Chmod) {
		eventType = EventChmod
	}

	return Event{
		Type: eventType,
		Path: event.Name,
	}
}
