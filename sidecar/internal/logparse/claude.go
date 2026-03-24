package logparse

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"time"
)

// ClaudeParser parses Claude Code JSONL session log files.
type ClaudeParser struct{}

// claudeLine is the minimal shape of a Claude JSONL entry we care about.
type claudeLine struct {
	Type      string    `json:"type"`
	Timestamp string    `json:"timestamp"`
	SessionID string    `json:"sessionId"`
	CWD       string    `json:"cwd"`
	Message   struct {
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"` // tool name when type == "tool_use"
			ID   string `json:"id"`   // tool call ID
		} `json:"content"`
	} `json:"message"`
}

// ParseFrom reads the file at path starting from byteOffset, extracts tool_use
// entries from assistant messages, and returns the new offset to resume from.
func (p *ClaudeParser) ParseFrom(path string, byteOffset int64) ([]ToolEvent, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, byteOffset, err
	}
	defer f.Close()

	if byteOffset > 0 {
		if _, err := f.Seek(byteOffset, io.SeekStart); err != nil {
			return nil, byteOffset, err
		}
	}

	var events []ToolEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4 MiB per line

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

			for _, c := range entry.Message.Content {
				if c.Type == "tool_use" && c.ID != "" && c.Name != "" {
					events = append(events, ToolEvent{
						Timestamp:  ts,
						Agent:      "claude",
						SessionID:  entry.SessionID,
						ToolName:   c.Name,
						ToolCallID: c.ID,
						CWD:        entry.CWD,
					})
				}
			}
		}

		pos += lineLen
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return events, pos, err
	}

	return events, pos, nil
}
