package core

import (
	"context"
	"slices"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// New Ticket (Spec code-central-1067): the human opens one implementation
// Ticket on a Spec from free text, as a Small Change does on the Small
// Changes Spec.

// NewTicketInput is new_ticket's input: the Spec and the free text, whose
// first line becomes the Ticket's title.
type NewTicketInput struct {
	Spec Ref    `json:"spec" desc:"The Spec to open the Ticket on." required:"true"`
	Text string `json:"text" desc:"The work to do, in Markdown; its first line becomes the Ticket's title." required:"true"`
	// Draft saves the Ticket as Draft, not Ready, so nothing is claimed
	// until the human moves it to Ready.
	Draft bool `json:"draft,omitempty" desc:"Save the Ticket as Draft instead of Ready, so nothing is claimed until the human moves it to Ready."`
	// Full asks for the Ticket in place of its Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// NewTicket is the human's new_ticket(spec, text, draft?). It writes a
// Ready (or with draft, Draft) Ticket titled with the text's first line,
// with the text as its description, on the Spec, with no Labels, Tags,
// Blockers or Stages. It is action_not_allowed where whyNotNewTicket
// says why.
func (c *Core) NewTicket(ctx context.Context, key string, in NewTicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "new_ticket"); err != nil {
		return Written[TicketView]{}, err
	}
	title := titleLine(in.Text)
	if title == "" {
		return Written[TicketView]{}, invalid("text", "a New Ticket needs text; its first line becomes the Ticket's title")
	}
	var v Written[TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		s, err := resolveSpec(w, key, "new_ticket", "spec", in.Spec)
		if err != nil {
			return err
		}
		if why := whyNotNewTicket(p, s, specTickets(w, key, s.Number)); why != "" {
			return notAllowed("%s", why)
		}
		now := c.now()
		t := domain.Ticket{
			Number: w.NextNumber(key), Title: title, State: submitState(in.Draft), Spec: s.Number,
			Created: now, Updated: now, Description: in.Text,
			Moves: submitMoves(in.Draft, now, "new_ticket"),
		}
		if err := w.PutTicket(key, t); err != nil {
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
	if StageOf(t.Labels) != StageTickets {
		return false
	}
	if t.State == domain.Done {
		return true
	}
	return t.State == domain.Cancelled && len(t.Moves) > 0 && t.Moves[len(t.Moves)-1].Via == implementVia
}
