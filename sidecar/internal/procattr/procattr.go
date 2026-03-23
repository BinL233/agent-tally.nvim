package procattr

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProcessInfo holds a running process's identity.
type ProcessInfo struct {
	PID  int
	Name string
	CWD  string
}

// Scanner periodically scans for running AI processes from the watchlist.
type Scanner struct {
	watchlist map[string]bool
	mu        sync.RWMutex
	running   []ProcessInfo
	cwdCache  map[int]string // PID -> CWD, avoids re-resolving every scan
}

// NewScanner creates a scanner that looks for the given process names.
func NewScanner(names []string) *Scanner {
	wl := make(map[string]bool, len(names))

	for _, n := range names {
		wl[n] = true
	}

	return &Scanner{watchlist: wl, cwdCache: make(map[int]string)}
}

// Start begins periodic scanning. Call in a goroutine.
func (s *Scanner) Start(stop <-chan struct{}) {
	s.scan()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.scan()
		}
	}
}

// Running returns the currently detected AI processes.
func (s *Scanner) Running() []ProcessInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ProcessInfo, len(s.running))
	copy(result, s.running)

	return result
}

// UpdateWatchlist replaces the watchlist and triggers an immediate rescan.
func (s *Scanner) UpdateWatchlist(names []string) {
	wl := make(map[string]bool, len(names))

	for _, n := range names {
		wl[n] = true
	}

	s.mu.Lock()
	s.watchlist = wl
	s.mu.Unlock()

	s.scan()
}

// Attribute finds which watchlist process is most likely responsible for
// a file write at the given path. Uses CWD-based matching: the process
// whose CWD is the longest prefix of filePath wins.
// When multiple processes tie on CWD length, returns the first found.
// Use AttributeAll to get all tied candidates for further disambiguation.
func (s *Scanner) Attribute(filePath string) ProcessInfo {
	candidates := s.AttributeAll(filePath)

	if len(candidates) == 0 {
		return ProcessInfo{}
	}

	return candidates[0]
}

// AttributeAll returns all watchlist processes whose CWD is the longest
// matching prefix of filePath. Usually one result; multiple means a tie
// (e.g. two tools running in the same directory).
func (s *Scanner) AttributeAll(filePath string) []ProcessInfo {
	s.mu.RLock()
	procs := make([]ProcessInfo, len(s.running))
	copy(procs, s.running)
	s.mu.RUnlock()

	cleanPath := filepath.Clean(filePath)
	var matches []ProcessInfo
	bestLen := 0

	for _, p := range procs {
		if p.CWD == "" {
			continue
		}

		cleanCWD := filepath.Clean(p.CWD)

		if strings.HasPrefix(cleanPath, cleanCWD+"/") || cleanPath == cleanCWD {
			if len(cleanCWD) > bestLen {
				matches = []ProcessInfo{p}
				bestLen = len(cleanCWD)
			} else if len(cleanCWD) == bestLen {
				matches = append(matches, p)
			}
		}
	}

	return matches
}

// scan queries the system for running processes matching the watchlist.
func (s *Scanner) scan() {
	out, err := exec.Command("ps", "-eo", "pid,comm").Output()
	if err != nil {
		return
	}

	s.mu.RLock()
	wl := s.watchlist
	s.mu.RUnlock()

	var found []ProcessInfo
	newCache := make(map[int]string)

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)

		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		// ps comm can be a full path; extract the basename.
		name := fields[1]

		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}

		if wl[name] {
			// Reuse cached CWD if available, only resolve for new PIDs.
			cwd, cached := s.cwdCache[pid]
			if !cached {
				cwd = resolveCWD(pid)
			}
			found = append(found, ProcessInfo{PID: pid, Name: name, CWD: cwd})
			newCache[pid] = cwd
		}
	}

	s.mu.Lock()
	s.running = found
	s.cwdCache = newCache
	s.mu.Unlock()
}
