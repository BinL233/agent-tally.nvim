package logparse

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"time"
)

// ClaudeParser parses Claude Code JSONL session log files.
// buf is allocated once and reused across ParseFrom calls to reduce GC pressure.
type ClaudeParser struct {
	buf []byte
}

// NewClaudeParser creates a ClaudeParser with a reusable 4 MiB line buffer.
func NewClaudeParser() *ClaudeParser {
	return &ClaudeParser{buf: make([]byte, 4*1024*1024)}
}

// claudeLine is the minimal shape of a Claude JSONL entry we care about.
type claudeLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	CWD       string `json:"cwd"`
	Message   struct {
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"` // tool name when type == "tool_use"
			ID   string `json:"id"`   // tool call ID
		} `json:"content"`
		StopReason *string `json:"stop_reason"` // nil = streaming start; non-nil = final entry
		Usage      struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ParseFrom reads the file at path starting from byteOffset, extracts tool_use
// events and per-request token usage, and returns the new offset to resume from.
func (p *ClaudeParser) ParseFrom(path string, byteOffset int64) (ParseResult, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return ParseResult{}, byteOffset, err
	}
	defer f.Close()

	if byteOffset > 0 {
		if _, err := f.Seek(byteOffset, io.SeekStart); err != nil {
			return ParseResult{}, byteOffset, err
		}
	}

	var result ParseResult
	scanner := bufio.NewScanner(f)
	scanner.Buffer(p.buf, len(p.buf))

	pos := byteOffset

	for scanner.Scan() {
		raw := scanner.Bytes()
		lineLen := int64(len(raw)) + 1 // +1 for newline

		var entry claudeLine
		if err := json.Unmarshal(raw, &entry); err != nil {
			pos += lineLen
			continue
		}

		if entry.Type == "assistant" {
			ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if ts.IsZero() {
				ts = time.Now()
			}

			// Tool events: extract from all assistant entries (tool_use blocks
			// appear in the final entry, but we check all to be safe).
			for _, c := range entry.Message.Content {
				if c.Type == "tool_use" && c.ID != "" && c.Name != "" {
					result.ToolEvents = append(result.ToolEvents, ToolEvent{
						Timestamp:  ts,
						Agent:      "claude",
						SessionID:  entry.SessionID,
						ToolName:   c.Name,
						ToolCallID: c.ID,
						CWD:        entry.CWD,
					})
				}
			}

			// Token usage: only on the final entry for each API call.
			// stop_reason is null (nil) on streaming-start entries and non-nil on
			// the completed response. requestId ties multiple entries together.
			if entry.Message.StopReason != nil && entry.RequestID != "" {
				u := entry.Message.Usage
				result.TokenUsages = append(result.TokenUsages, TokenUsage{
					Timestamp: ts,
					Agent:     "claude",
					SessionID: entry.SessionID,
					RequestID: entry.RequestID,
					TokensIn:  u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens,
					TokensOut: u.OutputTokens,
					CWD:       entry.CWD,
				})
			}
		}

		pos += lineLen
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return result, pos, err
	}

	return result, pos, nil
}
