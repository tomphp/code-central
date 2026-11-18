package core

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Integration contract (ticket 42, ADR 0004). An Integration is a
// separate program writing over HTTP as the integration Actor. On a
// linked Spec or Ticket it owns the definition (title, description or
// body, and a Ticket's Spec); Code Central owns the delivery. These
// operations are deliberately minimal: the first real Integration will
// settle their signatures.

// LinkView is a Spec's or Ticket's Link as reads show it: the external
// item its Integration mirrors. While it isn't detached, the title and
// description belong to the external system. Outbound marks a Link that
// doesn't own the item's definition: Code Central owns it and only
// writes to the external item.
type LinkView struct {
	Integration   string               `json:"integration"`
	ID            string               `json:"id"`
	URL           string               `json:"url,omitempty"`
	ExternalState domain.ExternalState `json:"external_state"`
	Detached      bool                 `json:"detached"`
	Outbound      bool                 `json:"outbound"`
}

func linkView(l domain.Link) *LinkView {
	if !l.Linked() {
		return nil
	}
	return &LinkView{Integration: l.Integration, ID: l.ID, URL: l.URL, ExternalState: l.ExternalState, Detached: l.Detached, Outbound: l.Outbound}
}

// LinkInput is the Link an Integration creates an item with. The
// external state defaults to open.
type LinkInput struct {
	Integration   string               `json:"integration"`
	ID            string               `json:"id"`
	URL           string               `json:"url,omitempty"`
	ExternalState domain.ExternalState `json:"external_state,omitempty"`
}

