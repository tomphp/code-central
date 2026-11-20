package core_test

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Land per Spec (code-central-371), through core's public operations.

// landWorld is acme under Merge with Land per Spec set to landPerSpec, a
// Requirement Spec acme-1 with its Stage Tickets (acme-2 grilling, acme-3
// spec, acme-4 tickets, all Done) and implementation Tickets acme-5 and
// acme-6, acme-6 blocked by acme-5, both Ready.
func landWorld(t *testing.T, landPerSpec bool) fixture {
	t.Helper()
	return landWorldOn(t, newFixture(t), landPerSpec)
}

// landWorldOn builds landWorld on f.
func landWorldOn(t *testing.T, f fixture, landPerSpec bool) fixture {
	t.Helper()
	f.project(t, "acme")
	f.spec(t, "acme", "Requirement")
	for _, stage := range domain.Stages() {
		f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: string(stage), Labels: []string{domain.StageLabel(stage)}})
	}
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "First"})
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "Second", BlockedBy: []api.Ref{"acme-5"}})
	f.landing(t, domain.Landing{Method: domain.LandingMerge, Remote: "origin", LandPerSpec: landPerSpec})
	for n := 2; n <= 4; n++ {
		f.moveTo(t, n, domain.Done)
	}
	return f
}

func (f fixture) landing(t *testing.T, l domain.Landing) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.Landing = l
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// setState puts Ticket n in state, moving it there now when it is
// Accepted, as the human's accept does.
func (f fixture) moveTo(t *testing.T, n int, state domain.State) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", n, store.Moves)
		tk.Moves = append(tk.Moves, domain.Move{From: tk.State, To: state, Time: f.clock.Now()})
		tk.State = state
		return w.PutTicket("acme", tk, store.Moves)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f fixture) getTicket(t *testing.T, id string) api.TicketView {
	t.Helper()
	v, err := f.core.GetTicket(ctx, "acme", api.GetTicketInput{Ticket: api.Ref(id)})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) specMerge(t *testing.T, id, commit string) {
	t.Helper()
	if _, err := f.core.ReportSpecMerge(ctx, "acme", api.ReportSpecMergeInput{Ticket: api.Ref(id), Commit: commit}); err != nil {
		t.Fatalf("report_spec_merge: %v", err)
	}
}

func (f fixture) specStatus(t *testing.T, spec api.Ref) *api.LandPerSpecView {
	t.Helper()
	v, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	return v.LandPerSpec
}

func (f fixture) landStatus(t *testing.T) *api.LandPerSpecView {
	t.Helper()
	v, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: "acme-1"})
	if err != nil {
		t.Fatal(err)
	}
	return v.LandPerSpec
}

func TestAnImplementationTicketOfARequirementIsSpecLandedUnderMergeWithLandPerSpecOn(t *testing.T) {
	f := landWorld(t, true)
	if !f.getTicket(t, "acme-5").SpecLanded {
		t.Error("acme-5 isn't Spec-landed")
	}
	if f.getTicket(t, "acme-4").SpecLanded {
		t.Error("a Stage Ticket is Spec-landed")
	}
}

func TestNoTicketIsSpecLandedWithLandPerSpecOffOrUnderBranch(t *testing.T) {
	f := landWorld(t, false)
	if f.getTicket(t, "acme-5").SpecLanded {
		t.Error("acme-5 is Spec-landed with the option off")
	}
	f.landing(t, domain.Landing{Method: domain.LandingBranch, LandPerSpec: true})
	if f.getTicket(t, "acme-5").SpecLanded {
		t.Error("acme-5 is Spec-landed under Branch")
	}
	if st := f.landStatus(t); st != nil {
		t.Errorf("land per spec status under Branch = %+v, want none", st)
	}
}

func TestPlainSpecsTicketsAreSpecLandedButLongLivedSpecsAndPrototypesAreNot(t *testing.T) {
	f := landWorld(t, true)
	f.spec(t, "acme", "Plain")
	plain := f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-7", Title: "Plain work"})
	f.longLived(t, f.spec(t, "acme", "CI"))
	long := f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-9", Title: "Long work"})
	proto := f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "Try", Labels: []string{domain.PrototypeLabel}})
	retro := f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-7", Title: "Look back", Labels: []string{domain.RetroLabel}})
	for id, want := range map[string]bool{plain.ID: true, long.ID: false, proto.ID: false, retro.ID: false} {
		if got := f.getTicket(t, id).SpecLanded; got != want {
			t.Errorf("%s Spec-landed = %v, want %v", id, got, want)
		}
	}
	if f.specStatus(t, "acme-7") == nil {
		t.Error("plain Spec has no Land per Spec status")
	}
	if f.specStatus(t, "acme-9") != nil {
		t.Error("Long-lived Spec has a Land per Spec status")
	}
}

