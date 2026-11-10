// The CI Watcher carries out Re-run requests: it re-runs a failed run's
// failed jobs through the fake gh and reports the run's new attempt,
// which clears the request.

package ciwatcher_test

import (
	"context"
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/store"
)

// rerunCalls are the fake gh's calls to re-run run 11's failed jobs.
func (w *watching) rerunCalls() []fakegh.Call {
	var out []fakegh.Call
	for _, c := range w.h.Calls() {
		if len(c.Args) >= 2 && c.Args[0] == "api" && c.Args[1] == "repos/acme/web/actions/runs/11/rerun-failed-jobs" {
			out = append(out, c)
		}
	}
	return out
}

// attempt waits for ticket's latest pushed commit's run 11 to be reported
// at attempt with status, and returns the Ticket.
func (w *watching) attempt(ticket string, attempt int, status store.CIRunStatus) core.TicketView {
	w.t.Helper()
	var tk core.TicketView
	w.waitFor("run 11's attempt reported", func() bool {
		var err error
		if tk, err = w.srv.Client.GetTicket(context.Background(), "acme", core.Ref(ticket)); err != nil {
			w.t.Fatal(err)
		}
		return slices.ContainsFunc(tk.CIRuns, func(r core.CIRunView) bool { return r.ID == 11 && r.Attempt == attempt && r.Status == status })
	})
	return tk
}

func TestTheCIWatcherRerunsTheFailedJobsAndReportsTheNewAttempt(t *testing.T) {
	w := newWatching(t)
	id := w.pushed("abc123")
	w.runsAnswer("abc123", listedRun{id: 11, attempt: 1, workflow: "ci.yml", status: "completed", conclude: "failure", updated: "2026-01-05T09:05:00Z"})
	w.jobsAnswer(11, 1, [2]string{"test", "failure"})
	w.start(w.h.Bin)
	w.ciResult(id, core.CIFailed)

	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := w.srv.Core.RerunCI(human, "acme", core.TicketInput{Ticket: core.Ref(id)}); return err })
	w.runsAnswer("abc123", listedRun{id: 11, attempt: 2, workflow: "ci.yml", status: "in_progress", updated: "2026-01-05T09:07:00Z"})
	w.jobsAnswer(11, 2, [2]string{"test", ""})
	w.waitingPoll()
	tk := w.attempt(id, 2, store.CIInProgress)
	if tk.CIResult != core.CIRunning {
		t.Fatalf("ci result %q, want running while attempt 2 runs", tk.CIResult)
	}
	calls := w.rerunCalls()
	if len(calls) != 1 || !slices.Equal(calls[0].Args[2:4], []string{"--method", "POST"}) {
		t.Fatalf("re-run calls = %+v, want one POST", calls)
	}
	watch, err := w.srv.Client.GetCIWatch(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(watch.Reruns) != 0 {
		t.Fatalf("reruns = %+v, want the request cleared by attempt 2", watch.Reruns)
	}

	w.runsAnswer("abc123", listedRun{id: 11, attempt: 2, workflow: "ci.yml", status: "completed", conclude: "success", updated: "2026-01-05T09:09:00Z"})
	w.jobsAnswer(11, 2, [2]string{"test", "success"})
	w.waitingPoll()
	w.attempt(id, 2, store.CICompleted)
	if tk := w.ciResult(id, core.CIPassed); tk.NeedsMeReason == core.ReasonCIFailed {
		t.Fatalf("reason %q, want acme-2 out of CI failed", tk.NeedsMeReason)
	}
	if n := len(w.rerunCalls()); n != 1 {
		t.Fatalf("%d re-run calls, want the request carried out once", n)
	}
}

// A request GitHub hasn't answered with a new attempt yet isn't carried
// out again on the next poll.
func TestTheCIWatcherAsksForAReRunOnce(t *testing.T) {
	w := newWatching(t)
	id := w.pushed("abc123")
	w.runsAnswer("abc123", listedRun{id: 11, attempt: 1, workflow: "ci.yml", status: "completed", conclude: "failure", updated: "2026-01-05T09:05:00Z"})
	w.jobsAnswer(11, 1, [2]string{"test", "failure"})
	w.start(w.h.Bin)
	w.ciResult(id, core.CIFailed)

	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := w.srv.Core.RerunCI(human, "acme", core.TicketInput{Ticket: core.Ref(id)}); return err })
	w.waitingPoll()
	w.waitingPoll()
	if n := len(w.rerunCalls()); n != 1 {
		t.Fatalf("%d re-run calls, want one while GitHub starts the new attempt", n)
	}
}
