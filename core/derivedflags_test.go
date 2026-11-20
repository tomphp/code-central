package core_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// The rules the UI reads from Go's views and only formats (Spec
// code-central-931): Edit and the Retro button, Spec kind, and a Spec's
// first need and whether a Factory is on it. Each case names the glossary
// rule it answers.

// ticketFlags is Ticket ref's editable and retroable, as get_ticket's
// default read gives them, after checking a read asking for one part and
// the whole read agree.
func (f fixture) ticketFlags(t *testing.T, ref api.Ref) (editable, retroable bool) {
	t.Helper()
	var got [][2]bool
	for _, in := range []api.GetTicketInput{{Ticket: ref}, {Ticket: ref, Parts: []string{api.PartComments}}, whole(ref)} {
		v, err := f.core.GetTicket(ctx, "acme", in)
		if err != nil {
			t.Fatalf("get_ticket %+v: %v", in, err)
		}
		got = append(got, [2]bool{v.Editable, v.Retroable})
	}
	if got[1] != got[0] || got[2] != got[0] {
		t.Fatalf("get_ticket %s: editable and retroable %v by read; every read sets them alike", ref, got)
	}
	return got[0][0], got[0][1]
}

// specRead is Spec ref as get_spec's full read gives it, after checking
// its lighter read and its find_specs row agree on what both carry.
func (f fixture) specRead(t *testing.T, ref api.Ref) api.SpecDetail {
	t.Helper()
	full, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: ref})
	if err != nil {
		t.Fatalf("get_spec %s: %v", ref, err)
	}
	light, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: ref, Parts: []string{api.PartOutline}})
	if err != nil {
		t.Fatalf("get_spec %s outline: %v", ref, err)
	}
	if light.Editable != full.Editable || light.Retroable != full.Retroable || light.Ticketable != full.Ticketable || light.Kind != full.Kind || light.Need != full.Need || light.Claimed != full.Claimed {
		t.Fatalf("get_spec %s: lighter read %+v, full read %+v; every read sets the derived fields alike", ref, light, full)
	}
	rows, err := f.core.FindSpecs(ctx, "acme", api.FindSpecsInput{Finished: true})
	if err != nil {
		t.Fatalf("find_specs: %v", err)
	}
	for _, r := range rows {
		if r.ID == full.ID {
			if r.Kind != full.Kind || r.Need != full.Need || r.Claimed != full.Claimed {
				t.Fatalf("find_specs row %+v, get_spec %+v: kind, need and claimed differ", r, full)
			}
			return full
		}
	}
	t.Fatalf("find_specs has no row for %s", full.ID)
	return full
}

// plainSpec is Project acme with Spec acme-1, neither a Requirement, a Map
// nor Long-lived, and a Ticket on it in each of states from acme-2 on, in
// claim order, each moved there by the human unless it is Draft or Ready.
func (f fixture) plainSpec(t *testing.T, states ...domain.State) {
	t.Helper()
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	for _, s := range states {
		tk := f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Work"})
		if s != domain.Draft {
			f.move(t, api.Ref(tk.ID), s)
		}
		f.clock.Advance(1)
	}
}

// detach closes linked Spec acme-1's and Ticket acme-2's external items
// and unlinks both, so their Links no longer own their definitions.
func (f fixture) detach(t *testing.T) {
	t.Helper()
	f.setExternal(t, "2", domain.ExternalClosed)
	for _, target := range []api.Ref{"2", "1"} {
		if _, err := f.core.Unlink(human, "acme", api.UnlinkInput{Target: target}); err != nil {
			t.Fatalf("unlink %s: %v", target, err)
		}
	}
}

// outbound is linked Spec acme-1 with its Ticket acme-2 an Incoming Issue
// that To Requirement turns into Requirement acme-3, whose Spec has an
// Outbound Link.
func (f fixture) outbound(t *testing.T) api.Ref {
	t.Helper()
	f.linked(t)
	f.incomingIssue(t, 1, 2)
	if _, err := f.core.ToRequirement(human, "acme", api.ToRequirementInput{Ticket: "acme-2"}); err != nil {
		t.Fatalf("to_requirement: %v", err)
	}
	sp := f.specRead(t, "3")
	if sp.Link == nil || !sp.Link.Outbound {
		t.Fatalf("acme-3's Link %+v, want an Outbound Link", sp.Link)
	}
	return "3"
}

// aMap is Project acme with Map acme-1 and its Charting Ticket acme-2.
func (f fixture) aMap(t *testing.T) {
	t.Helper()
	f.project(t, "acme")
	if _, err := f.core.SubmitMap(ctx, "acme", api.SubmitMapInput{Text: "Offline mode"}); err != nil {
		t.Fatalf("submit_map: %v", err)
	}
}

// archivedWork is Project acme, Archived, with plain Spec acme-1 Finished
// by Done Ticket acme-2, and Spec acme-3 with no Tickets.
func (f fixture) archivedWork(t *testing.T) {
	t.Helper()
	f.plainSpec(t, domain.Done)
	f.spec(t, "acme", "Empty")
	f.archive(t, "acme")
}

