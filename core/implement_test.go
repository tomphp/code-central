package core_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// specInReview is requirement with its spec Stage, acme-4, In Review.
func (f fixture) specInReview(t *testing.T) {
	t.Helper()
	f.requirement(t)
	f.setState(t, 3, store.Done)
	f.setState(t, 4, store.InReview)
}

func (f fixture) implement(t *testing.T, ref core.Ref) core.FinishResult {
	t.Helper()
	res, err := f.core.Implement(human, "acme", core.TicketInput{Ticket: ref})
	if err != nil {
		t.Fatalf("implement %s: %v", ref, err)
	}
	return res
}

func TestImplementAcceptsTheSpecStageCreatesOneTicketAndCancelsTheTicketsStage(t *testing.T) {
	f := newFixture(t)
	f.specInReview(t)
	res := f.implement(t, "acme-4")
	if res.Acknowledgement == nil || res.Acknowledgement.ID != "acme-4" || res.Acknowledgement.State != store.Done {
		t.Fatalf("result %+v", res)
	}
	if res.NextStep == "" || !strings.Contains(res.NextStep, "acme-6") {
		t.Fatalf("the next step should name the new Ticket: %q", res.NextStep)
	}

	spec := f.get(t, "acme-4")
	if m := spec.Moves[len(spec.Moves)-1]; m.From != store.InReview || m.To != store.Done || m.Actor != "human" || m.Via != "implement" {
		t.Fatalf("spec Stage move %+v", m)
	}

	build := f.get(t, "acme-6")
	wantDescription := "Implement the whole Spec [acme-2](acme-2). Its User Stories, Implementation Decisions and Testing Decisions are the acceptance criteria, including writing the glossary terms and ADRs it records. Read them with `get_spec` and `sections`.\n"
	if build.Title != "Implement: Add a dark mode" || build.State != store.Ready || build.Spec.ID != "acme-2" ||
		len(build.Labels) != 0 || len(build.BlockedBy) != 0 || build.Description != wantDescription {
		t.Fatalf("implementation Ticket %+v", build)
	}
	if len(build.Moves) != 1 || build.Moves[0].To != store.Ready || build.Moves[0].Actor != "human" || build.Moves[0].Via != "implement" {
		t.Fatalf("implementation Ticket moves %+v", build.Moves)
	}

	tickets := f.get(t, "acme-5")
	if tickets.State != store.Cancelled || !slices.Equal(tickets.Replacements, []string{"acme-6"}) {
		t.Fatalf("tickets Stage %+v", tickets)
	}
	if m := tickets.Moves[len(tickets.Moves)-1]; m.To != store.Cancelled || m.Via != "implement" {
		t.Fatalf("tickets Stage move %+v", m)
	}
	if len(tickets.Comments) != 1 || !strings.Contains(tickets.Comments[0].Body, "human chose Implement") || !strings.Contains(tickets.Comments[0].Body, "acme-6") {
		t.Fatalf("tickets Stage comments %+v", tickets.Comments)
	}
}

func TestImplementWithFullReturnsTheSpecStage(t *testing.T) {
	f := newFixture(t)
	f.specInReview(t)
	res, err := f.core.Implement(human, "acme", core.TicketInput{Ticket: "acme-4", Full: true})
	if err != nil || res.Ticket == nil || res.Ticket.ID != "acme-4" || res.Acknowledgement != nil {
		t.Fatalf("result %+v, %v", res, err)
	}
}

func TestAfterImplementTheBuildFollowsTheImplementationTicket(t *testing.T) {
	f := newFixture(t)
	f.specInReview(t)
	f.implement(t, "acme-4")
	r := f.requirementOf(t, "acme-2")
	if r.Build != (core.BuildProgress{Done: 0, Total: 1}) || !r.WaitingForFactory {
		t.Fatalf("build %+v waiting %v", r.Build, r.WaitingForFactory)
	}
	f.setState(t, 6, store.Done)
	r = f.requirementOf(t, "acme-2")
	if r.Build != (core.BuildProgress{Done: 1, Total: 1}) || r.WaitingForFactory {
		t.Fatalf("build %+v waiting %v", r.Build, r.WaitingForFactory)
	}
}

func TestImplementIsRefusedAndWritesNothingWhenItCannotRun(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f fixture, t *testing.T)
		ref   core.Ref
		want  string
	}{
		{"a grilling Stage", func(f fixture, t *testing.T) { f.setState(t, 3, store.InReview) }, "acme-3", "not a spec Stage"},
		{"an implementation Ticket", func(f fixture, t *testing.T) {
			f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-2", Title: "Build", State: store.Ready})
			f.setState(t, 6, store.InReview)
		}, "acme-6", "not a spec Stage"},
		{"a spec Stage not In Review", func(f fixture, t *testing.T) {}, "acme-4", "In Review"},
		{"a Cancelled tickets Stage", func(f fixture, t *testing.T) { f.setState(t, 4, store.InReview); f.setState(t, 5, store.Cancelled) }, "acme-4", "Cancelled"},
		{"a Done tickets Stage", func(f fixture, t *testing.T) { f.setState(t, 4, store.InReview); f.setState(t, 5, store.Done) }, "acme-4", "Done"},
		{"a claimed tickets Stage", func(f fixture, t *testing.T) { f.setState(t, 4, store.InReview); f.setState(t, 5, store.InProgress) }, "acme-4", "In Progress"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.requirement(t)
			c.setup(f, t)
			before := snapshotTickets(t, f)
			_, err := f.core.Implement(human, "acme", core.TicketInput{Ticket: c.ref})
			wantCode(t, err, core.ActionNotAllowed, "")
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("the refusal should name %q: %v", c.want, err)
			}
			if after := snapshotTickets(t, f); !slices.Equal(before, after) {
				t.Fatalf("a refused implement wrote: before %v after %v", before, after)
			}
		})
	}
}

func TestImplementIsAHumanAction(t *testing.T) {
	f := newFixture(t)
	f.specInReview(t)
	_, err := f.core.Implement(ctx, "acme", core.TicketInput{Ticket: "acme-4"})
	wantCode(t, err, core.ActionNotAllowed, "")
}

// snapshotTickets is every Ticket's ID, State and Moves' count, read back
// through get_ticket.
func snapshotTickets(t *testing.T, f fixture) []string {
	t.Helper()
	var out []string
	for n := 3; n <= 8; n++ {
		id := core.Ref("acme-" + string(rune('0'+n)))
		tk, err := f.core.GetTicket(ctx, "acme", whole(id))
		if err != nil {
			continue
		}
		out = append(out, tk.ID+" "+string(tk.State)+" "+string(rune('0'+len(tk.Moves)))+string(rune('0'+len(tk.Comments))))
	}
	return out
}
