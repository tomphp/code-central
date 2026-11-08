//go:build unix

// The Factory's state-file lock on Unix: an flock.

package factory

import (
	"errors"
	"os"
	"syscall"
)

// lockFile takes an exclusive flock on f, without waiting, for as long as
// f stays open.
func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return errLockHeld
	}
	return err
}
