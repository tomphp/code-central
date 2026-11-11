package core_test

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Land per Spec (code-central-371), through core's public operations.

// landWorld is acme under Merge with Land per Spec set to landPerSpec, a
// Requirement Spec acme-1 with its Stage Tickets (acme-2 grilling, acme-3
// spec, acme-4 tickets, all Done) and implementation Tickets acme-5 and
// acme-6, acme-6 blocked by acme-5, both Ready.
func landWorld(t *testing.T, landPerSpec bool) fixture {
	t.Helper()
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Requirement")
	for _, stage := range core.Stages {
		f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: stage, Labels: []string{core.StageLabel(stage)}})
	}
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "First"})
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Second", BlockedBy: []core.Ref{"acme-5"}})
	f.landing(t, store.Landing{Method: store.LandingMerge, Remote: "origin", LandPerSpec: landPerSpec})
	for n := 2; n <= 4; n++ {
		f.moveTo(t, n, store.Done)
	}
	return f
}

func (f fixture) landing(t *testing.T, l store.Landing) {
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
func (f fixture) moveTo(t *testing.T, n int, state store.State) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", n)
		tk.Moves = append(tk.Moves, store.Move{From: tk.State, To: state, Time: f.clock.Now()})
		tk.State = state
		return w.PutTicket("acme", tk)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f fixture) getTicket(t *testing.T, id string) core.TicketView {
	t.Helper()
	v, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: core.Ref(id)})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) specMerge(t *testing.T, id, commit string) {
	t.Helper()
	if _, err := f.core.ReportSpecMerge(ctx, "acme", core.ReportSpecMergeInput{Ticket: core.Ref(id), Commit: commit}); err != nil {
		t.Fatalf("report_spec_merge: %v", err)
	}
}

func (f fixture) landStatus(t *testing.T) *core.LandPerSpecView {
	t.Helper()
	v, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Requirement == nil {
		t.Fatal("acme-1 isn't a Requirement")
	}
	return v.Requirement.LandPerSpec
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
	f.landing(t, store.Landing{Method: store.LandingBranch, LandPerSpec: true})
	if f.getTicket(t, "acme-5").SpecLanded {
		t.Error("acme-5 is Spec-landed under Branch")
	}
	if st := f.landStatus(t); st != nil {
		t.Errorf("land per spec status under Branch = %+v, want none", st)
	}
}

func TestTicketsOfPlainSpecsAndPrototypesAreNotSpecLanded(t *testing.T) {
	f := landWorld(t, true)
	f.spec(t, "acme", "Plain")
	plain := f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-7", Title: "Plain work"})
	proto := f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Try", Labels: []string{core.PrototypeLabel}})
	for _, tk := range []core.TicketView{plain, proto} {
		if f.getTicket(t, tk.ID).SpecLanded {
			t.Errorf("%s is Spec-landed", tk.ID)
		}
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
	f.moveTo(t, 5, store.Accepted)
	f.clock.Advance(time.Minute)
	f.specMerge(t, "acme-5", "def456")
	sm := f.getTicket(t, "acme-5").SpecMerge
	if sm == nil || sm.Commit != "def456" || !sm.Counts || !sm.Merged.Equal(t0.Add(time.Minute)) {
		t.Errorf("spec merge = %+v, want def456 now, counting", sm)
	}
}

func TestReportSpecMergeIsRejectedForATicketNotAcceptedOrNotSpecLanded(t *testing.T) {
	f := landWorld(t, true)
	_, err := f.core.ReportSpecMerge(ctx, "acme", core.ReportSpecMergeInput{Ticket: "acme-5", Commit: "def456"})
	wantCode(t, err, core.ActionNotAllowed, "")
	f.landing(t, store.Landing{Method: store.LandingMerge, LandPerSpec: false})
	f.moveTo(t, 5, store.Accepted)
	_, err = f.core.ReportSpecMerge(ctx, "acme", core.ReportSpecMergeInput{Ticket: "acme-5", Commit: "def456"})
	wantCode(t, err, core.ActionNotAllowed, "")
}

func TestASpecMergeNoLaterThanTheLastMoveBackToReadyDoesNotCount(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, store.Accepted)
	f.specMerge(t, "acme-5", "def456")
	f.moveTo(t, 5, store.Ready) // sent back at the same second
	f.moveTo(t, 5, store.Accepted)
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
	f.moveTo(t, 6, store.Ready)
	f.moveTo(t, 5, store.InReview)
	if !f.getTicket(t, "acme-6").Blocked {
		t.Fatal("acme-6 isn't blocked by an In Review Blocker")
	}
	f.moveTo(t, 5, store.Accepted)
	if !f.getTicket(t, "acme-6").Blocked {
		t.Fatal("acme-6 isn't blocked by an Accepted Blocker with no Spec merge")
	}
	f.clock.Advance(time.Minute)
	f.specMerge(t, "acme-5", "def456")
	v := f.getTicket(t, "acme-6")
	if v.Blocked || len(v.Blockers) != 1 || !v.Blockers[0].Cleared || !v.Claimable {
		t.Errorf("acme-6 = blocked %v, blockers %+v, claimable %v; want cleared and claimable", v.Blocked, v.Blockers, v.Claimable)
	}
}

