package core

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// The CI view (Spec code-central-247, code-central-258): one Project's
// CI Runs, or every active Project's, with each CI Watcher's status. Like
// Usage it is not one of core's operations: the HTTP API serves it to
// the UI alone, and nothing over MCP or the CLI.

// ciViewDays is how far back the CI view lists finished runs.
const ciViewDays = 7

// CIOverviewInput is the CI view's read: one Project, or every active
// Project when Project is "". Running keeps only the runs that count as
// running towards a Ticket's CI result, as the Activity Dashboard reads
// them (code-central-276).
type CIOverviewInput struct {
	Project string
	Running bool
}

// CIOverview is the CI view: the CI Watchers' statuses, then the runs,
// in-progress ones first, newest started first, then those finished in
// the last ciViewDays days, newest finished first. A run re-run lists its
// latest attempt only.
type CIOverview struct {
	Projects []CIProjectStatus `json:"projects"`
	Runs     []CIRunRow        `json:"runs"`
}

// CIProjectStatus is one Project's CI Tracking and its CI Watcher's
// status, as the CI view heads its runs with them.
type CIProjectStatus struct {
	Project    string      `json:"project"`
	Tracking   bool        `json:"tracking"`
	Provider   string      `json:"provider"`
	Repository string      `json:"repository"`
	ProjectURL string      `json:"project_url"`
	Watcher    WatcherView `json:"watcher"`
}

// CIRunRow is one CI Run in the CI view, with its Project.
type CIRunRow struct {
	Project string `json:"project"`
	CIRunView
}

// CIOverview reads the CI view. One Project's read has that Project,
// Archived or with CI Tracking off; All Projects has every active Project
// with CI Tracking on. An unknown Project is not_found. With Running, the
// runs are only the unfinished ones counting towards a Ticket's CI result,
// newest started first.
func (c *Core) CIOverview(_ context.Context, in CIOverviewInput) (CIOverview, error) {
	out := CIOverview{Projects: []CIProjectStatus{}, Runs: []CIRunRow{}}
	now := c.now()
	type listed struct {
		project string
		run     store.CIRun
	}
	var rows []listed
	err := c.store.View(func(r store.Reader) error {
		var projects []store.Project
		if in.Project != "" {
			p, ok := r.Project(in.Project)
			if !ok {
				return &Error{Code: NotFound, Message: fmt.Sprintf("no Project %q", in.Project)}
			}
			projects = []store.Project{p}
		} else {
			for _, p := range r.Projects() {
				if p.CI.Tracking && p.Archived.IsZero() {
					projects = append(projects, p)
				}
			}
		}
		for _, p := range projects {
			out.Projects = append(out.Projects, CIProjectStatus{Project: p.Key, Tracking: p.CI.Tracking, Provider: string(p.CI.Provider), Repository: p.CI.Repository, ProjectURL: p.CI.ProjectURL, Watcher: c.watchers.view(watcherKey{WatcherCI, p.Key})})
			for _, run := range latestAttempts(keptCIRuns(r.CIRuns(p.Key), now)) {
				if in.Running {
					if runningForTicket(r, p.Key, run) {
						rows = append(rows, listed{p.Key, run})
					}
					continue
				}
				if ended := ciRunEnded(run); run.Status != store.CICompleted || (!ended.IsZero() && now.Sub(ended) <= ciViewDays*24*time.Hour) {
					rows = append(rows, listed{p.Key, run})
				}
			}
		}
		return nil
	})
	if err != nil {
		return CIOverview{}, err
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
		out.Runs = append(out.Runs, CIRunRow{Project: row.project, CIRunView: ciRunView(row.project, row.run)})
	}
	return out, nil
}

// runningForTicket reports whether run, its run ID's latest attempt among
// the kept runs, counts as running towards a Ticket's CI result: it is
// unfinished, tracked against a Ticket rather than a CI Rule, and on a
// commit some Ticket's CI result reads.
func runningForTicket(r store.Reader, key string, run store.CIRun) bool {
	if run.Status != store.CIQueued && run.Status != store.CIInProgress || run.Ticket == 0 {
		return false
	}
	return len(ticketsReading(r, key, run.Commit)) > 0 && !run.Ignored
}

// latestAttempts is runs with each run's latest attempt only.
func latestAttempts(runs []store.CIRun) []store.CIRun {
	latest := map[int64]int{}
	for _, run := range runs {
		latest[run.ID] = max(latest[run.ID], run.Attempt)
	}
	return slices.DeleteFunc(runs, func(run store.CIRun) bool { return run.Attempt < latest[run.ID] })
}
