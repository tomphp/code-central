package core

import (
	"context"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Small Changes (Spec code-central-475). A Small Change is one Ready
// implementation Ticket from the human's text, on the Project's Small
// Changes Spec, a Long-lived Spec: it has no Stages and lands on its own.

// SubmitSmallChange is submit_small_change(text, draft?). It writes, as human
// whoever calls, a Ready (or with draft, Draft) Ticket titled with the text's first line, with the
// text as its description, on the Small Changes Spec.
func (c *Core) SubmitSmallChange(_ context.Context, key string, in api.SubmitSmallChangeInput) (api.Written[api.TicketView], error) {
	title := titleLine(in.Text)
	if title == "" {
		return api.Written[api.TicketView]{}, api.InvalidField("text", "a Small Change needs text; its first line becomes the Ticket's title")
	}
	spec, err := c.projectSpecNumber(key, smallChangesSpec)
	if err != nil {
		return api.Written[api.TicketView]{}, err
	}
	var v api.Written[api.TicketView]
	err = c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		now := c.now()
		t := domain.Ticket{
			Number: w.NextNumber(key), Title: title, State: submitState(in.Draft), Spec: spec,
			Created: now, Updated: now, Description: in.Text,
			Moves: submitMoves(in.Draft, now, "submit_small_change"),
		}
		if err := whole.put(w, key, &t); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, domain.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}

// submitState is the State a submit writes its Tickets in.
func submitState(draft bool) domain.State {
	if draft {
		return domain.Draft
	}
	return domain.Ready
}

// submitMoves is the Moves of a Ticket a submit writes: its move to
// Ready, or none when it is a Draft.
func submitMoves(draft bool, now time.Time, via string) []domain.Move {
	if draft {
		return nil
	}
	return []domain.Move{{To: domain.Ready, Actor: string(api.Human), Time: now, Via: via}}
}

// smallChangesSpecBody is the Small Changes Spec's body.
const smallChangesSpecBody = "This Spec holds the Tickets the human opens with Small Change in + New. " +
	"Each is built from the human's text as it is, with no Stages, and lands on its own as soon as it is accepted.\n"
