// Package core holds every domain rule behind one list of named
// operations (see Operations). Front doors (httpapi, mcp, the CLI via
// client) only translate onto these methods.
//
// Every write runs inside one store.Update, so the check and the write
// are atomic under the store-wide lock, and writes exactly one file.
package core

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
)

// Core is the domain. Build one with New.
type Core struct {
	store  store.Store
	clock  clock.Clock
	ids    ids.Source
	events *events.Bus
	// activity is the in-memory last-activity record behind Silent Claims.
	activity *activity
	// transcripts holds each Claim's Session Events, in memory only.
	transcripts *transcripts
	// latestNotices throttles the ticket.changed a new latest event sends.
	latestNotices *latestNotices
	// usage records agents' MCP calls (Usage); nil records nothing.
	usage *usage.Recorder
	// ciCheck is the CI Watcher's check mode, run when CI Tracking is
	// turned on; nil checks nothing.
	ciCheck CICheck
	// issuesCheck is the Issue Watcher's check mode, run when GitHub
	// Issues is turned on; nil checks nothing.
	issuesCheck IssuesCheck
	// watchers holds each Project's Watchers' statuses, in memory only.
	watchers *watchers
	// heartbeats holds each Factory's latest Heartbeat, in memory only.
	heartbeats *heartbeats
	// uiURL is the UI's address, which a comment written to an issue
	// links back to; empty links relative to the UI.
	uiURL string
}

// New returns a Core persisting through s, telling time with clk and
// issuing Claim IDs from idSource. Every write sends a change notice on
// Events.
func New(s store.Store, clk clock.Clock, idSource ids.Source) *Core {
	bus := events.New()
	c := &Core{store: notifying{Store: archiveGuard{s}, bus: bus}, clock: clk, ids: idSource, events: bus}
	c.activity = newActivity(c.now())
	c.transcripts = &transcripts{all: map[markKey]*transcript{}, feeds: map[markKey]map[*TranscriptFeed]struct{}{}}
	c.latestNotices = &latestNotices{sent: map[markKey]time.Time{}, pending: map[markKey]bool{}}
	c.watchers = &watchers{all: map[watcherKey]watcherStatus{}}
	c.heartbeats = &heartbeats{all: map[heartbeatKey]heartbeat{}, seen: map[string]string{}}
	return c
}

// now is the current time as stored: UTC, whole seconds.
func (c *Core) now() time.Time { return c.clock.Now().UTC().Truncate(time.Second) }

// ProjectView is a Project as every read returns it.
type ProjectView struct {
	Key         string     `json:"key"`
	DisplayName string     `json:"display_name"`
	Created     time.Time  `json:"created"`
	Mode        store.Mode `json:"mode"`
	Archived    bool       `json:"archived"`
	// ArchivedAt is when the human archived the Project, while it is.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	// Landing is how the Project's accepted work lands, always filled in.
	Landing LandingView `json:"landing"`
	// CI is the Project's CI settings, always filled in.
	CI CIView `json:"ci"`
	// GitHubIssues is the Project's GitHub Issues settings, always filled
	// in.
	GitHubIssues GitHubIssuesView `json:"github_issues"`
	// Models is the Project's Models rows, always filled in.
	Models ModelsView `json:"models"`
	// ReportedModels is what the Factory last reported its claude
	// accepts, always filled in.
	ReportedModels ReportedModelsView `json:"reported_models"`
	// FactoryStatus is whether the Project's Factories are online, from
	// their Heartbeats, always filled in.
	FactoryStatus FactoryStatusView `json:"factory_status"`
}

// LandingView is a Project's Landing Method with its Push option.
type LandingView struct {
	Method store.LandingMethod `json:"method"`
	Push   bool                `json:"push"`
	Remote string              `json:"remote"`
	// LandPerSpec is the Merge option to land a Spec's work together,
	// shown whatever the method.
	LandPerSpec bool `json:"land_per_spec"`
}

