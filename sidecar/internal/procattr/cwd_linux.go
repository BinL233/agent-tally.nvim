//go:build linux

package procattr

import (
	"fmt"
	"os"
)

// resolveCWD returns the working directory of a process on Linux.
func resolveCWD(pid int) string {
	path, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return ""
	}

	return path
}