func TestATicketWithNoSpecMergeReadsAsNotMerged(t *testing.T) {
	f := landWorld(t, true)
	if sm := f.getTicket(t, "acme-5").SpecMerge; sm != nil {
		t.Errorf("spec merge = %+v, want none", sm)
	}
}

func TestReportSpecMergeRecordsTheCommitOfAnAcceptedSpecLandedTicket(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, domain.Accepted)
	f.clock.Advance(time.Minute)
	f.specMerge(t, "acme-5", "def456")
	sm := f.getTicket(t, "acme-5").SpecMerge
	if sm == nil || sm.Commit != "def456" || !sm.Counts || !sm.Merged.Equal(t0.Add(time.Minute)) {
		t.Errorf("spec merge = %+v, want def456 now, counting", sm)
	}
}

func TestReportSpecMergeIsRejectedForATicketNotAcceptedOrNotSpecLanded(t *testing.T) {
	f := landWorld(t, true)
	_, err := f.core.ReportSpecMerge(ctx, "acme", api.ReportSpecMergeInput{Ticket: "acme-5", Commit: "def456"})
	wantCode(t, err, api.ActionNotAllowed, "")
	f.landing(t, domain.Landing{Method: domain.LandingMerge, LandPerSpec: false})
	f.moveTo(t, 5, domain.Accepted)
	_, err = f.core.ReportSpecMerge(ctx, "acme", api.ReportSpecMergeInput{Ticket: "acme-5", Commit: "def456"})
	wantCode(t, err, api.ActionNotAllowed, "")
}

func TestASpecMergeNoLaterThanTheLastMoveBackToReadyDoesNotCount(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, domain.Accepted)
	f.specMerge(t, "acme-5", "def456")
	f.moveTo(t, 5, domain.Ready) // sent back at the same second
	f.moveTo(t, 5, domain.Accepted)
	if sm := f.getTicket(t, "acme-5").SpecMerge; sm == nil || sm.Counts {
		t.Errorf("spec merge after a send-back = %+v, want one that doesn't count", sm)
	}
	if f.getTicket(t, "acme-6").Blocked != true {
		t.Error("acme-6 is unblocked by a merge from before the send-back")
	}
	f.clock.Advance(time.Minute)
	f.specMerge(t, "acme-5", "def789")
	if sm := f.getTicket(t, "acme-5").SpecMerge; !sm.Counts {
		t.Errorf("a later spec merge = %+v, want one that counts", sm)
	}
}

func TestASameSpecBlockerClearsOnceItsSpecMergeIsReported(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 6, domain.Ready)
	f.moveTo(t, 5, domain.InReview)
	if !f.getTicket(t, "acme-6").Blocked {
		t.Fatal("acme-6 isn't blocked by an In Review Blocker")
	}
	f.moveTo(t, 5, domain.Accepted)
	if !f.getTicket(t, "acme-6").Blocked {
		t.Fatal("acme-6 isn't blocked by an Accepted Blocker with no Spec merge")
	}
	f.clock.Advance(time.Minute)
	f.specMerge(t, "acme-5", "def456")
	v := f.getTicket(t, "acme-6")
	if v.Blocked || len(v.Blockers) != 1 || !*v.Blockers[0].Cleared || !v.Claimable {
		t.Errorf("acme-6 = blocked %v, blockers %+v, claimable %v; want cleared and claimable", v.Blocked, v.Blockers, v.Claimable)
	}
}

func TestABlockerOnAnotherSpecDoesNotClearOnASpecMerge(t *testing.T) {
	f := landWorld(t, true)
	f.spec(t, "acme", "Other")
	other := f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-7", Title: "Other work"})
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-7", Title: "Stage", Labels: []string{domain.StageLabel("grilling")}})
	dependant := f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "Dependant", BlockedBy: []api.Ref{api.Ref(other.ID)}})
	f.moveTo(t, 8, domain.Accepted)
	f.specMerge(t, other.ID, "def456")
	if v := f.getTicket(t, dependant.ID); !v.Blocked || *v.Blockers[0].Cleared {
		t.Errorf("dependant = blocked %v, blockers %+v; want blocked by the other Spec's Blocker", v.Blocked, v.Blockers)
	}
}

func TestALongLivedSpecsBlockerDoesNotClearOnASpecMerge(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, domain.Accepted)
	f.landing(t, domain.Landing{Method: domain.LandingMerge, LandPerSpec: false})
	if !f.getTicket(t, "acme-6").Blocked {
		t.Error("acme-6 isn't blocked with the option off")
	}
}

