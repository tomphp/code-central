package core

import (
	"os"
	"strconv"
	"strings"
)

// residentSize is the process's resident size: statm's second field, in
// pages; 0 when it can't be read.
func residentSize() int64 {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * int64(os.Getpagesize())
}
