package core

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Prototypes (Spec code-central-213). A Prototype is a Ticket labelled
// prototype, on any Spec: the human asks for one with its text, and the
// Factory builds it on a local branch that never lands.

// SubmitPrototype is submit_prototype(spec, text). It writes, as human
// whoever calls, a Ready Prototype titled "Prototype: <first line>" with
// the text as its description.
func (c *Core) SubmitPrototype(_ context.Context, key string, in api.SubmitPrototypeInput) (api.Written[api.TicketView], error) {
	title := titleLine(in.Text)
	if title == "" {
		return api.Written[api.TicketView]{}, invalid("text", "a Prototype needs text; its first line becomes the Ticket's title")
	}
	if in.Spec == "" {
		return api.Written[api.TicketView]{}, invalid("spec", "a Prototype is on a Spec")
	}
	var v api.Written[api.TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		sp, err := resolveSpec(w, key, "submit_prototype", "spec", in.Spec)
		if err != nil {
			return err
		}
		now := c.now()
		t := domain.Ticket{
			Number: w.NextNumber(key), Title: "Prototype: " + title, State: domain.Ready, Spec: sp.Number,
			Labels: []string{PrototypeLabel}, Created: now, Updated: now, Description: in.Text,
			Moves: []domain.Move{{To: domain.Ready, Actor: string(api.Human), Time: now, Via: "submit_prototype"}},
		}
		if err := whole.put(w, key, &t); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, domain.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}