// SpecView is a Spec as every read returns it, and as its writes return
// it with full.
type SpecView struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Labels  []string          `json:"labels"`
	Tags    map[string]string `json:"tags"`
	Created time.Time         `json:"created"`
	Updated time.Time         `json:"updated"`
	Body    string            `json:"body"`
	Edits   []EditView        `json:"edits"`
	// Link is set on a linked Spec (ticket 42).
	Link *LinkView `json:"link,omitempty"`
}

// SpecRef names a Ticket's Spec.
type SpecRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// MoveView is one State move. An imported Ticket's one move has no From.
type MoveView struct {
	From  store.State `json:"from,omitempty"`
	To    store.State `json:"to"`
	Actor string      `json:"actor"`
	Time  time.Time   `json:"time"`
	Claim int         `json:"claim,omitempty"`
	Via   string      `json:"via,omitempty"`
	// Commit is the commit that put the work into the base, on a land.
	Commit string `json:"commit,omitempty"`
}

// ClaimView is one Claim as reads show it: never with its Claim ID.
type ClaimView struct {
	Number    int    `json:"number"`
	SessionID string `json:"session_id,omitempty"`
	// WorkingDir and ResumeCommand are as the holder last set them, empty
	// when unset: never a filled-in default.
	WorkingDir    string `json:"working_dir,omitempty"`
	ResumeCommand string `json:"resume_command,omitempty"`
	// Model and Effort are what the Claim's latest turn ran with, empty
	// meaning Claude Code's default.
	Model   string    `json:"model,omitempty"`
	Effort  string    `json:"effort,omitempty"`
	Started time.Time `json:"started"`
	// LastActivity is the latest write the Claim's agent made on the Ticket.
	LastActivity time.Time  `json:"last_activity"`
	Ended        *time.Time `json:"ended,omitempty"`
	EndedBy      string     `json:"ended_by,omitempty"`
	// LatestEvent is the Claim's latest Session Event, shown short; left
	// out when it has none. Computed, never stored.
	LatestEvent *LatestEvent `json:"latest_event,omitempty"`
}

// TicketView is the full view of one Ticket, as its writes return it with
// full. A read (get_ticket, claim) may leave parts out: each part left out
// is nil, so it has no key, and LeftOut counts what it had.
type TicketView struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	State        store.State       `json:"state"`
	Spec         SpecRef           `json:"spec"`
	Labels       []string          `json:"labels"`
	Tags         map[string]string `json:"tags"`
	BlockedBy    []string          `json:"blocked_by"`
	Blockers     []BlockerView     `json:"blockers"`
	Replacements []string          `json:"replacements"`
	Created      time.Time         `json:"created"`
	Updated      time.Time         `json:"updated"`
	Description  string            `json:"description"`
	// Branch and Commit are the work's branch and head commit, as the
	// last finish naming them gave them.
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	// PushedCommit is the latest commit the Factory pushed for the Ticket
	// while CI Tracking was on, and CIResult the CI result of its runs;
	// both left out while it has none. CIRuns are every CI Run tracked
	// against it, the latest pushed commit's first. Computed on read.
	PushedCommit *PushedCommitView `json:"pushed_commit,omitempty"`
	CIResult     CIResult          `json:"ci_result,omitempty"`
	CIRuns       []CIRunView       `json:"ci_runs,omitempty"`
	// LandAnyway says the human accepted the latest pushed commit's
	// failed CI result, so the Factory lands it (code-central-261).
	LandAnyway bool `json:"land_anyway,omitempty"`
	// CIFailureOptions are the failure options, each an operation's name,
	// the human may choose while the Ticket is CI failed; left out
	// otherwise, and empty when none applies. Computed on read.
	CIFailureOptions []string            `json:"ci_failure_options,omitzero"`
	Claims           []ClaimView         `json:"claims,omitzero"`
	Rounds           []RoundView         `json:"rounds,omitzero"`
	FailureReports   []FailureReportView `json:"failure_reports,omitzero"`
	Comments         []CommentView       `json:"comments,omitzero"`
	Moves            []MoveView          `json:"moves,omitzero"`
	Edits            []EditView          `json:"edits,omitzero"`
	// LeftOut counts each part a read left out that has anything, by
	// part name.
	LeftOut   map[string]int `json:"left_out,omitempty"`
	Blocked   bool           `json:"blocked"`
	Claimable bool           `json:"claimable"`
	// SilentSince is set while the Ticket is a Silent Claim: In Progress
	// with no activity for 30 minutes. It is the time of the last
	// activity, so the quiet time is now minus it. Computed, never stored.
	SilentSince *time.Time `json:"silent_since,omitempty"`
	// NeedsMeReason says why the Ticket waits on the human, if it does.
	NeedsMeReason NeedsMeReason `json:"needs_me_reason,omitempty"`
	// Link is set on a linked Ticket (ticket 42).
	Link *LinkView `json:"link,omitempty"`
	// AwaitingTriage is set on an Incoming Issue that came in while New
	// issues need me was on and that the human hasn't triaged yet.
	AwaitingTriage bool `json:"awaiting_triage,omitempty"`
	// TriageActions are the triage actions, each an operation's name, the
	// human may take on an Incoming Issue; left out on any other Ticket.
	// Computed on read.
	TriageActions []string `json:"triage_actions,omitzero"`
	// ModelChoice is the model and effort the Ticket's next turn runs
	// with, each with its source; only the UI's read carries it.
	ModelChoice *ModelChoice `json:"model_choice,omitempty"`
}

