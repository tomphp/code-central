package core

import (
	"context"
	"time"

	"github.com/tomoram/code-central/store"
)

// Small Changes (Spec code-central-475). A Small Change is one Ready
// implementation Ticket from the human's text, on the Project's Small
// Changes Spec, a Long-lived Spec: it has no Stages and lands on its own.

// SubmitSmallChangeInput is submit_small_change's input: the free text,
// whose first line becomes the Ticket's title.
type SubmitSmallChangeInput struct {
	Text string `json:"text"`
	// Draft saves every Ticket the submit writes as Draft, not Ready, so
	// nothing is claimed until the human moves them to Ready.
	Draft bool `json:"draft,omitempty"`
	// Full asks for the Ticket in place of its Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SubmitSmallChange is submit_small_change(text, draft?). It writes, as human
// whoever calls, a Ready (or with draft, Draft) Ticket titled with the text's first line, with the
// text as its description, on the Small Changes Spec.
func (c *Core) SubmitSmallChange(_ context.Context, key string, in SubmitSmallChangeInput) (Written[TicketView], error) {
	title := titleLine(in.Text)
	if title == "" {
		return Written[TicketView]{}, invalid("text", "a Small Change needs text; its first line becomes the Ticket's title")
	}
	spec, err := c.smallChangesSpec(key)
	if err != nil {
		return Written[TicketView]{}, err
	}
	var v Written[TicketView]
	err = c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		now := c.now()
		t := store.Ticket{
			Number: w.NextNumber(key), Title: title, State: submitState(in.Draft), Spec: spec,
			Created: now, Updated: now, Description: in.Text,
			Moves: submitMoves(in.Draft, now, "submit_small_change"),
		}
		if err := w.PutTicket(key, t); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, store.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}

// submitState is the State a submit writes its Tickets in.
func submitState(draft bool) store.State {
	if draft {
		return store.Draft
	}
	return store.Ready
}

// submitMoves is the Moves of a Ticket a submit writes: its move to
// Ready, or none when it is a Draft.
func submitMoves(draft bool, now time.Time, via string) []store.Move {
	if draft {
		return nil
	}
	return []store.Move{{To: store.Ready, Actor: string(Human), Time: now, Via: via}}
}

// smallChangesSpecBody is the Small Changes Spec's body.
const smallChangesSpecBody = "This Spec holds the Tickets the human opens with Small Change in + New. " +
	"Each is built from the human's text as it is, with no Stages, and lands on its own as soon as it is accepted.\n"

// smallChangesSpec is key's Small Changes Spec, a Long-lived Spec created
// on first use and found only by its stored number; the Spec is written
// before the Project names it.
func (c *Core) smallChangesSpec(key string) (int, error) {
	n := 0
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		if p, _ := w.Project(key); p.SmallChangesSpec != 0 {
			n = p.SmallChangesSpec
			return nil
		}
		now := c.now()
		n = w.NextNumber(key)
		return w.PutSpec(key, store.Spec{Number: n, Title: "Small Changes", Body: smallChangesSpecBody, Labels: []string{LongLivedLabel}, Created: now, Updated: now})
	})
	if err != nil {
		return 0, err
	}
	err = c.store.Update(func(w store.Writer) error {
		p, _ := w.Project(key)
		if p.SmallChangesSpec != 0 {
			n = p.SmallChangesSpec
			return nil
		}
		p.SmallChangesSpec = n
		return w.PutProject(p)
	})
	return n, err
}
