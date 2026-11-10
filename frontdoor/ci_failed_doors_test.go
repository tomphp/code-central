package frontdoor_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/store"
)

// CI failed and Land anyway (code-central-261): an Accepted Ticket whose
// CI result is failed waits in Needs Me as CI failed until the human
// chooses a failure option; land_anyway, the human's, is the first.
func init() {
	cases["land_anyway"] = opCase{
		setup: failedCI,
		ok:    call{http: landAnyway("3")},
		check: ticketAcknowledgement("acme-3", "Accepted", "land_anyway", "ci_failure_options", "needs_me_reason"),
		full:  func(t *testing.T, out map[string]any) { boolField(t, out, "land_anyway", true) },
		bad:   call{http: landAnyway("2")},
		code:  core.ActionNotAllowed,
	}
}

func landAnyway(n string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/" + n + "/land_anyway", `{}`}
}

func boolField(t *testing.T, out map[string]any, name string, want bool) {
	t.Helper()
	if got, _ := out[name].(bool); got != want {
		t.Fatalf("%s = %v, want %v in %v", name, out[name], want, out)
	}
}

// failedAt is when failedCI's run finished.
var failedAt = codecentraltest.Start.Add(5 * time.Minute)

// failedCI is pushedAccepted with a failed run of ci.yml, finished at
// failedAt, and a passed run of lint.yml, on acme-3's pushed commit.
func failedCI(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	pushedAccepted(t, srv)
	failed := ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CIFailure)
	failed.Finished = &failedAt
	reportRuns(t, srv, failed, ciRun(2, "lint.yml", "abc123", store.CICompleted, store.CISuccess))
}

// needsMeRowOf is id's Needs Me row, if it has one.
func needsMeRowOf(t *testing.T, srv *codecentraltest.Server, id string) (core.NeedsMeRow, bool) {
	t.Helper()
	v, err := srv.Core.NeedsMe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range v.Groups {
		for _, row := range g.Tickets {
			if row.ID == id {
				return row, true
			}
		}
	}
	return core.NeedsMeRow{}, false
}

func TestAnAcceptedTicketWhoseCIFailedIsInNeedsMeAsCIFailed(t *testing.T) {
	srv := newServer(t, failedCI)
	srv.Clock.Advance(time.Hour)
	row, ok := needsMeRowOf(t, srv, "acme-3")
	if !ok || row.Reason != core.ReasonCIFailed || !row.Since.Equal(failedAt) {
		t.Fatalf("row %+v, want CI failed since its run failed", row)
	}
	if row.CIFailure == nil || row.CIFailure.Commit != "abc123" || !slices.Equal(row.CIFailure.Workflows, []string{"ci.yml"}) {
		t.Fatalf("ci failure %+v, want abc123 failing ci.yml", row.CIFailure)
	}
	tk := ticket3(t, srv)
	if tk.NeedsMeReason != core.ReasonCIFailed || !slices.Equal(tk.CIFailureOptions, []string{"land_anyway"}) || tk.LandAnyway {
		t.Fatalf("reason %q, options %v, land anyway %v: want CI failed offering Land anyway", tk.NeedsMeReason, tk.CIFailureOptions, tk.LandAnyway)
	}
}

func TestCIFailedIsGroupedAheadOfToMerge(t *testing.T) {
	order := workflowNeedsMe(t)
	in, merge := slices.Index(order, core.ReasonInReview), slices.Index(order, core.ReasonToMerge)
	if ci := slices.Index(order, core.ReasonCIFailed); ci < 0 || ci < in || ci > merge {
		t.Fatalf("Needs Me order %v, want CI failed after In Review and ahead of To merge", order)
	}
}

// workflowNeedsMe is the Needs Me reasons in group order, as the
// published workflow lists them.
func workflowNeedsMe(t *testing.T) []core.NeedsMeReason {
	t.Helper()
	w, err := newServer(t).Core.GetWorkflow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []core.NeedsMeReason
	for _, r := range w.NeedsMe {
		out = append(out, r.Reason)
	}
	return out
}

