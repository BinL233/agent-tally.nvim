package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kaileying/agent-tally.nvim/sidecar/internal/config"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/logparse"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/procattr"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/store"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/tokencount"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/watcher"
)

// Daemon orchestrates the file watcher, store, process scanner, and IPC socket.
type Daemon struct {
	cfg        *config.Config
	store      store.Store
	watcher    watcher.Watcher
	scanner    *procattr.Scanner
	estimator  *tokencount.Estimator
	logScanner *logparse.Scanner

	listener net.Listener
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// New creates a new Daemon with the given configuration.
func New(cfg *config.Config) (*Daemon, error) {
	st, err := store.NewSQLite(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	w, err := watcher.NewPlatformWatcher()
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("init watcher: %w", err)
	}

	return &Daemon{
		cfg:        cfg,
		store:      st,
		watcher:    w,
		scanner:    procattr.NewScanner(cfg.Watchlist),
		estimator:  tokencount.NewEstimator(),
		logScanner: logparse.NewScanner(st, cfg.WatchPaths, cfg.LogScanInterval),
	}, nil
}

// Start initializes the database, starts all subsystems, and listens on the UNIX socket.
func (d *Daemon) Start(ctx context.Context) error {
	if err := d.store.Init(ctx); err != nil {
		return fmt.Errorf("init db: %w", err)
	}

	ctx, d.cancel = context.WithCancel(ctx)

	// Pre-snapshot all watched files so TokensInput is non-zero on first write.
	// Uses the same exclude list as the watcher to skip node_modules, .git, etc.
	excludeSet := make(map[string]bool, len(d.cfg.ExcludeDirs))
	for _, ex := range d.cfg.ExcludeDirs {
		excludeSet[ex] = true
	}
	for _, p := range d.cfg.WatchPaths {
		d.estimator.SnapshotDir(p, excludeSet)
	}

	// Start process scanner goroutine.
	scanStop := make(chan struct{})
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.scanner.Start(scanStop)
	}()

	// When context is cancelled, stop the scanner.
	go func() {
		<-ctx.Done()
		close(scanStop)
	}()

	// Start file watcher goroutine.
	events := make(chan watcher.Event, 256)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		if err := d.watcher.Start(d.cfg, events); err != nil && ctx.Err() == nil {
			log.Printf("watcher stopped with error: %v", err)
		}
	}()

	// Start event processor goroutine.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.processEvents(ctx, events)
	}()

	// Start log scanner goroutine.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.logScanner.Start(ctx)
	}()

	// Start IPC socket.
	if err := d.startSocket(ctx); err != nil {
		return fmt.Errorf("start socket: %w", err)
	}

	log.Printf("agent-tallyd running (socket=%s, db=%s)", d.cfg.SocketPath, d.cfg.DBPath)

	for _, p := range d.cfg.WatchPaths {
		log.Printf("  watch path: %s", p)
	}

	log.Printf("  watchlist: %v", d.cfg.Watchlist)

	return nil
}

// Stop gracefully shuts down the daemon.
func (d *Daemon) Stop() {
	if d.cancel != nil {
		d.cancel()
	}

	if d.listener != nil {
		d.listener.Close()
	}

	d.watcher.Stop()
	d.wg.Wait()
	d.store.Close()
	os.Remove(d.cfg.SocketPath)
	log.Println("agent-tallyd stopped")
}

// lsofAttribute runs lsof to find which candidate process has filePath open.
// This is used when CWD matching produces a tie between multiple processes.
// Returns the matching candidate, or an empty ProcessInfo if none found.
func lsofAttribute(candidates []procattr.ProcessInfo, filePath string) procattr.ProcessInfo {
	out, err := exec.Command("lsof", "-t", filePath).Output()
	if err != nil || len(out) == 0 {
		return procattr.ProcessInfo{}
	}

	pidSet := make(map[int]bool)

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
			pidSet[pid] = true
		}
	}

	for _, p := range candidates {
		if pidSet[p.PID] {
			return p
		}
	}

	return procattr.ProcessInfo{}
}

