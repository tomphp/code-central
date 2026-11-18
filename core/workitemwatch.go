package core

import (
	"context"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Work Item Watcher's own operations (Spec code-central-894):
// get_work_item_watch, the one read it polls, and
// report_work_item_watcher_status. Like the Issue Watcher, it writes as
// the integration Actor, through the linked-item operations and
// report_issue_written, and its status report is accepted from that
// Actor only.

// WorkItemWatch is get_work_item_watch: everything the Work Item Watcher
// needs on each poll, so it keeps no state of its own.
type WorkItemWatch struct {
	Project string `json:"project"`
	On      bool   `json:"on"`
	// Mode is how it picks its work items: saved_query or wiql.
	Mode domain.AzureBoardsMode `json:"mode"`
	// QueryURL is the Saved query's URL.
	QueryURL string `json:"query_url"`
	// ProjectURL is the Azure DevOps project a WIQL filter runs in, and
	// Where its WHERE clause.
	ProjectURL string `json:"project_url"`
	Where      string `json:"where"`
	// PATEnv is the name of the environment variable holding the Azure
	// DevOps PAT.
	PATEnv string `json:"pat_env"`
	// Spec is the Azure Boards Spec's ID, left out until Azure Boards is
	// first turned on.
	Spec string `json:"spec,omitempty"`
	// Issues are the Incoming Issues on the Azure Boards Spec, in number
	// order, so the Watcher sees which work items it has and which have
	// changed.
	Issues []IncomingIssue `json:"issues"`
	// Linked is every azure-boards Link ID in the Project, detached and
	// outbound ones too, once each, so the Watcher tells a new work item
	// from one already taken off the inbox.
	Linked []string `json:"linked"`
	// Writes are the items, Incoming Issues and outbound Specs, whose work
	// items have writes due, in number order (writeback.go).
	Writes []WorkItemWrites `json:"writes"`
}

// WorkItemWrites is an item whose work item has writes due, as
// get_work_item_watch lists it: what is due on a GitHub issue, with the
// status label as the work item's tag of the same name, and never a
// close, as a work item's State is the human's to move.
type WorkItemWrites struct {
	// Target is the item, such as acme-5.
	Target string   `json:"target"`
	Link   LinkView `json:"link"`
	// Comments are the comments due, in the order to post them.
	Comments []IssueComment `json:"comments"`
	// Tag is the status tag the work item should carry, in place of any
	// other code-central: tag; empty asks for none to be changed.
	Tag string `json:"tag,omitempty"`
}

// GetWorkItemWatch is the Work Item Watcher's get_work_item_watch.
func (c *Core) GetWorkItemWatch(_ context.Context, key string) (WorkItemWatch, error) {
	var out WorkItemWatch
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		a := azureBoardsView(p, WatcherView{})
		out = WorkItemWatch{Project: key, On: a.On, Mode: a.Mode, QueryURL: a.QueryURL, ProjectURL: a.ProjectURL, Where: a.Where, PATEnv: a.PATEnv,
			Spec: a.Spec, Issues: incomingIssues(r, key, IntegrationAzureBoards), Linked: linkIDs(r, key, IntegrationAzureBoards), Writes: []WorkItemWrites{}}
		for _, w := range c.integrationWrites(r, key, IntegrationAzureBoards) {
			if w.Label != "" || len(w.Comments) > 0 {
				out.Writes = append(out.Writes, WorkItemWrites{Target: w.Target, Link: w.Link, Comments: w.Comments, Tag: w.Label})
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
	for _, s := range r.Specs(key) {
		if s.Link.Integration == integration {
			byNumber[s.Number] = s.Link.ID
		}
	}
	for _, t := range r.Tickets(key) {
		if t.Link.Integration == integration {
			byNumber[t.Number] = t.Link.ID
		}
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
func (c *Core) ReportWorkItemWatcherStatus(ctx context.Context, key string, in ReportWatcherStatusInput) (Written[ProjectView], error) {
	if err := requireActor(ctx, "report_work_item_watcher_status", Integration); err != nil {
		return Written[ProjectView]{}, err
	}
	return c.reportWatcherStatus(WatcherWorkItems, key, in)
}
