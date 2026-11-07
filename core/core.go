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
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// Core is the domain. Build one with New.
type Core struct {
	store store.Store
	clock clock.Clock
	ids   ids.Source
}

// New returns a Core persisting through s, telling time with clk and
// issuing Claim IDs from idSource.
func New(s store.Store, clk clock.Clock, idSource ids.Source) *Core {
	return &Core{store: s, clock: clk, ids: idSource}
}

// now is the current time as stored: UTC, whole seconds.
func (c *Core) now() time.Time { return c.clock.Now().UTC().Truncate(time.Second) }

// ProjectView is a Project as every read returns it.
type ProjectView struct {
	Key         string    `json:"key"`
	DisplayName string    `json:"display_name"`
	Created     time.Time `json:"created"`
	Archived    bool      `json:"archived"`
}

// SpecView is a Spec as every read returns it.
type SpecView struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Labels  []string          `json:"labels"`
	Tags    map[string]string `json:"tags"`
	Created time.Time         `json:"created"`
	Updated time.Time         `json:"updated"`
	Body    string            `json:"body"`
	Edits   []EditView        `json:"edits"`
}

// SpecRef names a Ticket's Spec.
type SpecRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// MoveView is one State move.
type MoveView struct {
	From  store.State `json:"from"`
	To    store.State `json:"to"`
	Actor string      `json:"actor"`
	Time  time.Time   `json:"time"`
	Claim int         `json:"claim,omitempty"`
	Via   string      `json:"via,omitempty"`
}

// ClaimView is one Claim as reads show it: never with its Claim ID.
type ClaimView struct {
	Number    int       `json:"number"`
	SessionID string    `json:"session_id,omitempty"`
	Started   time.Time `json:"started"`
	// LastActivity is the latest write the Claim's agent made on the Ticket.
	LastActivity time.Time  `json:"last_activity"`
	Ended        *time.Time `json:"ended,omitempty"`
	EndedBy      string     `json:"ended_by,omitempty"`
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
	Replacements []string          `json:"replacements"`
	Created      time.Time         `json:"created"`
	Updated      time.Time         `json:"updated"`
	Description  string            `json:"description"`
	Claims       []ClaimView       `json:"claims"`
	// rounds (ticket 29) go here, before failure_reports.
	FailureReports []FailureReportView `json:"failure_reports"`
	Comments       []CommentView       `json:"comments"`
	Moves          []MoveView          `json:"moves"`
	Edits          []EditView          `json:"edits"`
	Blocked        bool                `json:"blocked"`
	Claimable      bool                `json:"claimable"`
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
}

// CreateProjectInput is create_project's input.
type CreateProjectInput struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name,omitempty"`
}

