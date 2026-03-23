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

	"github.com/kaileying/agent-tally.nvim/sidecar/internal/config"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/store"
	"github.com/kaileying/agent-tally.nvim/sidecar/internal/watcher"
)

// Daemon orchestrates the file watcher, store, and IPC socket.
type Daemon struct {
	cfg     *config.Config
	store   store.Store
	watcher watcher.Watcher

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
		cfg:     cfg,
		store:   st,
		watcher: w,
	}, nil
}

// Start initializes the database, starts the file watcher, and listens on the UNIX socket.
func (d *Daemon) Start(ctx context.Context) error {
	if err := d.store.Init(ctx); err != nil {
		return fmt.Errorf("init db: %w", err)
	}

	ctx, d.cancel = context.WithCancel(ctx)

	// Start file watcher goroutine.
	events := make(chan watcher.Event, 128)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.watcher.Start(ctx, d.cfg.WatchPaths, events); err != nil && ctx.Err() == nil {
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

// processEvents reads watcher events, filters by watchlist, and persists them.
func (d *Daemon) processEvents(ctx context.Context, events <-chan watcher.Event) {
	watchset := make(map[string]bool, len(d.cfg.Watchlist))

	for _, name := range d.cfg.Watchlist {
		watchset[name] = true
	}

	for {
		select {
		case <-ctx.Done():
			return

		case ev, ok := <-events:
			if !ok {
				return
			}

			// If the watcher provides a process name, filter it.
			// For now, fsnotify doesn't give PID, so we log all events.
			// TODO: integrate platform-specific PID resolution.
			storeEvent := &store.Event{
				Timestamp:   ev.Timestamp,
				PID:         ev.PID,
				ProcessName: ev.Process,
				FilePath:    ev.FilePath,
			}

			if err := d.store.InsertEvent(ctx, storeEvent); err != nil {
				log.Printf("failed to store event: %v", err)
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
		enc.Encode(RPCResponse{Result: map[string]any{
			"status":    "running",
			"watchlist": d.cfg.Watchlist,
			"db_path":   d.cfg.DBPath,
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

		enc.Encode(RPCResponse{Result: events})

	default:
		enc.Encode(RPCResponse{Error: fmt.Sprintf("unknown method: %s", req.Method)})
	}
}
