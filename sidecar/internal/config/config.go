package config

import (
	"os"
	"path/filepath"
)

// Config holds the daemon configuration.
type Config struct {
	// Watchlist is the set of process names to monitor (e.g., "nvim", "aider", "copilot-agent").
	Watchlist []string

	// WatchPaths is the list of directory paths to monitor for file-write events.
	WatchPaths []string

	// DBPath is the path to the SQLite database file.
	DBPath string

	// SocketPath is the UNIX domain socket path for IPC.
	SocketPath string
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	dataDir := dataHome()
	runtimeDir := runtimeHome()

	return &Config{
		Watchlist: []string{
			"nvim",
			"aider",
			"copilot-agent",
			"claude",
		},
		WatchPaths: []string{os.Getenv("HOME")},
		DBPath:     filepath.Join(dataDir, "agent-tally", "events.db"),
		SocketPath: filepath.Join(runtimeDir, "agent-tally.sock"),
	}
}

func dataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share")
}

func runtimeHome() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return os.TempDir()
}