// processEvents reads watcher events, attributes them to processes,
// estimates tokens, and persists them.
func (d *Daemon) processEvents(ctx context.Context, events <-chan watcher.Event) {
	for {
		select {
		case <-ctx.Done():
			return

		case ev, ok := <-events:
			if !ok {
				return
			}

			// Find all watchlist processes whose CWD matches the file path.
			candidates := d.scanner.AttributeAll(ev.FilePath)

			if len(candidates) == 0 {
				continue
			}

			// When multiple processes share the same CWD, use lsof to find
			// which one actually has the file open right now.
			var proc procattr.ProcessInfo
			if len(candidates) == 1 {
				proc = candidates[0]
			} else {
				proc = lsofAttribute(candidates, ev.FilePath)
				if proc.Name == "" {
					proc = candidates[0] // lsof missed it (file already closed); take first
				}
			}

			// If no AI process is responsible, skip this event.
			if proc.Name == "" {
				continue
			}

			// Estimate token delta.
			delta := d.estimator.Estimate(ev.FilePath)

			if delta.TokensOutput == 0 {
				continue
			}

			storeEvent := &store.Event{
				Timestamp:    ev.Timestamp,
				PID:          proc.PID,
				ProcessName:  proc.Name,
				FilePath:     ev.FilePath,
				TokensInput:  delta.TokensInput,
				TokensOutput: delta.TokensOutput,
			}

			if err := d.store.InsertEvent(ctx, storeEvent); err != nil {
				log.Printf("failed to store event: %v", err)
			} else {
				log.Printf("event: %s wrote %s (+%d tokens)", proc.Name, ev.FilePath, delta.TokensOutput)
			}
		}
	}
}

// startSocket creates a UNIX domain socket and accepts JSON-RPC connections.
func (d *Daemon) startSocket(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(d.cfg.SocketPath), 0o755); err != nil {
		return err
	}

	// Remove stale socket file if it exists.
	os.Remove(d.cfg.SocketPath)

	ln, err := net.Listen("unix", d.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("listen unix: %w", err)
	}
	d.listener = ln

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		for {
			conn, err := ln.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("accept error: %v", err)
				continue
			}

			go d.handleConn(ctx, conn)
		}
	}()

	return nil
}

