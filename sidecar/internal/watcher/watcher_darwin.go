//go:build darwin

package watcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/config"
)

// DarwinWatcher uses fsnotify (kqueue) with recursive directory walking.
type DarwinWatcher struct {
	watcher *fsnotify.Watcher
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewPlatformWatcher creates a new macOS file-system watcher.
func NewPlatformWatcher() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DarwinWatcher{
		watcher: w,
		ctx:     ctx,
		cancel:  cancel,
	}, nil
}

// Start begins monitoring paths for write events. It blocks until stopped.
func (d *DarwinWatcher) Start(cfg *config.Config, events chan<- Event) error {
	excludeSet := make(map[string]bool, len(cfg.ExcludeDirs))

	for _, dir := range cfg.ExcludeDirs {
		excludeSet[dir] = true
	}

	// Recursively add all subdirectories.
	totalDirs := 0

	for _, root := range cfg.WatchPaths {
		count := addRecursive(d.watcher, root, excludeSet, cfg.MaxDepth)
		totalDirs += count
		log.Printf("watching %s (%d dirs)", root, count)
	}

	log.Printf("total directories watched: %d", totalDirs)

	for {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()

		case ev, ok := <-d.watcher.Events:
			if !ok {
				return nil
			}

			// When a new directory is created, start watching it too.
			if ev.Op&fsnotify.Create != 0 {
				info, err := os.Stat(ev.Name)

				if err == nil && info.IsDir() {
					base := filepath.Base(ev.Name)

					if !excludeSet[base] {
						addRecursive(d.watcher, ev.Name, excludeSet, cfg.MaxDepth)
					}

					continue
				}
			}

			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			// Skip directories and hidden files.
			if strings.HasPrefix(filepath.Base(ev.Name), ".") {
				continue
			}

			info, err := os.Stat(ev.Name)
			if err != nil || info.IsDir() {
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
	d.cancel()
	return d.watcher.Close()
}

