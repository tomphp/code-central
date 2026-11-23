// Package coretest builds a whole Core over a Store and a fake clock, for
// the tests of core and of the areas under it, which can't import core
// themselves.
package coretest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// T0 is the fake clock's first time.
var T0 = time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC)

// Ctx is a context with no actor, as an agent's call has.
var Ctx = context.Background()

// Human is a context whose actor is the human.
var Human = api.WithActor(Ctx, api.Human)

// Fixture is a Core over Store and Clock.
type Fixture struct {
	Core  *core.Core
	Clock *clock.Fake
	Store store.Store
}

// New is a Fixture over a new in-memory Store.
func New(t *testing.T) Fixture {
	t.Helper()
	return On(store.NewMemory(), clock.NewFake(T0))
}

// On is a Fixture whose Core is a new service on s, reading clk.
func On(s store.Store, clk *clock.Fake) Fixture {
	return Fixture{Core: core.New(s, clk, &ids.Sequence{}), Clock: clk, Store: s}
}

// TwoServices is two services, one on laptop and one on desktop, on s and
// one fake clock, with Project acme.
func TwoServices(t *testing.T, s store.Store) (laptop, desktop Fixture) {
	t.Helper()
	clk := clock.NewFake(T0)
	laptop, desktop = On(s, clk), On(s, clk)
	laptop.Core.SetService(core.Service{ID: "a", Host: "laptop"})
	desktop.Core.SetService(core.Service{ID: "b", Host: "desktop"})
	laptop.Project(t, "acme")
	return laptop, desktop
}

// Project creates Project key.
func (f Fixture) Project(t *testing.T, key string) api.ProjectView {
	t.Helper()
	p, err := f.Core.CreateProject(Ctx, api.CreateProjectInput{Key: key})
	if err != nil {
		t.Fatalf("create_project: %v", err)
	}
	return p
}

// Spec creates a Spec on key.
func (f Fixture) Spec(t *testing.T, key, title string) api.SpecView {
	t.Helper()
	s, err := f.Core.CreateSpec(Ctx, key, api.CreateSpecInput{Title: title, Body: "body\n", Full: true})
	if err != nil {
		t.Fatalf("create_spec: %v", err)
	}
	return *s.Record
}

// Ticket creates a Ticket on key.
func (f Fixture) Ticket(t *testing.T, key string, in api.CreateTicketInput) api.TicketView {
	t.Helper()
	in.Full = true
	w, err := f.Core.CreateTicket(Ctx, key, in)
	if err != nil {
		t.Fatalf("create_ticket: %v", err)
	}
	if w.Record == nil {
		t.Fatal("create_ticket returned an acknowledgement")
	}
	return *w.Record
}

// Get is acme's Ticket ref, whole.
func (f Fixture) Get(t *testing.T, ref api.Ref) api.TicketView {
	t.Helper()
	v, err := f.Core.GetTicket(Ctx, "acme", api.GetTicketInput{Ticket: ref, Parts: api.TicketParts()})
	if err != nil {
		t.Fatalf("get_ticket %s: %v", ref, err)
	}
	return v
}

// LongLived gives s the long-lived Label, so its Tickets land one by one.
func (f Fixture) LongLived(t *testing.T, s api.SpecView) {
	t.Helper()
	if _, err := f.Core.AddLabels(Ctx, "acme", api.LabelsInput{Target: api.Ref(s.ID), Labels: []string{domain.LongLivedLabel}}); err != nil {
		t.Fatalf("add_labels: %v", err)
	}
}

// Work is a Project acme with a long-lived Spec acme-1 and the given
// Tickets on it, created a minute apart so their claim order is their
// creation order.
func (f Fixture) Work(t *testing.T, states ...domain.State) []api.TicketView {
	t.Helper()
	f.Project(t, "acme")
	f.LongLived(t, f.Spec(t, "acme", "Login"))
	var out []api.TicketView
	for i, s := range states {
		out = append(out, f.Ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "T" + string(rune('a'+i)), State: s}))
		f.Clock.Advance(time.Minute)
	}
	return out
}

// Claim claims a Ticket on acme, which must have one.
func (f Fixture) Claim(t *testing.T, in api.ClaimInput) api.ClaimResult {
	t.Helper()
	res, err := f.Core.Claim(Ctx, "acme", in)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if res.Ticket == nil {
		t.Fatal("claim: nothing claimed")
	}
	return res
}

// NoNotice fails if a notice is waiting.
func NoNotice(t *testing.T, notices <-chan events.Event) {
	t.Helper()
	select {
	case e := <-notices:
		t.Fatalf("unexpected notice %+v", e)
	default:
	}
}

// WantCode fails t unless err is an api.Error with code and field.
func WantCode(t *testing.T, err error, code api.Code, field string) {
	t.Helper()
	var e *api.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want an api.Error with code %s", err, code)
	}
	if e.Code != code || e.Field != field {
		t.Fatalf("got code %s field %q (%s), want %s field %q", e.Code, e.Field, e.Message, code, field)
	}
}
