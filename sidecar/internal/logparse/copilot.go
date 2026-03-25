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
// buf is allocated once per parser instance and reused across ParseFrom calls.
type CopilotParser struct {
	// cwd is pre-resolved via DiscoverCopilot so we don't re-read workspace.yaml.
	cwd string
	buf []byte
}

// NewCopilotParser creates a parser for a specific session cwd with a reusable 1 MiB buffer.
func NewCopilotParser(cwd string) *CopilotParser {
	return &CopilotParser{cwd: cwd, buf: make([]byte, 1024*1024)}
}

// copilotLine is the minimal shape of a Copilot events.jsonl entry.
type copilotLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Data      struct {
		ToolCallID string `json:"toolCallId"`
		ToolName   string `json:"toolName"`
		SessionID  string `json:"sessionId"`
		// ModelMetrics is present on session.shutdown events.
		ModelMetrics map[string]copilotModelMetric `json:"modelMetrics"`
	} `json:"data"`
}

type copilotModelMetric struct {
	Usage struct {
		InputTokens      int `json:"inputTokens"`
		OutputTokens     int `json:"outputTokens"`
		CacheReadTokens  int `json:"cacheReadTokens"`
		CacheWriteTokens int `json:"cacheWriteTokens"`
	} `json:"usage"`
}

// titleCase converts a lowercase tool name (e.g. "bash") to TitleCase ("Bash").
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ParseFrom reads the Copilot events.jsonl from byteOffset and extracts
// tool.execution_start events and session.shutdown token usage.
func (p *CopilotParser) ParseFrom(path string, byteOffset int64) (ParseResult, int64, error) {
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

		switch entry.Type {
		case "tool.execution_start":
			if entry.Data.ToolCallID == "" || entry.Data.ToolName == "" {
				break
			}
			ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if ts.IsZero() {
				ts = time.Now()
			}
			sid := entry.Data.SessionID
			if sid == "" {
				sid = sessionID
			}
			result.ToolEvents = append(result.ToolEvents, ToolEvent{
				Timestamp:  ts,
				Agent:      "copilot",
				SessionID:  sid,
				ToolName:   titleCase(entry.Data.ToolName),
				ToolCallID: entry.Data.ToolCallID,
				CWD:        p.cwd,
			})

		case "session.shutdown":
			ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if ts.IsZero() {
				ts = time.Now()
			}
			// One TokenUsage per model — request_id = sessionID+":"+model for dedup.
			for model, metrics := range entry.Data.ModelMetrics {
				u := metrics.Usage
				tokensIn := u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens
				tokensOut := u.OutputTokens
				if tokensIn == 0 && tokensOut == 0 {
					continue
				}
				result.TokenUsages = append(result.TokenUsages, TokenUsage{
					Timestamp: ts,
					Agent:     "copilot",
					SessionID: sessionID,
					RequestID: sessionID + ":" + model,
					TokensIn:  tokensIn,
					TokensOut: tokensOut,
					CWD:       p.cwd,
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
