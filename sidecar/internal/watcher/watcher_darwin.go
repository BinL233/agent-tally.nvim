//go:build darwin

package watcher

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DarwinWatcher uses fsnotify (kqueue) for file-write detection on macOS.
type DarwinWatcher struct {
	watcher *fsnotify.Watcher
}

// NewPlatformWatcher creates a new macOS file-system watcher.
func NewPlatformWatcher() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	return &DarwinWatcher{watcher: w}, nil
}

// Start begins monitoring paths for write events. It blocks until ctx is cancelled.
func (d *DarwinWatcher) Start(ctx context.Context, paths []string, events chan<- Event) error {
	for _, p := range paths {
		if err := d.watcher.Add(p); err != nil {
			log.Printf("warn: cannot watch %s: %v", p, err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case ev, ok := <-d.watcher.Events:
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

		case err, ok := <-d.watcher.Errors:
			if !ok {
				return nil
			}

			log.Printf("watcher error: %v", err)
		}
	}
}

// Stop closes the underlying fsnotify watcher.
func (d *DarwinWatcher) Stop() error {
	return d.watcher.Close()
}

// resolveProcess attempts to resolve a PID to its process name on macOS.
func resolveProcess(pid int) string {
	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
