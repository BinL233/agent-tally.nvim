//go:build !linux

package procattr

import (
	"os/exec"
	"strconv"
	"strings"
)

// getRawProcs uses `ps -eo pid,args` to enumerate running processes.
func getRawProcs() ([]rawProc, error) {
	out, err := exec.Command("ps", "-eo", "pid,args").Output()
	if err != nil {
		return nil, err
	}

	var procs []rawProc

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)

		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		procs = append(procs, rawProc{PID: pid, Args: fields[1:]})
	}

	return procs, nil
}