func (in LinkInput) link() (domain.Link, error) {
	l := domain.Link{Integration: strings.TrimSpace(in.Integration), ID: strings.TrimSpace(in.ID), URL: in.URL, ExternalState: in.ExternalState}
	if l.ExternalState == "" {
		l.ExternalState = domain.ExternalOpen
	}
	switch {
	case l.Integration == "" || strings.ContainsAny(l.Integration, "\r\n"):
		return l, invalid("link.integration", "a Link names its Integration on one line")
	case l.ID == "" || strings.ContainsAny(l.ID, "\r\n"):
		return l, invalid("link.id", "a Link needs the external item's ID on one line")
	case !l.ExternalState.Valid():
		return l, invalid("link.external_state", "unknown external state %q (open, closed, deleted or out_of_scope)", l.ExternalState)
	}
	if l.URL != "" && !absoluteURL(l.URL) {
		return l, invalid("link.url", "a Link's URL is an absolute http or https URL, not %q", l.URL)
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
func requireActor(ctx context.Context, op string, want Actor) error {
	if a := ActorOf(ctx); a != want {
		return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is an action for the %s Actor; this request is made as %s", op, want, a)}
	}
	return nil
}

// definitionOwned refuses a definition write by anyone but the
// integration Actor on an item whose Link owns its definition.
func definitionOwned(ctx context.Context, id string, l domain.Link) error {
	if a := ActorOf(ctx); l.Owned() && a != Integration {
		return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf(
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
			return DisplayID(key, found[0].Number), true
		}
		return "", false
	}
	if found := r.FindTickets(key, store.TicketFilter{Link: link}); len(found) > 0 {
		return DisplayID(key, found[0].Number), true
	}
	return "", false
}

// CreateLinkedSpecInput is create_linked_spec's input.
type CreateLinkedSpecInput struct {
	Title string    `json:"title"`
	Body  string    `json:"body,omitempty"`
	Link  LinkInput `json:"link"`
	// Full asks for the Spec in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// CreateLinkedSpec is the integration's create_linked_spec(title, body,
// link, full?).
func (c *Core) CreateLinkedSpec(ctx context.Context, key string, in CreateLinkedSpecInput) (Written[SpecView], error) {
	if err := requireActor(ctx, "create_linked_spec", Integration); err != nil {
		return Written[SpecView]{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return Written[SpecView]{}, invalid("title", "a Spec needs a title")
	}
	l, err := in.Link.link()
	if err != nil {
		return Written[SpecView]{}, err
	}
	var v Written[SpecView]
	err = c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		if id, ok := linkedTo(w, key, l, true); ok {
			return invalid("link.id", "%s %s is already linked to Spec %s", l.Integration, l.ID, id)
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

// CreateLinkedTicketInput is create_linked_ticket's input. State is
// Draft (the default) or Ready, as the Integration is configured.
type CreateLinkedTicketInput struct {
	Spec        Ref          `json:"spec"`
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	State       domain.State `json:"state,omitempty"`
	Link        LinkInput    `json:"link"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// CreateLinkedTicket is the integration's create_linked_ticket(spec,
// title, description, state, link, full?). On an Integration's inbox
// Spec, the Issues Spec or the Azure Boards Spec, while that
// Integration's New issues need me is on, Code Central marks the Ticket
// awaiting triage (Spec code-central-279, Spec code-central-894).
func (c *Core) CreateLinkedTicket(ctx context.Context, key string, in CreateLinkedTicketInput) (Written[TicketView], error) {
	if err := requireActor(ctx, "create_linked_ticket", Integration); err != nil {
		return Written[TicketView]{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return Written[TicketView]{}, invalid("title", "a Ticket needs a title")
	}
	state := in.State
	if state == "" {
		state = domain.Draft
	}
	if state != domain.Draft && state != domain.Ready {
		return Written[TicketView]{}, invalid("state", "an Integration creates a Ticket in Draft or Ready, not %q", state)
	}
	if in.Spec == "" {
		return Written[TicketView]{}, invalid("spec", "every Ticket belongs to a Spec")
	}
	l, err := in.Link.link()
	if err != nil {
		return Written[TicketView]{}, err
	}
	var v Written[TicketView]
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
			return invalid("link.id", "%s %s is already linked to Ticket %s", l.Integration, l.ID, id)
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

// UpdateLinkedSpecInput is update_linked_spec's input. A field left out
// is unchanged.
type UpdateLinkedSpecInput struct {
	Spec  Ref     `json:"spec" path:"n"`
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
	// Full asks for the Spec in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// UpdateLinkedSpec is the integration's update_linked_spec(spec, title?,
// body?, full?) on a Spec whose Link owns its definition, recorded in its
// edits.
func (c *Core) UpdateLinkedSpec(ctx context.Context, key string, in UpdateLinkedSpecInput) (Written[SpecView], error) {
	if err := requireActor(ctx, "update_linked_spec", Integration); err != nil {
		return Written[SpecView]{}, err
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return Written[SpecView]{}, invalid("title", "a Spec needs a title")
	}
	var v Written[SpecView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		s, err := resolveSpec(w, key, "update_linked_spec", "spec", in.Spec)
		if err != nil {
			return err
		}
		n := s.Number
		if err := ownedByLink(DisplayID(key, n), s.Link); err != nil {
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

// UpdateLinkedTicketInput is update_linked_ticket's input. A field left
// out is unchanged; spec moves the Ticket under another Spec (its
// external parent changed), and url refreshes its Link's URL (the
// external item moved), which isn't a definition edit.
type UpdateLinkedTicketInput struct {
	Ticket      Ref     `json:"ticket" path:"n"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Spec        *Ref    `json:"spec,omitempty"`
	URL         *string `json:"url,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// UpdateLinkedTicket is the integration's update_linked_ticket(ticket,
// title?, description?, spec?, full?) on a Ticket whose Link owns its
// definition, recorded in its edits.
func (c *Core) UpdateLinkedTicket(ctx context.Context, key string, in UpdateLinkedTicketInput) (Written[TicketView], error) {
	if err := requireActor(ctx, "update_linked_ticket", Integration); err != nil {
		return Written[TicketView]{}, err
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return Written[TicketView]{}, invalid("title", "a Ticket needs a title")
	}
	if in.URL != nil && !absoluteURL(*in.URL) {
		return Written[TicketView]{}, invalid("url", "a Link's URL is an absolute http or https URL, not %q", *in.URL)
	}
	return c.updateTicketIn(key, "update_linked_ticket", in.Ticket, in.Full, func(r store.Reader, t *domain.Ticket) (bool, error) {
		if err := ownedByLink(DisplayID(key, t.Number), t.Link); err != nil {
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
				prev["spec"] = DisplayID(key, t.Spec)
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
		return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is not linked, so Code Central owns its definition", id)}
	case l.Detached:
		return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s was unlinked from %s %s, so Code Central owns its definition now", id, l.Integration, l.ID)}
	case l.Outbound:
		return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s's Link to %s %s is outbound, so Code Central owns its definition and only writes to %s", id, l.Integration, l.ID, l.Integration)}
	}
	return nil
}

// recordEdit appends an edit by ctx's Actor when prev holds a change.
func (c *Core) recordEdit(ctx context.Context, edits *[]domain.Edit, prev map[string]string) bool {
	if len(prev) == 0 {
		return false
	}
	*edits = append(*edits, domain.Edit{Number: len(*edits) + 1, Actor: string(ActorOf(ctx)), Time: c.now(), Previous: prev})
	return true
}

// LinkedRow is a linked Spec or Ticket as the link operations return it.
// State is set for a Ticket.
type LinkedRow struct {
	Kind  RecordKind   `json:"kind"`
	ID    string       `json:"id"`
	Title string       `json:"title"`
	State domain.State `json:"state,omitempty"`
	Link  LinkView     `json:"link"`
}

// SetExternalStateInput is set_external_state's input.
type SetExternalStateInput struct {
	Target        Ref                  `json:"target"`
	ExternalState domain.ExternalState `json:"external_state"`
}

// SetExternalState is the integration's set_external_state(target,
// external_state): the external status as last seen. It never moves
// State; a linked Ticket that isn't open shows as closed externally in
// Needs Me instead.
func (c *Core) SetExternalState(ctx context.Context, key string, in SetExternalStateInput) (LinkedRow, error) {
	if err := requireActor(ctx, "set_external_state", Integration); err != nil {
		return LinkedRow{}, err
	}
	if !in.ExternalState.Valid() {
		return LinkedRow{}, invalid("external_state", "unknown external state %q (open, closed, deleted or out_of_scope)", in.ExternalState)
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

// UnlinkInput is the human's unlink(target).
type UnlinkInput struct {
	Target Ref `json:"target"`
}

// Unlink is the human's unlink(target): the Link is kept but detached,
// so the definition becomes editable, it stops counting for Needs Me,
// nothing more is written to the external item, and lookups still find
// it. There is no re-link.
func (c *Core) Unlink(ctx context.Context, key string, in UnlinkInput) (LinkedRow, error) {
	if err := requireActor(ctx, "unlink", Human); err != nil {
		return LinkedRow{}, err
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
func (c *Core) updateLink(ctx context.Context, key string, ref Ref, fn func(l *domain.Link) (map[string]string, error)) (LinkedRow, error) {
	var row LinkedRow
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		n, err := ref.number(key, "target")
		if err != nil {
			return err
		}
		id := DisplayID(key, n)
		notLinked := invalid("target", "%s is not linked", id)
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
		return notFound("there is no Spec or Ticket %s", id)
	})
	return row, err
}

func specLinkedRow(key string, s domain.Spec) LinkedRow {
	return LinkedRow{Kind: RecordSpec, ID: DisplayID(key, s.Number), Title: s.Title, Link: *linkView(s.Link)}
}

func ticketLinkedRow(key string, t domain.Ticket) LinkedRow {
	return LinkedRow{Kind: RecordTicket, ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Link: *linkView(t.Link)}
}

// FindLinkedInput is find_linked's filters: the Integration, and
// optionally one external ID.
type FindLinkedInput struct {
	Integration string `json:"integration"`
	ID          string `json:"id,omitempty"`
}

// FindLinked is find_linked(integration, id?): every Spec and Ticket in
// the Project linked by that Integration, detached and outbound ones
// too, so an Integration knows what it has already pulled in. Rows are
// in number order.
func (c *Core) FindLinked(_ context.Context, key string, in FindLinkedInput) ([]LinkedRow, error) {
	if strings.TrimSpace(in.Integration) == "" {
		return nil, invalid("integration", "name the Integration whose links to find")
	}
	rows := []LinkedRow{}
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		link := &store.LinkFilter{Integration: in.Integration, ID: in.ID}
		byNumber := map[int]LinkedRow{}
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
