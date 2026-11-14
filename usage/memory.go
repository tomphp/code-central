// The recorder's files in memory, beside the in-memory store: NewMemory's
// backing.

package usage

import (
	"bytes"
	"io"
	"io/fs"
	"maps"
	"slices"
)

// inMemory keeps each Project's files by day, each file the bytes of its
// lines. The Recorder's mu guards it.
type inMemory map[string]map[string][]byte

func (m inMemory) append(project, d string, line []byte) error {
	if m[project] == nil {
		m[project] = map[string][]byte{}
	}
	m[project][d] = append(m[project][d], line...)
	return nil
}

func (m inMemory) open(project, d string) (io.ReadCloser, error) {
	lines, ok := m[project][d]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(lines)), nil
}

func (m inMemory) days(project string) ([]string, error) {
	return slices.Collect(maps.Keys(m[project])), nil
}

func (m inMemory) remove(project, d string) error {
	delete(m[project], d)
	return nil
}

func (m inMemory) projects() []string { return slices.Sorted(maps.Keys(m)) }

// name has no path to give, so the log names the file in words.
func (m inMemory) name(project, d string) string {
	if d == "" {
		return project + "'s usage"
	}
	return project + "'s usage for " + d
}
