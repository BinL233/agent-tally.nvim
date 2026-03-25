// Package logparse extracts tool-use events from AI CLI agent log files.
package logparse

import "time"

// ToolEvent represents a single tool invocation extracted from a log file.
type ToolEvent struct {
	Timestamp  time.Time
	Agent      string // "claude" or "copilot"
	SessionID  string
	ToolName   string
	ToolCallID string // unique ID — used for deduplication
	CWD        string
}

// TokenUsage holds actual API token consumption for a single request.
// tokens_in = input + cache_creation + cache_read; tokens_out = output (incl. thinking).
type TokenUsage struct {
	Timestamp time.Time
	Agent     string
	SessionID string
	RequestID string // dedup key (Claude: requestId; Copilot: sessionId+":"+model)
	TokensIn  int
	TokensOut int
	CWD       string
}

// ParseResult holds all events extracted from a single log file pass.
type ParseResult struct {
	ToolEvents  []ToolEvent
	TokenUsages []TokenUsage
}

// Parser reads a JSONL log file from a given byte offset and returns new
// tool events, token usage records, and the new offset to resume from on the next call.
type Parser interface {
	ParseFrom(path string, byteOffset int64) (ParseResult, int64, error)
}
