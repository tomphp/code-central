package core_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Land per Spec under Pull Request (code-central-395): a Spec's Tickets
// gather on its Spec Branch, which is pushed as one piece of pushed work
// with one pull request, carried by the Spec's lead Ticket in Needs Me.

const specBranch = "spec/acme-1"

// specPullRequestWorld is landWorld under Pull Request with Land per Spec
// on.
func specPullRequestWorld(t *testing.T) fixture {
	t.Helper()
	f := landWorld(t, true)
	f.landing(t, store.Landing{Method: store.LandingPullRequest, Remote: "origin", LandPerSpec: true})
	return f
}

// acceptedOnSpecBranch finishes Ticket n with a summary, accepts it and
// merges it into the Spec Branch.
func (f fixture) acceptedOnSpecBranch(t *testing.T, n int, summary string) {
	t.Helper()
	at := f.clock.Now()
	err := f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", n)
		tk.Comments = append(tk.Comments, store.Comment{Number: 1, Actor: "agent", Time: at, Body: summary})
		tk.Moves = append(tk.Moves, store.Move{From: tk.State, To: store.InReview, Time: at})
		return w.PutTicket("acme", tk)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	f.moveTo(t, n, store.Accepted)
	f.specMerge(t, "acme-"+itoa(n), "merge"+itoa(n))
	f.clock.Advance(time.Minute)
}

func itoa(n int) string { return string(rune('0' + n)) }

// pushSpecBranch reports the Spec Branch pushed at commit for the Tickets.
func (f fixture) pushSpecBranch(t *testing.T, commit string, tickets ...string) {
	t.Helper()
	for _, id := range tickets {
		if _, err := f.core.ReportPushedCommit(ctx, "acme", core.ReportPushedCommitInput{Ticket: core.Ref(id), Commit: commit, Branch: specBranch}); err != nil {
			t.Fatalf("report_pushed_commit %s: %v", id, err)
		}
	}
	f.clock.Advance(time.Minute)
}

func TestUnderPullRequestARequirementsTicketsGatherOnItsSpecBranch(t *testing.T) {
	f := specPullRequestWorld(t)
	if !f.getTicket(t, "acme-5").SpecLanded {
		t.Fatal("acme-5 isn't Spec-landed under Pull Request")
	}
	f.acceptedOnSpecBranch(t, 5, "Built the first part.")
	f.acceptedOnSpecBranch(t, 6, "Built the second part.")
	st := f.landStatus(t)
	if st == nil || !st.Ready || !slices.Equal(st.Waiting, []string{"acme-5", "acme-6"}) {
		t.Fatalf("land per spec = %+v, want both waiting and ready", st)
	}
}

func TestASameSpecBlockerClearsOnItsSpecMergeUnderPullRequest(t *testing.T) {
	f := specPullRequestWorld(t)
	if v := f.getTicket(t, "acme-6"); !v.Blocked {
		t.Fatalf("acme-6 not blocked before acme-5 is merged")
	}
	f.acceptedOnSpecBranch(t, 5, "Built the first part.")
	if v := f.getTicket(t, "acme-6"); v.Blocked {
		t.Fatalf("acme-6 still blocked after acme-5 merged into the Spec Branch")
	}
}

func TestThePullRequestWatchListsTheSpecBranchAsOnePieceOfPushedWork(t *testing.T) {
	f := specPullRequestWorld(t)
	f.acceptedOnSpecBranch(t, 5, "Built the first part.")
	f.acceptedOnSpecBranch(t, 6, "Built the second part.")
	f.pushSpecBranch(t, "tip1", "acme-5", "acme-6")

	w := f.watch(t)
	if len(w.Work) != 1 {
		t.Fatalf("work = %+v, want the Spec Branch only", w.Work)
	}
	work := w.Work[0]
	if work.Branch != specBranch || work.Commit != "tip1" || !slices.Equal(work.Tickets, []string{"acme-5", "acme-6"}) {
		t.Fatalf("work = %+v", work)
	}
	if work.Title != "Requirement" {
		t.Errorf("title %q, want the Spec's title", work.Title)
	}
	for _, want := range []string{"acme-1", "acme-5", "First", "Built the first part.", "acme-6", "Second", "Built the second part."} {
		if !strings.Contains(work.Body, want) {
			t.Errorf("body %q lacks %q", work.Body, want)
		}
	}
}

func TestAPullRequestForTheSpecBranchIsRecordedOnEveryWaitingTicket(t *testing.T) {
	f := specPullRequestWorld(t)
	f.acceptedOnSpecBranch(t, 5, "one")
	f.acceptedOnSpecBranch(t, 6, "two")
	f.pushSpecBranch(t, "tip1", "acme-5", "acme-6")
	f.reportPR(t, core.ReportPullRequestInput{Branch: specBranch, State: store.PRMerged, MergeCommit: "m1"})
	for _, id := range []string{"acme-5", "acme-6"} {
		if pr := f.getTicket(t, id).PullRequest; pr == nil || pr.State != store.PRMerged || pr.MergeCommit != "m1" {
			t.Errorf("%s pull request = %+v, want merged at m1", id, pr)
		}
	}
}

