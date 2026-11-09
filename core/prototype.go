package core

import (
	"context"

	"github.com/tomoram/code-central/store"
)

// Prototypes (Spec code-central-213). A Prototype is a Ticket labelled
// prototype, on any Spec: the human asks for one with its text, and the
// Factory builds it on a local branch that never lands.

// SubmitPrototypeInput is submit_prototype's input: the Spec it is on and
// the Prototype's free text, whose first line becomes its title.
type SubmitPrototypeInput struct {
	Spec Ref    `json:"spec"`
	Text string `json:"text"`
	// Full asks for the Ticket in place of its Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SubmitPrototype is submit_prototype(spec, text). It writes, as human
// whoever calls, a Ready Prototype titled "Prototype: <first line>" with
// the text as its description.
func (c *Core) SubmitPrototype(_ context.Context, key string, in SubmitPrototypeInput) (Written[TicketView], error) {
	title := requirementTitle(in.Text)
	if title == "" {
		return Written[TicketView]{}, invalid("text", "a Prototype needs text; its first line becomes the Ticket's title")
	}
	if in.Spec == "" {
		return Written[TicketView]{}, invalid("spec", "a Prototype is on a Spec")
	}
	var v Written[TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		sp, err := resolveSpec(w, key, "submit_prototype", "spec", in.Spec)
		if err != nil {
			return err
		}
		now := c.now()
		t := store.Ticket{
			Number: w.NextNumber(key), Title: "Prototype: " + title, State: store.Ready, Spec: sp.Number,
			Labels: []string{PrototypeLabel}, Created: now, Updated: now, Description: in.Text,
			Moves: []store.Move{{To: store.Ready, Actor: string(Human), Time: now, Via: "submit_prototype"}},
		}
		if err := w.PutTicket(key, t); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, store.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}
