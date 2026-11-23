package core

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Integration contract (ticket 42, ADR 0004). An Integration is a
// separate program writing over HTTP as the integration Actor. On a
// linked Spec or Ticket it owns the definition (title, description or
// body, and a Ticket's Spec); Code Central owns the delivery. These
// operations are deliberately minimal: the first real Integration will
// settle their signatures.

func linkView(l domain.Link) *api.LinkView {
	if !l.Linked() {
		return nil
	}
	return &api.LinkView{Integration: l.Integration, ID: l.ID, URL: l.URL, ExternalState: l.ExternalState, Detached: l.Detached, Outbound: l.Outbound}
}

func linkOf(in api.LinkInput) (domain.Link, error) {
	l := domain.Link{Integration: strings.TrimSpace(in.Integration), ID: strings.TrimSpace(in.ID), URL: in.URL, ExternalState: in.ExternalState}
	if l.ExternalState == "" {
		l.ExternalState = domain.ExternalOpen
	}
	switch {
	case l.Integration == "" || strings.ContainsAny(l.Integration, "\r\n"):
		return l, api.InvalidField("link.integration", "a Link names its Integration on one line")
	case l.ID == "" || strings.ContainsAny(l.ID, "\r\n"):
		return l, api.InvalidField("link.id", "a Link needs the external item's ID on one line")
	case !l.ExternalState.Valid():
		return l, api.InvalidField("link.external_state", "unknown external state %q (open, closed, deleted or out_of_scope)", l.ExternalState)
	}
	if l.URL != "" && !absoluteURL(l.URL) {
		return l, api.InvalidField("link.url", "a Link's URL is an absolute http or https URL, not %q", l.URL)
	}
	return l, nil
}

// absoluteURL reports whether s is an absolute http or https URL, as a
// Link's URL is.
func absoluteURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// requireActor refuses op unless the request is made as want.
func requireActor(ctx context.Context, op string, want api.Actor) error {
	if a := api.ActorOf(ctx); a != want {
		return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf("%s is an action for the %s Actor; this request is made as %s", op, want, a)}
	}
	return nil
}

// definitionOwned refuses a definition write by anyone but the
// integration Actor on an item whose Link owns its definition.
func definitionOwned(ctx context.Context, id string, l domain.Link) error {
	if a := api.ActorOf(ctx); l.Owned() && a != api.Integration {
		return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf(
			"%s is linked to %s %s, which owns its definition: change it there, or unlink it first (this request is made as %s)",
			id, l.Integration, l.ID, a)}
	}
	return nil
}

// linkedTo is the Spec or Ticket in key already linked to the external
// item, detached or not, if any. Specs and Tickets are checked
// separately: an external item with no parent is a Spec and its one
// Ticket.
func linkedTo(r store.Reader, key string, l domain.Link, specs bool) (string, bool) {
	link := &store.LinkFilter{Integration: l.Integration, ID: l.ID}
	if specs {
		if found := r.FindSpecs(key, store.SpecFilter{Link: link}); len(found) > 0 {
			return domain.DisplayID(key, found[0].Number), true
		}
		return "", false
	}
	if found := r.FindTickets(key, store.TicketFilter{Link: link}); len(found) > 0 {
		return domain.DisplayID(key, found[0].Number), true
	}
	return "", false
}

// CreateLinkedSpec is the integration's create_linked_spec(title, body,
// link, full?).
func (c *Core) CreateLinkedSpec(ctx context.Context, key string, in api.CreateLinkedSpecInput) (api.Written[api.SpecView], error) {
	if err := requireActor(ctx, "create_linked_spec", api.Integration); err != nil {
		return api.Written[api.SpecView]{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return api.Written[api.SpecView]{}, api.InvalidField("title", "a Spec needs a title")
	}
	l, err := linkOf(in.Link)
	if err != nil {
		return api.Written[api.SpecView]{}, err
	}
	var v api.Written[api.SpecView]
	err = c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		if id, ok := linkedTo(w, key, l, true); ok {
			return api.InvalidField("link.id", "%s %s is already linked to Spec %s", l.Integration, l.ID, id)
		}
		now := c.now()
		s := domain.Spec{Number: w.NextNumber(key), Title: in.Title, Link: l, Created: now, Updated: now, Body: in.Body}
		if err := w.PutSpec(key, s); err != nil {
			return err
		}
		v, err = acknowledgeCreate(specView(key, domain.Spec{}), specView(key, s), in.Full)
		return err
	})
	return v, err
}

