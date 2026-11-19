package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Accepted and land (ticket 71, ADR 0006).

// inReviewOnBranch is acme-2 claimed (claim-1) and finished naming
// branch acme-2 at commit abc123, at t0+1h.
func (f fixture) inReviewOnBranch(t *testing.T) api.FinishResult {
	t.Helper()
	f.work(t, domain.Ready)
	f.claim(t, api.ClaimInput{})
	f.clock.Set(t0.Add(time.Hour))
	res, err := f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done on acme-2.", Branch: "acme-2", Commit: "abc123", Full: true})
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
	v, err := record(f.core.Accept(human, "acme", api.TicketInput{Ticket: "2", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	want := api.MoveView{From: domain.InReview, To: domain.Accepted, Actor: "human", Time: t0.Add(2 * time.Hour)}
	if v.State != domain.Accepted || v.Moves[len(v.Moves)-1] != want {
		t.Fatalf("state %s, last move %+v; want %+v", v.State, v.Moves[len(v.Moves)-1], want)
	}
}

func TestAcceptingWorkWithNoBranchStillGoesStraightToDone(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, api.ClaimInput{})
	// A commit alone is not a branch to land.
	if _, err := f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "S", Commit: "abc123"}); err != nil {
		t.Fatal(err)
	}
	v, err := record(f.core.Accept(human, "acme", api.TicketInput{Ticket: "2", Full: true}))
	if err != nil || v.State != domain.Done {
		t.Fatalf("state %s, err %v; want Done", v.State, err)
	}
}

// accepted is acme-2 finished on a branch and accepted, at t0+2h.
func (f fixture) accepted(t *testing.T) {
	t.Helper()
	f.inReviewOnBranch(t)
	f.clock.Set(t0.Add(2 * time.Hour))
	if _, err := f.core.Accept(human, "acme", api.TicketInput{Ticket: "2"}); err != nil {
		t.Fatalf("accept: %v", err)
	}
}

func TestLandMovesAcceptedWorkToDoneRecordingTheMergeCommit(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.clock.Set(t0.Add(3 * time.Hour))
	v, err := record(f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "def456", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	want := api.MoveView{From: domain.Accepted, To: domain.Done, Actor: "agent", Time: t0.Add(3 * time.Hour), Via: "merge", Commit: "def456"}
	if v.State != domain.Done || v.Moves[len(v.Moves)-1] != want {
		t.Fatalf("state %s, last move %+v; want %+v", v.State, v.Moves[len(v.Moves)-1], want)
	}
}

func TestTheHumanCanLandWorkStillInReview(t *testing.T) {
	f := newFixture(t)
	f.inReviewOnBranch(t)
	v, err := record(f.core.Land(human, "acme", api.LandInput{Ticket: "2", Commit: "abc123", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	if m := v.Moves[len(v.Moves)-1]; v.State != domain.Done || m.From != domain.InReview || m.Actor != "human" || m.Commit != "abc123" {
		t.Fatalf("state %s, last move %+v", v.State, m)
	}
}

func TestLandNeedsTheCommit(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	_, err := f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: " "})
	wantCode(t, err, api.Invalid, "commit")
	if v := f.get(t, "2"); v.State != domain.Accepted {
		t.Fatalf("state %s, want still Accepted", v.State)
	}
}

func TestLandIsOnlyFromAcceptedOrInReview(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	_, err := f.core.Land(human, "acme", api.LandInput{Ticket: "2", Commit: "abc123"})
	wantCode(t, err, api.ActionNotAllowed, "")
}

func TestAcceptedWorkStillBlocksUntilItLands(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Next", State: domain.Ready, BlockedBy: []api.Ref{"acme-2"}}) // acme-3
	if v := f.get(t, "3"); !v.Blocked {
		t.Fatal("an Accepted Blocker cleared the block")
	}
	if _, err := f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "def456"}); err != nil {
		t.Fatal(err)
	}
	if v := f.get(t, "3"); v.Blocked {
		t.Fatal("landing the Blocker should clear the block")
	}
}

func TestFromAcceptedTheHumanCancelsOrMovesButCannotReject(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	_, err := f.core.Reject(human, "acme", api.RejectInput{Ticket: "2", Comment: "No."})
	wantCode(t, err, api.ActionNotAllowed, "")
	_, err = f.core.Accept(human, "acme", api.TicketInput{Ticket: "2"})
	wantCode(t, err, api.ActionNotAllowed, "")
	if v := f.cancel(t, api.CancelInput{Ticket: "2"}, api.Human); v.State != domain.Cancelled {
		t.Fatalf("state %s, want Cancelled", v.State)
	}
	f.move(t, "2", domain.Accepted)
	if v := f.move(t, "2", domain.InReview); v.State != domain.InReview {
		t.Fatalf("state %s, want In Review", v.State)
	}
}

func TestAnAgentCannotCancelAcceptedWork(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Instead"}) // acme-3
	_, err := f.core.Cancel(ctx, "acme", api.CancelInput{Ticket: "2", Replacements: []api.Ref{"3"}})
	wantCode(t, err, api.ActionNotAllowed, "")
}
