// Guided progression (Spec code-central-213; ADR 0057, Wayfinder Labels):
// on each poll, the Factory starts the next decision of every guided Map
// that is idle. It lives here, in the poll, because nothing in Code
// Central schedules (ADR 0005).

package factory

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// startGuidedMaps approves the first frontier Ticket, in created order,
// of each guided Map that is idle: at most one Ticket per Map per poll.
func (f *factory) startGuidedMaps(ctx context.Context) {
	maps, err := f.client.FindSpecs(ctx, f.project, api.FindSpecsInput{
		Labels: []string{domain.MapLabel}, Tags: map[string]string{domain.GuidedTag: domain.GuidedOn}})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't find the guided Maps: %v", err)
		}
		return
	}
	for _, m := range maps {
		if f.halted(ctx) || f.paused() {
			return
		}
		// Rows leave Done and Cancelled out and are in created order.
		rows, err := f.client.FindTickets(ctx, f.project, api.FindTicketsInput{Spec: api.Ref(m.ID)})
		if err != nil {
			if ctx.Err() == nil {
				f.printf("warning: can't read the guided Map %s: %v", m.ID, err)
			}
			continue
		}
		next, ok := nextDecision(rows)
		if !ok {
			continue
		}
		if _, err := f.client.Approve(ctx, f.project, api.TicketInput{Ticket: api.Ref(next.ID)}); err != nil {
			if ctx.Err() == nil {
				f.printf("%s: approve: %v", next.ID, err)
			}
			continue
		}
		f.printf("%s approved (guided Map %s): %q", next.ID, m.ID, next.Title)
	}
}

// nextDecision is the Map's first frontier Ticket (Draft and not
// blocked) among its unfinished Tickets, in created order, when the Map
// is idle: every one of them but research is Draft. Any other State
// waits on the human or an agent, so it holds the Map.
func nextDecision(rows []api.TicketRow) (next api.TicketRow, ok bool) {
	for _, r := range rows {
		if r.State != domain.Draft && r.NodeType != "research" {
			return api.TicketRow{}, false
		}
	}
	for _, r := range rows {
		if r.State == domain.Draft && !r.Blocked {
			return r, true
		}
	}
	return api.TicketRow{}, false
}
