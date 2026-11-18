//go:build js

// The store under WebAssembly (the WASM Core, Spec code-central-580),
// where only the in-memory store runs: no other process to lock out and
// no disk to hand files to.

package store

import "os"

// storeLock holds nothing: lockStore never fails under js.
type storeLock struct{}

func lockStore(dir string) (*storeLock, error)         { return &storeLock{}, nil }
func lockFile(path, what string) (*storeLock, error)   { return &storeLock{}, nil }
func lockStaged(stage, dir string) (*storeLock, error) { return &storeLock{}, nil }
func (l *storeLock) release() error                    { return nil }

// flushFile and syncDir do nothing: there is no disk.
func flushFile(f *os.File) error { return nil }
func syncDir(dir string) error   { return nil }
