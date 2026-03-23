package store

import (
	"context"
	"time"
)

// Event represents a recorded file-write event with token attribution.
type Event struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	PID          int       `json:"pid"`
	ProcessName  string    `json:"process_name"`
	FilePath     string    `json:"file_path"`
	TokensInput  int       `json:"tokens_input"`
	TokensOutput int       `json:"tokens_output"`
}

// QueryFilter controls which events are returned by Query.
type QueryFilter struct {
	ProcessName string     `json:"process_name"`
	PathPrefix  string     `json:"path_prefix"`
	Since       *time.Time `json:"since"`
	Limit       int        `json:"limit"`
}

// FileTokenSummary holds aggregated token data for a single file.
type FileTokenSummary struct {
	FilePath     string `json:"file_path"`
	TokensOutput int    `json:"tokens_output"`
	EventCount   int    `json:"event_count"`
}

// Store is the interface for event persistence.
type Store interface {
	// Init creates tables and indexes if they don't exist.
	Init(ctx context.Context) error

	// InsertEvent records a new event.
	InsertEvent(ctx context.Context, e *Event) error

	// Query returns events matching the filter.
	Query(ctx context.Context, filter QueryFilter) ([]Event, error)

	// QueryByFile returns token totals grouped by file.
	QueryByFile(ctx context.Context, filter QueryFilter) ([]FileTokenSummary, error)

	// ClearAll deletes all events from the store.
	ClearAll(ctx context.Context) error

	// ClearByPath deletes all events whose file_path falls under pathPrefix.
	ClearByPath(ctx context.Context, pathPrefix string) error

	// Close releases database resources.
	Close() error
}
