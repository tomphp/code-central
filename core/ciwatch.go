package core

import (
	"context"
	"slices"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The CI Watcher's own operations (Spec code-central-247): get_ci_watch,
// the one read it polls, and report_ci_watcher_status. It writes as an
// ordinary agent client; there is no CI Watcher Actor. Its status is
// kept with every kind of Watcher's, in watcher.go.

// ciReruns is every Re-run request in the Repository name waiting at now,
// by Ticket, then run, once per run: Tickets sharing a pushed commit
// share its request, so the CI Watcher asks for each run once
// (code-central-432). A Cancelled Ticket's are left out.
func ciReruns(ci *ciState, name string) []CIRerun {
	out := []CIRerun{}
	key := ci.p.Key
	for _, t := range ci.r.FindTickets(key, store.TicketFilter{HasPushes: true}) {
		if t.State == domain.Cancelled {
			continue
		}
		rc := ci.ticket(t).in(name)
		if rc == nil {
			continue
		}
		commit := rc.latest.Commit
		for _, a := range rc.reruns {
			if slices.ContainsFunc(out, func(x CIRerun) bool { return x.Commit == commit && x.ID == a.ID && x.Attempt == a.Attempt }) {
				continue
			}
			out = append(out, CIRerun{Ticket: DisplayID(key, t.Number), Commit: commit, ID: a.ID, Attempt: a.Attempt, Requested: ci.response(rc.record, commit).Rerun})
		}
	}
	return out
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
		v := ciView(p, WatcherView{})
		home, _ := ciEntryOf(p, key)
		out = CIWatch{Project: key, Tracking: v.Tracking, NotWatched: home.NotWatched, Provider: providerOrDefault(home.Provider), Repository: home.Repository, ProjectURL: home.ProjectURL, PATEnv: v.PATEnv,
			IgnoredWorkflows: v.IgnoredWorkflows, Rules: []CIWatchRule{}, GHPath: r.Options().GHPath,
			Waiting: []CIWaitingCommit{}, Reruns: []CIRerun{}, Repositories: []CIWatchRepository{}, Pushed: []CITicketCommit{}, Landed: []CITicketCommit{}}
		last := ciRulesLastRuns(r.CIRuns(key))
		for i, rule := range v.Rules {
			w := CIWatchRule{CIRuleView: rule}
			if at, ok := last[p.CI.Rules[i].Spec]; ok {
				w.LastRun = &at
			}
			out.Rules = append(out.Rules, w)
		}
		runs := landingPolicies(p).RunsCIWatcher()
		var ci *ciState
		if runs {
			ci = c.ciStateOf(r, p)
			out.Waiting, out.Reruns = ciWaiting(ci, key), ciReruns(ci, key)
			out.Pushed, out.Landed = ciTicketCommits(r, key)
		}
		for _, t := range ciTargets(p) {
			if t.name == key {
				continue
			}
			w := CIWatchRepository{Name: t.name, Provider: t.provider, Repository: t.repository, ProjectURL: t.projectURL,
				Waiting: []CIWaitingCommit{}, Reruns: []CIRerun{}}
			if runs {
				w.Waiting, w.Reruns = ciWaiting(ci, t.name), ciReruns(ci, t.name)
			}
			out.Repositories = append(out.Repositories, w)
		}
		return nil
	})
	return out, err
}

// ciRulesLastRuns is when the latest finished run tracked against each
// CI Rule finished, by the number of the rule's Spec.
func ciRulesLastRuns(runs []domain.CIRun) map[int]time.Time {
	last := map[int]time.Time{}
	for _, run := range runs {
		if run.Rule != 0 && run.Finished.After(last[run.Rule]) {
			last[run.Rule] = run.Finished
		}
	}
	return last
}

// ciTicketCommits is every commit pushed for a Ticket, and every commit a
// Ticket landed at, each in its Repository, in Ticket order.
func ciTicketCommits(r store.Reader, key string) (pushed, landed []CITicketCommit) {
	pushed, landed = []CITicketCommit{}, []CITicketCommit{}
	for _, tc := range r.TicketCommits(key) {
		row := CITicketCommit{Ticket: DisplayID(key, tc.Ticket), Spec: DisplayID(key, tc.Spec), Repository: tc.Repository, Commit: tc.Commit}
		if tc.Kind == store.CommitLanded {
			landed = append(landed, row)
		} else {
			pushed = append(pushed, row)
		}
	}
	return pushed, landed
}

// ReportCIWatcherStatus is report_ci_watcher_status: the CI Watcher
// reports its status on every poll.
func (c *Core) ReportCIWatcherStatus(_ context.Context, key string, in ReportWatcherStatusInput) (Written[ProjectView], error) {
	return c.reportWatcherStatus(WatcherCI, key, in)
}
