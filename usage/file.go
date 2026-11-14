// The recorder's files on disk, beside the store's: New's backing.

package usage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// onDisk keeps the files under dir, at
// projects/<key>/usage/<YYYY-MM-DD>.jsonl.
type onDisk struct{ dir string }

// append opens the file for appending. There is no fsync: a crash may
// lose the last records, which usage can afford.
func (o onDisk) append(project, d string, line []byte) error {
	if err := os.MkdirAll(o.name(project, ""), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(o.name(project, d), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(line)
	return errors.Join(err, f.Close())
}

func (o onDisk) open(project, d string) (io.ReadCloser, error) { return os.Open(o.name(project, d)) }

// days is the days of project's files named <YYYY-MM-DD>.jsonl; any other
// name is left out.
func (o onDisk) days(project string) ([]string, error) {
	entries, err := os.ReadDir(o.name(project, ""))
	if err != nil {
		return nil, err
	}
	var days []string
	for _, e := range entries {
		if d, ok := strings.CutSuffix(e.Name(), ".jsonl"); ok {
			days = append(days, d)
		}
	}
	return days, nil
}

func (o onDisk) remove(project, d string) error { return os.Remove(o.name(project, d)) }

func (o onDisk) projects() []string {
	entries, err := os.ReadDir(filepath.Join(o.dir, "projects"))
	if err != nil {
		return nil
	}
	var keys []string
	for _, e := range entries {
		if e.IsDir() {
			keys = append(keys, e.Name())
		}
	}
	return slices.Sorted(slices.Values(keys))
}

// name is the file's path, or its folder's when d is "".
func (o onDisk) name(project, d string) string {
	dir := filepath.Join(o.dir, "projects", project, "usage")
	if d == "" {
		return dir
	}
	return filepath.Join(dir, d+".jsonl")
}
