package core

import "github.com/tomoram/code-central/store"

// Specs and Tickets share one number counter per Project, so a number
// that isn't the kind asked for may be the other kind. specNotTicket and
// ticketNotSpec, which resolveTicket and resolveSpec call, are the one
// place that says so, naming what to do instead.
// op is the snake_case operation name and field the input it came in: the
// operation's main target fails not_found, a nested field invalid with
// the field.

// resolveTicket reads the Ticket ref names as op's field.
func resolveTicket(r store.Reader, key, op, field string, ref Ref) (store.Ticket, error) {
	n, err := ref.number(key, field)
	if err != nil {
		return store.Ticket{}, err
	}
	return ticketNumbered(r, key, op, field, n)
}

// ticketNumbered is resolveTicket for a number already parsed.
func ticketNumbered(r store.Reader, key, op, field string, n int) (store.Ticket, error) {
	if t, ok := r.Ticket(key, n); ok {
		return t, nil
	}
	if err := specNotTicket(r, key, op, field, n); err != nil {
		return store.Ticket{}, err
	}
	return store.Ticket{}, missing(op, field, "there is no Ticket %s", DisplayID(key, n))
}

// resolveSpec reads the Spec ref names as op's field.
func resolveSpec(r store.Reader, key, op, field string, ref Ref) (store.Spec, error) {
	n, err := ref.number(key, field)
	if err != nil {
		return store.Spec{}, err
	}
	if s, ok := r.Spec(key, n); ok {
		return s, nil
	}
	if err := ticketNotSpec(r, key, op, field, n); err != nil {
		return store.Spec{}, err
	}
	return store.Spec{}, missing(op, field, "there is no Spec %s", DisplayID(key, n))
}

// specNotTicket is the error for n, which isn't a Ticket, given as op's
// field: what to do instead when n is a Spec, nil when it isn't.
func specNotTicket(r store.Reader, key, op, field string, n int) error {
	if _, ok := r.Spec(key, n); !ok {
		return nil
	}
	id := DisplayID(key, n)
	if mainTarget(op, field) {
		return notFound("%s", specAsTicket(op, id))
	}
	return invalid(field, "%s is a Spec, not a Ticket; give a Ticket of it (get_spec lists them)", id)
}

// ticketNotSpec is the error for n, which isn't a Spec, given as op's
// field: the Ticket's Spec and what to do instead when n is a Ticket, nil
// when it isn't.
func ticketNotSpec(r store.Reader, key, op, field string, n int) error {
	t, ok := r.Ticket(key, n)
	if !ok {
		return nil
	}
	id, spec := DisplayID(key, n), DisplayID(key, t.Spec)
	if mainTarget(op, field) {
		return notFound("%s is a Ticket of Spec %s; use %s", id, spec, specOps[op])
	}
	return invalid(field, "%s is a Ticket of Spec %s; pass %s as %s", id, spec, spec, field)
}

// specAsTicket is the message for Spec id given as op's Ticket.
func specAsTicket(op, id string) string {
	switch op {
	case "get_ticket":
		return id + " is a Spec; use get_spec"
	case "edit_ticket":
		return id + " is a Spec; use edit_spec"
	case "update_linked_ticket":
		return id + " is a Spec; use update_linked_spec"
	case "claim":
		return id + " is a Spec; pass it as spec to claim its next Ticket"
	}
	return id + " is a Spec, not a Ticket; get_spec lists its Tickets"
}

// specOps maps each operation whose main target is a Spec to the Ticket
// operation to use instead.
var specOps = map[string]string{
	"get_spec":           "get_ticket",
	"get_spec_questions": "get_ticket",
	"edit_spec":          "edit_ticket",
	"update_linked_spec": "update_linked_ticket",
}

// mainTarget reports whether field is op's main target: spec for the
// operations in specOps, ticket for every other.
func mainTarget(op, field string) bool {
	if _, ok := specOps[op]; ok {
		return field == "spec"
	}
	return field == "ticket"
}

// missing is a lookup's failure: not_found for op's main target, invalid
// with field for a nested one.
func missing(op, field, format string, args ...any) *Error {
	if mainTarget(op, field) {
		return notFound(format, args...)
	}
	return invalid(field, format, args...)
}
