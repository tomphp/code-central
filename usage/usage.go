// Package usage is the usage recorder (ADR 0016): what agents consume,
// kept beside the store as append-only JSON Lines files, one per Project
// per local day, at projects/<key>/usage/<YYYY-MM-DD>.jsonl, or in
// memory beside the in-memory store. Callers record and read records;
// they never see the files.
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

// Response is one model response's token usage in a Claim's session, as
// a Factory posted it: a model_response record.
type Response struct {
	// Project is the Project whose file holds the record, and Day that
	// file's local day, both set on read.
	Project string    `json:"-"`
	Day     string    `json:"-"`
	Time    time.Time `json:"time"`
	// Ticket is the Claim's Ticket's display ID, and Claim its number.
	Ticket    string `json:"ticket"`
	Claim     int    `json:"claim"`
	SessionID string `json:"session_id"`
	// Agent is the subagent the response was for, "" for the main session.
	Agent string `json:"agent,omitempty"`
	// AgentType and Description are the Agent Type and description of the
	// Agent call that started the subagent; both are "" for the main
	// session and for a response recorded before they were posted.
	AgentType   string `json:"agent_type,omitempty"`
	Description string `json:"description,omitempty"`
	// Kind is the kind of session, as the Factory named it.
	Kind string `json:"kind"`
	// Model is the model that gave the response, as Claude Code reported
	// it; "" when the Factory didn't post one.
	Model string `json:"model,omitempty"`
	Tokens
}

