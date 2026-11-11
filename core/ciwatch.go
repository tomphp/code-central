package core

import (
	"context"
	"time"

	"github.com/tomoram/code-central/store"
)

// The CI Watcher's own operations (Spec code-central-247): get_ci_watch,
// the one read it polls, and report_ci_watcher_status. It writes as an
// ordinary agent client; there is no CI Watcher Actor. Its status is
// kept with every kind of Watcher's, in watcher.go.

// CIWatch is get_ci_watch: everything the CI Watcher needs on each poll.
// The gh path Option is read on every poll, so changing it needs no
// restart.
type CIWatch struct {
	Project    string           `json:"project"`
	Tracking   bool             `json:"tracking"`
	Provider   store.CIProvider `json:"provider"`
	Repository string           `json:"repository"`
	ProjectURL string           `json:"project_url"`
	// PATEnv is the name of the environment variable holding the Azure
	// DevOps PAT, which the CI Watcher reads from its own environment.
	PATEnv           string        `json:"pat_env"`
	IgnoredWorkflows []string      `json:"ignored_workflows"`
	Rules            []CIWatchRule `json:"rules"`
	GHPath           string        `json:"gh_path"`
	// Waiting are the pushed commits still waiting on a final CI result,
	// or newly pushed, oldest push first: the CI Watcher lists their runs.
	Waiting []CIWaitingCommit `json:"waiting"`
	// Reruns are the Re-run requests waiting on the CI Watcher: it
	// re-runs each run's failed jobs, and a request clears once it
	// reports the run's new attempt.
	Reruns []CIRerun `json:"reruns"`
	// Pushed is every commit pushed for a Ticket, whose runs never fire a
	// CI Rule, and Landed every commit a Ticket landed at, which a fix
	// Ticket links; both by Ticket.
	Pushed []CITicketCommit `json:"pushed"`
	Landed []CITicketCommit `json:"landed"`
}

// CIRerun is one Re-run request: a run that failed on a Ticket's pushed
// commit, at the attempt that failed, and when the human asked.
type CIRerun struct {
	Ticket    string    `json:"ticket"`
	Commit    string    `json:"commit"`
	ID        int64     `json:"id"`
	Attempt   int       `json:"attempt"`
	Requested time.Time `json:"requested"`
}

// ciReruns is every Re-run request waiting at now, by Ticket, then run. A
// Cancelled Ticket's are left out.
func ciReruns(r store.Reader, key string, now time.Time) []CIRerun {
	out := []CIRerun{}
	for _, t := range r.Tickets(key) {
		if len(t.PushedCommits) == 0 || t.State == store.Cancelled {
			continue
		}
		latest := latestPushed(t)
		mine, _ := ticketRuns(r, key, t, now)
		for _, a := range pendingReruns(latest, mine, now) {
			out = append(out, CIRerun{Ticket: DisplayID(key, t.Number), Commit: latest.Commit, ID: a.ID, Attempt: a.Attempt, Requested: latest.Rerun})
		}
	}
	return out
}

// CIWatchRule is a CI Rule as the CI Watcher reads it: with when the
// latest run tracked against it finished, so it fires the rule only for
// runs finishing after that, and after the rule was created.
type CIWatchRule struct {
	CIRuleView
	// LastRun is nil until a run is tracked against the rule.
	LastRun *time.Time `json:"last_run,omitempty"`
}

// CITicketCommit is a commit pushed for, or landed by, a Ticket on Spec.
type CITicketCommit struct {
	Ticket string `json:"ticket"`
	Spec   string `json:"spec"`
	Commit string `json:"commit"`
}

// GetCIWatch is the CI Watcher's get_ci_watch. With CI Tracking off,
// nothing waits and nothing is re-run.
func (c *Core) GetCIWatch(_ context.Context, key string) (CIWatch, error) {
	var out CIWatch
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		v := ciView(key, p.CI, WatcherView{})
		out = CIWatch{Project: key, Tracking: v.Tracking, Provider: v.Provider, Repository: v.Repository, ProjectURL: v.ProjectURL, PATEnv: v.PATEnv,
			IgnoredWorkflows: v.IgnoredWorkflows, Rules: []CIWatchRule{}, GHPath: r.Options().GHPath,
			Waiting: []CIWaitingCommit{}, Reruns: []CIRerun{}, Pushed: []CITicketCommit{}, Landed: []CITicketCommit{}}
		last := ciRulesLastRuns(r.CIRuns(key))
		for i, rule := range v.Rules {
			w := CIWatchRule{CIRuleView: rule}
			if at, ok := last[p.CI.Rules[i].Spec]; ok {
				w.LastRun = &at
			}
			out.Rules = append(out.Rules, w)
		}
		if p.CI.Tracking {
			out.Waiting, out.Reruns = ciWaiting(r, key, c.now()), ciReruns(r, key, c.now())
			out.Pushed, out.Landed = ciTicketCommits(r, key)
		}
		return nil
	})
	return out, err
}

// ciRulesLastRuns is when the latest finished run tracked against each
// CI Rule finished, by the number of the rule's Spec.
func ciRulesLastRuns(runs []store.CIRun) map[int]time.Time {
	last := map[int]time.Time{}
	for _, run := range runs {
		if run.Rule != 0 && run.Finished.After(last[run.Rule]) {
			last[run.Rule] = run.Finished
		}
	}
	return last
}

// ciTicketCommits is every commit pushed for a Ticket, and every commit a
// Ticket landed at, in Ticket order.
func ciTicketCommits(r store.Reader, key string) (pushed, landed []CITicketCommit) {
	pushed, landed = []CITicketCommit{}, []CITicketCommit{}
	for _, t := range r.Tickets(key) {
		id, spec := DisplayID(key, t.Number), DisplayID(key, t.Spec)
		for _, p := range t.PushedCommits {
			pushed = append(pushed, CITicketCommit{Ticket: id, Spec: spec, Commit: p.Commit})
		}
		for _, m := range t.Moves {
			if m.Commit != "" {
				landed = append(landed, CITicketCommit{Ticket: id, Spec: spec, Commit: m.Commit})
			}
		}
	}
	return pushed, landed
}

// ReportCIWatcherStatus is report_ci_watcher_status: the CI Watcher
// reports its status on every poll.
func (c *Core) ReportCIWatcherStatus(_ context.Context, key string, in ReportWatcherStatusInput) (Written[ProjectView], error) {
	return c.reportWatcherStatus(WatcherCI, key, in)
}
