package logparse

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

// CopilotParser parses Copilot CLI event log files.
type CopilotParser struct {
	// cwd is pre-resolved via DiscoverCopilot so we don't re-read workspace.yaml.
	cwd string
}

// NewCopilotParser creates a parser for a specific session cwd.
func NewCopilotParser(cwd string) *CopilotParser {
	return &CopilotParser{cwd: cwd}
}

// copilotLine is the minimal shape of a Copilot events.jsonl entry.
type copilotLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Data      struct {
		ToolCallID string `json:"toolCallId"`
		ToolName   string `json:"toolName"`
		SessionID  string `json:"sessionId"`
	} `json:"data"`
}

// titleCase converts a lowercase tool name (e.g. "bash") to TitleCase ("Bash").
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ParseFrom reads the Copilot events.jsonl from byteOffset and extracts
// tool.execution_start events.
func (p *CopilotParser) ParseFrom(path string, byteOffset int64) ([]ToolEvent, int64, error) {
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
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Extract session ID from the filename's parent dir name.
	sessionID := sessionIDFromPath(path)
	pos := byteOffset

	for scanner.Scan() {
		raw := scanner.Bytes()
		lineLen := int64(len(raw)) + 1

		var entry copilotLine
		if err := json.Unmarshal(raw, &entry); err != nil {
			pos += lineLen
			continue
		}

		if entry.Type == "tool.execution_start" && entry.Data.ToolCallID != "" && entry.Data.ToolName != "" {
			ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if ts.IsZero() {
				ts = time.Now()
			}

			sid := entry.Data.SessionID
			if sid == "" {
				sid = sessionID
			}

			events = append(events, ToolEvent{
				Timestamp:  ts,
				Agent:      "copilot",
				SessionID:  sid,
				ToolName:   titleCase(entry.Data.ToolName),
				ToolCallID: entry.Data.ToolCallID,
				CWD:        p.cwd,
			})
		}

		pos += lineLen
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return events, pos, err
	}

	return events, pos, nil
}

// sessionIDFromPath extracts the session UUID from a path like
// ~/.copilot/session-state/<uuid>/events.jsonl
func sessionIDFromPath(path string) string {
	parts := strings.Split(path, string(os.PathSeparator))
	for i, p := range parts {
		if p == "session-state" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
