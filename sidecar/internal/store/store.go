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

// ToolEvent represents a single tool/skill invocation by an AI CLI agent.
type ToolEvent struct {
	ID         int64     `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Agent      string    `json:"agent"`
	SessionID  string    `json:"session_id"`
	ToolName   string    `json:"tool_name"`
	ToolCallID string    `json:"tool_call_id"`
	CWD        string    `json:"cwd"`
}

// ToolSummary holds aggregated skill-use data for dashboard display.
type ToolSummary struct {
	ToolName string `json:"tool_name"`
	Count    int    `json:"count"`
	Agent    string `json:"agent"`
}

// ToolFilter controls which skill events are returned.
type ToolFilter struct {
	Agent     string     `json:"agent"`
	CWDPrefix string     `json:"cwd_prefix"`
	Since     *time.Time `json:"since"`
	Limit     int        `json:"limit"`
}

// TokenUsage holds actual API token consumption for a single request.
type TokenUsage struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Agent     string    `json:"agent"`
	SessionID string    `json:"session_id"`
	RequestID string    `json:"request_id"` // unique per API call — used for dedup
	TokensIn  int       `json:"tokens_in"`  // input + cache_creation + cache_read
	TokensOut int       `json:"tokens_out"` // output (includes thinking)
	CWD       string    `json:"cwd"`
}

// TokenSummary holds aggregated token data for one agent, scoped to a project.
type TokenSummary struct {
	Agent     string `json:"agent"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
}

// TokenFilter controls which token_usage rows are returned.
type TokenFilter struct {
	Agent     string     `json:"agent"`
	CWDPrefix string     `json:"cwd_prefix"`
	Since     *time.Time `json:"since"`
	Limit     int        `json:"limit"`
}

// DaySummary holds aggregated token data for a single calendar day.
type DaySummary struct {
	Day       string `json:"day"`        // "2025-04-15"
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
}

// Store is the interface for event persistence.
type Store interface {
	// Init creates tables and indexes if they don't exist.
	Init(ctx context.Context) error

	// InsertEvent records a new event.
	InsertEvent(ctx context.Context, e *Event) error

	// BatchInsertEvents records multiple events in a single transaction.
	BatchInsertEvents(ctx context.Context, events []*Event) error

	// Query returns events matching the filter.
	Query(ctx context.Context, filter QueryFilter) ([]Event, error)

	// QueryByFile returns token totals grouped by file.
	QueryByFile(ctx context.Context, filter QueryFilter) ([]FileTokenSummary, error)

	// QueryByDay returns token totals grouped by calendar day.
	QueryByDay(ctx context.Context, filter QueryFilter) ([]DaySummary, error)

	// ClearAll deletes all events from the store.
	ClearAll(ctx context.Context) error

	// ClearByPath deletes all events whose file_path falls under pathPrefix.
	ClearByPath(ctx context.Context, pathPrefix string) error

	// InsertToolEvent records a tool invocation. Silently ignores duplicates.
	InsertToolEvent(ctx context.Context, e *ToolEvent) error

	// BatchInsertToolEvents records multiple tool events in a single transaction, ignoring duplicates.
	BatchInsertToolEvents(ctx context.Context, events []*ToolEvent) error

	// QueryTools returns aggregated tool-use counts matching the filter.
	QueryTools(ctx context.Context, filter ToolFilter) ([]ToolSummary, error)

	// BatchInsertTokenUsages records multiple token usage records, replacing duplicates by request_id.
	BatchInsertTokenUsages(ctx context.Context, usages []*TokenUsage) error

	// QueryTokenSummary returns per-agent token totals matching the filter.
	QueryTokenSummary(ctx context.Context, filter TokenFilter) ([]TokenSummary, error)

	// QueryTokenByDay returns actual API token totals grouped by calendar day.
	QueryTokenByDay(ctx context.Context, filter TokenFilter) ([]DaySummary, error)

	// GetLogOffset returns the byte offset of the last-processed position in a log file.
	GetLogOffset(ctx context.Context, logPath string) (int64, error)

	// SetLogOffset saves the byte offset for a log file.
	SetLogOffset(ctx context.Context, logPath string, offset int64) error

	// Close releases database resources.
	Close() error
}
