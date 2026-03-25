package tokencount

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// snapshot holds the last known file size and when it was last accessed.
type snapshot struct {
	size     int64
	lastSeen time.Time
}

const (
	snapshotTTL  = 30 * time.Minute
	maxSnapshots = 50_000
)

// Estimator tracks file snapshots and estimates token deltas on writes.
// It keeps in-memory snapshots of file sizes to compute deltas.
type Estimator struct {
	mu        sync.Mutex
	snapshots map[string]snapshot
}

// NewEstimator creates a new token estimator.
func NewEstimator() *Estimator {
	return &Estimator{
		snapshots: make(map[string]snapshot),
	}
}

// Delta holds the estimated token counts for a file change.
type Delta struct {
	TokensInput  int // tokens the AI likely read (previous file content)
	TokensOutput int // tokens added (new content)
}

// Estimate computes the token delta for a file write.
// It compares the current file size to the last known snapshot.
// New files or grown files produce output tokens.
func (e *Estimator) Estimate(filePath string) Delta {
	info, err := os.Stat(filePath)
	if err != nil {
		return Delta{}
	}

	currentSize := info.Size()
	now := time.Now()

	e.mu.Lock()
	snap, known := e.snapshots[filePath]
	e.snapshots[filePath] = snapshot{size: currentSize, lastSeen: now}
	e.mu.Unlock()

	if !known {
		// First time seeing this file — all content is new output, no input.
		return Delta{
			TokensOutput: bytesToTokens(currentSize),
		}
	}

	diff := currentSize - snap.size
	if diff <= 0 {
		// File shrank or unchanged — no new tokens to count.
		return Delta{}
	}

	// The AI read the existing file (input) then wrote new content (output).
	return Delta{
		TokensInput:  bytesToTokens(snap.size),
		TokensOutput: bytesToTokens(diff),
	}
}

// SnapshotDir walks dir, skipping excludeDirs, and records the current size of
// every file so that the first write to each file produces a non-zero TokensInput.
// Safe to call before or after the watcher starts — it never overwrites an existing snapshot.
func (e *Estimator) SnapshotDir(dir string, excludeDirs map[string]bool) {
	now := time.Now()
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if excludeDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		e.mu.Lock()
		if _, exists := e.snapshots[path]; !exists {
			e.snapshots[path] = snapshot{size: info.Size(), lastSeen: now}
		}
		e.mu.Unlock()

		return nil
	})
}

// Prune removes entries that have not been accessed within snapshotTTL, and
// enforces a maximum map size by evicting the oldest half when over the cap.
func (e *Estimator) Prune() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()

	for path, snap := range e.snapshots {
		if now.Sub(snap.lastSeen) > snapshotTTL {
			delete(e.snapshots, path)
		}
	}

	if len(e.snapshots) > maxSnapshots {
		type kv struct {
			path     string
			lastSeen time.Time
		}
		entries := make([]kv, 0, len(e.snapshots))
		for p, s := range e.snapshots {
			entries = append(entries, kv{p, s.lastSeen})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].lastSeen.Before(entries[j].lastSeen)
		})
		for i := 0; i < len(entries)/2; i++ {
			delete(e.snapshots, entries[i].path)
		}
	}
}

// StartPruning runs periodic pruning until ctx is cancelled. Call in a goroutine.
func (e *Estimator) StartPruning(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.Prune()
		}
	}
}

// bytesToTokens converts a byte count to estimated tokens (~4 bytes per token).
func bytesToTokens(n int64) int {
	if n <= 0 {
		return 0
	}

	tokens := float64(n) / 4.0

	if tokens < 1 {
		return 1
	}

	return int(tokens)
}
