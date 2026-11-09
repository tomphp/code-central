package store

import "os"

// writeFlushed writes a new file and hands its data to the disk with
// flushFile, which on macOS skips the drive-cache flush of a full fsync.
// A migration stages thousands of files, so it flushes each one this way
// and then makes them all durable at once with one full fsync
// (atomicWrite of the staged store.yaml) before any rename. On Windows
// flushFile does nothing, and that one fsync covers only store.yaml.
func writeFlushed(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := flushFile(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