func TestABlockerOnAnotherSpecDoesNotClearOnASpecMerge(t *testing.T) {
	f := landWorld(t, true)
	f.spec(t, "acme", "Other")
	other := f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-7", Title: "Other work"})
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-7", Title: "Stage", Labels: []string{core.StageLabel("grilling")}})
	dependant := f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Dependant", BlockedBy: []core.Ref{core.Ref(other.ID)}})
	f.moveTo(t, 8, store.Accepted)
	f.specMerge(t, other.ID, "def456")
	if v := f.getTicket(t, dependant.ID); !v.Blocked || v.Blockers[0].Cleared {
		t.Errorf("dependant = blocked %v, blockers %+v; want blocked by the other Spec's Blocker", v.Blocked, v.Blockers)
	}
}

func TestALongLivedSpecsBlockerDoesNotClearOnASpecMerge(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, store.Accepted)
	f.landing(t, store.Landing{Method: store.LandingMerge, LandPerSpec: false})
	if !f.getTicket(t, "acme-6").Blocked {
		t.Error("acme-6 isn't blocked with the option off")
	}
}

func TestTheLandPerSpecStatusIsReadyOnceAllTicketsAreAcceptedAndOneIsMerged(t *testing.T) {
	f := landWorld(t, true)
	if st := f.landStatus(t); st == nil || st.Ready || len(st.Waiting) != 0 {
		t.Fatalf("status with nothing accepted = %+v", st)
	}
	f.moveTo(t, 5, store.Accepted)
	f.specMerge(t, "acme-5", "def456")
	if st := f.landStatus(t); st.Ready || !slices.Equal(st.Waiting, []string{"acme-5"}) {
		t.Fatalf("status with acme-6 Ready = %+v, want acme-5 waiting, not ready", st)
	}
	f.moveTo(t, 6, store.Accepted)
	if st := f.landStatus(t); !st.Ready || !slices.Equal(st.Waiting, []string{"acme-5", "acme-6"}) {
		t.Fatalf("status with both Accepted = %+v, want ready", st)
	}
}

func TestACancelledTicketDoesNotHoldASpecBackButATicketAddedLaterDoes(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, store.Accepted)
	f.specMerge(t, "acme-5", "def456")
	f.moveTo(t, 6, store.Cancelled)
	if st := f.landStatus(t); !st.Ready {
		t.Fatalf("status = %+v, want ready", st)
	}
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Later"})
	if st := f.landStatus(t); st.Ready {
		t.Errorf("status with a Ticket added later = %+v, want not ready", st)
	}
}

func TestASpecIsNotReadyWhileItsTicketsStageIsNotFinished(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, store.Accepted)
	f.moveTo(t, 6, store.Accepted)
	f.specMerge(t, "acme-5", "def456")
	f.moveTo(t, 4, store.InProgress)
	if st := f.landStatus(t); st.Ready {
		t.Errorf("status = %+v, want not ready", st)
	}
	f.moveTo(t, 4, store.Cancelled) // Implement cancels the tickets Stage
	if st := f.landStatus(t); !st.Ready {
		t.Errorf("status = %+v, want ready", st)
	}
}

func TestASpecWithNothingMergedIsNotReady(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, store.Accepted)
	f.moveTo(t, 6, store.Accepted)
	if st := f.landStatus(t); st.Ready {
		t.Errorf("status = %+v, want not ready", st)
	}
}

func TestAWaitingTicketIsFlaggedAndLeftOutOfAcceptedButNotLanded(t *testing.T) {
	f := landWorld(t, true)
	f.moveTo(t, 5, store.Accepted)
	f.clock.Advance(core.NotLandedAfter + time.Minute)
	if !f.getTicket(t, "acme-5").WaitingForSpec {
		t.Error("acme-5 isn't waiting for the Spec")
	}
	if r := f.getTicket(t, "acme-5").NeedsMeReason; r != "" {
		t.Errorf("reason = %q, want none", r)
	}
	v, _ := f.core.NeedsMe(human)
	if v.Count != 0 {
		t.Errorf("needs me = %+v, want nothing", v)
	}
	// Turned off, the same Ticket is plain Accepted but not landed.
	f.landing(t, store.Landing{Method: store.LandingMerge, LandPerSpec: false})
	tk := f.getTicket(t, "acme-5")
	if tk.WaitingForSpec || tk.NeedsMeReason != core.ReasonAcceptedNotLanded {
		t.Errorf("acme-5 with the option off = waiting %v, reason %q", tk.WaitingForSpec, tk.NeedsMeReason)
	}
}
