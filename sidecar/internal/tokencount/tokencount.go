package tokencount

import (
	"os"
	"sync"
)

// Estimator tracks file snapshots and estimates token deltas on writes.
// It keeps in-memory snapshots of file sizes to compute deltas.
type Estimator struct {
	mu        sync.Mutex
	snapshots map[string]int64 // file path -> last known size in bytes
}

// NewEstimator creates a new token estimator.
func NewEstimator() *Estimator {
	return &Estimator{
		snapshots: make(map[string]int64),
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

	e.mu.Lock()
	prevSize, known := e.snapshots[filePath]
	e.snapshots[filePath] = currentSize
	e.mu.Unlock()

	if !known {
		// First time seeing this file — all content is new output, no input.
		return Delta{
			TokensOutput: bytesToTokens(currentSize),
		}
	}

	diff := currentSize - prevSize
	if diff <= 0 {
		// File shrank or unchanged — no new tokens to count.
		return Delta{}
	}

	// The AI read the existing file (input) then wrote new content (output).
	return Delta{
		TokensInput:  bytesToTokens(prevSize),
		TokensOutput: bytesToTokens(diff),
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
