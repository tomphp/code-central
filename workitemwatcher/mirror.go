// Mirroring the query: each poll, every work item it matches that has no
// Link in the Project arrives as an Incoming Issue on the Azure Boards
// Spec; while one waits for triage, Azure DevOps owns its text, so it
// follows its work item's edits, and whether the work item still matches.

package workitemwatcher

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/watchers"
)

// Integration is the name every Link the Work Item Watcher makes carries.
const Integration = core.IntegrationAzureBoards

// LinkID is the Link ID of org's work item id: org#id, org lowercased.
// Work item IDs are unique in an organization, so it stays the same when
// the work item moves to another of its projects.
func LinkID(org string, id int) string { return fmt.Sprintf("%s#%d", strings.ToLower(org), id) }

// workItemID is the ID of the work item Link ID linkID names, if it is
// one of org's.
func workItemID(org, linkID string) (int, bool) {
	o, num, ok := strings.Cut(linkID, "#")
	if !ok || o != org {
		return 0, false
	}
	id, err := strconv.Atoi(num)
	return id, err == nil
}

// mirror mirrors q's matches as watch's Incoming Issues. It reads the
// matches that are new, with no Link in the Project, detached and
// triaged ones included, so a work item taken off the inbox never comes
// in again, or are Incoming Issues. A new one arrives as a Draft Ticket;
// an Incoming Issue takes its work item's title, body and web link when
// they differ, and is open. An Incoming Issue no longer matched is read
// alone: deleted if Azure DevOps can't find it, out of scope otherwise,
// and out of scope without reading if it is another organization's. It
// returns Azure DevOps' rate limit, if it refused a call for it; anything
// else that goes wrong is logged and tried again on the next poll.
func (cfg Config) mirror(ctx context.Context, watch core.WorkItemWatch, q queried) *watchers.RateLimitError {
	linked := map[string]bool{}
	for _, id := range watch.Linked {
		linked[id] = true
	}
	incoming := map[string]core.IncomingIssue{}
	for _, i := range watch.Issues {
		incoming[i.Link.ID] = i
	}
	matched := map[int]bool{}
	var read []int
	for _, id := range q.ids {
		matched[id] = true
		if _, ok := incoming[LinkID(q.org, id)]; ok || !linked[LinkID(q.org, id)] {
			read = append(read, id)
		}
	}
	items, err := q.c.WorkItems(ctx, read)
	if err != nil {
		return cfg.failed(ctx, "reading the matching work items", err)
	}
	for _, w := range items {
		if i, ok := incoming[LinkID(q.org, w.ID)]; ok {
			cfg.follow(ctx, i, q.c, w)
		} else {
			cfg.arrive(ctx, watch.Spec, q, w)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
	for _, i := range watch.Issues {
		id, ours := workItemID(q.org, i.Link.ID)
		if ours && matched[id] {
			continue
		}
		state := store.ExternalOutOfScope
		if ours {
			_, err := q.c.WorkItem(ctx, id)
			switch {
			case errors.Is(err, azdo.ErrNotFound):
				state = store.ExternalDeleted
			case err != nil:
				if limit := cfg.failed(ctx, "reading work item "+i.Link.ID, err); limit != nil {
					return limit
				}
				continue
			}
		}
		cfg.setExternalState(ctx, i, state)
	}
	return nil
}

// arrive brings in work item w as a Draft Ticket on spec, the Azure
// Boards Spec.
func (cfg Config) arrive(ctx context.Context, spec string, q queried, w azdo.WorkItem) {
	link := q.c.WorkItemURL(w.Fields.TeamProject, w.ID)
	_, err := cfg.Client.CreateLinkedTicket(ctx, cfg.Project, core.CreateLinkedTicketInput{
		Spec: core.Ref(spec), Title: w.Fields.Title, Description: body(w, link), State: store.Draft,
		Link: core.LinkInput{Integration: Integration, ID: LinkID(q.org, w.ID), URL: link},
	})
	if err != nil && ctx.Err() == nil {
		cfg.logf("create_linked_ticket for %s: %v", LinkID(q.org, w.ID), err)
	}
}

// follow updates i to its work item w's title, body and web link where
// they differ, and opens it if it was out of scope or deleted.
func (cfg Config) follow(ctx context.Context, i core.IncomingIssue, c *azdo.Client, w azdo.WorkItem) {
	link := c.WorkItemURL(w.Fields.TeamProject, w.ID)
	up := core.UpdateLinkedTicketInput{Ticket: core.Ref(i.Ticket)}
	if w.Fields.Title != i.Title {
		up.Title = &w.Fields.Title
	}
	if b := body(w, link); b != i.Description {
		up.Description = &b
	}
	if link != i.Link.URL {
		up.URL = &link
	}
	if up.Title != nil || up.Description != nil || up.URL != nil {
		if _, err := cfg.Client.UpdateLinkedTicket(ctx, cfg.Project, up); err != nil && ctx.Err() == nil {
			cfg.logf("update_linked_ticket for %s: %v", i.Link.ID, err)
		}
	}
	cfg.setExternalState(ctx, i, store.ExternalOpen)
}

// setExternalState sets i's external state to state, if it isn't already.
func (cfg Config) setExternalState(ctx context.Context, i core.IncomingIssue, state store.ExternalState) {
	if state == i.Link.ExternalState {
		return
	}
	_, err := cfg.Client.SetExternalState(ctx, cfg.Project, core.SetExternalStateInput{Target: core.Ref(i.Ticket), ExternalState: state})
	if err != nil && ctx.Err() == nil {
		cfg.logf("set_external_state for %s: %v", i.Link.ID, err)
	}
}

// failed logs err from doing what, unless it is Azure DevOps'
// rate limit, which it returns, or the poll stopping.
func (cfg Config) failed(ctx context.Context, what string, err error) *watchers.RateLimitError {
	var limit *watchers.RateLimitError
	if errors.As(err, &limit) {
		return limit
	}
	if ctx.Err() == nil {
		cfg.logf("%s: %v", what, err)
	}
	return nil
}
