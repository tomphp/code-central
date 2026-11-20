package core

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Issue Watcher's own operations (Spec code-central-279):
// get_issue_watch, the one read it polls, and
// report_issue_watcher_status; report_issue_written is in writeback.go.
// Unlike the CI Watcher, it writes as the integration Actor, and its
// status report is accepted from that Actor only. Its status is kept with every kind of Watcher's, in watcher.go.

// GetIssueWatch is the Issue Watcher's get_issue_watch.
func (c *Core) GetIssueWatch(_ context.Context, key string) (api.IssueWatch, error) {
	var out api.IssueWatch
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		g := githubIssuesView(key, p.GitHubIssues, api.WatcherView{})
		out = api.IssueWatch{Project: key, On: g.On, Repositories: g.Repositories, Label: g.Label, GHPath: r.Options().GHPath, Spec: g.Spec,
			Issues: incomingIssues(r, key, domain.IntegrationGitHub), Writes: c.integrationWrites(r, key, domain.IntegrationGitHub, false)}
		return nil
	})
	return out, err
}

// incomingIssues are the Tickets on integration's inbox Spec, linked
// through it, that aren't finished and whose Link isn't detached, in
// number order, as its Watcher's watch lists them.
func incomingIssues(r store.Reader, key, integration string) []api.IncomingIssue {
	out := []api.IncomingIssue{}
	in := inboxOf(r, key, integration)
	if in.spec == 0 {
		return out
	}
	for _, t := range r.FindTickets(key, store.TicketFilter{Spec: []int{in.spec}, States: unfinished, Link: &store.LinkFilter{Integration: integration}}) {
		if t.Link.Owned() {
			out = append(out, api.IncomingIssue{Ticket: DisplayID(key, t.Number), Link: *linkView(t.Link), Title: t.Title, Description: t.Description})
		}
	}
	return out
}

// ReportIssueWatcherStatus is report_issue_watcher_status: the Issue
// Watcher reports its status on every poll, as the integration Actor.
func (c *Core) ReportIssueWatcherStatus(ctx context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error) {
	if err := requireActor(ctx, "report_issue_watcher_status", api.Integration); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	return c.reportWatcherStatus(api.WatcherIssues, key, in)
}
