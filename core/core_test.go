package core_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

var t0 = time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC)

type fixture struct {
	core  *core.Core
	clock *clock.Fake
	store store.Store
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	s := store.NewMemory()
	clk := clock.NewFake(t0)
	return fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s}
}

var ctx = context.Background()

func (f fixture) project(t *testing.T, key string) core.ProjectView {
	t.Helper()
	p, err := f.core.CreateProject(ctx, core.CreateProjectInput{Key: key})
	if err != nil {
		t.Fatalf("create_project: %v", err)
	}
	return p
}

func (f fixture) spec(t *testing.T, key, title string) core.SpecView {
	t.Helper()
	s, err := f.core.CreateSpec(ctx, key, core.CreateSpecInput{Title: title, Body: "body\n", Full: true})
	if err != nil {
		t.Fatalf("create_spec: %v", err)
	}
	return *s.Record
}

func (f fixture) ticket(t *testing.T, key string, in core.CreateTicketInput) core.TicketView {
	t.Helper()
	tk, err := f.core.CreateTicket(ctx, key, in)
	if err != nil {
		t.Fatalf("create_ticket: %v", err)
	}
	return tk
}

func wantCode(t *testing.T, err error, code core.Code, field string) {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want a core.Error with code %s", err, code)
	}
	if e.Code != code || e.Field != field {
		t.Fatalf("got code %s field %q (%s), want %s field %q", e.Code, e.Field, e.Message, code, field)
	}
}

func TestCreateProject(t *testing.T) {
	f := newFixture(t)
	p := f.project(t, "acme")
	if p.Key != "acme" || p.DisplayName != "acme" || !p.Created.Equal(t0) {
		t.Fatalf("display name should default to the key: %+v", p)
	}
	named, err := f.core.CreateProject(ctx, core.CreateProjectInput{Key: "web", DisplayName: "Web App"})
	if err != nil || named.DisplayName != "Web App" {
		t.Fatalf("got %+v, %v", named, err)
	}
	got, err := f.core.GetProject(ctx, "web")
	if err != nil || got.DisplayName != "Web App" {
		t.Fatalf("get_project: %+v, %v", got, err)
	}
}

func TestCreateProjectRejects(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, key := range []string{"", "Acme", "a/b", "..", "-x", "a b"} {
		_, err := f.core.CreateProject(ctx, core.CreateProjectInput{Key: key})
		wantCode(t, err, core.Invalid, "key")
	}
	_, err := f.core.CreateProject(ctx, core.CreateProjectInput{Key: "acme"})
	wantCode(t, err, core.Invalid, "key")
}

func TestGetUnknownProject(t *testing.T) {
	f := newFixture(t)
	_, err := f.core.GetProject(ctx, "nope")
	wantCode(t, err, core.UnknownProject, "")
}

func TestOperationsOnAnUnknownProject(t *testing.T) {
	f := newFixture(t)
	_, err := f.core.CreateSpec(ctx, "nope", core.CreateSpecInput{Title: "S"})
	wantCode(t, err, core.UnknownProject, "")
	_, err = f.core.CreateTicket(ctx, "nope", core.CreateTicketInput{Spec: "1", Title: "T"})
	wantCode(t, err, core.UnknownProject, "")
	_, err = f.core.GetTicket(ctx, "nope", "1")
	wantCode(t, err, core.UnknownProject, "")
	_, err = f.core.FindTickets(ctx, "nope", core.FindTicketsInput{})
	wantCode(t, err, core.UnknownProject, "")
}

func TestSpecsAndTicketsShareOneCounter(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	s := f.spec(t, "acme", "Login")
	if s.ID != "acme-1" || s.Title != "Login" || s.Body != "body\n" || !s.Created.Equal(t0) || !s.Updated.Equal(t0) {
		t.Fatalf("spec: %+v", s)
	}
	tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Form"})
	s2 := f.spec(t, "acme", "Logout")
	if tk.ID != "acme-2" || s2.ID != "acme-3" {
		t.Fatalf("ids: ticket %s, spec %s", tk.ID, s2.ID)
	}
}

func TestCreateTicket(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	f.clock.Advance(time.Minute)

	draft := f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Form", Description: "Build it.\n", Labels: []string{"ui"}})
	if draft.State != store.Draft {
		t.Fatalf("default State = %s, want Draft", draft.State)
	}
	if draft.Spec.ID != "acme-1" || draft.Spec.Title != "Login" {
		t.Fatalf("spec ref: %+v", draft.Spec)
	}
	if !draft.Created.Equal(t0.Add(time.Minute)) || draft.Description != "Build it.\n" || !slices.Equal(draft.Labels, []string{"ui"}) {
		t.Fatalf("view: %+v", draft)
	}
	if draft.Claimable || draft.Blocked {
		t.Fatalf("a Draft is not claimable: %+v", draft)
	}

	ready := f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "API", State: store.Ready})
	if ready.State != store.Ready || !ready.Claimable {
		t.Fatalf("ready: %+v", ready)
	}

	got, err := f.core.GetTicket(ctx, "acme", "acme-2")
	if err != nil || got.ID != "acme-2" || got.Title != "Form" {
		t.Fatalf("get_ticket: %+v, %v", got, err)
	}
	if got, err := f.core.GetTicket(ctx, "acme", "3"); err != nil || got.Title != "API" {
		t.Fatalf("get_ticket by bare number: %+v, %v", got, err)
	}
}

