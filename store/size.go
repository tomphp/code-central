package store

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
	"time"
)

// sizeLife is how long a File Store's measured size stands before Size
// walks its folder again: a Store of many thousand files takes a moment
// to walk, and the Storage tab needs no fresher figure.
const sizeLife = 60 * time.Second

// folderSize is a folder's size, measured by walking it, and kept for
// sizeLife.
type folderSize struct {
	mu   sync.Mutex
	at   time.Time
	size int64
}

// of is dir's size at now: the bytes of every regular file under it. A
// file deleted while the walk passes it counts for nothing.
func (c *folderSize) of(dir string, now time.Time) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && now.Sub(c.at) < sizeLife {
		return c.size, nil
	}
	var size int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			var info fs.FileInfo
			if info, err = d.Info(); err == nil {
				size += info.Size()
			}
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	})
	if err != nil {
		return 0, err
	}
	c.at, c.size = now, size
	return size, nil
}
