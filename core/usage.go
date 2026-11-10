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
// calls Code Central records itself and the model responses' token usage
// a Factory posts, read back as figures. It is not one of core's
// operations: the HTTP API serves the post and the read alone, and
// nothing over MCP or the CLI.

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

// UsageResponse is one model response's token usage, as a Factory posts
// it: when it came, the subagent it was for ("" for the main session),
// the kind of session, the model that gave it ("" when not posted), and
// its four token counts.
type UsageResponse struct {
	Time  time.Time `json:"time"`
	Agent string    `json:"agent,omitempty"`
	Kind  string    `json:"kind"`
	Model string    `json:"model,omitempty"`
	usage.Tokens
}

// PostUsage is POST /api/usage {session_id, responses}: it records the
// responses under the newest Claim with that session ID, live or ended,
// as Session Events are filed, and counts as a ping. With no such Claim
// they are dropped. A response with no time is dated now. It never fails.
func (c *Core) PostUsage(ctx context.Context, sessionID string, responses []UsageResponse) {
	if sessionID == "" {
		return
	}
	c.Ping(ctx, sessionID)
	if len(responses) == 0 || c.usage == nil {
		return
	}
	k, _, ok := c.newestClaim(sessionID)
	if !ok {
		return
	}
	rs := make([]usage.Response, len(responses))
	for i, r := range responses {
		if r.Time.IsZero() {
			r.Time = c.now()
		}
		rs[i] = usage.Response{
			Time: r.Time, Ticket: DisplayID(k.project, k.ticket), Claim: k.claim,
			SessionID: sessionID, Agent: r.Agent, Kind: r.Kind, Model: r.Model, Tokens: r.Tokens,
		}
	}
	c.usage.RecordResponses(k.project, rs)
}

// UsageInput is a Usage read: one Project, or every Project (Archived
// ones too) when Project is "", over the last Days local days.
type UsageInput struct {
	Project string
	// Days is one of UsageDays; 0 is the default.
	Days int
}

// UsageView is Usage as the read returns it.
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

// UsageKind is one kind of session's Context Size figures: how many
// sessions there were, and the average and largest of their peaks. The
// average is rounded to the nearest token.
type UsageKind struct {
	Kind        string `json:"kind"`
	Sessions    int    `json:"sessions"`
	AveragePeak int    `json:"average_peak"`
	LargestPeak int    `json:"largest_peak"`
}

// UsageSession is one recent session: a Claim's main session, or a
// subagent of it (Agent), with the Project, Ticket, Claim and kind of its
// latest response. Started is its first response's time, Peak the largest
// Context Size among its responses, and Models each distinct model its
// responses name, in order of first use.
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
	Models    []string  `json:"models"`
}

// UsageDay is one local day's token usage, by kind of session: the kinds
// with any usage that day, in kindOrder; none on a day with no usage.
type UsageDay struct {
	Day   string           `json:"day"`
	Kinds []UsageDayTokens `json:"kinds"`
}

// UsageDayTokens is one kind's tokens on one day, by token type.
type UsageDayTokens struct {
	Kind string `json:"kind"`
	usage.Tokens
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
	sessions := usageSessions(recs.Responses)
	recent := sessions[:min(len(sessions), RecentSessions)]
	return UsageView{
		Operations: operations(recs.Calls),
		Kinds:      kinds(sessions),
		Sessions:   recent,
		Days:       usageDays(recs.Days, recs.Responses),
	}, nil
}

// SessionUsageInput is one session's read: its Project, its session ID,
// and its subagent, "" for the main session.
type SessionUsageInput struct {
	Project string
	Session string
	Agent   string
}

// SessionUsageView is one session's responses, oldest first, for its
// line chart.
type SessionUsageView struct {
	Responses []SessionUsageResponse `json:"responses"`
}

// SessionUsageResponse is one of a session's model responses: when it
// came, its Context Size and its four token counts.
type SessionUsageResponse struct {
	Time        time.Time `json:"time"`
	ContextSize int       `json:"context_size"`
	usage.Tokens
}

// SessionUsage reads one session's responses over every day Usage keeps
// (today and the usage.KeepDays before it), whatever range the rest of
// Usage reads. An unknown Project is
// not_found; an unknown session has no responses.
func (c *Core) SessionUsage(_ context.Context, in SessionUsageInput) (SessionUsageView, error) {
	if in.Project == "" {
		return SessionUsageView{}, invalid("project", "project is required")
	}
	if in.Session == "" {
		return SessionUsageView{}, invalid("session", "session is required")
	}
	if !c.projectExists(in.Project) {
		return SessionUsageView{}, &Error{Code: NotFound, Message: fmt.Sprintf("no Project %q", in.Project)}
	}
	var mine []usage.Response
	for _, r := range c.usage.Read(in.Project, usage.KeepDays+1).Responses {
		if r.SessionID == in.Session && r.Agent == in.Agent {
			mine = append(mine, r)
		}
	}
	slices.SortStableFunc(mine, func(a, b usage.Response) int { return a.Time.Compare(b.Time) })
	out := SessionUsageView{Responses: []SessionUsageResponse{}}
	for _, r := range mine {
		out.Responses = append(out.Responses, SessionUsageResponse{Time: r.Time, ContextSize: r.ContextSize(), Tokens: r.Tokens})
	}
	return out, nil
}