func TestCreateTicketRejects(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Form"})
	cases := []struct {
		name  string
		in    core.CreateTicketInput
		field string
	}{
		{"no title", core.CreateTicketInput{Spec: "1"}, "title"},
		{"no spec", core.CreateTicketInput{Title: "T"}, "spec"},
		{"unknown spec", core.CreateTicketInput{Spec: "9", Title: "T"}, "spec"},
		{"spec is a ticket", core.CreateTicketInput{Spec: "2", Title: "T"}, "spec"},
		{"spec from another project", core.CreateTicketInput{Spec: "web-1", Title: "T"}, "spec"},
		{"state other than Draft or Ready", core.CreateTicketInput{Spec: "1", Title: "T", State: store.InProgress}, "state"},
		{"unknown state", core.CreateTicketInput{Spec: "1", Title: "T", State: "Doing"}, "state"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.core.CreateTicket(ctx, "acme", c.in)
			wantCode(t, err, core.Invalid, c.field)
		})
	}
}

func TestCreateSpecNeedsATitle(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.CreateSpec(ctx, "acme", core.CreateSpecInput{Body: "x"})
	wantCode(t, err, core.Invalid, "title")
}

func TestGetTicketNotFound(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	for _, ref := range []core.Ref{"1", "5", "acme-7"} {
		_, err := f.core.GetTicket(ctx, "acme", ref)
		wantCode(t, err, core.NotFound, "")
	}
	_, err := f.core.GetTicket(ctx, "acme", "banana")
	wantCode(t, err, core.Invalid, "ticket")
}

func TestFindTickets(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")  // 1
	f.spec(t, "acme", "Search") // 2
	f.clock.Set(t0.Add(2 * time.Minute))
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Login form", Labels: []string{"ui", "auth"}}) // 3
	f.clock.Set(t0.Add(1 * time.Minute))
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Search API", State: store.Ready}) // 4, created earlier
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Login API", State: store.Ready, Labels: []string{"auth"}})
	// A finished Ticket, written straight to the store since no operation
	// reaches Done yet.
	f.store.Update(func(w store.Writer) error {
		return w.PutTicket("acme", store.Ticket{Number: 6, Title: "Old login", State: store.Done, Spec: 1, Created: t0, Updated: t0})
	})

	ids := func(rows []core.TicketRow) []string {
		var out []string
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	cases := []struct {
		name string
		in   core.FindTicketsInput
		want []string
	}{
		{"all unfinished, in claim order", core.FindTicketsInput{}, []string{"acme-4", "acme-5", "acme-3"}},
		{"by state", core.FindTicketsInput{State: []store.State{store.Ready}}, []string{"acme-4", "acme-5"}},
		{"finished only when named", core.FindTicketsInput{State: []store.State{store.Done}}, []string{"acme-6"}},
		{"title, case-insensitive substring", core.FindTicketsInput{Title: "LOGIN"}, []string{"acme-5", "acme-3"}},
		{"labels all present", core.FindTicketsInput{Labels: []string{"auth", "ui"}}, []string{"acme-3"}},
		{"by spec", core.FindTicketsInput{Spec: "acme-2"}, []string{"acme-4"}},
		{"filters combine", core.FindTicketsInput{Title: "login", State: []store.State{store.Ready}}, []string{"acme-5"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, err := f.core.FindTickets(ctx, "acme", c.in)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(rows); !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}

	rows, _ := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{Labels: []string{"ui"}})
	r := rows[0]
	if r.Title != "Login form" || r.State != store.Draft || r.Spec != "acme-1" || !slices.Equal(r.Labels, []string{"ui", "auth"}) || r.Blocked || !r.Updated.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("row: %+v", r)
	}

	_, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{State: []store.State{"Doing"}})
	wantCode(t, err, core.Invalid, "state")
}

func TestOperationList(t *testing.T) {
	want := map[string]core.Audience{
		"create_project": core.HumanOnly,
		"get_project":    core.HumanOnly,
		"create_spec":    core.AgentAndHuman,
		"create_ticket":  core.AgentAndHuman,
		"get_ticket":     core.AgentAndHuman,
		"find_tickets":   core.AgentAndHuman,
		"edit_spec":      core.AgentAndHuman,
		"edit_ticket":    core.AgentAndHuman,
		"add_labels":     core.AgentAndHuman,
		"remove_labels":  core.AgentAndHuman,
		"set_tags":       core.AgentAndHuman,
		"remove_tags":    core.AgentAndHuman,
		"comment":        core.AgentAndHuman,
		"approve":        core.AgentAndHuman,
		"defer":          core.AgentAndHuman,
		"claim":          core.AgentAndHuman,
		"finish":         core.AgentAndHuman,
		"accept":         core.HumanOnly,
		"reject":         core.HumanOnly,
		"add_blocker":    core.AgentAndHuman,
		"remove_blocker": core.AgentAndHuman,
		"cancel":         core.AgentAndHuman,
		"get_spec":       core.AgentAndHuman,
		"find_specs":     core.AgentAndHuman,
	}
	for name, aud := range want {
		op, ok := core.Operation(name)
		if !ok || op.Audience != aud {
			t.Errorf("operation %s: %+v, %v", name, op, ok)
		}
	}
}
