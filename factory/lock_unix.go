//go:build unix

package factory

import (
	"errors"
	"os"
	"syscall"
)

// errLockHeld means another process holds the lock lockFile tried.
var errLockHeld = errors.New("the lock is held")

// lockFile takes an exclusive flock on f, without waiting, for as long as
// f stays open.
func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return errLockHeld
	}
	return err
}