// CreateLinkedTicket is the integration's create_linked_ticket(spec,
// title, description, state, link, full?). On an Integration's inbox
// Spec, the Issues Spec or the Azure Boards Spec, while that
// Integration's New issues need me is on, Code Central marks the Ticket
// awaiting triage (Spec code-central-279, Spec code-central-894).
func (c *Core) CreateLinkedTicket(ctx context.Context, key string, in api.CreateLinkedTicketInput) (api.Written[api.TicketView], error) {
	if err := requireActor(ctx, "create_linked_ticket", api.Integration); err != nil {
		return api.Written[api.TicketView]{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return api.Written[api.TicketView]{}, api.InvalidField("title", "a Ticket needs a title")
	}
	state := in.State
	if state == "" {
		state = domain.Draft
	}
	if state != domain.Draft && state != domain.Ready {
		return api.Written[api.TicketView]{}, api.InvalidField("state", "an Integration creates a Ticket in Draft or Ready, not %q", state)
	}
	if in.Spec == "" {
		return api.Written[api.TicketView]{}, api.InvalidField("spec", "every Ticket belongs to a Spec")
	}
	l, err := linkOf(in.Link)
	if err != nil {
		return api.Written[api.TicketView]{}, err
	}
	var v api.Written[api.TicketView]
	err = c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		sp, err := resolveSpec(w, key, "create_linked_ticket", "spec", in.Spec)
		if err != nil {
			return err
		}
		specN := sp.Number
		if id, ok := linkedTo(w, key, l, false); ok {
			return api.InvalidField("link.id", "%s %s is already linked to Ticket %s", l.Integration, l.ID, id)
		}
		now := c.now()
		t := domain.Ticket{Number: w.NextNumber(key), Title: in.Title, State: state, Spec: specN, Link: l, Created: now, Updated: now, Description: in.Description,
			AwaitingTriage: awaitsTriage(w, key, specN)}
		if err := whole.put(w, key, &t); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, domain.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}

// UpdateLinkedSpec is the integration's update_linked_spec(spec, title?,
// body?, full?) on a Spec whose Link owns its definition, recorded in its
// edits.
func (c *Core) UpdateLinkedSpec(ctx context.Context, key string, in api.UpdateLinkedSpecInput) (api.Written[api.SpecView], error) {
	if err := requireActor(ctx, "update_linked_spec", api.Integration); err != nil {
		return api.Written[api.SpecView]{}, err
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return api.Written[api.SpecView]{}, api.InvalidField("title", "a Spec needs a title")
	}
	var v api.Written[api.SpecView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		s, err := resolveSpec(w, key, "update_linked_spec", "spec", in.Spec)
		if err != nil {
			return err
		}
		n := s.Number
		if err := ownedByLink(domain.DisplayID(key, n), s.Link); err != nil {
			return err
		}
		before := specView(key, s)
		prev := map[string]string{}
		change(prev, "title", &s.Title, in.Title)
		change(prev, "body", &s.Body, in.Body)
		if c.recordEdit(ctx, &s.Edits, prev) {
			s.Updated = c.now()
			if err := w.PutSpec(key, s); err != nil {
				return err
			}
		}
		v, err = acknowledge(before, specView(key, s), in.Full)
		return err
	})
	return v, err
}

