package procattr

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type trackedFile struct {
	PID      int
	Name     string
	LastSeen time.Time
}

// FileTracker polls lsof for watched processes and caches which process most
// recently had each file open. Polling only runs when multiple watched processes
// share the same CWD (the only scenario where attribution is ambiguous).
type FileTracker struct {
	mu       sync.RWMutex
	recent   map[string]trackedFile
	ttl      time.Duration
	requests chan struct{}
}

// NewFileTracker creates a tracker that remembers file ownership for the given TTL.
func NewFileTracker(ttl time.Duration) *FileTracker {
	return &FileTracker{
		recent:   make(map[string]trackedFile),
		ttl:      ttl,
		requests: make(chan struct{}, 8),
	}
}

// Start polls lsof on a background ticker, and also responds to on-demand
// RequestPoll signals. A 500ms cooldown prevents lsof saturation under burst writes.
// Call in a goroutine; returns when stop is closed.
func (ft *FileTracker) Start(scanner *Scanner, stop <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	const cooldown = 500 * time.Millisecond
	var lastPoll time.Time

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			ft.poll(scanner)
			lastPoll = time.Now()
		case <-ft.requests:
			// Drain any additional pending requests before deciding to poll.
			for len(ft.requests) > 0 {
				<-ft.requests
			}
			if time.Since(lastPoll) >= cooldown {
				ft.poll(scanner)
				lastPoll = time.Now()
			}
		}
	}
}

// RequestPoll signals the tracker to run a poll as soon as the cooldown allows.
// Non-blocking: if the channel is full a poll is already pending, so the signal is dropped.
func (ft *FileTracker) RequestPoll() {
	select {
	case ft.requests <- struct{}{}:
	default:
	}
}

func (ft *FileTracker) poll(scanner *Scanner) {
	procs := scanner.Running()
	if len(procs) < 2 {
		return // single agent: no tie possible, skip lsof entirely
	}

	// Group processes by CWD. Only processes that share a CWD with at least
	// one other process can produce an ambiguous attribution.
	cwdGroups := make(map[string][]ProcessInfo, len(procs))
	for _, p := range procs {
		if p.CWD != "" {
			cwdGroups[p.CWD] = append(cwdGroups[p.CWD], p)
		}
	}

	// Collect only the PIDs that are part of an actual tie.
	pidMap := make(map[int]ProcessInfo)
	pids := make([]string, 0)

	for _, group := range cwdGroups {
		if len(group) >= 2 {
			for _, p := range group {
				pidMap[p.PID] = p
				pids = append(pids, strconv.Itoa(p.PID))
			}
		}
	}

	if len(pids) == 0 {
		return // no CWD ties, skip lsof entirely
	}

	// -F pn: machine-readable — lines prefixed 'p' (PID) or 'n' (path)
	// -w: suppress warnings
	out, err := exec.Command("lsof", "-w", "-p", strings.Join(pids, ","), "-F", "pn").Output()
	if err != nil {
		return
	}

	now := time.Now()
	currentPID := 0

	ft.mu.Lock()
	defer ft.mu.Unlock()

	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 2 {
			continue
		}

		switch line[0] {
		case 'p':
			pid, err := strconv.Atoi(strings.TrimSpace(line[1:]))
			if err == nil {
				currentPID = pid
			}
		case 'n':
			if currentPID == 0 {
				continue
			}

			filePath := line[1:]
			if filePath == "" || filePath[0] != '/' {
				continue // skip sockets, pipes, and other non-filesystem entries
			}

			proc, ok := pidMap[currentPID]
			if !ok {
				continue
			}

			if existing, ok := ft.recent[filePath]; !ok || now.After(existing.LastSeen) {
				ft.recent[filePath] = trackedFile{
					PID:      proc.PID,
					Name:     proc.Name,
					LastSeen: now,
				}
			}
		}
	}

	// Expire entries older than TTL.
	for path, tf := range ft.recent {
		if now.Sub(tf.LastSeen) > ft.ttl {
			delete(ft.recent, path)
		}
	}
}

// RecentWriter returns the watched process that most recently had filePath open,
// within the TTL window. Returns empty ProcessInfo if no record found.
func (ft *FileTracker) RecentWriter(filePath string) ProcessInfo {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	if tf, ok := ft.recent[filePath]; ok {
		return ProcessInfo{PID: tf.PID, Name: tf.Name}
	}

	return ProcessInfo{}
}
