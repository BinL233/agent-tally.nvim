package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/kaileying/agent-tally.nvim/sidecar/internal/config"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/daemon"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg := config.Default()

	var watchPaths string

	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "path to SQLite database")
	flag.StringVar(&cfg.SocketPath, "socket", cfg.SocketPath, "path to UNIX domain socket")
	flag.StringVar(&cfg.PIDFile, "pid", cfg.PIDFile, "path to PID file")
	flag.StringVar(&watchPaths, "watch", "", "comma-separated paths to watch (default: cwd)")
	flag.IntVar(&cfg.MaxDepth, "depth", cfg.MaxDepth, "max directory depth to watch")
	flag.Parse()

	if watchPaths != "" {
		cfg.WatchPaths = strings.Split(watchPaths, ",")
	}

	// Refuse to start if another instance is already running.
	if pid, running := alreadyRunning(cfg.PIDFile); running {
		fmt.Fprintf(os.Stderr, "agent-tallyd is already running (pid=%d, pid-file=%s)\n", pid, cfg.PIDFile)
		os.Exit(1)
	}

	// Write our PID so future invocations can detect us.
	if err := writePIDFile(cfg.PIDFile); err != nil {
		log.Fatalf("failed to write pid file: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d, err := daemon.New(cfg)
	if err != nil {
		os.Remove(cfg.PIDFile)
		log.Fatalf("failed to create daemon: %v", err)
	}

	if err := d.Start(ctx); err != nil {
		os.Remove(cfg.PIDFile)
		log.Fatalf("failed to start daemon: %v", err)
	}

	<-ctx.Done()
	d.Stop()
	os.Remove(cfg.PIDFile)
}

// alreadyRunning checks the PID file and returns (pid, true) if a live process owns it.
func alreadyRunning(pidFile string) (int, bool) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}

	// Signal 0 checks if the process exists without sending a real signal.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return 0, false
	}

	return pid, true
}

// writePIDFile writes the current process PID to the given path.
func writePIDFile(pidFile string) error {
	if pidFile == "" {
		return nil
	}

	return os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
}