// UpdateLinkedTicket is the integration's update_linked_ticket(ticket,
// title?, description?, spec?, full?) on a Ticket whose Link owns its
// definition, recorded in its edits.
func (c *Core) UpdateLinkedTicket(ctx context.Context, key string, in api.UpdateLinkedTicketInput) (api.Written[api.TicketView], error) {
	if err := requireActor(ctx, "update_linked_ticket", api.Integration); err != nil {
		return api.Written[api.TicketView]{}, err
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return api.Written[api.TicketView]{}, api.InvalidField("title", "a Ticket needs a title")
	}
	if in.URL != nil && !absoluteURL(*in.URL) {
		return api.Written[api.TicketView]{}, api.InvalidField("url", "a Link's URL is an absolute http or https URL, not %q", *in.URL)
	}
	return c.updateTicketIn(key, "update_linked_ticket", in.Ticket, in.Full, func(r store.Reader, t *domain.Ticket) (bool, error) {
		if err := ownedByLink(domain.DisplayID(key, t.Number), t.Link); err != nil {
			return false, err
		}
		moved := in.URL != nil && *in.URL != t.Link.URL
		if moved {
			t.Link.URL = *in.URL
		}
		prev := map[string]string{}
		if in.Spec != nil {
			sp, err := resolveSpec(r, key, "update_linked_ticket", "spec", *in.Spec)
			if err != nil {
				return false, err
			}
			n := sp.Number
			if n != t.Spec {
				prev["spec"] = domain.DisplayID(key, t.Spec)
				t.Spec = n
			}
		}
		change(prev, "title", &t.Title, in.Title)
		change(prev, "description", &t.Description, in.Description)
		return c.recordEdit(ctx, &t.Edits, prev) || moved, nil
	})
}

// ownedByLink refuses an Integration's update of an item its Link no
// longer owns.
func ownedByLink(id string, l domain.Link) error {
	switch {
	case !l.Linked():
		return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf("%s is not linked, so Code Central owns its definition", id)}
	case l.Detached:
		return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf("%s was unlinked from %s %s, so Code Central owns its definition now", id, l.Integration, l.ID)}
	case l.Outbound:
		return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf("%s's Link to %s %s is outbound, so Code Central owns its definition and only writes to %s", id, l.Integration, l.ID, l.Integration)}
	}
	return nil
}

// recordEdit appends an edit by ctx's Actor when prev holds a change.
func (c *Core) recordEdit(ctx context.Context, edits *[]domain.Edit, prev map[string]string) bool {
	if len(prev) == 0 {
		return false
	}
	*edits = append(*edits, domain.Edit{Number: len(*edits) + 1, Actor: string(api.ActorOf(ctx)), Time: c.now(), Previous: prev})
	return true
}

// SetExternalState is the integration's set_external_state(target,
// external_state): the external status as last seen. It never moves
// State; a linked Ticket that isn't open shows as closed externally in
// Needs Me instead.
func (c *Core) SetExternalState(ctx context.Context, key string, in api.SetExternalStateInput) (api.LinkedRow, error) {
	if err := requireActor(ctx, "set_external_state", api.Integration); err != nil {
		return api.LinkedRow{}, err
	}
	if !in.ExternalState.Valid() {
		return api.LinkedRow{}, api.InvalidField("external_state", "unknown external state %q (open, closed, deleted or out_of_scope)", in.ExternalState)
	}
	return c.updateLink(ctx, key, in.Target, func(l *domain.Link) (map[string]string, error) {
		if l.ExternalState == in.ExternalState {
			return nil, nil
		}
		prev := map[string]string{"external_state": string(l.ExternalState)}
		l.ExternalState = in.ExternalState
		return prev, nil
	})
}

// Unlink is the human's unlink(target): the Link is kept but detached,
// so the definition becomes editable, it stops counting for Needs Me,
// nothing more is written to the external item, and lookups still find
// it. There is no re-link.
func (c *Core) Unlink(ctx context.Context, key string, in api.UnlinkInput) (api.LinkedRow, error) {
	if err := requireActor(ctx, "unlink", api.Human); err != nil {
		return api.LinkedRow{}, err
	}
	return c.updateLink(ctx, key, in.Target, func(l *domain.Link) (map[string]string, error) {
		if l.Detached {
			return nil, nil
		}
		l.Detached = true
		return map[string]string{"detached": "false"}, nil
	})
}

