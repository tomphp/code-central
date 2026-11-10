package core

import (
	"context"

	"github.com/tomoram/code-central/store"
)

// The Issue Watcher's own operations (Spec code-central-279):
// get_issue_watch, the one read it polls, and
// report_issue_watcher_status. Unlike the CI Watcher, it writes as the
// integration Actor, and its status report is accepted from that Actor
// only. Its status is kept with every kind of Watcher's, in watcher.go.

// IssueWatch is get_issue_watch: everything the Issue Watcher needs on
// each poll, so it keeps no state of its own. The gh path Option is read
// on every poll, so changing it needs no restart.
type IssueWatch struct {
	Project    string `json:"project"`
	On         bool   `json:"on"`
	Repository string `json:"repository"`
	// Label is the filter label; empty lets every issue in.
	Label  string `json:"label"`
	GHPath string `json:"gh_path"`
	// Spec is the Issues Spec's ID, left out until GitHub Issues is first
	// turned on.
	Spec string `json:"spec,omitempty"`
	// Issues are the Incoming Issues, in number order, so the Issue
	// Watcher sees which issues it has and which have changed.
	Issues []IncomingIssue `json:"issues"`
}

// IncomingIssue is a Ticket on the Issues Spec that isn't finished and
// whose Link isn't detached, as get_issue_watch lists it.
type IncomingIssue struct {
	Ticket      string   `json:"ticket"`
	Link        LinkView `json:"link"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
}

// GetIssueWatch is the Issue Watcher's get_issue_watch.
func (c *Core) GetIssueWatch(_ context.Context, key string) (IssueWatch, error) {
	var out IssueWatch
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		g := githubIssuesView(key, p.GitHubIssues, WatcherView{})
		out = IssueWatch{Project: key, On: g.On, Repository: g.Repository, Label: g.Label, GHPath: r.Options().GHPath, Spec: g.Spec, Issues: []IncomingIssue{}}
		if p.GitHubIssues.Spec == 0 {
			return nil
		}
		for _, t := range r.Tickets(key) {
			if t.Spec == p.GitHubIssues.Spec && t.Link.Owned() && !finished(t.State) {
				out.Issues = append(out.Issues, IncomingIssue{Ticket: DisplayID(key, t.Number), Link: *linkView(t.Link), Title: t.Title, Description: t.Description})
			}
		}
		return nil
	})
	return out, err
}

// ReportIssueWatcherStatus is report_issue_watcher_status: the Issue
// Watcher reports its status on every poll, as the integration Actor.
func (c *Core) ReportIssueWatcherStatus(ctx context.Context, key string, in ReportWatcherStatusInput) (Written[ProjectView], error) {
	if err := requireActor(ctx, "report_issue_watcher_status", Integration); err != nil {
		return Written[ProjectView]{}, err
	}
	return c.reportWatcherStatus(WatcherIssues, key, in)
}
