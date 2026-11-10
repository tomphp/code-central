// CI Rules: the CI Watcher lists the completed runs on each rule's
// branch, and for a failure the rule matches opens a fix Ticket on the
// rule's Spec, or comments on the one still open, and reports the run
// tracked against the rule. Observed only through the HTTP API.

package ciwatcher_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/store"
)

// rule adds a CI Rule as the human and returns its Spec's ID.
func (w *watching) rule(workflow, branch string, state store.State) string {
	w.t.Helper()
	out, err := w.srv.Core.AddCIRule(core.WithActor(context.Background(), core.Human), "acme",
		core.CIRuleInput{Workflow: workflow, Branch: branch, State: state, Full: true})
	if err != nil {
		w.t.Fatal(err)
	}
	rules := out.Record.CI.Rules
	return rules[len(rules)-1].Spec
}

// branchRun is one completed run on a branch as GitHub's REST API lists
// it, finished at finished, minutes after the test clock's start.
type branchRun struct {
	id       int64
	workflow string
	branch   string
	commit   string
	conclude string
	finished int
}

// branchAnswer scripts GitHub's answer to listing the completed runs on
// branch.
func (w *watching) branchAnswer(branch string, runs ...branchRun) {
	w.t.Helper()
	var items []string
	for _, r := range runs {
		at := codecentraltest.Start.Add(time.Duration(r.finished) * time.Minute).Format(time.RFC3339)
		items = append(items, fmt.Sprintf(`{"id":%d,"run_attempt":1,"name":"Build %s","path":".github/workflows/%s","head_branch":%q,"head_sha":%q,`+
			`"status":"completed","conclusion":%q,"run_started_at":"2026-01-05T08:00:00Z","updated_at":%q,"html_url":"https://github.com/acme/web/actions/runs/%d"}`,
			r.id, r.workflow, r.workflow, r.branch, r.commit, r.conclude, at, r.id))
	}
	w.h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web/actions/runs?branch=" + url.QueryEscape(branch) + "&status=completed&per_page=100"},
		Stdout: `{"total_count":` + fmt.Sprint(len(runs)) + `,"workflow_runs":[` + strings.Join(items, ",") + `]}`})
}

// rulePoll waits for a poll to list the rules' branches, then for the
// loop to wait again, so the poll has done all it does.
func (w *watching) rulePoll() {
	w.t.Helper()
	n := w.listed("status=completed")
	if n > 0 {
		w.srv.Clock.BlockUntil(1)
		w.srv.Clock.Advance(ciwatcher.Interval)
	}
	w.waitFor("a poll listing the rules' branches", func() bool { return w.listed("status=completed") > n })
	w.srv.Clock.BlockUntil(1)
}

// fixTickets are the Tickets on spec find_tickets finds, which leaves
// out Cancelled ones, oldest first.
func (w *watching) fixTickets(spec string) []core.TicketView {
	w.t.Helper()
	ctx := context.Background()
	rows, err := w.srv.Client.FindTickets(ctx, "acme", core.FindTicketsInput{Spec: core.Ref(spec)})
	if err != nil {
		w.t.Fatal(err)
	}
	var out []core.TicketView
	for _, r := range rows {
		tk, err := w.srv.Client.GetTicket(ctx, "acme", core.Ref(r.ID))
		if err != nil {
			w.t.Fatal(err)
		}
		out = append(out, tk)
	}
	return out
}

// lastRun is when the latest run tracked against the rule on spec
// finished, as get_ci_watch says; nil with none.
func (w *watching) lastRun(spec string) *time.Time {
	w.t.Helper()
	watch, err := w.srv.Client.GetCIWatch(context.Background(), "acme")
	if err != nil {
		w.t.Fatal(err)
	}
	for _, r := range watch.Rules {
		if r.Spec == spec {
			return r.LastRun
		}
	}
	w.t.Fatalf("no CI Rule on %s", spec)
	return nil
}

// at is minutes after the test clock's start.
func at(minutes int) time.Time {
	return codecentraltest.Start.Add(time.Duration(minutes) * time.Minute)
}