// updateLink applies fn to the Link of the linked Spec or Ticket ref
// names (they share one counter). fn returns the edit's previous values,
// nil when nothing changed, and empty for a change kept out of the edit
// history.
func (c *Core) updateLink(ctx context.Context, key string, ref api.Ref, fn func(l *domain.Link) (map[string]string, error)) (api.LinkedRow, error) {
	var row api.LinkedRow
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		n, err := api.RefNumber(ref, key, "target")
		if err != nil {
			return err
		}
		id := domain.DisplayID(key, n)
		notLinked := api.InvalidField("target", "%s is not linked", id)
		if s, ok := w.Spec(key, n); ok {
			if !s.Link.Linked() {
				return notLinked
			}
			prev, err := fn(&s.Link)
			if err != nil {
				return err
			}
			if prev != nil {
				c.recordEdit(ctx, &s.Edits, prev)
				s.Updated = c.now()
				if err := w.PutSpec(key, s); err != nil {
					return err
				}
			}
			row = specLinkedRow(key, s)
			return nil
		}
		h := held{store.Edits}
		if t, ok := w.Ticket(key, n, h...); ok {
			if !t.Link.Linked() {
				return notLinked
			}
			prev, err := fn(&t.Link)
			if err != nil {
				return err
			}
			if prev != nil {
				c.recordEdit(ctx, &t.Edits, prev)
				t.Updated = c.now()
				if err := h.put(w, key, &t); err != nil {
					return err
				}
			}
			row = ticketLinkedRow(key, t)
			return nil
		}
		return api.NotFoundError("there is no Spec or Ticket %s", id)
	})
	return row, err
}

func specLinkedRow(key string, s domain.Spec) api.LinkedRow {
	return api.LinkedRow{Kind: api.RecordSpec, ID: domain.DisplayID(key, s.Number), Title: s.Title, Link: *linkView(s.Link)}
}

func ticketLinkedRow(key string, t domain.Ticket) api.LinkedRow {
	return api.LinkedRow{Kind: api.RecordTicket, ID: domain.DisplayID(key, t.Number), Title: t.Title, State: t.State, Link: *linkView(t.Link)}
}

// FindLinked is find_linked(integration, id?): every Spec and Ticket in
// the Project linked by that Integration, detached and outbound ones
// too, so an Integration knows what it has already pulled in. Rows are
// in number order.
func (c *Core) FindLinked(_ context.Context, key string, in api.FindLinkedInput) ([]api.LinkedRow, error) {
	if strings.TrimSpace(in.Integration) == "" {
		return nil, api.InvalidField("integration", "name the Integration whose links to find")
	}
	rows := []api.LinkedRow{}
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return api.UnknownProjectError(key)
		}
		link := &store.LinkFilter{Integration: in.Integration, ID: in.ID}
		byNumber := map[int]api.LinkedRow{}
		for _, s := range r.FindSpecs(key, store.SpecFilter{Link: link}) {
			byNumber[s.Number] = specLinkedRow(key, s)
		}
		for _, t := range r.FindTickets(key, store.TicketFilter{Link: link}) {
			byNumber[t.Number] = ticketLinkedRow(key, t)
		}
		for n := 1; n < r.NextNumber(key); n++ {
			if row, ok := byNumber[n]; ok {
				rows = append(rows, row)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// closedExternally reports whether t is a linked Ticket whose external
// item isn't open, whose Link owns its definition and that isn't
// finished, and since when: its last external state change, or its
// creation.
func closedExternally(t domain.Ticket) (time.Time, bool) {
	if !t.Link.Owned() || t.Link.ExternalState == domain.ExternalOpen || finished(t.State) {
		return time.Time{}, false
	}
	if !t.Head.ExternalStateChanged.IsZero() {
		return t.Head.ExternalStateChanged, true
	}
	return t.Created, true
}
