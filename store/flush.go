package store

import "os"

// writeFlushed writes a new file and hands its data to the disk, without
// the drive-cache flush a full fsync does on macOS (F_FULLFSYNC, several
// milliseconds a file). A migration stages thousands of files, so it
// flushes each one this way and then makes them all durable at once with
// one full fsync (atomicWrite of the staged store.yaml) before any rename.
// Elsewhere this is the platform's usual flush (flushFile).
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
