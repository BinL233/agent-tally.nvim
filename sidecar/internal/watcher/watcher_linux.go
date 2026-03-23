//go:build linux

package watcher

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fsnotify/fsnotify"
)

// LinuxWatcher uses fsnotify (inotify) for file-write detection on Linux.
// Future: replace with eBPF kprobes on vfs_write for PID-level attribution.
type LinuxWatcher struct {
	watcher *fsnotify.Watcher
}

// NewPlatformWatcher creates a new Linux file-system watcher.
func NewPlatformWatcher() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	return &LinuxWatcher{watcher: w}, nil
}

// Start begins monitoring paths for write events. It blocks until ctx is cancelled.
func (l *LinuxWatcher) Start(ctx context.Context, paths []string, events chan<- Event) error {
	for _, p := range paths {
		if err := l.watcher.Add(p); err != nil {
			log.Printf("warn: cannot watch %s: %v", p, err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case ev, ok := <-l.watcher.Events:
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

		case err, ok := <-l.watcher.Errors:
			if !ok {
				return nil
			}

			log.Printf("watcher error: %v", err)
		}
	}
}

// Stop closes the underlying fsnotify watcher.
func (l *LinuxWatcher) Stop() error {
	return l.watcher.Close()
}