// RPCRequest is a simple JSON-RPC-like request.
type RPCRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// RPCResponse is a simple JSON-RPC-like response.
type RPCResponse struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (d *Daemon) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var req RPCRequest
	if err := dec.Decode(&req); err != nil {
		enc.Encode(RPCResponse{Error: "invalid request"})
		return
	}

	switch req.Method {
	case "status":
		// Include currently detected AI processes.
		running := d.scanner.Running()
		procs := make([]string, 0, len(running))

		for _, p := range running {
			procs = append(procs, fmt.Sprintf("%s (pid:%d)", p.Name, p.PID))
		}

		enc.Encode(RPCResponse{Result: map[string]any{
			"status":           "running",
			"watchlist":        d.cfg.Watchlist,
			"watch_paths":      d.cfg.WatchPaths,
			"db_path":          d.cfg.DBPath,
			"active_processes": procs,
		}})

	case "query":
		var filter store.QueryFilter

		if req.Params != nil {
			json.Unmarshal(req.Params, &filter)
		}

		if filter.Limit == 0 {
			filter.Limit = 100
		}

		events, err := d.store.Query(ctx, filter)
		if err != nil {
			enc.Encode(RPCResponse{Error: err.Error()})
			return
		}

		// Always send an array, never null, so Lua can iterate safely.
		if events == nil {
			events = []store.Event{}
		}

		enc.Encode(RPCResponse{Result: events})

	case "query-by-file":
		var filter store.QueryFilter

		if req.Params != nil {
			json.Unmarshal(req.Params, &filter)
		}

		if filter.Limit == 0 {
			filter.Limit = 50
		}

		files, err := d.store.QueryByFile(ctx, filter)
		if err != nil {
			enc.Encode(RPCResponse{Error: err.Error()})
			return
		}

		enc.Encode(RPCResponse{Result: files})

	case "watchlist-get":
		enc.Encode(RPCResponse{Result: d.cfg.Watchlist})

	case "watchlist-update":
		var params struct {
			Watchlist []string `json:"watchlist"`
		}

		if req.Params != nil {
			json.Unmarshal(req.Params, &params)
		}

		if len(params.Watchlist) > 0 {
			d.cfg.Watchlist = params.Watchlist
			d.scanner.UpdateWatchlist(params.Watchlist)
		}

		enc.Encode(RPCResponse{Result: map[string]any{"ok": true}})

	case "record-event":
		var ev store.Event

		if req.Params != nil {
			json.Unmarshal(req.Params, &ev)
		}

		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now()
		}

		if ev.ProcessName == "" {
			enc.Encode(RPCResponse{Error: "process_name is required"})
			return
		}

		if err := d.store.InsertEvent(ctx, &ev); err != nil {
			enc.Encode(RPCResponse{Error: err.Error()})
			return
		}

		enc.Encode(RPCResponse{Result: map[string]any{"ok": true}})

	case "watch-add":
		var params struct {
			Path string `json:"path"`
		}

		if req.Params != nil {
			json.Unmarshal(req.Params, &params)
		}

		if params.Path == "" {
			enc.Encode(RPCResponse{Error: "path is required"})
			return
		}

		for _, p := range d.cfg.WatchPaths {
			if p == params.Path {
				enc.Encode(RPCResponse{Result: map[string]any{"ok": true}})
				return
			}
		}

		d.watcher.AddPath(params.Path, d.cfg)

		excludeSet := make(map[string]bool, len(d.cfg.ExcludeDirs))
		for _, ex := range d.cfg.ExcludeDirs {
			excludeSet[ex] = true
		}
		d.estimator.SnapshotDir(params.Path, excludeSet)

		d.cfg.WatchPaths = append(d.cfg.WatchPaths, params.Path)

		// Also register the path with the log scanner so skill events are
		// collected for paths added dynamically after daemon startup.
		go d.logScanner.AddCWD(ctx, params.Path)

		enc.Encode(RPCResponse{Result: map[string]any{"ok": true}})

	case "watch-remove":
		var params struct {
			Path string `json:"path"`
		}

		if req.Params != nil {
			json.Unmarshal(req.Params, &params)
		}

		if params.Path == "" {
			enc.Encode(RPCResponse{Error: "path is required"})
			return
		}

		newPaths := make([]string, 0, len(d.cfg.WatchPaths))
		for _, p := range d.cfg.WatchPaths {
			if p == params.Path {
				d.watcher.RemovePath(p)
			} else {
				newPaths = append(newPaths, p)
			}
		}
		d.cfg.WatchPaths = newPaths
		enc.Encode(RPCResponse{Result: map[string]any{"ok": true}})

	case "clear":
		if err := d.store.ClearAll(ctx); err != nil {
			enc.Encode(RPCResponse{Error: err.Error()})
			return
		}

		enc.Encode(RPCResponse{Result: map[string]any{"ok": true}})

	case "query-tools":
		var filter store.ToolFilter

		if req.Params != nil {
			json.Unmarshal(req.Params, &filter)
		}

		if filter.Limit == 0 {
			filter.Limit = 100
		}

		// If this cwd hasn't been registered with the log scanner yet,
		// add it and scan synchronously so results are fresh on first open.
		if filter.CWDPrefix != "" {
			d.logScanner.AddCWD(ctx, filter.CWDPrefix)
		}

		summaries, err := d.store.QueryTools(ctx, filter)
		if err != nil {
			enc.Encode(RPCResponse{Error: err.Error()})
			return
		}

		if summaries == nil {
			summaries = []store.ToolSummary{}
		}

		enc.Encode(RPCResponse{Result: summaries})

	case "query-by-day":
		var filter store.QueryFilter

		if req.Params != nil {
			json.Unmarshal(req.Params, &filter)
		}

		// Default to the past 365 days when no since is provided.
		if filter.Since == nil {
			t := time.Now().AddDate(-1, 0, 0)
			filter.Since = &t
		}

		days, err := d.store.QueryByDay(ctx, filter)
		if err != nil {
			enc.Encode(RPCResponse{Error: err.Error()})
			return
		}

		if days == nil {
			days = []store.DaySummary{}
		}

		enc.Encode(RPCResponse{Result: days})

	case "record-tool":
		var ev store.ToolEvent

		if req.Params != nil {
			json.Unmarshal(req.Params, &ev)
		}

		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now()
		}

		if ev.ToolName == "" {
			enc.Encode(RPCResponse{Error: "tool_name is required"})
			return
		}

		if err := d.store.InsertToolEvent(ctx, &ev); err != nil {
			enc.Encode(RPCResponse{Error: err.Error()})
			return
		}

		enc.Encode(RPCResponse{Result: map[string]any{"ok": true}})

	case "scan-logs":
		n := d.logScanner.ScanOnce(ctx)
		enc.Encode(RPCResponse{Result: map[string]any{"ok": true, "new_events": n}})

	case "clear-path":
		var params struct {
			Path string `json:"path"`
		}

		if req.Params != nil {
			json.Unmarshal(req.Params, &params)
		}

		if params.Path == "" {
			enc.Encode(RPCResponse{Error: "path is required"})
			return
		}

		if err := d.store.ClearByPath(ctx, params.Path); err != nil {
			enc.Encode(RPCResponse{Error: err.Error()})
			return
		}

		enc.Encode(RPCResponse{Result: map[string]any{"ok": true}})

	default:
		enc.Encode(RPCResponse{Error: fmt.Sprintf("unknown method: %s", req.Method)})
	}
}