func TestTheSpecsToMergeIsOneItemOnItsLeadTicket(t *testing.T) {
	f := specPullRequestWorld(t)
	f.acceptedOnSpecBranch(t, 5, "one")
	f.acceptedOnSpecBranch(t, 6, "two")
	if got := f.getTicket(t, "acme-5").NeedsMeReason; got != "" {
		t.Fatalf("reason %q before the Spec Branch is pushed, want none", got)
	}
	f.pushSpecBranch(t, "tip1", "acme-5", "acme-6")
	f.reportPR(t, core.ReportPullRequestInput{Branch: specBranch, State: store.PRConflicting})

	wantReason(t, f.getTicket(t, "acme-5"), core.ReasonToMerge)
	if got := f.getTicket(t, "acme-6").NeedsMeReason; got != "" {
		t.Fatalf("acme-6 reason %q, want none: the Spec is one item on acme-5", got)
	}
	row, ok := longRow(t, f, "acme-5")
	if !ok || row.PullRequest == nil || row.PullRequest.State != store.PRConflicting || row.Branch != specBranch {
		t.Fatalf("row %+v, want the Spec's conflicting pull request on its lead", row)
	}
	if _, ok := longRow(t, f, "acme-6"); ok {
		t.Fatal("acme-6 has a Needs Me row")
	}
}

func TestASpecsToMergeOffersNewFixTicketNotSendBack(t *testing.T) {
	f := specPullRequestWorld(t)
	f.acceptedOnSpecBranch(t, 5, "one")
	f.acceptedOnSpecBranch(t, 6, "two")
	f.pushSpecBranch(t, "tip1", "acme-5", "acme-6")
	f.reportPR(t, core.ReportPullRequestInput{Branch: specBranch, State: store.PROpen})

	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "acme-5", Note: "x"}); err == nil {
		t.Fatal("send_back of a Spec's lead succeeded")
	}
	fix, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-5", Note: "reviewers asked for a rename", Full: true})
	if err != nil {
		t.Fatalf("new_fix_ticket: %v", err)
	}
	if fix.Record == nil || fix.Record.State != store.Ready || fix.Record.Spec.ID != "acme-1" {
		t.Fatalf("fix Ticket = %+v, want a Ready Ticket on acme-1", fix.Record)
	}
	for _, id := range []string{"acme-5", "acme-6"} {
		if got := f.getTicket(t, id).State; got != store.Accepted {
			t.Errorf("%s is %s, want it still waiting Accepted", id, got)
		}
	}
}

func TestASingleTicketsToMergeDoesNotOfferNewFixTicket(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	if _, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "2", Note: "x"}); err == nil {
		t.Fatal("new_fix_ticket on a single Ticket's To merge succeeded")
	}
}

func TestASpecsClosedPullRequestIsOneItemOffersNewFixTicketAndCancel(t *testing.T) {
	f := specPullRequestWorld(t)
	f.acceptedOnSpecBranch(t, 5, "one")
	f.acceptedOnSpecBranch(t, 6, "two")
	f.pushSpecBranch(t, "tip1", "acme-5", "acme-6")
	f.reportPR(t, core.ReportPullRequestInput{Branch: specBranch, State: store.PRClosed})

	wantReason(t, f.getTicket(t, "acme-5"), core.ReasonPullRequestClosed)
	if got := f.getTicket(t, "acme-6").NeedsMeReason; got != "" {
		t.Fatalf("acme-6 reason %q, want none", got)
	}
	if _, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-5", Note: "redo it"}); err != nil {
		t.Fatalf("new_fix_ticket: %v", err)
	}

	if _, err := f.core.Cancel(human, "acme", core.CancelInput{Ticket: "acme-5"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	for _, id := range []string{"acme-5", "acme-6"} {
		if got := f.getTicket(t, id).State; got != store.Cancelled {
			t.Errorf("%s is %s, want Cancelled with the Spec's waiting Tickets", id, got)
		}
	}
}

func TestATicketOutsideLandPerSpecGetsItsOwnPullRequestUnderPullRequest(t *testing.T) {
	f := landWorld(t, true)
	f.landing(t, store.Landing{Method: store.LandingPullRequest, Remote: "origin", LandPerSpec: false})
	f.moveTo(t, 5, store.Accepted)
	if _, err := f.core.ReportPushedCommit(ctx, "acme", core.ReportPushedCommitInput{Ticket: "acme-5", Commit: "c5", Branch: "acme-5"}); err != nil {
		t.Fatal(err)
	}
	w := f.watch(t)
	if len(w.Work) != 1 || w.Work[0].Branch != "acme-5" || w.Work[0].Title != "First" || !slices.Equal(w.Work[0].Tickets, []string{"acme-5"}) {
		t.Fatalf("work = %+v, want acme-5's own branch", w.Work)
	}
	if f.getTicket(t, "acme-5").SpecLanded {
		t.Error("acme-5 is Spec-landed with Land per Spec off")
	}
}
