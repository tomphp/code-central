// The store's single-writer lock on Windows: a named mutex rather than a
// lock file, because Windows won't rename a folder with a file open
// inside it, and a migration renames the store's folder while locked.

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// storeLock is an open handle on the store's named mutex. The handle is
// held rather than the mutex owned, since ownership belongs to an OS
// thread and goroutines move between threads; Windows closes the handle
// if the process dies, so a crash never leaves the store locked.
type storeLock struct{ h windows.Handle }

// lockStore takes the lock on the store at dir: it creates the mutex
// Local\code-central-store-<hash>, and the store is locked when the mutex
// already exists. The hash is of the absolute, cleaned and lower-cased
// path, since Windows paths are case-insensitive.
func lockStore(dir string) (*storeLock, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(abs))))
	name, err := windows.UTF16PtrFromString(`Local\code-central-store-` + hex.EncodeToString(sum[:]))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("%s: %w", dir, ErrLocked)
	}
	if err != nil {
		return nil, err
	}
	return &storeLock{h: h}, nil
}

// lockStaged locks the staged store at stage before it goes live in
// place of the store at dir. The mutex is named for dir's path, which
// this process already holds, so there is nothing more to take: it
// returns nil, and the lock is never let go across the renames.
func lockStaged(stage, dir string) (*storeLock, error) { return nil, nil }

// release lets the lock go. A nil lock holds nothing.
func (l *storeLock) release() error {
	if l == nil {
		return nil
	}
	return windows.CloseHandle(l.h)
}
