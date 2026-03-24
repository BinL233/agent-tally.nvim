//go:build linux

package procattr

import (
	"fmt"
	"os"
	"path/filepath"
)

// resolveExeName returns the basename of the actual executable for a process
// on Linux. This is needed because some programs (e.g. Python-based tools)
// rename their comm to a thread name like "MainThread", hiding the real
// binary name from ps -eo pid,comm.
func resolveExeName(pid int) string {
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return ""
	}

	return filepath.Base(exe)
}
