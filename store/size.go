package store

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// folderSize is dir's size: the bytes of every regular file under it,
// walked each time, since the Storage tab shows the size as it is now. A
// file deleted while the walk passes it counts for nothing.
func folderSize(dir string) (int64, error) {
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
	return size, nil
}
