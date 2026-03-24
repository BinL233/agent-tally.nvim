//go:build linux

package watcher

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/config"
)

// LinuxWatcher uses fsnotify (inotify) with recursive directory walking.
type LinuxWatcher struct {
	watcher *fsnotify.Watcher
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewPlatformWatcher creates a new Linux file-system watcher.
func NewPlatformWatcher() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &LinuxWatcher{
		watcher: w,
		ctx:     ctx,
		cancel:  cancel,
	}, nil
}

// Start begins monitoring paths for write events. It blocks until stopped.
func (l *LinuxWatcher) Start(cfg *config.Config, events chan<- Event) error {
	excludeSet := make(map[string]bool, len(cfg.ExcludeDirs))

	for _, dir := range cfg.ExcludeDirs {
		excludeSet[dir] = true
	}

	for _, root := range cfg.WatchPaths {
		count := addRecursive(l.watcher, root, excludeSet, cfg.MaxDepth)
		log.Printf("watching %s (%d dirs)", root, count)
	}

	for {
		select {
		case <-l.ctx.Done():
			return l.ctx.Err()

		case ev, ok := <-l.watcher.Events:
			if !ok {
				return nil
			}

			if ev.Op&fsnotify.Create != 0 {
				info, err := os.Stat(ev.Name)

				if err == nil && info.IsDir() {
					base := filepath.Base(ev.Name)

					if !excludeSet[base] {
						addRecursive(l.watcher, ev.Name, excludeSet, cfg.MaxDepth)
					}

					continue
				}
			}

			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

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
	l.cancel()
	return l.watcher.Close()
}

// AddPath adds a new root directory and its subdirectories to the active watcher.
func (l *LinuxWatcher) AddPath(path string, cfg *config.Config) error {
	excludeSet := make(map[string]bool, len(cfg.ExcludeDirs))
	for _, dir := range cfg.ExcludeDirs {
		excludeSet[dir] = true
	}
	count := addRecursive(l.watcher, path, excludeSet, cfg.MaxDepth)
	log.Printf("watch-add %s (%d dirs)", path, count)
	return nil
}

// RemovePath removes a root directory and all its subdirectories from the active watcher.
func (l *LinuxWatcher) RemovePath(path string) error {
	filepath.WalkDir(path, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}
		l.watcher.Remove(p)
		return nil
	})
	log.Printf("watch-remove %s", path)
	return nil
}
