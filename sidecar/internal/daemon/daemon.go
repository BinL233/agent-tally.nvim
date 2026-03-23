package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaileying/agent-tally.nvim/sidecar/internal/config"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/procattr"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/store"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/tokencount"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/watcher"
)

// Daemon orchestrates the file watcher, store, process scanner, and IPC socket.
type Daemon struct {
	cfg       *config.Config
	store     store.Store
	watcher   watcher.Watcher
	scanner   *procattr.Scanner
	estimator *tokencount.Estimator

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
		cfg:       cfg,
		store:     st,
		watcher:   w,
		scanner:   procattr.NewScanner(cfg.Watchlist),
		estimator: tokencount.NewEstimator(),
	}, nil
}

// Start initializes the database, starts all subsystems, and listens on the UNIX socket.
func (d *Daemon) Start(ctx context.Context) error {
	if err := d.store.Init(ctx); err != nil {
		return fmt.Errorf("init db: %w", err)
	}

	ctx, d.cancel = context.WithCancel(ctx)

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

			// Try to attribute the file write to a watchlist process.
			proc := d.scanner.Attribute(ev.FilePath)

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

	case "clear":
		if err := d.store.ClearAll(ctx); err != nil {
			enc.Encode(RPCResponse{Error: err.Error()})
			return
		}

		enc.Encode(RPCResponse{Result: map[string]any{"ok": true}})

	default:
		enc.Encode(RPCResponse{Error: fmt.Sprintf("unknown method: %s", req.Method)})
	}
}
