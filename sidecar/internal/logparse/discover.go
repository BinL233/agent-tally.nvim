package logparse

import (
	"os"
	"path/filepath"
	"strings"
)

// claudeProjectDir returns the path to the Claude project log directory for a
// given cwd. Claude Code encodes the cwd by replacing every character that is
// not [a-zA-Z0-9-] with "-" (slashes, dots, underscores, etc. all become "-").
func claudeProjectDir(cwd string) string {
	home, _ := os.UserHomeDir()
	var b strings.Builder
	for _, r := range cwd {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return filepath.Join(home, ".claude", "projects", b.String())
}

// DiscoverClaude returns all Claude Code JSONL session log files that belong
// to the given cwd, including any subagent logs.
func DiscoverClaude(cwd string) []string {
	dir := claudeProjectDir(cwd)

	// Top-level session files: <dir>/<uuid>.jsonl
	top, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))

	// Subagent files: <dir>/<session-uuid>/subagents/*.jsonl
	sub, _ := filepath.Glob(filepath.Join(dir, "*/subagents/*.jsonl"))

	return append(top, sub...)
}

// DiscoverCopilot returns all Copilot CLI event log files whose session cwd
// matches the given cwd.
func DiscoverCopilot(cwd string) []string {
	home, _ := os.UserHomeDir()
	sessionRoot := filepath.Join(home, ".copilot", "session-state")

	entries, err := os.ReadDir(sessionRoot)
	if err != nil {
		return nil
	}

	var result []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		eventsFile := filepath.Join(sessionRoot, entry.Name(), "events.jsonl")
		if _, err := os.Stat(eventsFile); err != nil {
			continue
		}

		// Read workspace.yaml to check cwd.
		if copilotSessionCWD(filepath.Join(sessionRoot, entry.Name())) == cwd {
			result = append(result, eventsFile)
		}
	}

	return result
}

// copilotSessionCWD reads the cwd from a Copilot session directory by
// inspecting workspace.yaml or the first session.start event in events.jsonl.
func copilotSessionCWD(sessionDir string) string {
	// Try workspace.yaml first.
	yamlPath := filepath.Join(sessionDir, "workspace.yaml")
	if data, err := os.ReadFile(yamlPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "cwd:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "cwd:"))
			}
			if strings.HasPrefix(line, "workingDirectory:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "workingDirectory:"))
			}
		}
	}

	return ""
}
