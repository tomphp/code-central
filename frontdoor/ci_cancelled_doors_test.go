package frontdoor_test

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// CI cancelled: an Accepted Ticket whose CI result is cancelled waits in
// Needs Me offering Re-run, Land anyway and Send back, but no New fix
// Ticket, since a cancelled run has nothing to fix.

// cancelledCI is pushedAccepted with a cancelled run of ci.yml, finished
// at failedAt, and a passed run of lint.yml.
func cancelledCI(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	pushedAccepted(t, srv)
	run := ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CICancelled)
	run.Finished = &failedAt
	reportRuns(t, srv, run, ciRun(2, "lint.yml", "abc123", store.CICompleted, store.CISuccess))
}

func TestAnAcceptedTicketWhoseCIWasCancelledIsInNeedsMeAsCICancelled(t *testing.T) {
	srv := newServer(t, cancelledCI)
	row, ok := needsMeRowOf(t, srv, "acme-3")
	if !ok || row.Reason != core.ReasonCICancelled || !row.Since.Equal(failedAt) || row.CIFailure != nil {
		t.Fatalf("row %+v, want CI cancelled since its run finished, with no failure", row)
	}
	tk := ticket3(t, srv)
	if tk.CIResult != core.CICancelled || tk.NeedsMeReason != core.ReasonCICancelled {
		t.Fatalf("ci result %q, reason %q, want cancelled", tk.CIResult, tk.NeedsMeReason)
	}
	if want := []string{"rerun_ci", "land_anyway", "send_back"}; !slices.Equal(tk.CIFailureOptions, want) {
		t.Fatalf("options %v, want %v", tk.CIFailureOptions, want)
	}
	httpRejected(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/new_fix_ticket", `{"note":"x"}`}, core.ActionNotAllowed)
}

func TestAFailedRunAlongsideACancelledOneStaysCIFailed(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CICancelled), ciRun(2, "lint.yml", "abc123", store.CICompleted, store.CIFailure))
	if tk := ticket3(t, srv); tk.NeedsMeReason != core.ReasonCIFailed {
		t.Fatalf("reason %q, want CI failed", tk.NeedsMeReason)
	}
}

func TestCICancelledDropsOutOnceHandled(t *testing.T) {
	for name, call := range map[string]httpCall{
		"re-run":      rerunCI("3"),
		"land anyway": landAnyway("3"),
		"send back":   sendBack("3", "the run was cancelled; push again"),
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t, cancelledCI)
			httpOK(t, srv, call)
			if row, ok := needsMeRowOf(t, srv, "acme-3"); ok {
				t.Fatalf("row %+v, want acme-3 out of Needs Me", row)
			}
		})
	}
}

func TestRerunOfACancelledRunWaitsAndNothingReRunsByItself(t *testing.T) {
	srv := newServer(t, cancelledCI)
	if w := ciWatch(t, srv); len(w.Reruns) != 0 {
		t.Fatalf("reruns %+v, want none from a cancellation", w.Reruns)
	}
	httpOK(t, srv, rerunCI("3"))
	w := ciWatch(t, srv)
	if !slices.Equal(w.Reruns, []core.CIRerun{{Ticket: "acme-3", Commit: "abc123", ID: 1, Attempt: 1, Requested: codecentraltest.Start}}) {
		t.Fatalf("reruns %+v, want the cancelled run 1 requested", w.Reruns)
	}
	if tk := ticket3(t, srv); tk.CIResult != core.CIRunning {
		t.Fatalf("ci result %q, want running", tk.CIResult)
	}
	// Its new attempt passing reads as passed.
	again := ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CISuccess)
	again.Attempt = 2
	reportRuns(t, srv, again)
	if tk := ticket3(t, srv); tk.CIResult != core.CIPassed || tk.NeedsMeReason == core.ReasonCICancelled {
		t.Fatalf("ci result %q, reason %q, want passed", tk.CIResult, tk.NeedsMeReason)
	}
}
