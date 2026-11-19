package core

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The CI view (Spec code-central-247, code-central-258): one Project's
// CI Runs, or every active Project's, with each CI Watcher's status. Like
// Usage it is not one of core's operations: the HTTP API serves it to
// the UI alone, and nothing over MCP or the CLI.

// ciViewDays is how far back the CI view lists finished runs.
const ciViewDays = 7

// CIOverview reads the CI view. One Project's read has that Project,
// Archived or with CI Tracking off; All Projects has every active Project
// with CI Tracking on. An unknown Project is not_found. With Running, the
// runs are only the unfinished ones counting towards a Ticket's CI result,
// newest started first.
func (c *Core) CIOverview(_ context.Context, in api.CIOverviewInput) (api.CIOverview, error) {
	out := api.CIOverview{Projects: []api.CIProjectStatus{}, Runs: []api.CIRunRow{}}
	now := c.now()
	type listed struct {
		project string
		run     domain.CIRun
	}
	var rows []listed
	err := c.store.View(func(r store.Reader) error {
		var projects []domain.Project
		if in.Project != "" {
			p, ok := r.Project(in.Project)
			if !ok {
				return &api.Error{Code: api.NotFound, Message: fmt.Sprintf("no Project %q", in.Project)}
			}
			projects = []domain.Project{p}
		} else {
			for _, p := range r.Projects() {
				if landingPolicies(p).RunsCIWatcher() && p.Archived.IsZero() {
					projects = append(projects, p)
				}
			}
		}
		for _, p := range projects {
			v := ciView(p, c.watchers.view(watcherKey{api.WatcherCI, p.Key}, now))
			out.Projects = append(out.Projects, api.CIProjectStatus{Project: p.Key, Tracking: v.Tracking, Provider: v.Provider, Repository: v.Repository, ProjectURL: v.ProjectURL, Watcher: v.Watcher})
			ci := c.ciStateOf(r, p)
			for _, run := range latestAttempts(keptCIRuns(r.CIRuns(p.Key), now)) {
				if in.Running {
					if runningForTicket(ci, run) {
						rows = append(rows, listed{p.Key, run})
					}
					continue
				}
				if ended := ciRunEnded(run); run.Status != domain.CICompleted || (!ended.IsZero() && now.Sub(ended) <= ciViewDays*24*time.Hour) {
					rows = append(rows, listed{p.Key, run})
				}
			}
		}
		return nil
	})
	if err != nil {
		return api.CIOverview{}, err
	}
	slices.SortStableFunc(rows, func(a, b listed) int {
		aEnded, bEnded := ciRunEnded(a.run), ciRunEnded(b.run)
		switch {
		case aEnded.IsZero() != bEnded.IsZero():
			if aEnded.IsZero() {
				return -1
			}
			return 1
		case aEnded.IsZero():
			return b.run.Started.Compare(a.run.Started)
		}
		return bEnded.Compare(aEnded)
	})
	for _, row := range rows {
		out.Runs = append(out.Runs, api.CIRunRow{Project: row.project, CIRunView: ciRunView(row.project, row.run)})
	}
	return out, nil
}

// runningForTicket reports whether run, its run ID's latest attempt among
// the kept runs, counts as running towards a Ticket's CI result: it is
// unfinished, tracked against a Ticket rather than a CI Rule, and on a
// commit some Ticket's CI result reads.
func runningForTicket(ci *ciState, run domain.CIRun) bool {
	if run.Status != domain.CIQueued && run.Status != domain.CIInProgress || run.Ticket == 0 {
		return false
	}
	return ci.reads(run.Repository, run.Commit) && !run.Ignored
}

// latestAttempts is runs with each run's latest attempt only, a run being
// its Repository's run ID.
func latestAttempts(runs []domain.CIRun) []domain.CIRun {
	type runID struct {
		repository string
		id         int64
	}
	latest := map[runID]int{}
	for _, run := range runs {
		k := runID{run.Repository, run.ID}
		latest[k] = max(latest[k], run.Attempt)
	}
	return slices.DeleteFunc(runs, func(run domain.CIRun) bool { return run.Attempt < latest[runID{run.Repository, run.ID}] })
}