func TestOnlyAFailedCIResultIsCIFailed(t *testing.T) {
	for name, runs := range map[string][]core.CIRunView{
		"running": {ciRun(1, "ci.yml", "abc123", store.CIInProgress, "")},
		"passed":  {ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CISuccess)},
		"ignored": {ciRun(1, "nightly.yml", "abc123", store.CICompleted, store.CIFailure)},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t, pushedAccepted)
			reportRuns(t, srv, runs...)
			if row, ok := needsMeRowOf(t, srv, "acme-3"); ok && row.Reason == core.ReasonCIFailed {
				t.Fatalf("row %+v, want no CI failed", row)
			}
			if tk := ticket3(t, srv); tk.CIFailureOptions != nil {
				t.Fatalf("options %v, want none", tk.CIFailureOptions)
			}
		})
	}
}

func TestLandAnywayClearsCIFailedAndTheFactoryReadsIt(t *testing.T) {
	srv := newServer(t, failedCI)
	httpOK(t, srv, landAnyway("3"))
	if row, ok := needsMeRowOf(t, srv, "acme-3"); ok {
		t.Fatalf("row %+v, want acme-3 out of Needs Me once the human chose Land anyway", row)
	}
	tk := ticket3(t, srv)
	if !tk.LandAnyway || tk.CIResult != core.CIFailed || tk.CIFailureOptions != nil {
		t.Fatalf("land anyway %v, ci result %q, options %v: want Land anyway on the failed result, no options left", tk.LandAnyway, tk.CIResult, tk.CIFailureOptions)
	}
	// Choosing it again changes nothing.
	out := httpOK(t, srv, landAnyway("3"))
	if changed, _ := out["changed"].([]any); len(changed) != 0 {
		t.Fatalf("changed %v, want nothing on a second Land anyway", changed)
	}
}

// A newer pushed commit, after a send-back and re-accept, is a new CI
// result: the human's Land anyway was for the old one.
func TestLandAnywayHoldsOnlyForThePushedCommitItWasChosenFor(t *testing.T) {
	srv := newServer(t, failedCI)
	httpOK(t, srv, landAnyway("3"))
	pushCommit(t, srv, "def456")
	reportRuns(t, srv, ciRun(3, "ci.yml", "def456", store.CICompleted, store.CIFailure))
	tk := ticket3(t, srv)
	if tk.LandAnyway || tk.NeedsMeReason != core.ReasonCIFailed {
		t.Fatalf("land anyway %v, reason %q: want def456's failure CI failed again", tk.LandAnyway, tk.NeedsMeReason)
	}
}

func TestLandAnywayNeedsAFailedCIResult(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, *codecentraltest.Server){
		"never pushed": trackedAccepted,
		"running":      pushedAccepted,
		"passed": func(t *testing.T, srv *codecentraltest.Server) {
			pushedAccepted(t, srv)
			reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CISuccess))
		},
		"not Accepted": func(t *testing.T, srv *codecentraltest.Server) {
			ciTrackingOn(t, srv)
			pushOn(t, srv)
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t, setup)
			httpRejected(t, srv, landAnyway("3"), core.ActionNotAllowed)
			if ticket3(t, srv).LandAnyway {
				t.Fatal("Land anyway recorded on a refused call")
			}
		})
	}
}

// Under Branch + Push the human's merge decides, so Land anyway isn't
// offered, and is refused.
func TestLandAnywayIsNotOfferedUnderBranch(t *testing.T) {
	srv := newServer(t, failedCI)
	httpOK(t, srv, setLandingMethod(`{"method":"branch","push":true,"remote":"origin"}`))
	tk := ticket3(t, srv)
	if tk.NeedsMeReason != core.ReasonCIFailed || tk.CIFailureOptions == nil || len(tk.CIFailureOptions) != 0 {
		t.Fatalf("reason %q, options %#v: want CI failed with an empty list of options", tk.NeedsMeReason, tk.CIFailureOptions)
	}
	httpRejected(t, srv, landAnyway("3"), core.ActionNotAllowed)
}

func TestLandAnywayIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t, failedCI)
	status, b := httpDoAs(t, srv, core.Agent, landAnyway("3"))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) {
		t.Fatalf("status %d, want %d for %s", status, httpapi.Status(core.ActionNotAllowed), e.Code)
	}
	if ticket3(t, srv).LandAnyway {
		t.Fatal("an agent's Land anyway was recorded")
	}
}
