package core

import (
	"context"
	"slices"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// New Ticket (Spec code-central-1067): the human opens one implementation
// Ticket on a Spec from free text, as a Small Change does on the Small
// Changes Spec.

// NewTicket is the human's new_ticket(spec, text, draft?). It writes a
// Ready (or with draft, Draft) Ticket titled with the text's first line,
// with the text as its description, on the Spec, with no Labels, Tags,
// Blockers or Stages. It is action_not_allowed where whyNotNewTicket
// says why.
func (c *Core) NewTicket(ctx context.Context, key string, in api.NewTicketInput) (api.Written[api.TicketView], error) {
	if err := humanOnly(ctx, "new_ticket"); err != nil {
		return api.Written[api.TicketView]{}, err
	}
	title := titleLine(in.Text)
	if title == "" {
		return api.Written[api.TicketView]{}, invalid("text", "a New Ticket needs text; its first line becomes the Ticket's title")
	}
	var v api.Written[api.TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		s, err := resolveSpec(w, key, "new_ticket", "spec", in.Spec)
		if err != nil {
			return err
		}
		if why := whyNotNewTicket(p, s, specTickets(w, key, s.Number, store.Moves)); why != "" {
			return notAllowed("%s", why)
		}
		now := c.now()
		t := domain.Ticket{
			Number: w.NextNumber(key), Title: title, State: submitState(in.Draft), Spec: s.Number,
			Created: now, Updated: now, Description: in.Text,
			Moves: submitMoves(in.Draft, now, "new_ticket"),
		}
		if err := w.PutTicket(key, t, whole...); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, domain.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}

// whyNotNewTicket is why the human may not open a New Ticket on Spec s,
// with these Tickets, in Project p, or "" when they may: not on a Map or a
// Long-lived Spec, on a Requirement only once its Stages are finished
// (its tickets Stage Done, or Cancelled by Implement), and never while
// p is Archived. A Finished Spec takes one. new_ticket refuses with it,
// and SpecDetail's ticketable is whether it is "".
func whyNotNewTicket(p domain.Project, s domain.Spec, tickets []domain.Ticket) string {
	id := DisplayID(p.Key, s.Number)
	switch {
	case !p.Archived.IsZero():
		return archivedError(p.Key).Message
	case isMap(s.Labels):
		return id + " is a Map, which charts work rather than holding it, so it takes no New Ticket"
	case slices.Contains(s.Labels, LongLivedLabel):
		return id + " is a Long-lived Spec, so it takes no New Ticket; its Tickets come from its own action"
	}
	if !slices.ContainsFunc(tickets, func(t domain.Ticket) bool { return StageOf(t.Labels) != "" }) {
		return ""
	}
	if !slices.ContainsFunc(tickets, finishesStages) {
		return id + " is a Requirement whose Stages aren't finished, so it takes no New Ticket until its tickets Stage is Done or Implement replaces it"
	}
	return ""
}

// finishesStages reports whether t is a Requirement's tickets Stage that
// finishes its Stages: Done, or Cancelled by Implement.
func finishesStages(t domain.Ticket) bool {
	if StageOf(t.Labels) != domain.StageTickets {
		return false
	}
	if t.State == domain.Done {
		return true
	}
	return t.State == domain.Cancelled && len(t.Moves) > 0 && t.Moves[len(t.Moves)-1].Via == implementVia
}