func TestAFailureARuleMatchesOpensAFixTicketOnItsSpec(t *testing.T) {
	w := newWatching(t)
	spec := w.rule("ci.yml", "main", store.Ready)
	w.branchAnswer("main", branchRun{id: 21, workflow: "ci.yml", branch: "main", commit: "fff111", conclude: "failure", finished: 5})
	w.jobsAnswer(21, 1, [2]string{"test", "failure"}, [2]string{"lint", "success"}, [2]string{"e2e", "timed_out"})
	w.start(w.h.Bin)
	w.rulePoll()

	tks := w.fixTickets(spec)
	if len(tks) != 1 {
		t.Fatalf("Tickets on %s = %d, want one fix Ticket", spec, len(tks))
	}
	tk := tks[0]
	if tk.State != store.Ready || len(tk.Labels) != 0 {
		t.Fatalf("fix Ticket is %s with labels %v, want Ready, the rule's State, with no Stage Label", tk.State, tk.Labels)
	}
	if !strings.Contains(tk.Title, "ci.yml") || !strings.Contains(tk.Title, "main") {
		t.Fatalf("title %q, want the workflow and branch", tk.Title)
	}
	for _, want := range []string{"https://github.com/acme/web/actions/runs/21", "fff111",
		"[test](https://github.com/acme/web/actions/runs/21/job/1)", "[e2e](https://github.com/acme/web/actions/runs/21/job/3)"} {
		if !strings.Contains(tk.Description, want) {
			t.Fatalf("description lacks %q:\n%s", want, tk.Description)
		}
	}
	if strings.Contains(tk.Description, "lint") {
		t.Fatalf("description names lint, which passed:\n%s", tk.Description)
	}
	if got := w.lastRun(spec); got == nil || !got.Equal(at(5)) {
		t.Fatalf("rule's last run = %v, want run 21 tracked against it", got)
	}

	// The run is fired on once: later polls see it, and leave it be.
	w.rulePoll()
	if tks := w.fixTickets(spec); len(tks) != 1 || len(tks[0].Comments) != 0 {
		t.Fatalf("after another poll, Tickets = %d with %d comments, want the one fix Ticket untouched", len(tks), len(tks[0].Comments))
	}
}

func TestAPassingRunOrAnotherWorkflowOrBranchDoesntFireARule(t *testing.T) {
	w := newWatching(t)
	spec := w.rule("ci.yml", "main", store.Draft)
	w.branchAnswer("main",
		branchRun{id: 21, workflow: "ci.yml", branch: "main", commit: "fff111", conclude: "success", finished: 5},
		branchRun{id: 22, workflow: "lint.yml", branch: "main", commit: "fff111", conclude: "failure", finished: 5},
		branchRun{id: 23, workflow: "ci.yml", branch: "main-2", commit: "fff111", conclude: "failure", finished: 5})
	w.start(w.h.Bin)
	w.rulePoll()
	if tks := w.fixTickets(spec); len(tks) != 0 {
		t.Fatalf("Tickets on %s = %d, want none", spec, len(tks))
	}
	if got := w.lastRun(spec); got != nil {
		t.Fatalf("rule's last run = %v, want none tracked against it", got)
	}
}

func TestAFailingLandCommitLinksItsTicketAndSpec(t *testing.T) {
	w := newWatching(t)
	ctx := context.Background()
	must(t, func() error {
		_, err := w.srv.Core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"})
		return err
	})
	must(t, func() error {
		_, err := w.srv.Core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Login form", State: store.Ready})
		return err
	})
	res, err := w.srv.Core.Claim(ctx, "acme", core.ClaimInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	must(t, func() error {
		_, err := w.srv.Core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: res.ClaimID, Summary: "Done.", Branch: "acme-2", Commit: "aaa000"})
		return err
	})
	must(t, func() error {
		_, err := w.srv.Client.Land(ctx, "acme", core.LandInput{Ticket: "acme-2", Commit: "land99"})
		return err
	})
	spec := w.rule("any", "main", store.Ready)
	w.branchAnswer("main", branchRun{id: 21, workflow: "ci.yml", branch: "main", commit: "land99", conclude: "failure", finished: 5})
	w.jobsAnswer(21, 1, [2]string{"test", "failure"})
	w.start(w.h.Bin)
	w.rulePoll()

	tks := w.fixTickets(spec)
	if len(tks) != 1 {
		t.Fatalf("Tickets on %s = %d, want one fix Ticket", spec, len(tks))
	}
	for _, want := range []string{"acme-2", "acme-1"} {
		if !strings.Contains(tks[0].Description, want) {
			t.Fatalf("description lacks %s, the Ticket that landed the commit and its Spec:\n%s", want, tks[0].Description)
		}
	}
}

