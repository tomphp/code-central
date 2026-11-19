//go:build !linux && !windows

package core

// residentSize is 0: this platform gives no resident size without cgo,
// so the Storage tab leaves it out.
func residentSize() int64 { return 0 }
