//go:build darwin

package procattr

import (
	"fmt"
	"os/exec"
	"strings"
)

// resolveCWD returns the working directory of a process on macOS.
func resolveCWD(pid int) string {
	out, err := exec.Command("lsof", "-p", fmt.Sprintf("%d", pid), "-a", "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}

	// Output format:
	//   p<pid>
	//   fcwd
	//   n<path>
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") && len(line) > 1 {
			return line[1:]
		}
	}

	return ""
}
