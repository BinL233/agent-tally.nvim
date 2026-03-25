//go:build linux

package procattr

import (
	"os"
	"strconv"
	"strings"
)

// getRawProcs reads /proc directly to enumerate running processes.
// This avoids spawning a `ps` subprocess every scan cycle.
func getRawProcs() ([]rawProc, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	var procs []rawProc

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // not a PID directory
		}

		cmdlineBytes, err := os.ReadFile("/proc/" + entry.Name() + "/cmdline")
		if err != nil || len(cmdlineBytes) == 0 {
			continue
		}

		// cmdline is NUL-separated; trim trailing NUL before splitting.
		raw := strings.TrimRight(string(cmdlineBytes), "\x00")
		args := strings.Split(raw, "\x00")

		if len(args) == 0 || args[0] == "" {
			continue
		}

		procs = append(procs, rawProc{PID: pid, Args: args})
	}

	return procs, nil
}
