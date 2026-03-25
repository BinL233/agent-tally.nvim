package procattr

import (
	"path/filepath"
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

// rawProc is the minimal process data returned by platform-specific listing.
type rawProc struct {
	PID  int
	Args []string // argv[0] is the binary path; remaining are arguments
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

const (
	baseScanInterval = 2 * time.Second
	maxScanInterval  = 30 * time.Second
)

// Start begins periodic scanning with adaptive backoff when no agents are running.
// The interval doubles every 3 consecutive idle scans, up to maxScanInterval.
// Resets to baseScanInterval as soon as an agent is detected.
// Call in a goroutine.
func (s *Scanner) Start(stop <-chan struct{}) {
	s.scan()

	interval := baseScanInterval
	idleScans := 0
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.scan()
			procs := s.Running()
			if len(procs) == 0 {
				idleScans++
				if idleScans >= 3 {
					newInterval := interval * 2
					if newInterval > maxScanInterval {
						newInterval = maxScanInterval
					}
					if newInterval != interval {
						interval = newInterval
						ticker.Reset(interval)
					}
				}
			} else if interval != baseScanInterval {
				idleScans = 0
				interval = baseScanInterval
				ticker.Reset(interval)
			} else {
				idleScans = 0
			}
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
// getRawProcs is platform-specific (getprocs_linux.go / getprocs_other.go).
func (s *Scanner) scan() {
	procs, err := getRawProcs()
	if err != nil {
		return
	}

	s.mu.RLock()
	wl := s.watchlist
	s.mu.RUnlock()

	var found []ProcessInfo
	newCache := make(map[int]string)

	for _, rp := range procs {
		if len(rp.Args) == 0 {
			continue
		}

		// rp.Args[0] is the binary path; extract the basename.
		name := rp.Args[0]
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}

		// If comm doesn't match, fall back to the real executable name.
		matchName := name
		if !wl[name] {
			if exe := resolveExeName(rp.PID); exe != "" && wl[exe] {
				matchName = exe
			}
		}

		// Last resort: check the full command line for path components that
		// match a watchlist entry (e.g. Cursor's agent binary).
		if !wl[matchName] {
			fullArgs := strings.Join(rp.Args, " ")
			for key := range wl {
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
			cwd, cached := s.cwdCache[rp.PID]
			if !cached {
				cwd = resolveCWD(rp.PID)
			}
			found = append(found, ProcessInfo{PID: rp.PID, Name: matchName, CWD: cwd})
			newCache[rp.PID] = cwd
		}
	}

	s.mu.Lock()
	s.running = found
	s.cwdCache = newCache
	s.mu.Unlock()
}
