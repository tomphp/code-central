// Handing the store's files to the disk on Windows.

package store

import "os"

// flushFile leaves f's data to the Windows cache, which writes it to
// the disk within seconds. Windows has no cheap per-file flush:
// FlushFileBuffers flushes the drive's whole cache, and doing that for
// each of 10,000 staged files takes minutes, far outside Claude Code's
// ~31s reconnect window. So a power cut just after a migration can
// leave staged files unwritten; the old store is kept in its backup
// (running-as-a-service.md says how to restore it).
func flushFile(f *os.File) error { return nil }

// syncDir does nothing on Windows: a directory can't be opened for
// writing to flush it, and NTFS journals renames.
func syncDir(dir string) error { return nil }
