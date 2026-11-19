package core_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// New Ticket (Spec code-central-1067): the human opens one implementation
// Ticket on a Spec from free text, on any Spec but a Map or a Long-lived
// Spec, on a Requirement once its Stages are finished, and on a Finished
// Spec too.

const followUp = "Rename the toggle\n\nReview asked for \"Dark theme\", not \"Night mode\".\n"

func (f fixture) newTicket(t *testing.T, in api.NewTicketInput) api.Written[api.TicketView] {
	t.Helper()
	res, err := f.core.NewTicket(human, "acme", in)
	if err != nil {
		t.Fatalf("new_ticket: %v", err)
	}
	return res
}

func TestNewTicketOpensAReadyTicketOnTheSpecTitledWithTheTextsFirstLine(t *testing.T) {
	f := newFixture(t)
	f.plainSpec(t, domain.Done)
	tk := f.newTicket(t, api.NewTicketInput{Spec: "acme-1", Text: followUp, Full: true}).Record
	if tk.ID != "acme-3" || tk.Title != "Rename the toggle" || tk.Description != followUp || tk.State != domain.Ready ||
		tk.Spec.ID != "acme-1" || len(tk.Labels) != 0 || len(tk.Tags) != 0 || len(tk.BlockedBy) != 0 || !tk.Claimable {
		t.Fatalf("new Ticket %+v", tk)
	}
	if len(tk.Moves) != 1 || tk.Moves[0].Actor != "human" || tk.Moves[0].To != domain.Ready || tk.Moves[0].Via != "new_ticket" {
		t.Fatalf("moves %+v", tk.Moves)
	}
}

func TestNewTicketReturnsAnAcknowledgementUnlessFull(t *testing.T) {
	f := newFixture(t)
	f.plainSpec(t)
	res := f.newTicket(t, api.NewTicketInput{Spec: "1", Text: followUp})
	if res.Record != nil || res.Acknowledgement.ID != "acme-2" || res.Acknowledgement.State != domain.Ready {
		t.Fatalf("result %+v", res)
	}
}

func TestADraftNewTicketIsADraftTicketWithNoMoves(t *testing.T) {
	f := newFixture(t)
	f.plainSpec(t)
	tk := f.newTicket(t, api.NewTicketInput{Spec: "acme-1", Text: followUp, Draft: true, Full: true}).Record
	if tk.State != domain.Draft || tk.Claimable || len(tk.Moves) != 0 {
		t.Fatalf("Draft new Ticket %+v", tk)
	}
}

func TestNewTicketWithNoTextIsInvalid(t *testing.T) {
	f := newFixture(t)
	f.plainSpec(t)
	_, err := f.core.NewTicket(human, "acme", api.NewTicketInput{Spec: "acme-1", Text: " \n# \n"})
	wantCode(t, err, api.Invalid, "text")
}

func TestNewTicketIsTheHumans(t *testing.T) {
	f := newFixture(t)
	f.plainSpec(t)
	_, err := f.core.NewTicket(ctx, "acme", api.NewTicketInput{Spec: "acme-1", Text: followUp})
	wantCode(t, err, api.ActionNotAllowed, "")
}

func TestNewTicketOnAMissingSpecIsInvalid(t *testing.T) {
	f := newFixture(t)
	f.plainSpec(t)
	_, err := f.core.NewTicket(human, "acme", api.NewTicketInput{Spec: "acme-9", Text: followUp})
	wantCode(t, err, api.Invalid, "spec")
}

// New Ticket: offered on any Spec but a Map or a Long-lived Spec, on a
// Requirement only once its Stages are finished, and on a Finished Spec
// too; never in an Archived Project. The write and the read agree.
func TestNewTicketIsAllowedWhereTheSpecReadSaysItIsTicketable(t *testing.T) {
	for name, c := range map[string]struct {
		setup func(f fixture, t *testing.T) api.Ref
		// refusal is part of the refusal's message, or "" when allowed.
		refusal string
	}{
		"a Spec":          {func(f fixture, t *testing.T) api.Ref { f.plainSpec(t, domain.Ready); return "1" }, ""},
		"an empty Spec":   {func(f fixture, t *testing.T) api.Ref { f.plainSpec(t); return "1" }, ""},
		"a Finished Spec": {func(f fixture, t *testing.T) api.Ref { f.plainSpec(t, domain.Done, domain.Cancelled); return "1" }, ""},
		"a Map":           {func(f fixture, t *testing.T) api.Ref { f.aMap(t); return "1" }, "is a Map"},
		"a Long-lived Spec": {func(f fixture, t *testing.T) api.Ref {
			f.plainSpec(t)
			f.longLived(t, api.SpecView{ID: "acme-1"})
			return "1"
		}, "Long-lived"},
		"a Requirement with running Stages": {func(f fixture, t *testing.T) api.Ref { f.requirement(t); return "2" }, "Stages"},
		"a Requirement in its tickets Stage": {func(f fixture, t *testing.T) api.Ref {
			f.requirement(t)
			f.setState(t, 3, domain.Done)
			f.setState(t, 4, domain.Done)
			return "2"
		}, "Stages"},
		"a Requirement whose Stages are Done": {func(f fixture, t *testing.T) api.Ref { f.stagesDone(t, domain.Done); return "2" }, ""},
		"a Requirement after Implement": {func(f fixture, t *testing.T) api.Ref {
			f.specInReview(t)
			f.implement(t, "acme-4")
			return "2"
		}, ""},
		"a cancelled Requirement": {func(f fixture, t *testing.T) api.Ref {
			f.requirement(t)
			for _, n := range []api.Ref{"3", "4", "5"} {
				f.move(t, n, domain.Cancelled)
			}
			return "2"
		}, "Stages"},
		"an Archived Project": {func(f fixture, t *testing.T) api.Ref { f.archivedWork(t); return "1" }, "archived"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ref := c.setup(f, t)
			want := c.refusal == ""
			if got := f.specRead(t, ref).Ticketable; got != want {
				t.Fatalf("Spec %s ticketable %v, want %v", ref, got, want)
			}
			_, err := f.core.NewTicket(human, "acme", api.NewTicketInput{Spec: ref, Text: followUp})
			if want {
				if err != nil {
					t.Fatalf("new_ticket on %s: %v", ref, err)
				}
				return
			}
			wantCode(t, err, api.ActionNotAllowed, "")
			if !strings.Contains(err.Error(), c.refusal) {
				t.Fatalf("new_ticket on %s refused with %q, want it to say %q", ref, err, c.refusal)
			}
		})
	}
}

// Under Pull Request + Land per Spec, a New Ticket joins the Spec's
// pending batch, so the Spec isn't ready to land while it is open.
func TestANewTicketKeepsItsSpecFromBeingReadyToLandUnderLandPerSpec(t *testing.T) {
	f := specPullRequestWorld(t)
	f.acceptedOnSpecBranch(t, 5, "Built the first part.")
	f.acceptedOnSpecBranch(t, 6, "Built the second part.")
	if st := f.landStatus(t); st == nil || !st.Ready {
		t.Fatalf("land per spec = %+v, want ready before the New Ticket", st)
	}
	tk := f.newTicket(t, api.NewTicketInput{Spec: "acme-1", Text: followUp, Full: true}).Record
	if !tk.SpecLanded {
		t.Fatalf("New Ticket %s isn't Spec-landed", tk.ID)
	}
	if st := f.landStatus(t); st.Ready || !slices.Equal(st.Waiting, []string{"acme-5", "acme-6"}) {
		t.Fatalf("land per spec = %+v, want not ready, acme-5 and acme-6 waiting", st)
	}
}