// RecentSessions is how many sessions a Usage read lists.
const RecentSessions = 50

// usageSessions are the sessions the responses belong to, newest first.
// A session is its session ID plus its agent, so each subagent is one of
// its own, and a session resumed under a newer Claim stays one session.
// The responses are taken oldest first, whatever order they were read in,
// so each session's models are in order of first use.
func usageSessions(responses []usage.Response) []UsageSession {
	responses = slices.Clone(responses)
	slices.SortStableFunc(responses, func(a, b usage.Response) int { return a.Time.Compare(b.Time) })
	type key struct{ session, agent string }
	byKey := map[key]*UsageSession{}
	latest := map[key]time.Time{}
	for _, r := range responses {
		k := key{r.SessionID, r.Agent}
		s := byKey[k]
		if s == nil {
			s = &UsageSession{SessionID: r.SessionID, Agent: r.Agent, Started: r.Time, Models: []string{}}
			byKey[k] = s
		}
		s.Responses++
		s.Peak = max(s.Peak, r.ContextSize())
		if r.Model != "" && !slices.Contains(s.Models, r.Model) {
			s.Models = append(s.Models, r.Model)
		}
		if r.Time.Before(s.Started) {
			s.Started = r.Time
		}
		if last, ok := latest[k]; !ok || !r.Time.Before(last) {
			latest[k] = r.Time
			s.Project, s.Ticket, s.Claim, s.Kind = r.Project, r.Ticket, r.Claim, r.Kind
		}
	}
	sessions := []UsageSession{}
	for _, s := range byKey {
		sessions = append(sessions, *s)
	}
	slices.SortFunc(sessions, func(a, b UsageSession) int {
		return cmp.Or(b.Started.Compare(a.Started), cmp.Compare(a.SessionID, b.SessionID), cmp.Compare(a.Agent, b.Agent))
	})
	return sessions
}

// usageDays is a row for each of days, oldest first, with the responses'
// tokens on that day summed by kind.
func usageDays(days []string, responses []usage.Response) []UsageDay {
	byDay := map[string]map[string]*UsageDayTokens{}
	for _, r := range responses {
		kinds := byDay[r.Day]
		if kinds == nil {
			kinds = map[string]*UsageDayTokens{}
			byDay[r.Day] = kinds
		}
		k := kinds[r.Kind]
		if k == nil {
			k = &UsageDayTokens{Kind: r.Kind}
			kinds[r.Kind] = k
		}
		k.Add(r.Tokens)
	}
	rows := make([]UsageDay, len(days))
	for i, d := range days {
		rows[i] = UsageDay{Day: d, Kinds: []UsageDayTokens{}}
		for _, k := range byDay[d] {
			rows[i].Kinds = append(rows[i].Kinds, *k)
		}
		slices.SortFunc(rows[i].Kinds, func(a, b UsageDayTokens) int { return compareKinds(a.Kind, b.Kind) })
	}
	return rows
}

// kindOrder is the order kinds are listed in, as the Context Report
// lists them; any other kind follows, by name.
var kindOrder = []string{"grilling", "spec", "tickets", "implementation", "review", "subagent"}

// kinds is each kind of session's Context Size figures, in kindOrder.
func kinds(sessions []UsageSession) []UsageKind {
	byKind := map[string]*UsageKind{}
	total := map[string]int{}
	for _, s := range sessions {
		k := byKind[s.Kind]
		if k == nil {
			k = &UsageKind{Kind: s.Kind}
			byKind[s.Kind] = k
		}
		k.Sessions++
		k.LargestPeak = max(k.LargestPeak, s.Peak)
		total[s.Kind] += s.Peak
	}
	rows := []UsageKind{}
	for _, k := range byKind {
		k.AveragePeak = average(total[k.Kind], k.Sessions)
		rows = append(rows, *k)
	}
	slices.SortFunc(rows, func(a, b UsageKind) int { return compareKinds(a.Kind, b.Kind) })
	return rows
}

// compareKinds orders kinds by kindOrder, then any other by name.
func compareKinds(a, b string) int {
	rank := func(kind string) int {
		if i := slices.Index(kindOrder, kind); i >= 0 {
			return i
		}
		return len(kindOrder)
	}
	return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(a, b))
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