// CreateSpecInput is create_spec's input.
type CreateSpecInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// CreateTicketInput is create_ticket's input. State defaults to Draft.
type CreateTicketInput struct {
	Spec        Ref         `json:"spec"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	State       store.State `json:"state,omitempty"`
	Labels      []string    `json:"labels,omitempty"`
}

// FindTicketsInput is find_tickets' filters; every filter given must match.
type FindTicketsInput struct {
	State  []store.State `json:"state,omitempty"`
	Title  string        `json:"title,omitempty"`
	Labels []string      `json:"labels,omitempty"`
	Spec   Ref           `json:"spec,omitempty"`
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
	p := store.Project{Key: in.Key, DisplayName: name, Created: c.now(), Mode: "committed"}
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

// CreateSpec is create_spec(title, body).
func (c *Core) CreateSpec(_ context.Context, key string, in CreateSpecInput) (SpecView, error) {
	if strings.TrimSpace(in.Title) == "" {
		return SpecView{}, invalid("title", "a Spec needs a title")
	}
	var v SpecView
	err := c.store.Update(func(w store.Writer) error {
		if _, ok := w.Project(key); !ok {
			return unknownProject(key)
		}
		now := c.now()
		s := store.Spec{Number: w.NextNumber(key), Title: in.Title, Created: now, Updated: now, Body: in.Body}
		if err := w.PutSpec(key, s); err != nil {
			return err
		}
		v = specView(key, s)
		return nil
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
	var v TicketView
	err := c.store.Update(func(w store.Writer) error {
		if _, ok := w.Project(key); !ok {
			return unknownProject(key)
		}
		specN, err := in.Spec.number(key, "spec")
		if err != nil {
			return err
		}
		if _, ok := w.Spec(key, specN); !ok {
			return invalid("spec", "there is no Spec %s", DisplayID(key, specN))
		}
		now := c.now()
		t := store.Ticket{
			Number: w.NextNumber(key), Title: in.Title, State: state, Spec: specN,
			Labels: in.Labels, Created: now, Updated: now, Description: in.Description,
		}
		if err := w.PutTicket(key, t); err != nil {
			return err
		}
		v = ticketView(w, key, t)
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
		n, err := ref.number(key, "ticket")
		if err != nil {
			return err
		}
		t, ok := r.Ticket(key, n)
		if !ok {
			return notFound("there is no Ticket %s", DisplayID(key, n))
		}
		v = ticketView(r, key, t)
		return nil
	})
	return v, err
}

// FindTickets is find_tickets(state?, title?, labels?, spec?). Done and
// Cancelled Tickets are left out unless the State filter names them.
// Rows are in claim order: created, then number.
func (c *Core) FindTickets(_ context.Context, key string, in FindTicketsInput) ([]TicketRow, error) {
	for _, s := range in.State {
		if !s.Valid() {
			return nil, invalid("state", "unknown State %q", s)
		}
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
			specN = n
		}
		tickets := r.Tickets(key)
		slices.SortStableFunc(tickets, claimOrder)
		for _, t := range tickets {
			if !matches(t, in, specN) {
				continue
			}
			rows = append(rows, TicketRow{
				ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Spec: DisplayID(key, t.Spec),
				Labels: orEmpty(t.Labels), Blocked: blocked(r, key, t), Updated: t.Updated,
			})
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
	for _, l := range in.Labels {
		if !slices.Contains(t.Labels, l) {
			return false
		}
	}
	return specN == 0 || t.Spec == specN
}

func claimOrder(a, b store.Ticket) int {
	if c := a.Created.Compare(b.Created); c != 0 {
		return c
	}
	return cmp.Compare(a.Number, b.Number)
}

func finished(s store.State) bool { return s == store.Done || s == store.Cancelled }

// blocked is computed. Blockers arrive with a later ticket; for now a
// Ticket is never blocked.
func blocked(store.Reader, string, store.Ticket) bool { return false }

func projectView(p store.Project) ProjectView {
	return ProjectView{Key: p.Key, DisplayName: p.DisplayName, Created: p.Created, Archived: p.Archived}
}

func specView(key string, s store.Spec) SpecView {
	return SpecView{
		ID: DisplayID(key, s.Number), Title: s.Title, Labels: orEmpty(s.Labels), Tags: orEmptyMap(s.Tags),
		Created: s.Created, Updated: s.Updated, Body: s.Body, Edits: editViews(s.Edits),
	}
}

func ticketView(r store.Reader, key string, t store.Ticket) TicketView {
	sp, _ := r.Spec(key, t.Spec)
	v := TicketView{
		ID: DisplayID(key, t.Number), Title: t.Title, State: t.State,
		Spec:   SpecRef{ID: DisplayID(key, t.Spec), Title: sp.Title},
		Labels: orEmpty(t.Labels), Tags: orEmptyMap(t.Tags),
		BlockedBy: displayIDs(key, t.BlockedBy), Replacements: displayIDs(key, t.Replacements),
		Created: t.Created, Updated: t.Updated, Description: t.Description,
		Claims: []ClaimView{}, FailureReports: failureReportViews(t.FailureReports), Comments: commentViews(t.Comments), Moves: []MoveView{}, Edits: editViews(t.Edits),
		Blocked: blocked(r, key, t),
	}
	for _, c := range t.Claims {
		cv := ClaimView{Number: c.Number, SessionID: c.SessionID, Started: c.Started, LastActivity: lastActivity(t, c), EndedBy: c.EndedBy}
		if !c.Live() {
			ended := c.Ended
			cv.Ended = &ended
		}
		v.Claims = append(v.Claims, cv)
	}
	for _, m := range t.Moves {
		v.Moves = append(v.Moves, MoveView{From: m.From, To: m.To, Actor: m.Actor, Time: m.Time, Claim: m.Claim, Via: m.Via})
	}
	v.Claimable = t.State == store.Ready && !v.Blocked
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
