package store

import "os"

// flushFile hands f's data to the disk with FlushFileBuffers.
func flushFile(f *os.File) error { return f.Sync() }

// syncDir does nothing on Windows: a directory can't be opened for
// writing to flush it, and NTFS journals renames.
func syncDir(dir string) error { return nil }
