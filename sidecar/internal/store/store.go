package store

import (
	"context"
	"time"
)

// Event represents a recorded file-write event with token attribution.
type Event struct {
	ID           int64
	Timestamp    time.Time
	PID          int
	ProcessName  string
	FilePath     string
	TokensInput  int
	TokensOutput int
	MCPSkillUsed string
}

// QueryFilter controls which events are returned by Query.
type QueryFilter struct {
	ProcessName string
	Since       *time.Time
	Limit       int
}

// Store is the interface for event persistence.
type Store interface {
	// Init creates tables and indexes if they don't exist.
	Init(ctx context.Context) error

	// InsertEvent records a new event.
	InsertEvent(ctx context.Context, e *Event) error

	// Query returns events matching the filter.
	Query(ctx context.Context, filter QueryFilter) ([]Event, error)

	// Close releases database resources.
	Close() error
}
