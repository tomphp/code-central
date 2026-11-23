package core_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/core/coretest"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/ops"
	"github.com/tomoram/code-central/store"
)

var t0 = coretest.T0

type fixture struct {
	core  *core.Core
	clock *clock.Fake
	store store.Store
}

func newFixture(t *testing.T) fixture { return fixtureOf(coretest.New(t)) }

func fixtureOf(f coretest.Fixture) fixture {
	return fixture{core: f.Core, clock: f.Clock, store: f.Store}
}

func (f fixture) shared() coretest.Fixture {
	return coretest.Fixture{Core: f.core, Clock: f.clock, Store: f.store}
}

var ctx = coretest.Ctx

// reloaded runs setup on a fixture over a file store, closes the store,
// and returns a fixture over the store opened again from its files,
// closed at the test's end.
func reloaded(t *testing.T, setup func(f fixture)) fixture {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	setup(fixtureOf(coretest.On(s, clock.NewFake(t0))))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if s, err = store.Open(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return fixtureOf(coretest.On(s, clock.NewFake(t0)))
}

func (f fixture) project(t *testing.T, key string) api.ProjectView {
	return f.shared().Project(t, key)
}

func (f fixture) spec(t *testing.T, key, title string) api.SpecView {
	return f.shared().Spec(t, key, title)
}

func (f fixture) ticket(t *testing.T, key string, in api.CreateTicketInput) api.TicketView {
	return f.shared().Ticket(t, key, in)
}

// record is the Ticket a write asked for with full returned.
func record(w api.Written[api.TicketView], err error) (api.TicketView, error) {
	if err != nil {
		return api.TicketView{}, err
	}
	if w.Record == nil {
		return api.TicketView{}, errors.New("the write returned an acknowledgement: ask for the Ticket with Full")
	}
	return *w.Record, nil
}

func wantCode(t *testing.T, err error, code api.Code, field string) {
	t.Helper()
	coretest.WantCode(t, err, code, field)
}

func TestCreateProject(t *testing.T) {
	f := newFixture(t)
	p := f.project(t, "acme")
	if p.Key != "acme" || p.DisplayName != "acme" || !p.Created.Equal(t0) {
		t.Fatalf("display name should default to the key: %+v", p)
	}
	named, err := f.core.CreateProject(ctx, api.CreateProjectInput{Key: "web", DisplayName: "Web App"})
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
		_, err := f.core.CreateProject(ctx, api.CreateProjectInput{Key: key})
		wantCode(t, err, api.Invalid, "key")
	}
	_, err := f.core.CreateProject(ctx, api.CreateProjectInput{Key: "acme"})
	wantCode(t, err, api.Invalid, "key")
}

func TestGetUnknownProject(t *testing.T) {
	f := newFixture(t)
	_, err := f.core.GetProject(ctx, "nope")
	wantCode(t, err, api.UnknownProject, "")
}

func TestOperationsOnAnUnknownProject(t *testing.T) {
	f := newFixture(t)
	_, err := f.core.CreateSpec(ctx, "nope", api.CreateSpecInput{Title: "S"})
	wantCode(t, err, api.UnknownProject, "")
	_, err = f.core.CreateTicket(ctx, "nope", api.CreateTicketInput{Spec: "1", Title: "T"})
	wantCode(t, err, api.UnknownProject, "")
	_, err = f.core.GetTicket(ctx, "nope", whole("1"))
	wantCode(t, err, api.UnknownProject, "")
	_, err = f.core.FindTickets(ctx, "nope", api.FindTicketsInput{})
	wantCode(t, err, api.UnknownProject, "")
}

func TestSpecsAndTicketsShareOneCounter(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	s := f.spec(t, "acme", "Login")
	if s.ID != "acme-1" || s.Title != "Login" || s.Body != "body\n" || !s.Created.Equal(t0) || !s.Updated.Equal(t0) {
		t.Fatalf("spec: %+v", s)
	}
	tk := f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Form"})
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

	draft := f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "Form", Description: "Build it.\n", Labels: []string{"ui"}})
	if draft.State != domain.Draft {
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

	ready := f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "API", State: domain.Ready})
	if ready.State != domain.Ready || !ready.Claimable {
		t.Fatalf("ready: %+v", ready)
	}

	got, err := f.core.GetTicket(ctx, "acme", whole("acme-2"))
	if err != nil || got.ID != "acme-2" || got.Title != "Form" {
		t.Fatalf("get_ticket: %+v, %v", got, err)
	}
	if got, err := f.core.GetTicket(ctx, "acme", whole("3")); err != nil || got.Title != "API" {
		t.Fatalf("get_ticket by bare number: %+v, %v", got, err)
	}
}

