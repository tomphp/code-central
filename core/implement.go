package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// implementVia is the Via of every Move implement makes.
const implementVia = "implement"

// implementDescription is the implementation Ticket's description, with
// the Spec's ID as its one argument.
const implementDescription = "Implement the whole Spec [%[1]s](%[1]s). Its User Stories, Implementation Decisions and Testing Decisions are the acceptance criteria, including writing the glossary terms and ADRs it records. Read them with `get_spec` and `sections`.\n"

// implementReason is the comment the tickets Stage's Cancel carries, with
// the new Ticket's ID as its one argument.
const implementReason = "The human chose Implement: %s builds the Spec without the tickets Stage."

// Implement is the human's implement(ticket): the other way to accept a
// Requirement's spec Stage, besides accept (Split into Tickets). In one
// write the spec Stage is accepted, one Ready implementation Ticket is
// created on the Spec, and the tickets Stage is Cancelled with that
// Ticket as its Replacement. The result is finish's: the spec Stage's
// Acknowledgement, or with full the Ticket, and a next step naming the
// new Ticket.
func (c *Core) Implement(ctx context.Context, key string, in TicketInput) (FinishResult, error) {
	var res FinishResult
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		t, err := resolveTicket(w, key, "implement", "ticket", in.Ticket)
		if err != nil {
			return err
		}
		var before TicketView
		if !in.Full {
			before = c.ticketView(w, key, t)
		}
		done, err := c.implementStep(ctx, w, key, &t, "", c.now())
		if err != nil {
			return err
		}
		if err := w.PutImport(key, done.records(t)); err != nil {
			return err
		}
		ack, err := acknowledge(before, c.ticketView(w, key, t), in.Full)
		if err != nil {
			return err
		}
		res = FinishResult{Acknowledgement: &ack.Acknowledgement, Ticket: ack.Record,
			NextStep: strings.ReplaceAll(nextStep("implement"), "<id>", done.newID)}
		if ack.Record != nil {
			res.Acknowledgement = nil
		}
		return nil
	})
	return res, err
}

// implemented is what an implement step made besides the spec Stage: the
// new implementation Ticket and the Cancelled tickets Stage.
type implemented struct {
	build, tickets store.Ticket
	newID          string
}

// records is the one write an implement makes, with the spec Stage t.
func (d implemented) records(t store.Ticket) store.Import {
	return store.Import{Tickets: []store.Ticket{d.build}, Rewritten: []store.Ticket{t, d.tickets}}
}

// implementStep is the implement step on the spec Stage t, the step the
// human's implement and an auto-accept share: t is accepted (Done, Via
// implement, with cause), one implementation Ticket is built on the Spec
// and the tickets Stage is Cancelled with that Ticket as its
// Replacement. The human's own implement (an empty cause) builds the
// Ticket Ready; an auto-accept builds it in the tickets Stage's State,
// so a Draft Requirement stays unbuilt. It is action_not_allowed unless
// t is a spec Stage In Review whose tickets Stage can still run.
func (c *Core) implementStep(ctx context.Context, r store.Reader, key string, t *store.Ticket, cause store.AutoAcceptCause, now time.Time) (implemented, error) {
	id := DisplayID(key, t.Number)
	if StageOf(t.Labels) != "spec" {
		return implemented{}, notAllowed("%s is not a spec Stage, so implement is not allowed: only a Requirement's spec Stage is implemented", id)
	}
	if t.State != store.InReview {
		return implemented{}, notAllowed("%s is %s, so implement is not allowed: the spec Stage must be In Review", id, t.State)
	}
	var tickets store.Ticket
	found := false
	for _, o := range r.Tickets(key) {
		if o.Spec == t.Spec && StageOf(o.Labels) == "tickets" {
			tickets, found = o, true
			break
		}
	}
	if !found {
		return implemented{}, notAllowed("%s has no tickets Stage, so implement is not allowed", DisplayID(key, t.Spec))
	}
	if tickets.State != store.Draft && tickets.State != store.Ready {
		return implemented{}, notAllowed("the tickets Stage %s is %s, so implement is not allowed: it must still be able to run (Draft or Ready, not claimed)",
			DisplayID(key, tickets.Number), tickets.State)
	}
	spec, _ := r.Spec(key, t.Spec)
	specID := DisplayID(key, t.Spec)

	if err := transitionTo(ctx, t, key, "implement", "", store.Done, now); err != nil {
		return implemented{}, err
	}
	t.Moves[len(t.Moves)-1].Via = implementVia
	t.Moves[len(t.Moves)-1].Cause = cause
	t.Updated = now

	state := store.Ready
	if cause != "" {
		state = tickets.State
	}
	n := r.NextNumber(key)
	newID := DisplayID(key, n)
	build := store.Ticket{
		Number: n, Title: "Implement: " + spec.Title, State: state, Spec: t.Spec,
		Created: now, Updated: now, Description: fmt.Sprintf(implementDescription, specID),
		Moves: []store.Move{{To: state, Actor: string(Human), Time: now, Via: implementVia}},
	}

	if err := transition(ctx, &tickets, key, "cancel", "", now); err != nil {
		return implemented{}, err
	}
	tickets.Moves[len(tickets.Moves)-1].Via = implementVia
	tickets.Replacements = []int{n}
	addComment(&tickets, Human, now, 0, fmt.Sprintf(implementReason, newID))
	tickets.Updated = now
	return implemented{build: build, tickets: tickets, newID: newID}, nil
}

func notAllowed(format string, args ...any) error {
	return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf(format, args...)}
}
