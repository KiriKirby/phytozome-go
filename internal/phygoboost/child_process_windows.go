//go:build windows

package phygoboost

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sampleChildProcessCount takes exactly one system process snapshot. The prior
// gopsutil path took an additional Toolhelp snapshot for every process while
// resolving PPIDs, which made idle sampling O(N²) in the system process count.
func sampleChildProcessCount() int {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snapshot)

	parent := uint32(os.Getpid())
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return 0
	}
	children := 0
	for {
		if entry.ParentProcessID == parent {
			children++
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	return children
}
