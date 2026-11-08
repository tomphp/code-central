//go:build unix

package store

import (
	"os"
	"runtime"
	"syscall"
)

// flushFile hands f's data to the disk. On macOS that is a plain fsync,
// without the drive-cache flush of a full one (F_FULLFSYNC, several
// milliseconds a file); elsewhere a plain fsync is already the full one.
func flushFile(f *os.File) error {
	if runtime.GOOS == "darwin" {
		return syscall.Fsync(int(f.Fd()))
	}
	return f.Sync()
}

// syncDir makes dir's entries, such as a rename into it, durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
