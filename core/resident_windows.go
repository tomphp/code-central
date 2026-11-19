package core

import (
	"syscall"
	"unsafe"
)

// processMemoryCounters is Windows's PROCESS_MEMORY_COUNTERS.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

var getProcessMemoryInfo = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// residentSize is the process's working set, Windows's resident size; 0
// when it can't be read.
func residentSize() int64 {
	if getProcessMemoryInfo.Find() != nil {
		return 0
	}
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	var c processMemoryCounters
	c.cb = uint32(unsafe.Sizeof(c))
	ok, _, _ := getProcessMemoryInfo.Call(uintptr(process), uintptr(unsafe.Pointer(&c)), uintptr(c.cb))
	if ok == 0 {
		return 0
	}
	return int64(c.workingSetSize)
}
