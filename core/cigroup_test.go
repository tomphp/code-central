package core_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Per-Spec CI failed, cancelled and unavailable (code-central-379),
// through core's public operations.

// groupWorld is landWorld with Land per Spec on, acme under Merge + Push
// with CI Tracking, and acme-5 then acme-6 Accepted a minute apart with
// the Spec's merge pushed as abc123 for both.
func groupWorld(t *testing.T) fixture {
	t.Helper()
	f := landWorld(t, true)
	f.landing(t, store.Landing{Method: store.LandingMerge, Push: true, Remote: "origin", LandPerSpec: true})
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI = store.CI{Tracking: true, Provider: store.CIGitHubActions, Repository: "acme/web"}
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, 5, store.Accepted)
	f.clock.Advance(time.Minute)
	f.moveTo(t, 6, store.Accepted)
	f.clock.Advance(time.Minute)
	for _, n := range []int{5, 6} {
		err = f.store.Update(func(w store.Writer) error {
			tk, _ := w.Ticket("acme", n)
			tk.PushedCommits = []store.PushedCommit{{Commit: "abc123", Pushed: f.clock.Now()}}
			return w.PutTicket("acme", tk)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f fixture) reportGroupRuns(t *testing.T, conclusion store.CIConclusion) {
	t.Helper()
	f.reportRun(t, "acme-5", store.CICompleted, conclusion)
	started := f.clock.Now()
	run := core.CIRunView{ID: 2, Attempt: 1, Workflow: "ci.yml", Name: "CI", Branch: "main", Commit: "abc123",
		Status: store.CICompleted, Conclusion: conclusion, Started: &started, Finished: &started, URL: "https://github.com/acme/web/actions/runs/2", Ticket: "acme-6"}
	if _, err := f.core.ReportCIRuns(ctx, "acme", core.ReportCIRunsInput{Runs: []core.CIRunView{run}}); err != nil {
		t.Fatal(err)
	}
}

func notAllowed(t *testing.T, err error) {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.ActionNotAllowed {
		t.Fatalf("error %v, want action_not_allowed", err)
	}
}

func TestCIFailedOfAWaitingGroupGoesToTheLeadTicketOnly(t *testing.T) {
	f := groupWorld(t)
	f.reportGroupRuns(t, store.CIFailure)
	lead, other := f.getTicket(t, "acme-5"), f.getTicket(t, "acme-6")
	if lead.NeedsMeReason != core.ReasonCIFailed || other.NeedsMeReason != "" {
		t.Fatalf("reasons %q and %q, want CI failed on acme-5 only", lead.NeedsMeReason, other.NeedsMeReason)
	}
	g := lead.CIGroup
	if g == nil || g.Spec != "acme-1" || g.Waiting != 2 || !slices.Equal(g.Tickets, []string{"acme-5", "acme-6"}) {
		t.Fatalf("group %+v, want acme-1 with acme-5 and acme-6 waiting", g)
	}
	if other.CIGroup == nil || other.CIGroup.Lead != "acme-5" || other.CIFailureOptions != nil {
		t.Fatalf("other: group %+v, options %v", other.CIGroup, other.CIFailureOptions)
	}
	v, _ := f.core.NeedsMe(human)
	if v.Count != 1 || v.Groups[0].Tickets[0].ID != "acme-5" || v.Groups[0].Tickets[0].CIGroup == nil || v.Groups[0].Tickets[0].CIGroup.Waiting != 2 {
		t.Fatalf("needs me %+v, want one row for acme-5 naming the group", v)
	}
	if want := []string{"rerun_ci", "land_anyway", "new_fix_ticket"}; !slices.Equal(lead.CIFailureOptions, want) {
		t.Fatalf("options %v, want %v with Send back to fix left out", lead.CIFailureOptions, want)
	}
}

func TestCICancelledOfAWaitingGroupGoesToTheLeadTicketOnly(t *testing.T) {
	f := groupWorld(t)
	f.reportGroupRuns(t, store.CICancelled)
	if r1, r2 := f.getTicket(t, "acme-5").NeedsMeReason, f.getTicket(t, "acme-6").NeedsMeReason; r1 != core.ReasonCICancelled || r2 != "" {
		t.Fatalf("reasons %q and %q, want CI cancelled on acme-5 only", r1, r2)
	}
	if want := []string{"rerun_ci", "land_anyway"}; !slices.Equal(f.getTicket(t, "acme-5").CIFailureOptions, want) {
		t.Fatalf("options %v, want %v", f.getTicket(t, "acme-5").CIFailureOptions, want)
	}
}

func TestCIUnavailableOfAWaitingGroupGoesToTheLeadTicketOnly(t *testing.T) {
	f := groupWorld(t)
	f.clock.Advance(core.CIUnavailableAfter + time.Minute)
	if r1, r2 := f.getTicket(t, "acme-5").NeedsMeReason, f.getTicket(t, "acme-6").NeedsMeReason; r1 != core.ReasonCIUnavailable || r2 != "" {
		t.Fatalf("reasons %q and %q, want CI unavailable on acme-5 only", r1, r2)
	}
	v, _ := f.core.NeedsMe(human)
	if v.Count != 1 || v.Groups[0].Tickets[0].CIGroup == nil || v.Groups[0].Tickets[0].CIGroup.Spec != "acme-1" {
		t.Fatalf("needs me %+v, want one row naming the Spec", v)
	}
}

func TestLandAnywayOnTheLeadLandsEveryWaitingTicket(t *testing.T) {
	f := groupWorld(t)
	f.reportGroupRuns(t, store.CIFailure)
	if _, err := f.core.LandAnyway(human, "acme", core.TicketInput{Ticket: "acme-6"}); err == nil {
		t.Fatal("land anyway on a non-lead Ticket was allowed")
	} else {
		notAllowed(t, err)
	}
	if _, err := f.core.LandAnyway(human, "acme", core.TicketInput{Ticket: "acme-5"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"acme-5", "acme-6"} {
		if tk := f.getTicket(t, id); !tk.LandAnyway || tk.NeedsMeReason != "" {
			t.Errorf("%s: land anyway %v, reason %q, want accepted and nothing waiting", id, tk.LandAnyway, tk.NeedsMeReason)
		}
	}
}

func TestRerunOnTheLeadReRunsTheSharedCommitForEveryWaitingTicket(t *testing.T) {
	f := groupWorld(t)
	f.reportGroupRuns(t, store.CIFailure)
	if _, err := f.core.RerunCI(human, "acme", core.TicketInput{Ticket: "acme-5"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"acme-5", "acme-6"} {
		if tk := f.getTicket(t, id); tk.CIResult != core.CIRunning || tk.NeedsMeReason != "" {
			t.Errorf("%s: result %q, reason %q, want running with no reason", id, tk.CIResult, tk.NeedsMeReason)
		}
	}
}

func TestNewFixTicketOnTheLeadLandsTheGroupAndOpensAFixTicketOnTheSpec(t *testing.T) {
	f := groupWorld(t)
	f.reportGroupRuns(t, store.CIFailure)
	if _, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-5", Note: "ci.yml failed"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"acme-5", "acme-6"} {
		if tk := f.getTicket(t, id); !tk.LandAnyway || tk.NeedsMeReason != "" {
			t.Errorf("%s: land anyway %v, reason %q", id, tk.LandAnyway, tk.NeedsMeReason)
		}
	}
	fix := f.getTicket(t, "acme-7")
	if fix.State != store.Ready || fix.Spec.ID != "acme-1" {
		t.Fatalf("fix Ticket %s on %q, want Ready on acme-1", fix.State, fix.Spec.ID)
	}
}

func TestSendBackToFixIsRejectedInAWaitingGroup(t *testing.T) {
	f := groupWorld(t)
	f.reportGroupRuns(t, store.CIFailure)
	for _, id := range []core.Ref{"acme-5", "acme-6"} {
		_, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: id, Note: "fix it"})
		notAllowed(t, err)
	}
	if tk := f.getTicket(t, "acme-5"); tk.State != store.Accepted {
		t.Fatalf("acme-5 is %s, want Accepted", tk.State)
	}
}

func TestACITicketOutsideLandPerSpecKeepsItsOwnReasonAndOptions(t *testing.T) {
	f := groupWorld(t)
	f.landing(t, store.Landing{Method: store.LandingMerge, Push: true, Remote: "origin", LandPerSpec: false})
	f.reportGroupRuns(t, store.CIFailure)
	for _, id := range []string{"acme-5", "acme-6"} {
		tk := f.getTicket(t, id)
		if tk.NeedsMeReason != core.ReasonCIFailed || tk.CIGroup != nil {
			t.Errorf("%s: reason %q, group %+v, want its own CI failed", id, tk.NeedsMeReason, tk.CIGroup)
		}
		if want := []string{"rerun_ci", "land_anyway", "send_back", "new_fix_ticket"}; !slices.Equal(tk.CIFailureOptions, want) {
			t.Errorf("%s: options %v, want %v", id, tk.CIFailureOptions, want)
		}
	}
}
