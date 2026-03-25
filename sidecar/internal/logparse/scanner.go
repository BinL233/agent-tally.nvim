package logparse

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/kaileying/agent-tally.nvim/sidecar/internal/store"
)

// discoveryCache holds a cached list of log file paths with an expiry time.
type discoveryCache struct {
	files   []string
	expires time.Time
}

const discoveryCacheTTL = 30 * time.Second

// Scanner periodically scans AI agent log files and persists new tool events.
type Scanner struct {
	st           store.Store
	cwds         []string
	mu           sync.Mutex
	interval     time.Duration
	claude       *ClaudeParser
	copilot      map[string]*CopilotParser // cached by cwd
	claudeCache  map[string]discoveryCache
	copilotCache map[string]discoveryCache
}

// NewScanner creates a new log scanner for the given watch paths.
func NewScanner(st store.Store, cwds []string, interval time.Duration) *Scanner {
	return &Scanner{
		st:           st,
		cwds:         cwds,
		interval:     interval,
		claude:       NewClaudeParser(),
		copilot:      make(map[string]*CopilotParser),
		claudeCache:  make(map[string]discoveryCache),
		copilotCache: make(map[string]discoveryCache),
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
func (s *Scanner) AddCWD(ctx context.Context, cwd string) {
	s.mu.Lock()
	for _, existing := range s.cwds {
		if existing == cwd {
			s.mu.Unlock()
			return
		}
	}
	s.mu.Unlock()

	// Discover and scan before registering, so a concurrent query-skills
	// cannot observe the cwd as "already registered" before the first scan.
	claudeFiles := DiscoverClaude(cwd)
	copilotFiles := DiscoverCopilot(cwd)

	for _, path := range claudeFiles {
		s.processFile(ctx, path, s.claude)
	}
	for _, path := range copilotFiles {
		parser := s.getCopilotParser(cwd)
		s.processFile(ctx, path, parser)
	}

	// Register and warm the discovery cache.
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.cwds {
		if existing == cwd {
			return
		}
	}

	expires := time.Now().Add(discoveryCacheTTL)
	s.claudeCache[cwd] = discoveryCache{files: claudeFiles, expires: expires}
	s.copilotCache[cwd] = discoveryCache{files: copilotFiles, expires: expires}
	s.cwds = append(s.cwds, cwd)
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
		for _, path := range s.discoverClaude(cwd) {
			n := s.processFile(ctx, path, s.claude)
			total += n
		}

		for _, path := range s.discoverCopilot(cwd) {
			parser := s.getCopilotParser(cwd)
			n := s.processFile(ctx, path, parser)
			total += n
		}
	}

	return total
}

// discoverClaude returns Claude log files for cwd, using a 30s cache to avoid
// repeated Glob calls on every scan cycle.
func (s *Scanner) discoverClaude(cwd string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c, ok := s.claudeCache[cwd]; ok && time.Now().Before(c.expires) {
		return c.files
	}

	files := DiscoverClaude(cwd)
	s.claudeCache[cwd] = discoveryCache{files: files, expires: time.Now().Add(discoveryCacheTTL)}

	return files
}

// discoverCopilot returns Copilot log files for cwd, using a 30s cache to avoid
// repeated directory enumeration on every scan cycle.
func (s *Scanner) discoverCopilot(cwd string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c, ok := s.copilotCache[cwd]; ok && time.Now().Before(c.expires) {
		return c.files
	}

	files := DiscoverCopilot(cwd)
	s.copilotCache[cwd] = discoveryCache{files: files, expires: time.Now().Add(discoveryCacheTTL)}

	return files
}

// getCopilotParser returns a cached CopilotParser for the given cwd, creating
// one if needed. Reusing the parser reuses its internal read buffer.
func (s *Scanner) getCopilotParser(cwd string) *CopilotParser {
	s.mu.Lock()
	defer s.mu.Unlock()

	if p, ok := s.copilot[cwd]; ok {
		return p
	}

	p := NewCopilotParser(cwd)
	s.copilot[cwd] = p

	return p
}

// processFile parses a single log file from its last-known offset, inserts new
// events in a single batch transaction, and updates the offset.
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

	if len(events) > 0 {
		storeEvents := make([]*store.ToolEvent, len(events))
		for i := range events {
			storeEvents[i] = &store.ToolEvent{
				Timestamp:  events[i].Timestamp,
				Agent:      events[i].Agent,
				SessionID:  events[i].SessionID,
				ToolName:   events[i].ToolName,
				ToolCallID: events[i].ToolCallID,
				CWD:        events[i].CWD,
			}
		}

		if err := s.st.BatchInsertToolEvents(ctx, storeEvents); err != nil {
			log.Printf("logparse: insert skill events: %v", err)
		} else {
			inserted = len(storeEvents)
		}
	}

	if err := s.st.SetLogOffset(ctx, path, newOffset); err != nil {
		log.Printf("logparse: set offset for %s: %v", path, err)
	}

	if inserted > 0 {
		log.Printf("logparse: %s: +%d tool events", path, inserted)
	}

	return inserted
}