// Edit: a Draft Ticket, while its Link doesn't own its definition and its
// Project isn't Archived.
func TestEditIsOfferedOnADraftTicketWhoseDefinitionIsCodeCentralsInAProjectThatTakesWrites(t *testing.T) {
	for name, c := range map[string]struct {
		setup func(f fixture, t *testing.T)
		want  bool
	}{
		"a Draft Ticket":                       {func(f fixture, t *testing.T) { f.plainSpec(t, domain.Draft) }, true},
		"a Ready Ticket isn't Draft":           {func(f fixture, t *testing.T) { f.plainSpec(t, domain.Ready) }, false},
		"an owned Link defines the Ticket":     {func(f fixture, t *testing.T) { f.linked(t) }, false},
		"a detached Link no longer defines it": {func(f fixture, t *testing.T) { f.linked(t); f.detach(t) }, true},
		"an Archived Project takes no edits": {func(f fixture, t *testing.T) {
			f.plainSpec(t, domain.Draft)
			f.move(t, "2", domain.Cancelled)
			f.archive(t, "acme")
		}, false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			c.setup(f, t)
			if editable, _ := f.ticketFlags(t, "2"); editable != c.want {
				t.Fatalf("acme-2 editable %v, want %v", editable, c.want)
			}
		})
	}
}

// Edit: a Spec that isn't Finished and isn't a Map, while its Link doesn't
// own its definition and its Project isn't Archived.
func TestEditIsOfferedOnASpecThatIsNeitherFinishedNorAMapWhoseDefinitionIsCodeCentrals(t *testing.T) {
	for name, c := range map[string]struct {
		setup func(f fixture, t *testing.T) api.Ref
		want  bool
	}{
		"an unfinished Spec":                    {func(f fixture, t *testing.T) api.Ref { f.plainSpec(t, domain.Ready); return "1" }, true},
		"a Spec with no Tickets isn't Finished": {func(f fixture, t *testing.T) api.Ref { f.plainSpec(t); return "1" }, true},
		"a Finished Spec": {func(f fixture, t *testing.T) api.Ref {
			f.plainSpec(t, domain.Done, domain.Cancelled)
			return "1"
		}, false},
		"a Map":                                {func(f fixture, t *testing.T) api.Ref { f.aMap(t); return "1" }, false},
		"an owned Link defines the Spec":       {func(f fixture, t *testing.T) api.Ref { f.linked(t); return "1" }, false},
		"a detached Link no longer defines it": {func(f fixture, t *testing.T) api.Ref { f.linked(t); f.detach(t); return "1" }, true},
		"an Outbound Link is Code Central's":   {func(f fixture, t *testing.T) api.Ref { return f.outbound(t) }, true},
		"an Archived Project takes no edits":   {func(f fixture, t *testing.T) api.Ref { f.archivedWork(t); return "3" }, false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ref := c.setup(f, t)
			if got := f.specRead(t, ref).Editable; got != c.want {
				t.Fatalf("Spec %s editable %v, want %v", ref, got, c.want)
			}
		})
	}
}

// Retro button: a Finished Ticket, Done or Cancelled, in a Project that
// isn't Archived.
func TestTheRetroButtonIsOfferedOnAFinishedTicketInAProjectThatTakesWrites(t *testing.T) {
	for name, c := range map[string]struct {
		state domain.State
		want  bool
	}{
		"a Done Ticket":                      {domain.Done, true},
		"a Cancelled Ticket":                 {domain.Cancelled, true},
		"an In Review Ticket isn't finished": {domain.InReview, false},
		"a Draft Ticket isn't finished":      {domain.Draft, false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.plainSpec(t, domain.Draft)
			if c.state != domain.Draft {
				f.move(t, "2", c.state)
			}
			if _, retroable := f.ticketFlags(t, "2"); retroable != c.want {
				t.Fatalf("acme-2 retroable %v, want %v", retroable, c.want)
			}
		})
	}
	t.Run("an Archived Project takes no Retro", func(t *testing.T) {
		f := newFixture(t)
		f.archivedWork(t)
		if _, retroable := f.ticketFlags(t, "2"); retroable {
			t.Fatal("acme-2 retroable in an Archived Project")
		}
	})
}

