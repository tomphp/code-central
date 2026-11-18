// Package usage is the usage recorder (ADR 0016, amended by ADR 0069):
// what agents consume, kept by the Store: in the File Store as
// append-only JSON Lines files, one per Project per local day, at
// projects/<key>/usage/<YYYY-MM-DD>.jsonl, or in memory beside the
// in-memory store. Callers record and read records through a Recorder;
// they never see where they are kept.
//
// Recording never fails its caller: an append that fails is written to
// the service log and dropped. Reading skips a line it can't parse and
// counts it in the log, unlike the store's YAML, where a bad file stops
// startup.
//
// Retention: the days more than 90 before today are deleted at startup,
// and again whenever the recorder appends the first record of a new day.
// There is no timer.
package usage

import (
	"fmt"
	"io"
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

// Records are the records a read found, oldest day first, and Days the
// local days it read, oldest first, as YYYY-MM-DD. Skipped is where a
// Store passed over lines it couldn't parse.
type Records struct {
	Calls     []Call
	Responses []Response
	Days      []string
	Skipped   []Skipped
}

// KeepDays is how many days before today a day's records are kept: ones
// from earlier than that are deleted.
const KeepDays = 90

// Recorder appends and reads usage records over a Store. Its zero value
// is not usable; build one with NewRecorder, New or NewMemory. A nil
// Recorder records nothing and reads nothing.
type Recorder struct {
	store Store
	clock clock.Clock
	loc   *time.Location
	log   io.Writer
	// mu guards prunedOn. The Store has its own lock, so an append never
	// waits on a store write.
	mu sync.Mutex
	// prunedOn is the day the old records were last deleted on, under mu.
	prunedOn string
}

// New returns a Recorder keeping its files under dir (the store's
// directory), dating each record by its day in loc, reading "today" from
// clk, and writing failures and skipped lines to log. It deletes the
// files older than KeepDays days before it returns.
func New(dir string, clk clock.Clock, loc *time.Location, log io.Writer) *Recorder {
	return NewRecorder(NewFiles(dir), clk, loc, log)
}

// NewMemory returns a Recorder keeping its records in memory, as the
// in-memory store keeps its own, and otherwise dating, reading and
// deleting them as New's does.
func NewMemory(clk clock.Clock, loc *time.Location, log io.Writer) *Recorder {
	return NewRecorder(NewMemoryStore(), clk, loc, log)
}

// NewRecorder returns a Recorder over s, dating each record by its day in
// loc, reading "today" from clk, and writing failures and skipped lines to
// log. It deletes the days older than KeepDays days before it returns.
func NewRecorder(s Store, clk clock.Clock, loc *time.Location, log io.Writer) *Recorder {
	if log == nil {
		log = io.Discard
	}
	r := &Recorder{store: s, clock: clk, loc: loc, log: log}
	r.prune()
	return r
}

// RecordCall appends c to project's records for c's day. A failure is
// logged and dropped.
func (r *Recorder) RecordCall(project string, c Call) {
	if r == nil {
		return
	}
	if err := r.append(project, c.Time, Record{Call: &c}); err != nil {
		fmt.Fprintf(r.log, "code-central serve: usage: dropped an %s record for %s: %v\n", TypeCall, project, err)
	}
}

// RecordResponses appends each of rs to project's records for its day. A
// failure is logged and drops only that record.
func (r *Recorder) RecordResponses(project string, rs []Response) {
	if r == nil {
		return
	}
	for _, resp := range rs {
		if err := r.append(project, resp.Time, Record{Response: &resp}); err != nil {
			fmt.Fprintf(r.log, "code-central serve: usage: dropped a %s record for %s: %v\n", TypeResponse, project, err)
		}
	}
}

// append adds rec to project's records for t's day.
func (r *Recorder) append(project string, t time.Time, rec Record) error {
	r.pruneOnANewDay()
	return r.store.Append(project, day(t.In(r.loc)), rec)
}

// Read returns the records of the last days local days, today among
// them, for project, or for every Project when project is "".
func (r *Recorder) Read(project string, days int) Records {
	if r == nil {
		return Records{}
	}
	today := r.today()
	var list []string
	for i := days - 1; i >= 0; i-- {
		list = append(list, day(today.AddDate(0, 0, -i)))
	}
	out, err := r.store.Read(project, list)
	if err != nil {
		fmt.Fprintf(r.log, "code-central serve: usage: %v\n", err)
	}
	for _, s := range out.Skipped {
		fmt.Fprintf(r.log, "code-central serve: usage: skipped %d unreadable line(s) in %s\n", s.Lines, s.Where)
	}
	out.Days = list
	return out
}

// pruneOnANewDay deletes the old days if it hasn't yet today.
func (r *Recorder) pruneOnANewDay() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if day(r.today()) != r.prunedOn {
		r.prune()
	}
}

// prune deletes every Project's records for days more than KeepDays days
// before today, with mu held or before the Recorder is shared. What it
// can't delete is logged and left for the next day.
func (r *Recorder) prune() {
	today := r.today()
	r.prunedOn = day(today)
	if err := r.store.Prune(day(today.AddDate(0, 0, -KeepDays))); err != nil {
		fmt.Fprintf(r.log, "code-central serve: usage: %v\n", err)
	}
}

// today is the clock's time in the service's time zone.
func (r *Recorder) today() time.Time { return r.clock.Now().In(r.loc) }

// day is t's date as a store names it.
func day(t time.Time) string { return t.Format(time.DateOnly) }
