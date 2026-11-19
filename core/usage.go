package core

import (
	"cmp"
	"context"
	"fmt"
	"slices"

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
func UsageDays() []int { return []int{7, 30, 90} }

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

// PostUsage is POST /api/usage {session_id, responses}: it records the
// responses under the newest Claim with that session ID, live or ended,
// as Session Events are filed, and counts as a ping. With no such Claim
// it records them under the current Conversation with that session ID,
// with no Ticket and Claim 0, so they count at Project scope only; with
// neither they are dropped. A response with no time is dated now. The
// models they name join the Project's Reported Models. It never fails.
func (c *Core) PostUsage(ctx context.Context, sessionID string, responses []UsageResponse) {
	if sessionID == "" {
		return
	}
	c.Ping(ctx, sessionID)
	if len(responses) == 0 {
		return
	}
	k, _, ok := c.newestClaim(sessionID)
	if !ok {
		// A Conversation's session's usage counts at Project scope:
		// no Ticket and Claim 0 (Spec code-central-633).
		if k.project, ok = c.conversationProject(sessionID); !ok {
			return
		}
	}
	c.addModelIDs(k.project, responses)
	if c.usage == nil {
		return
	}
	ticket := ""
	if k.ticket != 0 {
		ticket = DisplayID(k.project, k.ticket)
	}
	rs := make([]usage.Response, len(responses))
	for i, r := range responses {
		if r.Time.IsZero() {
			r.Time = c.now()
		}
		rs[i] = usage.Response{
			Time: r.Time, Ticket: ticket, Claim: k.claim,
			SessionID: sessionID, Agent: r.Agent, AgentType: r.AgentType, Description: r.Description, Kind: r.Kind, Model: r.Model, Tokens: r.Tokens,
		}
	}
	c.usage.RecordResponses(k.project, rs)
}

// Usage reads Usage. An unknown Project is not_found, and a Days outside
// UsageDays is invalid.
func (c *Core) Usage(_ context.Context, in UsageInput) (UsageView, error) {
	if in.Days == 0 {
		in.Days = UsageDays()[0]
	}
	if !slices.Contains(UsageDays(), in.Days) {
		return UsageView{}, invalid("days", "days must be 7, 30 or 90, not %d", in.Days)
	}
	if in.Project != "" && !c.projectExists(in.Project) {
		return UsageView{}, &Error{Code: NotFound, Message: fmt.Sprintf("no Project %q", in.Project)}
	}
	recs := c.usage.Read(in.Project, in.Days)
	all, mains := usageSessions(recs.Responses)
	return UsageView{
		Operations: operations(recs.Calls),
		Kinds:      kinds(all),
		Sessions:   mains[:min(len(mains), RecentSessions)],
		Days:       usageDays(recs.Days, recs.Responses),
	}, nil
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
		out.AgentType = cmp.Or(r.AgentType, out.AgentType)
		out.Description = cmp.Or(r.Description, out.Description)
		out.Responses = append(out.Responses, SessionUsageResponse{Time: r.Time, ContextSize: r.ContextSize(), Tokens: r.Tokens})
	}
	return out, nil
}

// RecentSessions is how many main sessions a Usage read lists, each with
// all of its Subagents.
const RecentSessions = 50

// usageSessions are the sessions the responses belong to: all of them,
// main sessions and Subagents alike, in no order, and the main sessions,
// newest first, each with its Subagents. A session is its session ID
// plus its agent, so each Subagent is one of its own, and a session
// resumed under a newer Claim stays one session. The responses are taken
// oldest first, whatever order they were read in, so each session starts
// at its first, takes its Claim and kind from its latest, and lists its
// models in order of first use.
func usageSessions(responses []usage.Response) (all, mains []UsageSession) {
	responses = slices.Clone(responses)
	slices.SortStableFunc(responses, func(a, b usage.Response) int { return a.Time.Compare(b.Time) })
	type key struct{ session, agent string }
	byKey := map[key]*UsageSession{}
	// latestSubagent is each session ID's latest Subagent response.
	latestSubagent := map[string]usage.Response{}
	for _, r := range responses {
		k := key{r.SessionID, r.Agent}
		s := byKey[k]
		if s == nil {
			s = &UsageSession{SessionID: r.SessionID, Agent: r.Agent, Started: r.Time, Models: []string{}, Subagents: []UsageSession{}}
			byKey[k] = s
		}
		s.Responses++
		s.Peak = max(s.Peak, r.ContextSize())
		s.Tokens += r.Total()
		if r.Model != "" && !slices.Contains(s.Models, r.Model) {
			s.Models = append(s.Models, r.Model)
		}
		s.Project, s.Ticket, s.Claim, s.Kind = r.Project, r.Ticket, r.Claim, r.Kind
		if r.Agent != "" {
			s.AgentType = cmp.Or(r.AgentType, s.AgentType)
			s.Description = cmp.Or(r.Description, s.Description)
			latestSubagent[r.SessionID] = r
		}
	}

	byID := map[string]*UsageSession{}
	for k, s := range byKey {
		all = append(all, *s)
		if k.agent == "" {
			byID[k.session] = s
		}
	}
	// A main session with no responses of its own in the range, because
	// its turn started before it.
	for id, r := range latestSubagent {
		if byID[id] == nil {
			byID[id] = &UsageSession{Project: r.Project, Ticket: r.Ticket, Claim: r.Claim, SessionID: id, Models: []string{}, Subagents: []UsageSession{}}
		}
	}
	for k, s := range byKey {
		if k.agent != "" {
			m := byID[k.session]
			m.Subagents = append(m.Subagents, *s)
		}
	}
	mains = []UsageSession{}
	for _, m := range byID {
		slices.SortFunc(m.Subagents, func(a, b UsageSession) int {
			return cmp.Or(a.Started.Compare(b.Started), cmp.Compare(a.Agent, b.Agent))
		})
		if m.Responses == 0 {
			m.Started = m.Subagents[0].Started
		}
		total := m.Tokens
		for _, s := range m.Subagents {
			total += s.Tokens
		}
		m.TotalTokens = &total
		mains = append(mains, *m)
	}
	slices.SortFunc(mains, func(a, b UsageSession) int {
		return cmp.Or(b.Started.Compare(a.Started), cmp.Compare(a.SessionID, b.SessionID))
	})
	return all, mains
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
