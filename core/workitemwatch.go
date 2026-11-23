package core

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Work Item Watcher's own operations (Spec code-central-894):
// get_work_item_watch, the one read it polls, and
// report_work_item_watcher_status. Like the Issue Watcher, it writes as
// the integration Actor, through the linked-item operations and
// report_issue_written, and its status report is accepted from that
// Actor only.

// GetWorkItemWatch is the Work Item Watcher's get_work_item_watch.
func (c *Core) GetWorkItemWatch(_ context.Context, key string) (api.WorkItemWatch, error) {
	var out api.WorkItemWatch
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return api.UnknownProjectError(key)
		}
		a := azureBoardsView(p, api.WatcherView{})
		out = api.WorkItemWatch{Project: key, On: a.On, Mode: a.Mode, QueryURL: a.QueryURL, ProjectURL: a.ProjectURL, Where: a.Where, PATEnv: a.PATEnv,
			Spec: a.Spec, Issues: incomingIssues(r, key, domain.IntegrationAzureBoards), Linked: linkIDs(r, key, domain.IntegrationAzureBoards), Writes: []api.WorkItemWrites{}}
		issues := c.integrationWritesByNumber(r, key, domain.IntegrationAzureBoards, p.AzureBoards.HideCodeCentral)
		states := stateWrites(r, key, p.AzureBoards.WorkItemStates)
		for n := 1; n < r.NextNumber(key); n++ {
			w, ok := states[n]
			if i, due := issues[n]; due && (i.Label != "" || len(i.Comments) > 0) {
				w, ok = api.WorkItemWrites{Target: i.Target, Link: i.Link, Comments: i.Comments, Tag: i.Label, State: w.State}, true
			}
			if ok {
				out.Writes = append(out.Writes, w)
			}
		}
		return nil
	})
	return out, err
}

// linkIDs is the ID of every integration Link in key, on a Spec or a
// Ticket, detached and outbound ones too, once each, in the order of the
// first item to carry it.
func linkIDs(r store.Reader, key, integration string) []string {
	byNumber := map[int]string{}
	link := &store.LinkFilter{Integration: integration}
	for _, s := range r.FindSpecs(key, store.SpecFilter{Link: link}) {
		byNumber[s.Number] = s.Link.ID
	}
	for _, t := range r.FindTickets(key, store.TicketFilter{Link: link}) {
		byNumber[t.Number] = t.Link.ID
	}
	out, seen := []string{}, map[string]bool{}
	for n := 1; n < r.NextNumber(key); n++ {
		if id, ok := byNumber[n]; ok && !seen[id] {
			out, seen[id] = append(out, id), true
		}
	}
	return out
}

// ReportWorkItemWatcherStatus is report_work_item_watcher_status: the
// Work Item Watcher reports its status on every poll, as the integration
// Actor.
func (c *Core) ReportWorkItemWatcherStatus(ctx context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error) {
	if err := requireActor(ctx, "report_work_item_watcher_status", api.Integration); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	return c.reportWatcherStatus(api.WatcherWorkItems, key, in)
}