// TicketRow is one find_tickets result.
type TicketRow struct {
	ID      string      `json:"id"`
	Title   string      `json:"title"`
	State   store.State `json:"state"`
	Spec    string      `json:"spec"`
	Labels  []string    `json:"labels"`
	Blocked bool        `json:"blocked"`
	Updated time.Time   `json:"updated"`
	Link    *LinkView   `json:"link,omitempty"`
	// NodeType is the Ticket's type on a Map (see NodeType); left out for
	// a Ticket on any other Spec.
	NodeType string `json:"node_type,omitempty"`
	// BlockedBy is the Ticket's Blockers on a Map, the Map view's edges;
	// left out for a Ticket on any other Spec.
	BlockedBy []string `json:"blocked_by,omitempty"`
	// LatestEvent is the latest Session Event of the Ticket's live Claim,
	// shown short; left out when there is none.
	LatestEvent *LatestEvent `json:"latest_event,omitempty"`
	// CIResult is the CI result of the Ticket's latest pushed commit, for
	// its row's badge; left out for a Ticket never pushed while CI
	// Tracking was on.
	CIResult CIResult `json:"ci_result,omitempty"`
}

// CreateProjectInput is create_project's input.
type CreateProjectInput struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name,omitempty"`
	// Mode is committed (the default) or local.
	Mode store.Mode `json:"mode,omitempty"`
}

