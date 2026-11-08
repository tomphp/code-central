//go:build unix

// The store's single-writer lock on Unix: an flock on <store>/lock.

package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// storeLock is the exclusive flock on a store's lock file, held while the
// file stays open.
type storeLock struct{ f *os.File }

// lockStore takes the exclusive lock on the store at dir.
func lockStore(dir string) (*storeLock, error) {
	f, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%s: %w", dir, ErrLocked)
		}
		return nil, err
	}
	return &storeLock{f: f}, nil
}

// lockStaged locks the staged store at stage before it goes live in
// place of the store at dir. The flock is on a file inside the folder,
// so it moves with it: the staged store needs its own.
func lockStaged(stage, dir string) (*storeLock, error) { return lockStore(stage) }

// release lets the lock go. A nil lock holds nothing.
func (l *storeLock) release() error {
	if l == nil {
		return nil
	}
	syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
