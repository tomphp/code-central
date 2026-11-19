package ops

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
)

// Defining work: creating, reading, finding and shaping Specs and Tickets.

// CreateSpec is create_spec.
var CreateSpec = &Definition[api.CreateSpecInput, api.Written[api.SpecView]]{
	Name: "create_spec", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/create_spec",
	Doc:  "Create a Spec, the description of a piece of work delivered through one or more Tickets; returns an acknowledgement with its ID, or the Spec with full.",
	Call: (*core.Core).CreateSpec,
}

// CreateTicket is create_ticket.
var CreateTicket = &Definition[api.CreateTicketInput, api.Written[api.TicketView]]{
	Name: "create_ticket", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/create_ticket",
	Doc:  "Create a Ticket in a Spec, as Draft (the default) or Ready. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
	Call: (*core.Core).CreateTicket,
}

// GetTicket is get_ticket. With no part named, the HTTP API returns the
// whole Ticket: the UI and the Factory read every part. The whole Ticket
// carries the Model Choice too, which the UI's Ticket panel shows; the
// CLI, an agent door, names its parts, so it leaves it out as MCP does.
var GetTicket = &Definition[api.GetTicketInput, api.TicketView]{
	Name: "get_ticket", Audience: AgentAndHuman,
	HTTP: "GET /api/p/{key}/tickets/{n}",
	Doc:  "Get one Ticket: its description, State, Blockers, past Claims and how each ended, every Failure Report and every comment; left_out counts what it leaves out, and parts (claims, failure_reports, comments, rounds, moves, edits) asks for exactly those.",
	Call: (*core.Core).GetTicket,
	HTTPDefaults: func(in *api.GetTicketInput) {
		if len(in.Parts) == 0 {
			in.Parts, in.WithModelChoice = api.TicketParts(), true
		}
	},
}

// FindTickets is find_tickets: rows, or with full each whole Ticket as
// get_ticket returns it. As get_ticket's, the HTTP default with full and no
// part named is every part with the Model Choice.
var FindTickets = &Definition[api.FindTicketsInput, any]{
	Name: "find_tickets", Audience: AgentAndHuman,
	HTTP: "GET /api/p/{key}/tickets",
	Doc:  "Find this Project's Tickets in claim order; every filter given must match. Each row carries needs_me_reason when the Ticket waits on the human, the same as get_ticket. With full, each is the whole Ticket as get_ticket returns it (the default parts, brief), in place of the row.",
	Call: func(c *core.Core, ctx context.Context, key string, in api.FindTicketsInput) (any, error) {
		if !in.Full {
			if len(in.Parts) > 0 {
				return nil, &api.Error{Code: api.Invalid, Field: "part", Message: "part names the parts of full Tickets, so it needs full=true"}
			}
			return c.FindTickets(ctx, key, in)
		}
		return c.FindTicketViews(ctx, key, in)
	},
	HTTPDefaults: func(in *api.FindTicketsInput) {
		if in.Full && len(in.Parts) == 0 {
			in.Parts, in.WithModelChoice = api.TicketParts(), true
		}
	},
}

// EditSpec is edit_spec.
var EditSpec = &Definition[api.EditSpecInput, api.Written[api.SpecView]]{
	Name: "edit_spec", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/specs/{n}/edit_spec",
	Doc:  "Edit a Spec's title or body; a body given replaces the old one whole, and the old text is kept in its edit history. Returns an acknowledgement naming the changed fields, or the Spec with full.",
	Call: (*core.Core).EditSpec,
}

// EditTicket is edit_ticket.
var EditTicket = &Definition[api.EditTicketInput, api.Written[api.TicketView]]{
	Name: "edit_ticket", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/edit_ticket",
	Doc:  "Edit a Ticket's title or description; the old text is kept in its edit history. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
	Call: (*core.Core).EditTicket,
}

// AddLabels is add_labels.
var AddLabels = &Definition[api.LabelsInput, api.TargetView]{
	Name: "add_labels", Audience: AgentAndHuman, Write: true,
	HTTP:         "POST /api/p/{key}/add_labels",
	Doc:          "Add Labels to a Spec or Ticket, leaving its other Labels alone.",
	Descriptions: map[string]string{"labels": "Labels to add."},
	Call:         (*core.Core).AddLabels,
}

// RemoveLabels is remove_labels.
var RemoveLabels = &Definition[api.LabelsInput, api.TargetView]{
	Name: "remove_labels", Audience: AgentAndHuman, Write: true,
	HTTP:         "POST /api/p/{key}/remove_labels",
	Doc:          "Remove Labels from a Spec or Ticket, leaving its other Labels alone.",
	Descriptions: map[string]string{"labels": "Labels to remove."},
	Call:         (*core.Core).RemoveLabels,
}

// SetTags is set_tags.
var SetTags = &Definition[api.SetTagsInput, api.TargetView]{
	Name: "set_tags", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/set_tags",
	Doc:  "Set Tags on a Spec or Ticket, overwriting only the keys named; keys are lowercase [a-z0-9._-], values one non-empty line.",
	Call: (*core.Core).SetTags,
}

// RemoveTags is remove_tags.
var RemoveTags = &Definition[api.RemoveTagsInput, api.TargetView]{
	Name: "remove_tags", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/remove_tags",
	Doc:  "Remove Tags from a Spec or Ticket by key.",
	Call: (*core.Core).RemoveTags,
}

// Comment is comment.
var Comment = &Definition[api.CommentInput, api.Written[api.TicketView]]{
	Name: "comment", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/comment",
	Doc:  "Comment on any Ticket, even one another agent holds; no Claim ID is needed. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
	Call: (*core.Core).Comment,
}

// Approve is approve.
var Approve = &Definition[api.TicketInput, api.Written[api.TicketView]]{
	Name: "approve", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/approve",
	Doc:  "Approve a Draft Ticket you have finished defining, moving it to Ready so it can be claimed. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
	Call: (*core.Core).Approve,
}

// Defer is defer.
var Defer = &Definition[api.DeferInput, api.Written[api.TicketView]]{
	Name: "defer", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/defer",
	Doc:  "Defer a Ready Ticket that isn't ready back to Draft, with the reason stored as a comment. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
	Call: (*core.Core).Defer,
}

// GetSpec is get_spec.
var GetSpec = &Definition[api.GetSpecInput, api.SpecDetail]{
	Name: "get_spec", Audience: AgentAndHuman,
	HTTP: "GET /api/p/{key}/specs/{n}",
	Doc:  "Get a Spec with its body, a count of its Tickets per State, and its Tickets as find_tickets rows; parts (body, tickets, edits, outline) and sections (heading texts) read less and add the body's outline, each heading with its Section's size in bytes; parts [\"outline\"] alone is the cheapest read; sections alone return only the body.",
	Call: (*core.Core).GetSpec,
}

// FindSpecs is find_specs.
var FindSpecs = &Definition[api.FindSpecsInput, []api.SpecRow]{
	Name: "find_specs", Audience: AgentAndHuman,
	HTTP: "GET /api/p/{key}/specs",
	Doc:  "Find this Project's Specs with their State summaries; finished Specs are left out unless finished is true.",
	Call: (*core.Core).FindSpecs,
}