// CreateSpecInput is create_spec's input.
type CreateSpecInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// Full asks for the Spec in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// CreateTicketInput is create_ticket's input. State defaults to Draft.
type CreateTicketInput struct {
	Spec        Ref         `json:"spec"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	State       store.State `json:"state,omitempty"`
	Labels      []string    `json:"labels,omitempty"`
	BlockedBy   []Ref       `json:"blocked_by,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// FindTicketsInput is find_tickets' filters; every filter given must match.
type FindTicketsInput struct {
	State  []store.State `json:"state,omitempty"`
	Title  string        `json:"title,omitempty"`
	Labels []string      `json:"labels,omitempty"`
	// Tags must all be present with exactly these values.
	Tags map[string]string `json:"tags,omitempty"`
	Spec Ref               `json:"spec,omitempty"`
	// Claimable, when given, keeps only Tickets that are (or aren't) claimable.
	Claimable *bool `json:"claimable,omitempty"`
}

var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// CreateProject is create_project(key, display_name). The display name
// defaults to the key.
func (c *Core) CreateProject(_ context.Context, in CreateProjectInput) (ProjectView, error) {
	if !keyPattern.MatchString(in.Key) {
		return ProjectView{}, invalid("key", "a Project Key is lowercase letters, digits, '.', '_' and '-', starting with a letter or digit")
	}
	name := in.DisplayName
	if name == "" {
		name = in.Key
	}
	mode := in.Mode
	if mode == "" {
		mode = ModeCommitted
	}
	if !mode.Valid() {
		return ProjectView{}, invalidMode(mode)
	}
	p := store.Project{Key: in.Key, DisplayName: name, Created: c.now(), Mode: mode}
	err := c.store.Update(func(w store.Writer) error {
		if _, ok := w.Project(in.Key); ok {
			return invalid("key", "a Project with key %q already exists", in.Key)
		}
		return w.PutProject(p)
	})
	if err != nil {
		return ProjectView{}, err
	}
	return c.projectView(p), nil
}

// GetProject is get_project(key).
func (c *Core) GetProject(_ context.Context, key string) (ProjectView, error) {
	var v ProjectView
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		v = c.projectView(p)
		return nil
	})
	return v, err
}

// CreateSpec is create_spec(title, body, full?).
func (c *Core) CreateSpec(_ context.Context, key string, in CreateSpecInput) (Written[SpecView], error) {
	if strings.TrimSpace(in.Title) == "" {
		return Written[SpecView]{}, invalid("title", "a Spec needs a title")
	}
	var v Written[SpecView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		now := c.now()
		s := store.Spec{Number: w.NextNumber(key), Title: in.Title, Created: now, Updated: now, Body: in.Body}
		if err := w.PutSpec(key, s); err != nil {
			return err
		}
		var err error
		v, err = acknowledgeCreate(specView(key, store.Spec{}), specView(key, s), in.Full)
		return err
	})
	return v, err
}

// CreateTicket is create_ticket(spec, title, description, state, labels,
// blocked_by, full?).
func (c *Core) CreateTicket(_ context.Context, key string, in CreateTicketInput) (Written[TicketView], error) {
	if strings.TrimSpace(in.Title) == "" {
		return Written[TicketView]{}, invalid("title", "a Ticket needs a title")
	}
	state := in.State
	if state == "" {
		state = store.Draft
	}
	if state != store.Draft && state != store.Ready {
		return Written[TicketView]{}, invalid("state", "a new Ticket is Draft or Ready, not %q", state)
	}
	if in.Spec == "" {
		return Written[TicketView]{}, invalid("spec", "every Ticket belongs to a Spec")
	}
	if err := checkTicketLabels("labels", in.Labels); err != nil {
		return Written[TicketView]{}, err
	}
	var v Written[TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		sp, err := resolveSpec(w, key, "create_ticket", "spec", in.Spec)
		if err != nil {
			return err
		}
		specN := sp.Number
		// A new Ticket has no dependants yet, so its Blockers can't form a cycle.
		blockedBy, err := ticketRefs(w, key, "create_ticket", "blocked_by", in.BlockedBy)
		if err != nil {
			return err
		}
		now := c.now()
		t := store.Ticket{
			Number: w.NextNumber(key), Title: in.Title, State: state, Spec: specN,
			Labels: in.Labels, BlockedBy: blockedBy, Created: now, Updated: now, Description: in.Description,
		}
		if err := w.PutTicket(key, t); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, store.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}

// GetTicket is get_ticket(ticket, parts?): the Ticket with the parts asked
// for, or its default parts.
func (c *Core) GetTicket(_ context.Context, key string, in GetTicketInput) (TicketView, error) {
	parts, err := in.parts()
	if err != nil {
		return TicketView{}, err
	}
	var v TicketView
	err = c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		t, err := resolveTicket(r, key, "get_ticket", "ticket", in.Ticket)
		if err != nil {
			return err
		}
		if v = c.ticketView(r, key, t); len(in.Parts) == 0 {
			v = defaultView(v)
		} else {
			v = v.only(parts)
		}
		if in.WithModelChoice {
			mc := modelChoice(p, t)
			v.ModelChoice = &mc
		}
		return nil
	})
	return v, err
}

