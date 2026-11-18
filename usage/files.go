// The File Store's form of usage: ADR 0016's JSON Lines files beside the
// store's own.

package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Files keeps usage under dir, at projects/<key>/usage/<YYYY-MM-DD>.jsonl,
// one JSON object per line with a "type": mcp_call or model_response.
type Files struct {
	dir string
	// mu is the usage's own lock, apart from the store's: appends take it
	// to write, reads to read, so a read never sees half a line.
	mu sync.RWMutex
}

// NewFiles returns the usage kept under dir, the store's directory.
func NewFiles(dir string) *Files { return &Files{dir: dir} }

// The record types, each line's "type".
const (
	TypeCall     = "mcp_call"
	TypeResponse = "model_response"
)

// callLine and responseLine are each record type's line of a file: its
// type, then the record's fields.
type callLine struct {
	Type string `json:"type"`
	Call
}

type responseLine struct {
	Type string `json:"type"`
	Response
}

// Append opens the file for appending. There is no fsync: a crash may lose
// the last records, which usage can afford.
func (f *Files) Append(project, day string, r Record) error {
	var v any
	switch {
	case r.Call != nil:
		v = callLine{TypeCall, *r.Call}
	case r.Response != nil:
		v = responseLine{TypeResponse, *r.Response}
	default:
		return errors.New("usage: a record holds neither a call nor a response")
	}
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(f.folder(project), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(f.name(project, day), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	_, err = file.Write(append(line, '\n'))
	return errors.Join(err, file.Close())
}

func (f *Files) Read(project string, days []string) (Records, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	projects := []string{project}
	if project == "" {
		projects = f.projects()
	}
	var out Records
	var errs []error
	for _, d := range days {
		for _, p := range projects {
			errs = append(errs, f.readFile(p, d, &out))
		}
	}
	return out, errors.Join(errs...)
}

// readFile adds the records of project's file for day d to out, skipping
// and counting any line that doesn't parse. A missing file holds no
// records.
func (f *Files) readFile(project, d string, out *Records) error {
	path := f.name(project, d)
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	defer file.Close()
	skipped := 0
	sc := bufio.NewScanner(file)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var line struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			skipped++
			continue
		}
		var err error
		switch line.Type {
		case TypeCall:
			out.Calls, err = appendRecord(out.Calls, sc.Bytes())
		case TypeResponse:
			out.Responses, err = appendRecord(out.Responses, sc.Bytes())
			if err == nil {
				resp := &out.Responses[len(out.Responses)-1]
				resp.Project, resp.Day = project, d
			}
		default:
			// A type this build doesn't know, from a newer one: not an error.
		}
		if err != nil {
			skipped++
		}
	}
	if skipped > 0 {
		out.Skipped = append(out.Skipped, Skipped{path, skipped})
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("stopped reading %s: %w", path, err)
	}
	return nil
}

// appendRecord adds the record line holds to recs.
func appendRecord[T any](recs []T, line []byte) ([]T, error) {
	var rec T
	if err := json.Unmarshal(line, &rec); err != nil {
		return recs, err
	}
	return append(recs, rec), nil
}

// Prune deletes the files whose day is before the given one. A name that
// isn't a day is left alone, and a file it can't delete is left for the
// next call.
func (f *Files) Prune(before string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var errs []error
	for _, p := range f.projects() {
		entries, err := os.ReadDir(f.folder(p))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot read %s: %w", f.folder(p), err))
			continue
		}
		for _, e := range entries {
			d, ok := strings.CutSuffix(e.Name(), ".jsonl")
			if !ok {
				continue
			}
			if _, err := time.Parse(time.DateOnly, d); err != nil || d >= before {
				continue
			}
			if err := os.Remove(f.name(p, d)); err != nil {
				errs = append(errs, fmt.Errorf("cannot delete %s: %w", f.name(p, d), err))
			}
		}
	}
	return errors.Join(errs...)
}

// Days lists each Project's files whose name is a day. A name that
// isn't a day is left out, as Prune leaves it.
func (f *Files) Days() (map[string][]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := map[string][]string{}
	var errs []error
	for _, p := range f.projects() {
		entries, err := os.ReadDir(f.folder(p))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot read %s: %w", f.folder(p), err))
			continue
		}
		for _, e := range entries { // ReadDir sorts by name, so oldest first
			d, ok := strings.CutSuffix(e.Name(), ".jsonl")
			if !ok {
				continue
			}
			if _, err := time.Parse(time.DateOnly, d); err == nil {
				out[p] = append(out[p], d)
			}
		}
	}
	return out, errors.Join(errs...)
}

// projects is every Project with a folder, sorted.
func (f *Files) projects() []string {
	entries, err := os.ReadDir(filepath.Join(f.dir, "projects"))
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

// folder is project's usage folder.
func (f *Files) folder(project string) string {
	return filepath.Join(f.dir, "projects", project, "usage")
}

// name is the path of project's file for day d.
func (f *Files) name(project, d string) string {
	return filepath.Join(f.folder(project), d+".jsonl")
}
