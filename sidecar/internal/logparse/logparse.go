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

// Parser reads a JSONL log file from a given byte offset and returns new
// tool events and the new offset to resume from on the next call.
type Parser interface {
	ParseFrom(path string, byteOffset int64) (events []ToolEvent, newOffset int64, err error)
}
