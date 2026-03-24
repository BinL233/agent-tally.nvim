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
// Uses `ps -eo pid,args` so the full command line is available for
// path-based matching (e.g. Cursor's agent binary is named "agent" but its
// args reference a "cursor-agent" path component).
func (s *Scanner) scan() {
	out, err := exec.Command("ps", "-eo", "pid,args").Output()
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

		// fields[1] is the binary path; extract the basename.
		name := fields[1]
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}

		// If comm doesn't match, fall back to the real executable name.
		// Some programs (e.g. Python-based tools) rename their thread to
		// something like "MainThread", hiding the binary name from comm.
		matchName := name
		if !wl[name] {
			if exe := resolveExeName(pid); exe != "" && wl[exe] {
				matchName = exe
			}
		}

		// Last resort: check the full command line for path components that
		// match a watchlist entry. This catches tools like Cursor whose AI
		// agent binary is named "agent" but whose args contain "cursor-agent".
		if !wl[matchName] && len(fields) > 1 {
			fullArgs := strings.Join(fields[1:], " ")
			for key := range wl {
				// Match "/key-" (e.g. "cursor-agent") or "/key/" or "/key " etc.
				if strings.Contains(fullArgs, "/"+key+"-") ||
					strings.Contains(fullArgs, "/"+key+"/") ||
					strings.Contains(fullArgs, "/"+key+" ") {
					matchName = key
					break
				}
			}
		}

		if wl[matchName] {
			// Reuse cached CWD if available, only resolve for new PIDs.
			cwd, cached := s.cwdCache[pid]
			if !cached {
				cwd = resolveCWD(pid)
			}
			found = append(found, ProcessInfo{PID: pid, Name: matchName, CWD: cwd})
			newCache[pid] = cwd
		}
	}

	s.mu.Lock()
	s.running = found
	s.cwdCache = newCache
	s.mu.Unlock()
}
