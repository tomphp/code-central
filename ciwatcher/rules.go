// CI Rules: on each poll the CI Watcher lists the completed runs on each
// rule's branch, and for a failure a rule matches opens a fix Ticket on
// the rule's Spec, or comments on the one still open, through the
// ordinary operations, then reports the run tracked against the rule.

package ciwatcher

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// fireRules fires watch's CI Rules for the failed runs on their branches.
// A run fires one rule: one naming its workflow before an any workflow
// one. A rule fires only for runs finishing after it was created and
// after the latest run tracked against it, so a run is fired on once
// even across restarts; a run that fired but whose report failed is
// remembered, and only reported again. A run finishing in the same
// second as the latest one tracked against its rule, but on a later
// poll, is missed.
func (w *watcher) fireRules(ctx context.Context, p Provider, watch core.CIWatch) {
	pushed := map[string]bool{}
	for _, c := range watch.Pushed {
		pushed[c.Commit] = true
	}
	var branches []string
	for _, r := range watch.Rules {
		if !slices.Contains(branches, r.Branch) {
			branches = append(branches, r.Branch)
		}
	}
	for _, branch := range branches {
		var rules []core.CIWatchRule
		var since time.Time
		for _, r := range watch.Rules {
			if r.Branch == branch {
				if s := ruleSince(r); len(rules) == 0 || s.Before(since) {
					since = s
				}
				rules = append(rules, r)
			}
		}
		runs, err := p.CompletedRunsOnBranch(ctx, branch, since)
		if err != nil {
			if ctx.Err() == nil {
				w.logf("listing the runs on %s: %v", branch, err)
			}
			continue
		}
		slices.SortFunc(runs, func(a, b Run) int { return cmp.Or(a.Finished.Compare(*b.Finished), cmp.Compare(a.ID, b.ID)) })
		for _, run := range runs {
			if !run.Conclusion.Failed() || pushed[run.Commit] {
				continue
			}
			rule, ok := matchRule(rules, run, watch.IgnoredWorkflows)
			if !ok || !run.Finished.After(ruleSince(rule)) {
				continue
			}
			w.fire(ctx, p, watch, rule, run)
		}
	}
}

// ruleSince is when r starts firing: after it was created, and after the
// latest run tracked against it.
func ruleSince(r core.CIWatchRule) time.Time {
	if r.LastRun != nil && r.LastRun.After(r.Created) {
		return *r.LastRun
	}
	return r.Created
}

// matchRule is the rule of rules, all on run's branch, that run fires: one
// naming its workflow, else one for any workflow unless it is ignored.
func matchRule(rules []core.CIWatchRule, run Run, ignored []string) (core.CIWatchRule, bool) {
	if i := slices.IndexFunc(rules, func(r core.CIWatchRule) bool { return r.Workflow == run.Workflow }); i >= 0 {
		return rules[i], true
	}
	if slices.Contains(ignored, run.Workflow) {
		return core.CIWatchRule{}, false
	}
	i := slices.IndexFunc(rules, func(r core.CIWatchRule) bool { return r.Workflow == store.AnyWorkflow })
	if i < 0 {
		return core.CIWatchRule{}, false
	}
	return rules[i], true
}

// fire opens a fix Ticket for run on rule's Spec, or comments on the
// rule's unfinished one, then reports run tracked against rule.
func (w *watcher) fire(ctx context.Context, p Provider, watch core.CIWatch, rule core.CIWatchRule, run Run) {
	key := runKey{run.ID, run.Attempt}
	jobs, err := p.Jobs(ctx, run.ID, run.Attempt)
	if err != nil {
		if ctx.Err() == nil {
			w.logf("%s: listing the jobs of run %d: %v", rule.Spec, run.ID, err)
		}
		return
	}
	run.Jobs, run.Rule = jobs, rule.Spec
	if !w.fired[key] {
		if err := w.openOrComment(ctx, watch, rule, run); err != nil {
			if ctx.Err() == nil {
				w.logf("%s: firing on run %d: %v", rule.Spec, run.ID, err)
			}
			return
		}
		w.fired[key] = true
	}
	if _, err := w.Client.ReportCIRuns(ctx, w.Project, core.ReportCIRunsInput{Runs: []core.CIRunView{run.CIRunView}}); err != nil && ctx.Err() == nil {
		w.logf("report_ci_runs: run %d: %v", run.ID, err)
	}
}

// openOrComment comments on the oldest unfinished Ticket on rule's Spec
// with run, so failures don't pile up, or opens a fix Ticket for it.
func (w *watcher) openOrComment(ctx context.Context, watch core.CIWatch, rule core.CIWatchRule, run Run) error {
	rows, err := w.Client.FindTickets(ctx, w.Project, core.FindTicketsInput{Spec: core.Ref(rule.Spec)})
	if err != nil {
		return err
	}
	for _, t := range rows {
		if t.State != store.Done && t.State != store.Cancelled {
			_, err := w.Client.Comment(ctx, w.Project, core.CommentInput{Ticket: core.Ref(t.ID), Body: fixComment(watch, run)})
			return err
		}
	}
	_, err = w.Client.CreateTicket(ctx, w.Project, core.CreateTicketInput{Spec: core.Ref(rule.Spec), State: rule.State,
		Title: fmt.Sprintf("Fix %s failing on %s", run.Workflow, run.Branch), Description: fixDescription(watch, rule, run)})
	return err
}

// fixDescription is a fix Ticket's description: the failed run, its
// failed jobs, the Ticket its commit landed, and what fixing it means.
func fixDescription(watch core.CIWatch, rule core.CIWatchRule, run Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** (`%s`) failed on `%s` at commit `%s`, so the CI Rule on Spec %s opened this Ticket to fix it.\n\n",
		run.Name, run.Workflow, run.Branch, run.Commit, rule.Spec)
	b.WriteString(runDetails(watch, run))
	fmt.Fprintf(&b, "\n## Acceptance criteria\n\n- [ ] `%s` passes on `%s` again.\n", run.Workflow, run.Branch)
	return b.String()
}

// fixComment is the comment on an unfinished fix Ticket for a further
// failure.
func fixComment(watch core.CIWatch, run Run) string {
	return fmt.Sprintf("**%s** (`%s`) failed on `%s` again, at commit `%s`.\n\n%s", run.Name, run.Workflow, run.Branch, run.Commit, runDetails(watch, run))
}

// runDetails links run and names its failed jobs, and the Tickets its
// commit landed with their Specs.
func runDetails(watch core.CIWatch, run Run) string {
	var failed []string
	for _, j := range run.Jobs {
		if j.Conclusion.Failed() {
			failed = append(failed, fmt.Sprintf("[%s](%s)", j.Name, j.URL))
		}
	}
	jobs := strings.Join(failed, ", ")
	if jobs == "" {
		jobs = "none; the run itself failed"
	}
	s := fmt.Sprintf("- Run: %s\n- Failed jobs: %s\n", run.URL, jobs)
	for _, c := range watch.Landed {
		if c.Commit == run.Commit {
			s += fmt.Sprintf("- The commit landed %s, on Spec %s.\n", c.Ticket, c.Spec)
		}
	}
	return s
}
