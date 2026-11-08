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
	WorkingDir    string    `json:"working_dir,omitempty"`
	ResumeCommand string    `json:"resume_command,omitempty"`
	Started       time.Time `json:"started"`
	// LastActivity is the latest write the Claim's agent made on the Ticket.
	LastActivity time.Time  `json:"last_activity"`
	Ended        *time.Time `json:"ended,omitempty"`
	EndedBy      string     `json:"ended_by,omitempty"`
	// LatestEvent is the Claim's latest Session Event, shown short; left
	// out when it has none. Computed, never stored.
	LatestEvent *LatestEvent `json:"latest_event,omitempty"`
}

// TicketView is the full view of one Ticket, the same through every door.
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
	Branch         string              `json:"branch,omitempty"`
	Commit         string              `json:"commit,omitempty"`
	Claims         []ClaimView         `json:"claims"`
	Rounds         []RoundView         `json:"rounds"`
	FailureReports []FailureReportView `json:"failure_reports"`
	Comments       []CommentView       `json:"comments"`
	Moves          []MoveView          `json:"moves"`
	Edits          []EditView          `json:"edits"`
	Blocked        bool                `json:"blocked"`
	Claimable      bool                `json:"claimable"`
	// SilentSince is set while the Ticket is a Silent Claim: In Progress
	// with no activity for 30 minutes. It is the time of the last
	// activity, so the quiet time is now minus it. Computed, never stored.
	SilentSince *time.Time `json:"silent_since,omitempty"`
	// NeedsMeReason says why the Ticket waits on the human, if it does.
	NeedsMeReason NeedsMeReason `json:"needs_me_reason,omitempty"`
	// Link is set on a linked Ticket (ticket 42).
	Link *LinkView `json:"link,omitempty"`
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
	// LatestEvent is the latest Session Event of the Ticket's live Claim,
	// shown short; left out when there is none.
	LatestEvent *LatestEvent `json:"latest_event,omitempty"`
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
	return projectView(p), nil
}

// GetProject is get_project(key).
func (c *Core) GetProject(_ context.Context, key string) (ProjectView, error) {
	var v ProjectView
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		v = projectView(p)
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

// CreateTicket is create_ticket(spec, title, description, state, labels).
func (c *Core) CreateTicket(_ context.Context, key string, in CreateTicketInput) (TicketView, error) {
	if strings.TrimSpace(in.Title) == "" {
		return TicketView{}, invalid("title", "a Ticket needs a title")
	}
	state := in.State
	if state == "" {
		state = store.Draft
	}
	if state != store.Draft && state != store.Ready {
		return TicketView{}, invalid("state", "a new Ticket is Draft or Ready, not %q", state)
	}
	if in.Spec == "" {
		return TicketView{}, invalid("spec", "every Ticket belongs to a Spec")
	}
	if err := checkStageLabels("labels", in.Labels); err != nil {
		return TicketView{}, err
	}
	var v TicketView
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
		v = c.ticketView(w, key, t)
		return nil
	})
	return v, err
}

// GetTicket is get_ticket(ticket).
func (c *Core) GetTicket(_ context.Context, key string, ref Ref) (TicketView, error) {
	var v TicketView
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		t, err := resolveTicket(r, key, "get_ticket", "ticket", ref)
		if err != nil {
			return err
		}
		v = c.ticketView(r, key, t)
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
			// A Spec that doesn't exist matches no Tickets.
			if err := ticketNotSpec(r, key, "find_tickets", "spec", n); err != nil {
				return err
			}
			specN = n
		}
		tickets := r.Tickets(key)
		slices.SortStableFunc(tickets, claimOrder)
		for _, t := range tickets {
			if !matches(t, in, specN) {
				continue
			}
			row := c.ticketRow(r, key, t)
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

func (c *Core) ticketRow(r store.Reader, key string, t store.Ticket) TicketRow {
	row := TicketRow{
		ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Spec: DisplayID(key, t.Spec),
		Labels: orEmpty(t.Labels), Blocked: blocked(r, key, t), Updated: t.Updated, Link: linkView(t.Link),
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

func projectView(p store.Project) ProjectView {
	v := ProjectView{Key: p.Key, DisplayName: p.DisplayName, Created: p.Created, Mode: p.Mode}
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
	v := TicketView{
		ID: DisplayID(key, t.Number), Title: t.Title, State: t.State,
		Spec:   SpecRef{ID: DisplayID(key, t.Spec), Title: sp.Title},
		Labels: orEmpty(t.Labels), Tags: orEmptyMap(t.Tags),
		BlockedBy: displayIDs(key, t.BlockedBy), Replacements: displayIDs(key, t.Replacements),
		Blockers: blockerViews(r, key, t),
		Created:  t.Created, Updated: t.Updated, Description: t.Description,
		Branch: t.Branch, Commit: t.Commit,
		Claims: []ClaimView{}, Rounds: roundViews(t.Rounds), FailureReports: failureReportViews(t.FailureReports),
		Comments: commentViews(t.Comments), Moves: []MoveView{}, Edits: editViews(t.Edits),
		Blocked: blocked(r, key, t), Link: linkView(t.Link),
	}
	for _, cl := range t.Claims {
		cv := ClaimView{Number: cl.Number, SessionID: cl.SessionID, WorkingDir: cl.WorkingDir, ResumeCommand: cl.ResumeCommand, Started: cl.Started, LastActivity: c.claimActivity(t, cl), EndedBy: cl.EndedBy,
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
	v.Claimable = t.State == store.Ready && !v.Blocked
	v.SilentSince = c.silentSince(t)
	v.NeedsMeReason, _ = c.reasonOf(r, key, t)
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
