// Guided progression (Spec code-central-213; ADR 0016, Wayfinder Labels):
// on each poll, the Factory starts the next decision of every guided Map
// that is idle. It lives here, in the poll, because nothing in Code
// Central schedules (ADR 0005).

package factory

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// startGuidedMaps approves the first frontier Ticket, in created order,
// of each guided Map that is idle: at most one Ticket per Map per poll.
func (f *factory) startGuidedMaps(ctx context.Context) {
	maps, err := f.client.FindSpecs(ctx, f.project, core.FindSpecsInput{
		Labels: []string{core.MapLabel}, Tags: map[string]string{core.GuidedTag: core.GuidedOn}})
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
		rows, err := f.client.FindTickets(ctx, f.project, core.FindTicketsInput{Spec: core.Ref(m.ID)})
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
		if _, err := f.client.Approve(ctx, f.project, core.TicketInput{Ticket: core.Ref(next.ID)}); err != nil {
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
func nextDecision(rows []core.TicketRow) (next core.TicketRow, ok bool) {
	for _, r := range rows {
		if r.State != store.Draft && r.NodeType != "research" {
			return core.TicketRow{}, false
		}
	}
	for _, r := range rows {
		if r.State == store.Draft && !r.Blocked {
			return r, true
		}
	}
	return core.TicketRow{}, false
}
