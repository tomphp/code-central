package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Accepted and land (ticket 71, ADR 0006).

// inReviewOnBranch is acme-2 claimed (claim-1) and finished naming
// branch acme-2 at commit abc123, at t0+1h.
func (f fixture) inReviewOnBranch(t *testing.T) core.FinishResult {
	t.Helper()
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	f.clock.Set(t0.Add(time.Hour))
	res, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done on acme-2.", Branch: "acme-2", Commit: "abc123"})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	return res
}

func TestFinishRecordsTheBranchAndCommitItNames(t *testing.T) {
	f := newFixture(t)
	res := f.inReviewOnBranch(t)
	if res.Ticket.Branch != "acme-2" || res.Ticket.Commit != "abc123" {
		t.Fatalf("finish returned branch %q commit %q", res.Ticket.Branch, res.Ticket.Commit)
	}
	if v := f.get(t, "2"); v.Branch != "acme-2" || v.Commit != "abc123" {
		t.Fatalf("read returns branch %q commit %q", v.Branch, v.Commit)
	}
}

func TestAcceptingWorkOnABranchMovesItToAcceptedNotDone(t *testing.T) {
	f := newFixture(t)
	f.inReviewOnBranch(t)
	f.clock.Set(t0.Add(2 * time.Hour))
	v, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	want := core.MoveView{From: store.InReview, To: store.Accepted, Actor: "human", Time: t0.Add(2 * time.Hour)}
	if v.State != store.Accepted || v.Moves[len(v.Moves)-1] != want {
		t.Fatalf("state %s, last move %+v; want %+v", v.State, v.Moves[len(v.Moves)-1], want)
	}
}

func TestAcceptingWorkWithNoBranchStillGoesStraightToDone(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	// A commit alone is not a branch to land.
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "S", Commit: "abc123"}); err != nil {
		t.Fatal(err)
	}
	v, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
	if err != nil || v.State != store.Done {
		t.Fatalf("state %s, err %v; want Done", v.State, err)
	}
}

// accepted is acme-2 finished on a branch and accepted, at t0+2h.
func (f fixture) accepted(t *testing.T) {
	t.Helper()
	f.inReviewOnBranch(t)
	f.clock.Set(t0.Add(2 * time.Hour))
	if _, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"}); err != nil {
		t.Fatalf("accept: %v", err)
	}
}

func TestLandMovesAcceptedWorkToDoneRecordingTheMergeCommit(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.clock.Set(t0.Add(3 * time.Hour))
	v, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: "def456"})
	if err != nil {
		t.Fatal(err)
	}
	want := core.MoveView{From: store.Accepted, To: store.Done, Actor: "agent", Time: t0.Add(3 * time.Hour), Via: "merge", Commit: "def456"}
	if v.State != store.Done || v.Moves[len(v.Moves)-1] != want {
		t.Fatalf("state %s, last move %+v; want %+v", v.State, v.Moves[len(v.Moves)-1], want)
	}
}

func TestTheHumanCanLandWorkStillInReview(t *testing.T) {
	f := newFixture(t)
	f.inReviewOnBranch(t)
	v, err := f.core.Land(human, "acme", core.LandInput{Ticket: "2", Commit: "abc123"})
	if err != nil {
		t.Fatal(err)
	}
	if m := v.Moves[len(v.Moves)-1]; v.State != store.Done || m.From != store.InReview || m.Actor != "human" || m.Commit != "abc123" {
		t.Fatalf("state %s, last move %+v", v.State, m)
	}
}

func TestLandNeedsTheCommit(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	_, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: " "})
	wantCode(t, err, core.Invalid, "commit")
	if v := f.get(t, "2"); v.State != store.Accepted {
		t.Fatalf("state %s, want still Accepted", v.State)
	}
}

func TestLandIsOnlyFromAcceptedOrInReview(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	_, err := f.core.Land(human, "acme", core.LandInput{Ticket: "2", Commit: "abc123"})
	wantCode(t, err, core.ActionNotAllowed, "")
}

func TestAcceptedWorkStillBlocksUntilItLands(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Next", State: store.Ready, BlockedBy: []core.Ref{"acme-2"}}) // acme-3
	if v := f.get(t, "3"); !v.Blocked {
		t.Fatal("an Accepted Blocker cleared the block")
	}
	if _, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: "def456"}); err != nil {
		t.Fatal(err)
	}
	if v := f.get(t, "3"); v.Blocked {
		t.Fatal("landing the Blocker should clear the block")
	}
}

func TestFromAcceptedTheHumanCancelsOrMovesButCannotReject(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	_, err := f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: "No."})
	wantCode(t, err, core.ActionNotAllowed, "")
	_, err = f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")
	if v := f.cancel(t, core.CancelInput{Ticket: "2"}, core.Human); v.State != store.Cancelled {
		t.Fatalf("state %s, want Cancelled", v.State)
	}
	f.move(t, "2", store.Accepted)
	if v := f.move(t, "2", store.InReview); v.State != store.InReview {
		t.Fatalf("state %s, want In Review", v.State)
	}
}

func TestAnAgentCannotCancelAcceptedWork(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Instead"}) // acme-3
	_, err := f.core.Cancel(ctx, "acme", core.CancelInput{Ticket: "2", Replacements: []core.Ref{"3"}})
	wantCode(t, err, core.ActionNotAllowed, "")
}
