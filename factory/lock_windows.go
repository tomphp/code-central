// The Factory's state-file lock on Windows: a LockFileEx byte-range lock.

package factory

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive LockFileEx lock on f, without waiting, for
// as long as f stays open. Windows locks bar reads too, so the lock is on
// one byte far past the end, leaving the holder's pid readable.
func lockFile(f *os.File) error {
	far := &windows.Overlapped{Offset: 0xffffffff, OffsetHigh: 0x7fffffff}
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, far)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errLockHeld
	}
	return err
}
