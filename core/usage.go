package core

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
)

// Usage (Spec code-central-201) is what agents have consumed: the MCP
// calls Code Central records itself, read back as figures. It is not one
// of core's operations: the HTTP API serves the read alone, and nothing
// over MCP or the CLI.

// UsageDays are the ranges a Usage read takes, in local days; the first
// is the default.
var UsageDays = []int{7, 30, 90}

// SetUsage makes c record Usage with u. Call it before serving; without
// one, nothing is recorded and every read is empty.
func (c *Core) SetUsage(u *usage.Recorder) { c.usage = u }

// RecordCall records an agent's MCP call to key's Project, timed now. A
// call to a Project that doesn't exist is not recorded: there is nowhere
// to file it, and a usage folder with no project.yaml would stop the
// store loading. That check is a store read, as the call itself made; the
// append then takes only the recorder's own lock. Recording never fails
// the call.
func (c *Core) RecordCall(key string, call usage.Call) {
	if c.usage == nil || !c.projectExists(key) {
		return
	}
	call.Time = c.now()
	c.usage.RecordCall(key, call)
}

func (c *Core) projectExists(key string) bool {
	found := false
	_ = c.store.View(func(r store.Reader) error {
		_, found = r.Project(key)
		return nil
	})
	return found
}

// UsageInput is a Usage read: one Project, or every Project (Archived
// ones too) when Project is "", over the last Days local days.
type UsageInput struct {
	Project string
	// Days is one of UsageDays; 0 is the default.
	Days int
}

// UsageView is Usage as the read returns it. Kinds, Sessions and Days
// stay empty until the Factory posts token usage.
type UsageView struct {
	Operations []UsageOperation `json:"operations"`
	Kinds      []UsageKind      `json:"kinds"`
	Sessions   []UsageSession   `json:"sessions"`
	Days       []UsageDay       `json:"days"`
}

// UsageOperation is one MCP operation's row: its calls, their total and
// average request and response sizes in bytes, and how many were
// rejected. Averages are rounded to the nearest byte.
type UsageOperation struct {
	Op                   string `json:"op"`
	Calls                int    `json:"calls"`
	RequestBytes         int    `json:"request_bytes"`
	AverageRequestBytes  int    `json:"average_request_bytes"`
	ResponseBytes        int    `json:"response_bytes"`
	AverageResponseBytes int    `json:"average_response_bytes"`
	Errors               int    `json:"errors"`
}

// UsageKind is one kind of session's Context Size figures.
type UsageKind struct {
	Kind        string `json:"kind"`
	Sessions    int    `json:"sessions"`
	AveragePeak int    `json:"average_peak"`
	LargestPeak int    `json:"largest_peak"`
}

// UsageSession is one recent session: a Claim's main session, or a
// subagent of it (Agent).
type UsageSession struct {
	Project   string    `json:"project"`
	Ticket    string    `json:"ticket"`
	Claim     int       `json:"claim"`
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent,omitempty"`
	Kind      string    `json:"kind"`
	Started   time.Time `json:"started"`
	Peak      int       `json:"peak"`
	Responses int       `json:"responses"`
}

// UsageDay is one local day's token usage, by kind of session.
type UsageDay struct {
	Day   string           `json:"day"`
	Kinds []UsageDayTokens `json:"kinds"`
}

// UsageDayTokens is one kind's tokens on one day, by token type.
type UsageDayTokens struct {
	Kind             string `json:"kind"`
	InputTokens      int    `json:"input_tokens"`
	CacheWriteTokens int    `json:"cache_write_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens"`
	OutputTokens     int    `json:"output_tokens"`
}

// Usage reads Usage. An unknown Project is not_found, and a Days outside
// UsageDays is invalid.
func (c *Core) Usage(_ context.Context, in UsageInput) (UsageView, error) {
	if in.Days == 0 {
		in.Days = UsageDays[0]
	}
	if !slices.Contains(UsageDays, in.Days) {
		return UsageView{}, invalid("days", "days must be 7, 30 or 90, not %d", in.Days)
	}
	if in.Project != "" && !c.projectExists(in.Project) {
		return UsageView{}, &Error{Code: NotFound, Message: fmt.Sprintf("no Project %q", in.Project)}
	}
	recs := c.usage.Read(in.Project, in.Days)
	return UsageView{
		Operations: operations(recs.Calls),
		Kinds:      []UsageKind{},
		Sessions:   []UsageSession{},
		Days:       []UsageDay{},
	}, nil
}

// operations is the operations table: one row per op, most called
// first, then by name.
func operations(calls []usage.Call) []UsageOperation {
	byOp := map[string]*UsageOperation{}
	for _, call := range calls {
		row := byOp[call.Op]
		if row == nil {
			row = &UsageOperation{Op: call.Op}
			byOp[call.Op] = row
		}
		row.Calls++
		row.RequestBytes += call.RequestBytes
		row.ResponseBytes += call.ResponseBytes
		if call.Error != "" {
			row.Errors++
		}
	}
	rows := []UsageOperation{}
	for _, row := range byOp {
		row.AverageRequestBytes = average(row.RequestBytes, row.Calls)
		row.AverageResponseBytes = average(row.ResponseBytes, row.Calls)
		rows = append(rows, *row)
	}
	slices.SortFunc(rows, func(a, b UsageOperation) int {
		return cmp.Or(cmp.Compare(b.Calls, a.Calls), cmp.Compare(a.Op, b.Op))
	})
	return rows
}

// average is total / n rounded to the nearest whole number, halves up.
func average(total, n int) int { return (2*total + n) / (2 * n) }
