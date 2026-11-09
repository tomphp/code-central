// Package usage is the usage recorder (ADR 0016): what agents consume,
// kept beside the store as append-only JSON Lines files, one per Project
// per local day, at projects/<key>/usage/<YYYY-MM-DD>.jsonl. Callers
// record and read records; they never see the files.
//
// Recording never fails its caller: an append that fails is written to
// the service log and dropped. Reading skips a line it can't parse and
// counts it in the log, unlike the store's YAML, where a bad file stops
// startup.
//
// Retention: a file whose day is more than 90 days before today is
// deleted at startup, and again whenever the recorder opens the first
// file of a new day. There is no timer.
package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/clock"
)

// Call is one agent MCP tool call: an mcp_call record.
type Call struct {
	Time time.Time `json:"time"`
	// Op is the operation's name.
	Op string `json:"op"`
	// ID is the Ticket or Spec the call named or returned, "" for none.
	ID string `json:"id,omitempty"`
	// RequestBytes is the length of the call's arguments JSON.
	RequestBytes int `json:"request_bytes"`
	// ResponseBytes is the length of the text the agent was given.
	ResponseBytes int `json:"response_bytes"`
	// Error is a rejected call's error code, "" on success.
	Error string `json:"error,omitempty"`
}

// Records are the records a read found, oldest file first.
type Records struct {
	Calls []Call
}

// keep is how many days before today a file is kept: one whose day is
// earlier than that is deleted.
const keep = 90

// The record types, each line's "type".
const typeCall = "mcp_call"

// record is one line of a file: its type, then that type's fields.
type record struct {
	Type string `json:"type"`
	Call
}

// Recorder appends and reads usage records. Its zero value is not
// usable; build one with New. A nil Recorder records nothing and reads
// nothing.
type Recorder struct {
	dir   string
	clock clock.Clock
	loc   *time.Location
	log   io.Writer
	// mu is the recorder's own lock, apart from the store-wide one, so an
	// append never waits on a store write: appends take it to write,
	// reads to read, so a read never sees half a line.
	mu sync.RWMutex
	// prunedOn is the day the old files were last deleted on, under mu.
	prunedOn string
}

// New returns a Recorder keeping its files under dir (the store's
// directory), dating each record by its day in loc, reading "today" from
// clk, and writing failures and skipped lines to log. It deletes the
// files older than keep days before it returns.
func New(dir string, clk clock.Clock, loc *time.Location, log io.Writer) *Recorder {
	if log == nil {
		log = io.Discard
	}
	r := &Recorder{dir: dir, clock: clk, loc: loc, log: log}
	r.prune()
	return r
}

// RecordCall appends c to project's file for c's day. A failure is
// logged and dropped.
func (r *Recorder) RecordCall(project string, c Call) {
	if r == nil {
		return
	}
	if err := r.append(project, c.Time, record{typeCall, c}); err != nil {
		fmt.Fprintf(r.log, "code-central serve: usage: dropped an %s record for %s: %v\n", typeCall, project, err)
	}
}

// append writes v as one line to the end of project's file for t's day,
// opened for appending. There is no fsync: a crash may lose the last
// records, which usage can afford.
func (r *Recorder) append(project string, t time.Time, v record) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if day(r.today()) != r.prunedOn {
		r.prune()
	}
	dir := r.projectDir(project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, day(t.In(r.loc))+".jsonl"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return errors.Join(err, f.Close())
}

// Read returns the records of the last days local days, today among
// them, for project, or for every Project when project is "".
func (r *Recorder) Read(project string, days int) Records {
	var out Records
	if r == nil {
		return out
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	projects := []string{project}
	if project == "" {
		projects = r.projects()
	}
	today := r.today()
	for i := days - 1; i >= 0; i-- {
		d := day(today.AddDate(0, 0, -i))
		for _, p := range projects {
			r.readFile(filepath.Join(r.projectDir(p), d+".jsonl"), &out)
		}
	}
	return out
}

// prune deletes every Project's files whose day is more than keep days
// before today, with mu held or before the Recorder is shared. A file it
// can't delete is logged and left for the next day.
func (r *Recorder) prune() {
	today := r.today()
	r.prunedOn = day(today)
	oldest := day(today.AddDate(0, 0, -keep))
	for _, p := range r.projects() {
		dir := r.projectDir(p)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				fmt.Fprintf(r.log, "code-central serve: usage: cannot read %s: %v\n", dir, err)
			}
			continue
		}
		for _, e := range entries {
			d, ok := strings.CutSuffix(e.Name(), ".jsonl")
			if _, err := time.Parse(time.DateOnly, d); !ok || err != nil || d >= oldest {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if err := os.Remove(path); err != nil {
				fmt.Fprintf(r.log, "code-central serve: usage: cannot delete %s: %v\n", path, err)
			}
		}
	}
}

// today is the clock's time in the service's time zone.
func (r *Recorder) today() time.Time { return r.clock.Now().In(r.loc) }

// projects is every Project with a usage folder.
func (r *Recorder) projects() []string {
	entries, err := os.ReadDir(filepath.Join(r.dir, "projects"))
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

// readFile adds path's records to out, skipping and counting any line
// that doesn't parse. A missing file holds no records.
func (r *Recorder) readFile(path string, out *Records) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		fmt.Fprintf(r.log, "code-central serve: usage: cannot read %s: %v\n", path, err)
		return
	}
	defer f.Close()
	skipped := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var rec record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			skipped++
			continue
		}
		switch rec.Type {
		case typeCall:
			out.Calls = append(out.Calls, rec.Call)
		default:
			// A type this build doesn't know, from a newer one: not an error.
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(r.log, "code-central serve: usage: stopped reading %s: %v\n", path, err)
	}
	if skipped > 0 {
		fmt.Fprintf(r.log, "code-central serve: usage: skipped %d unreadable line(s) in %s\n", skipped, path)
	}
}

func (r *Recorder) projectDir(project string) string {
	return filepath.Join(r.dir, "projects", project, "usage")
}

// day is t's date as a file names it.
func day(t time.Time) string { return t.Format(time.DateOnly) }
