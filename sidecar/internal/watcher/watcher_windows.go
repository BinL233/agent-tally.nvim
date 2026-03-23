//go:build windows

package watcher

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WindowsWatcher uses fsnotify (ReadDirectoryChangesW) for file-write detection on Windows.
// Future: replace with ETW (Microsoft-Windows-Kernel-File provider) for PID-level attribution.
type WindowsWatcher struct {
	watcher *fsnotify.Watcher
}

// NewPlatformWatcher creates a new Windows file-system watcher.
func NewPlatformWatcher() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	return &WindowsWatcher{watcher: w}, nil
}

// Start begins monitoring paths for write events. It blocks until ctx is cancelled.
func (w *WindowsWatcher) Start(ctx context.Context, paths []string, events chan<- Event) error {
	for _, p := range paths {
		if err := w.watcher.Add(p); err != nil {
			log.Printf("warn: cannot watch %s: %v", p, err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case ev, ok := <-w.watcher.Events:
			if !ok {
				return nil
			}

			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			events <- Event{
				Timestamp: time.Now(),
				FilePath:  ev.Name,
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return nil
			}

			log.Printf("watcher error: %v", err)
		}
	}
}

// Stop closes the underlying fsnotify watcher.
func (w *WindowsWatcher) Stop() error {
	return w.watcher.Close()
}
