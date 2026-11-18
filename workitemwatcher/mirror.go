// Mirroring the query: each poll, every work item it matches that has no
// Link in the Project arrives as an Incoming Issue on the Azure Boards
// Spec; while one waits for triage, Azure DevOps owns its text, so it
// follows its work item's edits, and whether the work item still matches.

package workitemwatcher

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/ratelimit"
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
// they differ, and is open. Then each Incoming Issue no longer matched
// leaves (leave). It returns Azure DevOps' rate limit, if it refused a
// call for it; anything else that goes wrong is logged and tried again on
// the next poll.
func (cfg Config) mirror(ctx context.Context, watch core.WorkItemWatch, q queryRun) *ratelimit.Error {
	linked := map[string]bool{}
	for _, id := range watch.Linked {
		linked[id] = true
	}
	incoming := map[string]core.IncomingIssue{}
	for _, i := range watch.Issues {
		incoming[i.Link.ID] = i
	}
	var read []int
	for _, id := range q.ids {
		if _, ok := incoming[LinkID(q.org, id)]; ok || !linked[LinkID(q.org, id)] {
			read = append(read, id)
		}
	}
	items, err := q.c.WorkItems(ctx, read)
	if err != nil {
		return cfg.limitOrLog(ctx, "reading the matching work items", err)
	}
	for _, w := range items {
		if i, ok := incoming[LinkID(q.org, w.ID)]; ok {
			cfg.follow(ctx, q, i, w)
		} else {
			cfg.arrive(ctx, q, watch.Spec, w)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
	return cfg.leave(ctx, q, watch.Issues)
}

// leave sets each of issues that q didn't match out of scope, or deleted
// when its work item, read alone, can't be: Azure DevOps answers one that
// was deleted, or that the PAT may not read, as not found, or refuses it.
// One of another organization than q's is out of scope without reading.
func (cfg Config) leave(ctx context.Context, q queryRun, issues []core.IncomingIssue) *ratelimit.Error {
	for _, i := range issues {
		id, ours := workItemID(q.org, i.Link.ID)
		if ours && slices.Contains(q.ids, id) {
			continue
		}
		state := domain.ExternalOutOfScope
		if ours {
			_, err := q.c.WorkItem(ctx, id)
			switch {
			case errors.Is(err, azdo.ErrNotFound), errors.Is(err, azdo.ErrMissingScope):
				state = domain.ExternalDeleted
			case err != nil:
				if limit := cfg.limitOrLog(ctx, "reading work item "+i.Link.ID, err); limit != nil {
					return limit
				}
				continue
			}
		}
		cfg.setExternalState(ctx, i, state)
	}
	return nil
}

// arrive brings in work item w, one of q's matches, as a Draft Ticket on
// spec, the Azure Boards Spec.
func (cfg Config) arrive(ctx context.Context, q queryRun, spec string, w azdo.WorkItem) {
	link := q.c.WorkItemURL(w.Fields.TeamProject, w.ID)
	_, err := cfg.Client.CreateLinkedTicket(ctx, cfg.Project, core.CreateLinkedTicketInput{
		Spec: core.Ref(spec), Title: w.Fields.Title, Description: body(w, link), State: domain.Draft,
		Link: core.LinkInput{Integration: Integration, ID: LinkID(q.org, w.ID), URL: link},
	})
	if err != nil && ctx.Err() == nil {
		cfg.logf("create_linked_ticket for %s: %v", LinkID(q.org, w.ID), err)
	}
}

// follow updates i to its work item w, one of q's matches: its title,
// body and web link where they differ, and opens it if it was out of
// scope or deleted.
func (cfg Config) follow(ctx context.Context, q queryRun, i core.IncomingIssue, w azdo.WorkItem) {
	link := q.c.WorkItemURL(w.Fields.TeamProject, w.ID)
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
	cfg.setExternalState(ctx, i, domain.ExternalOpen)
}

// setExternalState sets i's external state to state, if it isn't already.
func (cfg Config) setExternalState(ctx context.Context, i core.IncomingIssue, state domain.ExternalState) {
	if state == i.Link.ExternalState {
		return
	}
	_, err := cfg.Client.SetExternalState(ctx, cfg.Project, core.SetExternalStateInput{Target: core.Ref(i.Ticket), ExternalState: state})
	if err != nil && ctx.Err() == nil {
		cfg.logf("set_external_state for %s: %v", i.Link.ID, err)
	}
}

// limitOrLog is err, from doing what, as Azure DevOps' rate limit, when
// it is one; any other error is logged, unless the poll is stopping, and
// is nil.
func (cfg Config) limitOrLog(ctx context.Context, what string, err error) *ratelimit.Error {
	var limit *ratelimit.Error
	if errors.As(err, &limit) {
		return limit
	}
	if ctx.Err() == nil {
		cfg.logf("%s: %v", what, err)
	}
	return nil
}
