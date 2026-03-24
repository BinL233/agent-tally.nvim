package config

import (
	"os"
	"path/filepath"
	"time"
)

// Config holds the daemon configuration.
type Config struct {
	// Watchlist is the set of process names to monitor (e.g., "claude", "copilot-agent").
	Watchlist []string

	// WatchPaths is the list of directory paths to monitor for file-write events.
	WatchPaths []string

	// ExcludeDirs are directory basenames to skip during recursive watching.
	ExcludeDirs []string

	// DBPath is the path to the SQLite database file.
	DBPath string

	// SocketPath is the UNIX domain socket path for IPC.
	SocketPath string

	// PIDFile is the path to the PID file used to prevent duplicate instances.
	PIDFile string

	// MaxDepth limits how deep recursive directory walking goes (0 = unlimited).
	MaxDepth int

	// LogScanInterval controls how often AI agent log files are scanned for skill events.
	LogScanInterval time.Duration
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	dataDir := dataHome()
	runtimeDir := runtimeHome()

	// Default watch path: current working directory, not $HOME.
	cwd, err := os.Getwd()
	if err != nil {
		cwd = os.Getenv("HOME")
	}

	return &Config{
		Watchlist: []string{
			"claude",
			"copilot",
			"cursor",
		},
		WatchPaths: []string{cwd},
		ExcludeDirs: []string{
			".git", "node_modules", ".next", "__pycache__",
			".venv", "venv", ".cache", ".DS_Store",
			"dist", "build", "target", "vendor",
		},
		DBPath:     filepath.Join(dataDir, "agent-tally", "events.db"),
		SocketPath: filepath.Join(runtimeDir, "agent-tally.sock"),
		PIDFile:    filepath.Join(runtimeDir, "agent-tally.pid"),
		MaxDepth:        10,
		LogScanInterval: 5 * time.Second,
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
