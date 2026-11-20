//go:build unix

package main

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes the exclusive flock on the file at path without waiting.
// held is false when another process has it. The OS drops the lock when
// the process dies.
func tryLock(path string) (release func(), held bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, true, nil
		}
		return nil, false, err
	}
	return func() { f.Close() }, false, nil
}