// FindTickets is find_tickets(state?, title?, labels?, tags?, spec?, claimable?). Done and
// Cancelled Tickets are left out unless the State filter names them.
// Rows are in claim order: created, then number.
func (c *Core) FindTickets(_ context.Context, key string, in FindTicketsInput) ([]TicketRow, error) {
	for _, s := range in.State {
		if !s.Valid() {
			return nil, invalid("state", "unknown State %q", s)
		}
	}
	if err := checkTagFilter(in.Tags); err != nil {
		return nil, err
	}
	rows := []TicketRow{}
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		specN := 0
		if in.Spec != "" {
			n, err := in.Spec.number(key, "spec")
			if err != nil {
				return err
			}
			// A Ticket is rejected naming its Spec; a Spec that doesn't
			// exist matches no Tickets.
			if err := ticketNotSpec(r, key, "find_tickets", "spec", n); err != nil {
				return err
			}
			specN = n
		}
		maps := map[int]bool{}
		for _, s := range r.Specs(key) {
			maps[s.Number] = isMap(s.Labels)
		}
		tickets := r.Tickets(key)
		slices.SortStableFunc(tickets, claimOrder)
		for _, t := range tickets {
			if !matches(t, in, specN) {
				continue
			}
			row := c.ticketRow(r, key, t, maps[t.Spec])
			if in.Claimable != nil && *in.Claimable != (t.State == store.Ready && !row.Blocked) {
				continue
			}
			rows = append(rows, row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func matches(t store.Ticket, in FindTicketsInput, specN int) bool {
	if len(in.State) > 0 {
		if !slices.Contains(in.State, t.State) {
			return false
		}
	} else if finished(t.State) {
		return false
	}
	if in.Title != "" && !strings.Contains(strings.ToLower(t.Title), strings.ToLower(in.Title)) {
		return false
	}
	if !hasAll(t.Labels, t.Tags, in.Labels, in.Tags) {
		return false
	}
	return specN == 0 || t.Spec == specN
}

// hasAll reports whether every wanted Label is present and every wanted
// Tag pair matches exactly.
func hasAll(labels []string, tags map[string]string, wantLabels []string, wantTags map[string]string) bool {
	for _, l := range wantLabels {
		if !slices.Contains(labels, l) {
			return false
		}
	}
	for k, v := range wantTags {
		if got, ok := tags[k]; !ok || got != v {
			return false
		}
	}
	return true
}

// ticketRow is t as a find_tickets row; onMap says t is on a Map, so the
// row carries its node type.
func (c *Core) ticketRow(r store.Reader, key string, t store.Ticket, onMap bool) TicketRow {
	row := TicketRow{
		ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Spec: DisplayID(key, t.Spec),
		Labels: orEmpty(t.Labels), Blocked: c.blockRule(r, key).blocked(t), Updated: t.Updated, Link: linkView(t.Link),
		CIResult: ticketCIResult(r, key, t, c.now()),
	}
	if onMap {
		row.NodeType = NodeType(t.Labels)
		row.BlockedBy = displayIDs(key, t.BlockedBy)
	}
	if cl, ok := liveClaim(t); ok {
		row.LatestEvent = c.latestEvent(key, t.Number, cl.Number)
	}
	return row
}

func claimOrder(a, b store.Ticket) int {
	if c := a.Created.Compare(b.Created); c != 0 {
		return c
	}
	return cmp.Compare(a.Number, b.Number)
}

func finished(s store.State) bool { return s == store.Done || s == store.Cancelled }

func (c *Core) projectView(p store.Project) ProjectView {
	v := ProjectView{Key: p.Key, DisplayName: p.DisplayName, Created: p.Created, Mode: p.Mode,
		Landing: LandingView{Method: p.Landing.Method, Push: p.Landing.Push, Remote: p.Landing.Remote, LandPerSpec: p.Landing.LandPerSpec}, CI: ciView(p.Key, p.CI, c.watchers.view(watcherKey{WatcherCI, p.Key})),
		GitHubIssues: githubIssuesView(p.Key, p.GitHubIssues, c.watchers.view(watcherKey{WatcherIssues, p.Key})), Models: modelsView(p.Models),
		ReportedModels: reportedModelsView(p.ReportedModels), FactoryStatus: c.heartbeats.view(p.Key, c.clock.Now())}
	if !p.Archived.IsZero() {
		at := p.Archived
		v.Archived, v.ArchivedAt = true, &at
	}
	return v
}

func specView(key string, s store.Spec) SpecView {
	return SpecView{
		ID: DisplayID(key, s.Number), Title: s.Title, Labels: orEmpty(s.Labels), Tags: orEmptyMap(s.Tags),
		Created: s.Created, Updated: s.Updated, Body: s.Body, Edits: editViews(s.Edits), Link: linkView(s.Link),
	}
}

func (c *Core) ticketView(r store.Reader, key string, t store.Ticket) TicketView {
	sp, _ := r.Spec(key, t.Spec)
	rule := c.blockRule(r, key)
	v := TicketView{
		ID: DisplayID(key, t.Number), Title: t.Title, State: t.State,
		Spec:   SpecRef{ID: DisplayID(key, t.Spec), Title: sp.Title},
		Labels: orEmpty(t.Labels), Tags: orEmptyMap(t.Tags),
		BlockedBy: displayIDs(key, t.BlockedBy), Replacements: displayIDs(key, t.Replacements),
		Blockers: rule.blockerViews(t),
		Created:  t.Created, Updated: t.Updated, Description: t.Description,
		Branch: t.Branch, Commit: t.Commit,
		Claims: []ClaimView{}, Rounds: roundViews(t.Rounds), FailureReports: failureReportViews(t.FailureReports),
		Comments: commentViews(t.Comments), Moves: []MoveView{}, Edits: editViews(t.Edits),
		Blocked: rule.blocked(t), Link: linkView(t.Link),
	}
	for _, cl := range t.Claims {
		cv := ClaimView{Number: cl.Number, SessionID: cl.SessionID, WorkingDir: cl.WorkingDir, ResumeCommand: cl.ResumeCommand, Model: cl.Model, Effort: cl.Effort, Started: cl.Started, LastActivity: c.claimActivity(t, cl), EndedBy: cl.EndedBy,
			LatestEvent: c.latestEvent(key, t.Number, cl.Number)}
		if !cl.Live() {
			ended := cl.Ended
			cv.Ended = &ended
		}
		v.Claims = append(v.Claims, cv)
	}
	for _, m := range t.Moves {
		v.Moves = append(v.Moves, MoveView{From: m.From, To: m.To, Actor: m.Actor, Time: m.Time, Claim: m.Claim, Via: m.Via, Commit: m.Commit})
	}
	v.PushedCommit, v.CIResult, v.CIRuns = ticketCI(r, key, t, c.now())
	v.LandAnyway, v.CIFailureOptions = choseLandAnyway(t), ciFailureOptions(r, key, t, c.now())
	v.Claimable = t.State == store.Ready && !v.Blocked
	v.SilentSince = c.silentSince(t)
	v.NeedsMeReason, _ = c.reasonOf(r, key, t)
	v.AwaitingTriage, v.TriageActions = t.AwaitingTriage, ticketTriageActions(r, key, t)
	return v
}

func displayIDs(key string, ns []int) []string {
	out := []string{}
	for _, n := range ns {
		out = append(out, DisplayID(key, n))
	}
	return out
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
