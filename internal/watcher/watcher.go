package watcher

import (
	"fmt"
	"path/filepath"

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
	watcher *fsnotify.Watcher
	Events  chan Event
	Errors  chan error
	done    chan struct{}
}

func NewWatcher() (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	w := &Watcher{
		watcher: fw,
		Events:  make(chan Event, 100),
		Errors:  make(chan error, 10),
		done:    make(chan struct{}),
	}

	go w.eventLoop()

	return w, nil
}

func (w *Watcher) Add(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	if err := w.watcher.Add(absPath); err != nil {
		return fmt.Errorf("failed to watch path: %w", err)
	}

	return nil
}

func (w *Watcher) Remove(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	if err := w.watcher.Remove(absPath); err != nil {
		return fmt.Errorf("failed to unwatch path: %w", err)
	}

	return nil
}

func (w *Watcher) Close() error {
	close(w.done)
	return w.watcher.Close()
}

func (w *Watcher) eventLoop() {
	for {
		select {
		case <-w.done:
			close(w.Events)
			close(w.Errors)
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.Events <- w.convertEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.Errors <- err
		}
	}
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
