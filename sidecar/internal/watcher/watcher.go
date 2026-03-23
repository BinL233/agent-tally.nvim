package watcher

import (
	"time"

	"github.com/kaileying/agent-tally.nvim/sidecar/internal/config"
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
	// Start begins monitoring and sends events to the channel.
	// It blocks until the context is cancelled or an error occurs.
	Start(cfg *config.Config, events chan<- Event) error

	// Stop terminates monitoring and releases resources.
	Stop() error
}