// Retro button: a Finished Spec that isn't Long-lived, a finished Map
// too, in a Project that isn't Archived.
func TestTheRetroButtonIsOfferedOnAFinishedSpecThatIsNotLongLived(t *testing.T) {
	for name, c := range map[string]struct {
		setup func(f fixture, t *testing.T) api.Ref
		want  bool
	}{
		"a Finished Spec":    {func(f fixture, t *testing.T) api.Ref { f.plainSpec(t, domain.Done, domain.Cancelled); return "1" }, true},
		"an unfinished Spec": {func(f fixture, t *testing.T) api.Ref { f.plainSpec(t, domain.Done, domain.Ready); return "1" }, false},
		"a Finished Long-lived Spec": {func(f fixture, t *testing.T) api.Ref {
			f.plainSpec(t, domain.Done)
			f.longLived(t, api.SpecView{ID: "acme-1"})
			return "1"
		}, false},
		"a finished Map": {func(f fixture, t *testing.T) api.Ref {
			f.aMap(t)
			f.move(t, "2", domain.Done)
			return "1"
		}, true},
		"an Archived Project takes no Retro": {func(f fixture, t *testing.T) api.Ref { f.archivedWork(t); return "1" }, false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ref := c.setup(f, t)
			if got := f.specRead(t, ref).Retroable; got != c.want {
				t.Fatalf("Spec %s retroable %v, want %v", ref, got, c.want)
			}
		})
	}
}

// Spec kind: a Requirement, else a Map, else a Long-lived Spec, else a
// Spec, so a Spec that is two of them reads as the first.
func TestASpecsKindIsRequirementThenMapThenLongLivedThenSpec(t *testing.T) {
	requirement := func(f fixture, t *testing.T) api.Ref {
		f.requirement(t)
		return "2"
	}
	for name, c := range map[string]struct {
		setup func(f fixture, t *testing.T) api.Ref
		want  api.SpecKind
	}{
		"a Requirement": {requirement, api.SpecKindRequirement},
		"a Map":         {func(f fixture, t *testing.T) api.Ref { f.aMap(t); return "1" }, api.SpecKindMap},
		"a Long-lived Spec": {func(f fixture, t *testing.T) api.Ref {
			f.plainSpec(t)
			f.longLived(t, api.SpecView{ID: "acme-1"})
			return "1"
		}, api.SpecKindLongLived},
		"a plain Spec": {func(f fixture, t *testing.T) api.Ref { f.plainSpec(t); return "1" }, api.SpecKindSpec},
		"a Long-lived Requirement is a Requirement": {func(f fixture, t *testing.T) api.Ref {
			ref := requirement(f, t)
			f.longLived(t, api.SpecView{ID: "acme-2"})
			return ref
		}, api.SpecKindRequirement},
		"a Long-lived Map is a Map": {func(f fixture, t *testing.T) api.Ref {
			f.aMap(t)
			f.longLived(t, api.SpecView{ID: "acme-1"})
			return "1"
		}, api.SpecKindMap},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ref := c.setup(f, t)
			if got := f.specRead(t, ref).Kind; got != c.want {
				t.Fatalf("Spec %s kind %q, want %q", ref, got, c.want)
			}
		})
	}
}

// Spec row: a plain Spec's headline says the reason of the first of its
// Tickets, in claim order, that needs the human, read as Needs Me reads
// it; none when none does.
func TestASpecsNeedIsTheReasonOfItsFirstTicketInClaimOrderThatNeedsTheHuman(t *testing.T) {
	f := newFixture(t)
	f.plainSpec(t, domain.Ready, domain.Ready, domain.Ready)
	if got := f.specRead(t, "1").Need; got != "" {
		t.Fatalf("need %q while no Ticket needs the human, want none", got)
	}
	// acme-4 fails first, then acme-3 goes to In Review: acme-3 comes
	// first in claim order, though Needs Me lists Failed first.
	f.claim(t, api.ClaimInput{Ticket: "4"})
	if _, err := f.core.ReportFailure(ctx, "acme", report("4", "claim-1")); err != nil {
		t.Fatalf("report_failure: %v", err)
	}
	if got := f.specRead(t, "1").Need; got != api.ReasonFailed {
		t.Fatalf("need %q, want %q", got, api.ReasonFailed)
	}
	f.move(t, "3", domain.InReview)
	if got := f.specRead(t, "1").Need; got != api.ReasonInReview {
		t.Fatalf("need %q, want acme-3's %q", got, api.ReasonInReview)
	}
}

// Spec row: a Factory is on a Spec while any of its Tickets holds a live
// Claim, a Silent one too; a stale Claim, one that has ended, never comes
// back.
func TestASpecIsClaimedWhileAnyOfItsTicketsHoldsALiveClaim(t *testing.T) {
	f := newFixture(t)
	f.plainSpec(t, domain.Ready, domain.Ready)
	if f.specRead(t, "1").Claimed {
		t.Fatal("claimed with no Claim")
	}
	f.claim(t, api.ClaimInput{Ticket: "3"})
	if !f.specRead(t, "1").Claimed {
		t.Fatal("not claimed while acme-3 holds a live Claim")
	}
	f.clock.Advance(core.SilentAfter)
	if sp := f.specRead(t, "1"); !sp.Claimed || sp.Need != api.ReasonSilentClaim {
		t.Fatalf("claimed %v, need %q while acme-3's Claim is Silent, want claimed and %q", sp.Claimed, sp.Need, api.ReasonSilentClaim)
	}
	if _, err := f.core.Release(human, "acme", api.ReleaseInput{Ticket: "3"}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if f.specRead(t, "1").Claimed {
		t.Fatal("claimed after acme-3's Claim ended")
	}
}