// Tokens are a model response's four token counts.
type Tokens struct {
	InputTokens      int `json:"input_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	OutputTokens     int `json:"output_tokens"`
}

// Add adds o's counts to t's.
func (t *Tokens) Add(o Tokens) {
	t.InputTokens += o.InputTokens
	t.CacheWriteTokens += o.CacheWriteTokens
	t.CacheReadTokens += o.CacheReadTokens
	t.OutputTokens += o.OutputTokens
}

// ContextSize is how many tokens the model read for the response: how
// full the session's context window was.
func (t Tokens) ContextSize() int { return t.InputTokens + t.CacheWriteTokens + t.CacheReadTokens }

// Total is all four counts as one number: what the response used.
func (t Tokens) Total() int { return t.ContextSize() + t.OutputTokens }

// Records are the records a read found, oldest file first, and Days the
// local days it read, oldest first, as YYYY-MM-DD.
type Records struct {
	Calls     []Call
	Responses []Response
	Days      []string
}

// KeepDays is how many days before today a file is kept: one whose day is
// earlier than that is deleted.
const KeepDays = 90

// The record types, each line's "type".
const (
	typeCall     = "mcp_call"
	typeResponse = "model_response"
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

// Recorder appends and reads usage records. Its zero value is not
// usable; build one with New or NewMemory. A nil Recorder records nothing
// and reads nothing.
type Recorder struct {
	files dayFiles
	clock clock.Clock
	loc   *time.Location
	log   io.Writer
	// mu is the recorder's own lock, apart from the store-wide one, so an
	// append never waits on a store write: appends take it to write,
	// reads to read, so a read never sees half a line. files is used
	// only under it.
	mu sync.RWMutex
	// prunedOn is the day the old files were last deleted on, under mu.
	prunedOn string
}

// dayFiles is where a Recorder keeps its files, one per Project per
// local day, the day as YYYY-MM-DD: on disk or in memory.
type dayFiles interface {
	// append adds line to the end of project's file for day d.
	append(project, d string, line []byte) error
	// open opens project's file for day d; a missing one is
	// fs.ErrNotExist.
	open(project, d string) (io.ReadCloser, error)
	// days is the days project has a file for; a Project with none may
	// be fs.ErrNotExist.
	days(project string) ([]string, error)
	// remove deletes project's file for day d.
	remove(project, d string) error
	// projects is every Project with a usage folder, sorted.
	projects() []string
	// name is how the log names project's file for day d, or its folder
	// when d is "".
	name(project, d string) string
}

// New returns a Recorder keeping its files under dir (the store's
// directory), dating each record by its day in loc, reading "today" from
// clk, and writing failures and skipped lines to log. It deletes the
// files older than KeepDays days before it returns.
func New(dir string, clk clock.Clock, loc *time.Location, log io.Writer) *Recorder {
	return newRecorder(onDisk{dir}, clk, loc, log)
}

// NewMemory returns a Recorder keeping its files in memory, as the
// in-memory store keeps its records, and otherwise dating, reading and
// deleting them as New's does.
func NewMemory(clk clock.Clock, loc *time.Location, log io.Writer) *Recorder {
	return newRecorder(inMemory{}, clk, loc, log)
}

func newRecorder(files dayFiles, clk clock.Clock, loc *time.Location, log io.Writer) *Recorder {
	if log == nil {
		log = io.Discard
	}
	r := &Recorder{files: files, clock: clk, loc: loc, log: log}
	r.prune()
	return r
}

// RecordCall appends c to project's file for c's day. A failure is
// logged and dropped.
func (r *Recorder) RecordCall(project string, c Call) {
	if r == nil {
		return
	}
	if err := r.append(project, c.Time, callLine{typeCall, c}); err != nil {
		fmt.Fprintf(r.log, "code-central serve: usage: dropped an %s record for %s: %v\n", typeCall, project, err)
	}
}

// RecordResponses appends each of rs to project's file for its day. A
// failure is logged and drops only that record.
func (r *Recorder) RecordResponses(project string, rs []Response) {
	if r == nil {
		return
	}
	for _, resp := range rs {
		if err := r.append(project, resp.Time, responseLine{typeResponse, resp}); err != nil {
			fmt.Fprintf(r.log, "code-central serve: usage: dropped a %s record for %s: %v\n", typeResponse, project, err)
		}
	}
}

// append writes v as one line to the end of project's file for t's day.
func (r *Recorder) append(project string, t time.Time, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if day(r.today()) != r.prunedOn {
		r.prune()
	}
	return r.files.append(project, day(t.In(r.loc)), append(line, '\n'))
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
		projects = r.files.projects()
	}
	today := r.today()
	for i := days - 1; i >= 0; i-- {
		d := day(today.AddDate(0, 0, -i))
		out.Days = append(out.Days, d)
		for _, p := range projects {
			r.readFile(p, d, &out)
		}
	}
	return out
}

// prune deletes every Project's files whose day is more than KeepDays days
// before today, with mu held or before the Recorder is shared. A file it
// can't delete is logged and left for the next day.
func (r *Recorder) prune() {
	today := r.today()
	r.prunedOn = day(today)
	oldest := day(today.AddDate(0, 0, -KeepDays))
	for _, p := range r.files.projects() {
		days, err := r.files.days(p)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				fmt.Fprintf(r.log, "code-central serve: usage: cannot read %s: %v\n", r.files.name(p, ""), err)
			}
			continue
		}
		for _, d := range days {
			if _, err := time.Parse(time.DateOnly, d); err != nil || d >= oldest {
				continue
			}
			if err := r.files.remove(p, d); err != nil {
				fmt.Fprintf(r.log, "code-central serve: usage: cannot delete %s: %v\n", r.files.name(p, d), err)
			}
		}
	}
}

// today is the clock's time in the service's time zone.
func (r *Recorder) today() time.Time { return r.clock.Now().In(r.loc) }

// readFile adds the records of project's file for day d to out, skipping
// and counting any line that doesn't parse. A missing file holds no
// records.
func (r *Recorder) readFile(project, d string, out *Records) {
	path := r.files.name(project, d)
	f, err := r.files.open(project, d)
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
		var line struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			skipped++
			continue
		}
		var err error
		switch line.Type {
		case typeCall:
			out.Calls, err = appendRecord(out.Calls, sc.Bytes())
		case typeResponse:
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
	if err := sc.Err(); err != nil {
		fmt.Fprintf(r.log, "code-central serve: usage: stopped reading %s: %v\n", path, err)
	}
	if skipped > 0 {
		fmt.Fprintf(r.log, "code-central serve: usage: skipped %d unreadable line(s) in %s\n", skipped, path)
	}
}

// appendRecord adds the record line holds to recs.
func appendRecord[T any](recs []T, line []byte) ([]T, error) {
	var rec T
	if err := json.Unmarshal(line, &rec); err != nil {
		return recs, err
	}
	return append(recs, rec), nil
}

// day is t's date as a file names it.
func day(t time.Time) string { return t.Format(time.DateOnly) }