func TestAFurtherFailureCommentsOnTheRulesUnfinishedTicket(t *testing.T) {
	w := newWatching(t)
	spec := w.rule("ci.yml", "main", store.Ready)
	first := branchRun{id: 21, workflow: "ci.yml", branch: "main", commit: "fff111", conclude: "failure", finished: 5}
	w.branchAnswer("main", first)
	w.jobsAnswer(21, 1, [2]string{"test", "failure"})
	w.start(w.h.Bin)
	w.rulePoll()

	second := branchRun{id: 22, workflow: "ci.yml", branch: "main", commit: "fff222", conclude: "cancelled", finished: 9}
	w.branchAnswer("main", first, second)
	w.jobsAnswer(22, 1, [2]string{"build", "cancelled"})
	w.rulePoll()
	tks := w.fixTickets(spec)
	if len(tks) != 1 {
		t.Fatalf("Tickets on %s = %d, want still the one fix Ticket", spec, len(tks))
	}
	if len(tks[0].Comments) != 1 {
		t.Fatalf("comments = %+v, want one for the further failure", tks[0].Comments)
	}
	body := tks[0].Comments[0].Body
	for _, want := range []string{"https://github.com/acme/web/actions/runs/22", "[build](https://github.com/acme/web/actions/runs/22/job/1)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("comment lacks %q:\n%s", want, body)
		}
	}
	if got := w.lastRun(spec); got == nil || !got.Equal(at(9)) {
		t.Fatalf("rule's last run = %v, want run 22 tracked against it", got)
	}

	// Once the fix Ticket is finished, the next failure opens another.
	must(t, func() error {
		_, err := w.srv.Core.Cancel(core.WithActor(context.Background(), core.Human), "acme", core.CancelInput{Ticket: core.Ref(tks[0].ID), Reason: "Flaky."})
		return err
	})
	third := branchRun{id: 23, workflow: "ci.yml", branch: "main", commit: "fff333", conclude: "failure", finished: 12}
	w.branchAnswer("main", first, second, third)
	w.jobsAnswer(23, 1, [2]string{"test", "failure"})
	w.rulePoll()
	if again := w.fixTickets(spec); len(again) != 1 || again[0].ID == tks[0].ID || len(again[0].Comments) != 0 {
		t.Fatalf("open Tickets on %s = %+v, want a new fix Ticket once the first is finished", spec, again)
	}
}

func TestARunOnAPushedCommitNeverFiresARule(t *testing.T) {
	w := newWatching(t)
	w.pushed("abc123")
	spec := w.rule("any", "main", store.Ready)
	w.runsAnswer("abc123", listedRun{id: 21, attempt: 1, workflow: "ci.yml", status: "completed", conclude: "failure", updated: "2026-01-05T09:05:00Z"})
	w.branchAnswer("main", branchRun{id: 21, workflow: "ci.yml", branch: "main", commit: "abc123", conclude: "failure", finished: 5})
	w.jobsAnswer(21, 1, [2]string{"test", "failure"})
	w.start(w.h.Bin)
	w.rulePoll()
	w.rulePoll()
	if tks := w.fixTickets(spec); len(tks) != 0 {
		t.Fatalf("Tickets on %s = %d, want none for the pushed commit's run", spec, len(tks))
	}
	if got := w.lastRun(spec); got != nil {
		t.Fatalf("rule's last run = %v, want the run tracked against its Ticket only", got)
	}
}

func TestRunsFinishingBeforeTheRuleExistedDontFireIt(t *testing.T) {
	w := newWatching(t)
	w.srv.Clock.Advance(10 * time.Minute)
	spec := w.rule("ci.yml", "main", store.Ready)
	w.branchAnswer("main", branchRun{id: 21, workflow: "ci.yml", branch: "main", commit: "fff111", conclude: "failure", finished: 9})
	w.jobsAnswer(21, 1, [2]string{"test", "failure"})
	w.start(w.h.Bin)
	w.rulePoll()
	if tks := w.fixTickets(spec); len(tks) != 0 {
		t.Fatalf("Tickets on %s = %d, want none for a run finished before the rule", spec, len(tks))
	}
}

func TestAnIgnoredWorkflowFiresOnlyARuleNamingIt(t *testing.T) {
	w := newWatching(t)
	must(t, func() error {
		_, err := w.srv.Core.SetCITracking(core.WithActor(context.Background(), core.Human), "acme",
			core.SetCITrackingInput{Tracking: true, Repository: "acme/web", IgnoredWorkflows: []string{"nightly.yml"}})
		return err
	})
	anyRule := w.rule("any", "main", store.Ready)
	nightlyRule := w.rule("nightly.yml", "main", store.Draft)
	w.branchAnswer("main", branchRun{id: 21, workflow: "nightly.yml", branch: "main", commit: "fff111", conclude: "failure", finished: 5})
	w.jobsAnswer(21, 1, [2]string{"soak", "failure"})
	w.start(w.h.Bin)
	w.rulePoll()
	if tks := w.fixTickets(anyRule); len(tks) != 0 {
		t.Fatalf("Tickets on %s = %d, want none: any workflow leaves out Ignored workflows", anyRule, len(tks))
	}
	tks := w.fixTickets(nightlyRule)
	if len(tks) != 1 || tks[0].State != store.Draft {
		t.Fatalf("Tickets on %s = %+v, want one Draft fix Ticket", nightlyRule, tks)
	}
}
