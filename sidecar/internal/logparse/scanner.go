package logparse

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/kaileying/agent-tally.nvim/sidecar/internal/store"
)

// Scanner periodically scans AI agent log files and persists new tool events.
type Scanner struct {
	st       store.Store
	cwds     []string
	mu       sync.Mutex
	interval time.Duration
	claude   *ClaudeParser
}

// NewScanner creates a new log scanner for the given watch paths.
func NewScanner(st store.Store, cwds []string, interval time.Duration) *Scanner {
	return &Scanner{
		st:       st,
		cwds:     cwds,
		interval: interval,
		claude:   &ClaudeParser{},
	}
}

// Start begins periodic scanning. Call in a goroutine; returns when ctx is cancelled.
func (s *Scanner) Start(ctx context.Context) {
	s.scan(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scan(ctx)
		}
	}
}

// ScanOnce runs a single scan pass. Used for on-demand refresh.
func (s *Scanner) ScanOnce(ctx context.Context) int {
	return s.scan(ctx)
}

// AddCWD adds a new directory to the scan list and immediately runs a scan
// for it. Call this when watch-add registers a new project path.
//
// The cwd is registered AFTER scanning completes so that a concurrent caller
// (e.g. query-skills) cannot observe the cwd as "already registered" before
// the initial scan has finished.
func (s *Scanner) AddCWD(ctx context.Context, cwd string) {
	s.mu.Lock()
	for _, existing := range s.cwds {
		if existing == cwd {
			s.mu.Unlock()
			return
		}
	}
	// Release the lock before scanning — processFile uses the DB for dedup so
	// concurrent scans of the same path are safe.
	s.mu.Unlock()

	// Scan the new cwd immediately so the dashboard shows data right away.
	for _, path := range DiscoverClaude(cwd) {
		s.processFile(ctx, path, s.claude)
	}
	for _, path := range DiscoverCopilot(cwd) {
		s.processFile(ctx, path, NewCopilotParser(cwd))
	}

	// Register for periodic scanning only after the initial scan is done.
	s.mu.Lock()
	for _, existing := range s.cwds {
		if existing == cwd {
			s.mu.Unlock()
			return
		}
	}
	s.cwds = append(s.cwds, cwd)
	s.mu.Unlock()
}

// scan discovers and parses all relevant log files, returning the number of
// new tool events inserted.
func (s *Scanner) scan(ctx context.Context) int {
	s.mu.Lock()
	cwds := make([]string, len(s.cwds))
	copy(cwds, s.cwds)
	s.mu.Unlock()

	total := 0

	for _, cwd := range cwds {
		// Claude Code logs.
		for _, path := range DiscoverClaude(cwd) {
			n := s.processFile(ctx, path, s.claude)
			total += n
		}

		// Copilot CLI logs.
		for _, path := range DiscoverCopilot(cwd) {
			parser := NewCopilotParser(cwd)
			n := s.processFile(ctx, path, parser)
			total += n
		}
	}

	return total
}

// processFile parses a single log file from its last-known offset, inserts new
// events, and updates the offset.
func (s *Scanner) processFile(ctx context.Context, path string, parser Parser) int {
	offset, err := s.st.GetLogOffset(ctx, path)
	if err != nil {
		log.Printf("logparse: get offset for %s: %v", path, err)
		return 0
	}

	events, newOffset, err := parser.ParseFrom(path, offset)
	if err != nil {
		log.Printf("logparse: parse %s: %v", path, err)
		return 0
	}

	if newOffset <= offset {
		return 0
	}

	inserted := 0

	for i := range events {
		se := &store.ToolEvent{
			Timestamp:  events[i].Timestamp,
			Agent:      events[i].Agent,
			SessionID:  events[i].SessionID,
			ToolName:   events[i].ToolName,
			ToolCallID: events[i].ToolCallID,
			CWD:        events[i].CWD,
		}

		if err := s.st.InsertToolEvent(ctx, se); err != nil {
			log.Printf("logparse: insert skill event: %v", err)
			continue
		}

		inserted++
	}

	if err := s.st.SetLogOffset(ctx, path, newOffset); err != nil {
		log.Printf("logparse: set offset for %s: %v", path, err)
	}

	if inserted > 0 {
		log.Printf("logparse: %s: +%d tool events", path, inserted)
	}

	return inserted
}