func TestTheLandPerSpecStatusIsReadyOnceAllTicketsAreAcceptedAndOneIsMerged(t *testing.T) {
	f := landWorld(t, true)
	if st := f.landStatus(t); st == nil || st.Ready || len(st.Waiting) != 0 {
		t.Fatalf("status with nothing accepted = %+v", st)
	}
	f.moveTo(t, 5, domain.Accepted)
	f.specMerge(t, "acme-5", "def456")
	if st := f.landStatus(t); st.Ready || !slices.Equal(st.Waiting, []string{"acme-5"}) {
		t.Fatalf("status with acme-6 Ready = %+v, want acme-5 waiting, not ready", st)
	}
	f.moveTo(t, 6, domain.Accepted)
	if st := f.landStatus(t); !st.Ready || !slices.Equal(st.Waiting, []string{"acme-5", "acme-6"}) {
		t.Fatalf("status with both Accepted = %+v, want ready", st)
	}
}

func TestACancelledTicketDoesNotHoldASpecBackButATicketAddedLaterDoes(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, domain.Accepted)
	f.specMerge(t, "acme-5", "def456")
	f.moveTo(t, 6, domain.Cancelled)
	if st := f.landStatus(t); !st.Ready {
		t.Fatalf("status = %+v, want ready", st)
	}
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "Later"})
	if st := f.landStatus(t); st.Ready {
		t.Errorf("status with a Ticket added later = %+v, want not ready", st)
	}
}

func TestAPlainSpecIsReadyOnceEveryTicketIsFinishedAndOneIsMergedAndADraftHoldsItBack(t *testing.T) {
	f := landWorld(t, true)
	f.spec(t, "acme", "Plain") // acme-7
	a := f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-7", Title: "A"})
	b := f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-7", Title: "B"})
	f.moveTo(t, 8, domain.Accepted)
	f.specMerge(t, a.ID, "def456")
	if st := f.specStatus(t, "acme-7"); st == nil || st.Ready || !slices.Equal(st.Waiting, []string{a.ID}) {
		t.Fatalf("status with %s still a Draft = %+v, want %s waiting, not ready", b.ID, st, a.ID)
	}
	f.moveTo(t, 9, domain.Cancelled)
	if st := f.specStatus(t, "acme-7"); !st.Ready {
		t.Fatalf("status with every Ticket finished = %+v, want ready", st)
	}
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-7", Title: "Late"})
	if st := f.specStatus(t, "acme-7"); st.Ready {
		t.Errorf("status with a Ticket added later = %+v, want not ready", st)
	}
}

func TestASpecIsNotReadyWhileItsTicketsStageIsNotFinished(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, domain.Accepted)
	f.moveTo(t, 6, domain.Accepted)
	f.specMerge(t, "acme-5", "def456")
	f.moveTo(t, 4, domain.InProgress)
	if st := f.landStatus(t); st.Ready {
		t.Errorf("status = %+v, want not ready", st)
	}
	f.moveTo(t, 4, domain.Cancelled) // Implement cancels the tickets Stage
	if st := f.landStatus(t); !st.Ready {
		t.Errorf("status = %+v, want ready", st)
	}
}

func TestASpecWithNothingMergedIsNotReady(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, domain.Accepted)
	f.moveTo(t, 6, domain.Accepted)
	if st := f.landStatus(t); st.Ready {
		t.Errorf("status = %+v, want not ready", st)
	}
}

func TestAWaitingTicketIsFlaggedAndLeftOutOfAcceptedButNotLanded(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, domain.Accepted)
	f.clock.Advance(core.NotLandedAfter + time.Minute)
	if !f.getTicket(t, "acme-5").WaitingForSpec {
		t.Error("acme-5 isn't waiting for the Spec")
	}
	if r := f.getTicket(t, "acme-5").NeedsMeReason; r != "" {
		t.Errorf("reason = %q, want none", r)
	}
	rows, _ := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{State: []domain.State{domain.Accepted}})
	if len(rows) != 1 || !rows[0].WaitingForSpec {
		t.Errorf("find_tickets rows = %+v, want acme-5 waiting for the Spec", rows)
	}
	v, _ := f.core.NeedsMe(human)
	if v.Count != 0 {
		t.Errorf("needs me = %+v, want nothing", v)
	}
	// Turned off, the same Ticket is plain Accepted but not landed.
	f.landing(t, domain.Landing{Method: domain.LandingMerge, LandPerSpec: false})
	tk := f.getTicket(t, "acme-5")
	if tk.WaitingForSpec || tk.NeedsMeReason != api.ReasonAcceptedNotLanded {
		t.Errorf("acme-5 with the option off = waiting %v, reason %q", tk.WaitingForSpec, tk.NeedsMeReason)
	}
}