func TestCreateTicketRejects(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Form"})
	cases := []struct {
		name  string
		in    api.CreateTicketInput
		field string
	}{
		{"no title", api.CreateTicketInput{Spec: "1"}, "title"},
		{"no spec", api.CreateTicketInput{Title: "T"}, "spec"},
		{"unknown spec", api.CreateTicketInput{Spec: "9", Title: "T"}, "spec"},
		{"spec is a ticket", api.CreateTicketInput{Spec: "2", Title: "T"}, "spec"},
		{"spec from another project", api.CreateTicketInput{Spec: "web-1", Title: "T"}, "spec"},
		{"state other than Draft or Ready", api.CreateTicketInput{Spec: "1", Title: "T", State: domain.InProgress}, "state"},
		{"unknown state", api.CreateTicketInput{Spec: "1", Title: "T", State: "Doing"}, "state"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.core.CreateTicket(ctx, "acme", c.in)
			wantCode(t, err, api.Invalid, c.field)
		})
	}
}

func TestCreateSpecNeedsATitle(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.CreateSpec(ctx, "acme", api.CreateSpecInput{Body: "x"})
	wantCode(t, err, api.Invalid, "title")
}

func TestGetTicketNotFound(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	for _, ref := range []api.Ref{"1", "5", "acme-7"} {
		_, err := f.core.GetTicket(ctx, "acme", whole(ref))
		wantCode(t, err, api.NotFound, "")
	}
	_, err := f.core.GetTicket(ctx, "acme", whole("banana"))
	wantCode(t, err, api.Invalid, "ticket")
}

func TestFindTickets(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")  // 1
	f.spec(t, "acme", "Search") // 2
	f.clock.Set(t0.Add(2 * time.Minute))
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Login form", Labels: []string{"ui", "auth"}}) // 3
	f.clock.Set(t0.Add(1 * time.Minute))
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "2", Title: "Search API", State: domain.Ready}) // 4, created earlier
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Login API", State: domain.Ready, Labels: []string{"auth"}})
	// A finished Ticket, written straight to the store since no operation
	// reaches Done yet.
	f.store.Update(func(w store.Writer) error {
		return w.PutTicket("acme", domain.Ticket{Number: 6, Title: "Old login", State: domain.Done, Spec: 1, Created: t0, Updated: t0})
	})

	ids := func(rows []api.TicketRow) []string {
		var out []string
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	cases := []struct {
		name string
		in   api.FindTicketsInput
		want []string
	}{
		{"all unfinished, in claim order", api.FindTicketsInput{}, []string{"acme-4", "acme-5", "acme-3"}},
		{"by state", api.FindTicketsInput{State: []domain.State{domain.Ready}}, []string{"acme-4", "acme-5"}},
		{"finished only when named", api.FindTicketsInput{State: []domain.State{domain.Done}}, []string{"acme-6"}},
		{"title, case-insensitive substring", api.FindTicketsInput{Title: "LOGIN"}, []string{"acme-5", "acme-3"}},
		{"labels all present", api.FindTicketsInput{Labels: []string{"auth", "ui"}}, []string{"acme-3"}},
		{"by spec", api.FindTicketsInput{Spec: "acme-2"}, []string{"acme-4"}},
		{"filters combine", api.FindTicketsInput{Title: "login", State: []domain.State{domain.Ready}}, []string{"acme-5"}},
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

	rows, _ := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{Labels: []string{"ui"}})
	r := rows[0]
	if r.Title != "Login form" || r.State != domain.Draft || r.Spec != "acme-1" || !slices.Equal(r.Labels, []string{"ui", "auth"}) || r.Blocked || !r.Created.Equal(t0.Add(2*time.Minute)) || !r.Updated.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("row: %+v", r)
	}

	_, err := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{State: []domain.State{"Doing"}})
	wantCode(t, err, api.Invalid, "state")
}

func TestOperationList(t *testing.T) {
	want := map[string]ops.Audience{
		"create_project": ops.HumanOnly,
		"get_project":    ops.HumanOnly,
		"create_spec":    ops.AgentAndHuman,
		"create_ticket":  ops.AgentAndHuman,
		"get_ticket":     ops.AgentAndHuman,
		"find_tickets":   ops.AgentAndHuman,
		"edit_spec":      ops.AgentAndHuman,
		"edit_ticket":    ops.AgentAndHuman,
		"add_labels":     ops.AgentAndHuman,
		"remove_labels":  ops.AgentAndHuman,
		"set_tags":       ops.AgentAndHuman,
		"remove_tags":    ops.AgentAndHuman,
		"comment":        ops.AgentAndHuman,
		"approve":        ops.AgentAndHuman,
		"defer":          ops.AgentAndHuman,
		"claim":          ops.AgentAndHuman,
		"finish":         ops.AgentAndHuman,
		"accept":         ops.HumanOnly,
		"reject":         ops.HumanOnly,
		"add_blocker":    ops.AgentAndHuman,
		"remove_blocker": ops.AgentAndHuman,
		"cancel":         ops.AgentAndHuman,
		"get_spec":       ops.AgentAndHuman,
		"find_specs":     ops.AgentAndHuman,
	}
	for name, aud := range want {
		op, ok := ops.Lookup(name)
		if !ok || op.Audience != aud {
			t.Errorf("operation %s: %+v, %v", name, op, ok)
		}
	}
}
