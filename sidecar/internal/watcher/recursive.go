package watcher

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// addRecursive walks a directory tree and adds each directory to the watcher.
// Skips hidden directories, excluded basenames, and respects maxDepth.
func addRecursive(w *fsnotify.Watcher, root string, excludes map[string]bool, maxDepth int) int {
	count := 0
	rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))

	filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}

		base := filepath.Base(path)

		if path != root && strings.HasPrefix(base, ".") {
			return fs.SkipDir
		}

		if excludes[base] {
			return fs.SkipDir
		}

		if maxDepth > 0 {
			depth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - rootDepth

			if depth > maxDepth {
				return fs.SkipDir
			}
		}

		if err := w.Add(path); err != nil {
			return nil
		}

		count++
		return nil
	})

	return count
}
