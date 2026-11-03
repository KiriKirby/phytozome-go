//go:build !windows

package phygoboost

import (
	"os"

	"github.com/shirou/gopsutil/v4/process"
)

func sampleChildProcessCount() int {
	parent := int32(os.Getpid())
	children := 0
	procs, err := process.Processes()
	if err != nil {
		return 0
	}
	for _, proc := range procs {
		if proc == nil {
			continue
		}
		ppid, err := proc.Ppid()
		if err == nil && ppid == parent {
			children++
		}
	}
	return children
}
