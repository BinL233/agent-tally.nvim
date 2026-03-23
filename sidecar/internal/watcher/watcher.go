package watcher

import (
	"context"
	"time"
)

// Event represents a detected file-write event.
type Event struct {
	Timestamp time.Time
	PID       int
	Process   string
	FilePath  string
}

// Watcher monitors file-system write events and emits them to a channel.
type Watcher interface {
	// Start begins monitoring the given paths and sends events to the channel.
	// It blocks until the context is cancelled or an error occurs.
	Start(ctx context.Context, paths []string, events chan<- Event) error

	// Stop terminates monitoring and releases resources.
	Stop() error
}
