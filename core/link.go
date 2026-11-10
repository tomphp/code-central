package core

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// The Integration contract (ticket 42, ADR 0004). An Integration is a
// separate program writing over HTTP as the integration Actor. On a
// linked Spec or Ticket it owns the definition (title, description or
// body, and a Ticket's Spec); Code Central owns the delivery. These
// operations are deliberately minimal: the first real Integration will
// settle their signatures.

// LinkView is a Spec's or Ticket's Link as reads show it.
type LinkView struct {
	Integration   string              `json:"integration"`
	ID            string              `json:"id"`
	URL           string              `json:"url,omitempty"`
	ExternalState store.ExternalState `json:"external_state"`
	Detached      bool                `json:"detached"`
}

func linkView(l store.Link) *LinkView {
	if !l.Linked() {
		return nil
	}
	return &LinkView{Integration: l.Integration, ID: l.ID, URL: l.URL, ExternalState: l.ExternalState, Detached: l.Detached}
}

// LinkInput is the Link an Integration creates an item with. The
// external state defaults to open.
type LinkInput struct {
	Integration   string              `json:"integration"`
	ID            string              `json:"id"`
	URL           string              `json:"url,omitempty"`
	ExternalState store.ExternalState `json:"external_state,omitempty"`
}

func (in LinkInput) link() (store.Link, error) {
	l := store.Link{Integration: strings.TrimSpace(in.Integration), ID: strings.TrimSpace(in.ID), URL: in.URL, ExternalState: in.ExternalState}
	if l.ExternalState == "" {
		l.ExternalState = store.ExternalOpen
	}
	switch {
	case l.Integration == "" || strings.ContainsAny(l.Integration, "\r\n"):
		return l, invalid("link.integration", "a Link names its Integration on one line")
	case l.ID == "" || strings.ContainsAny(l.ID, "\r\n"):
		return l, invalid("link.id", "a Link needs the external item's ID on one line")
	case !l.ExternalState.Valid():
		return l, invalid("link.external_state", "unknown external state %q (open, closed, deleted or out_of_scope)", l.ExternalState)
	}
	if l.URL != "" {
		u, err := url.Parse(l.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return l, invalid("link.url", "a Link's URL is an absolute http or https URL, not %q", l.URL)
		}
	}
	return l, nil
}

// requireActor refuses op unless the request is made as want.
func requireActor(ctx context.Context, op string, want Actor) error {
	if a := ActorOf(ctx); a != want {
		return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is an action for the %s Actor; this request is made as %s", op, want, a)}
	}
	return nil
}

// definitionOwned refuses a definition write by anyone but the
// integration Actor on an item whose Link isn't detached.
func definitionOwned(ctx context.Context, id string, l store.Link) error {
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
func linkedTo(r store.Reader, key string, l store.Link, specs bool) (string, bool) {
	same := func(o store.Link) bool { return o.Integration == l.Integration && o.ID == l.ID }
	if specs {
		for _, s := range r.Specs(key) {
			if same(s.Link) {
				return DisplayID(key, s.Number), true
			}
		}
		return "", false
	}
	for _, t := range r.Tickets(key) {
		if same(t.Link) {
			return DisplayID(key, t.Number), true
		}
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
		s := store.Spec{Number: w.NextNumber(key), Title: in.Title, Link: l, Created: now, Updated: now, Body: in.Body}
		if err := w.PutSpec(key, s); err != nil {
			return err
		}
		v, err = acknowledgeCreate(specView(key, store.Spec{}), specView(key, s), in.Full)
		return err
	})
	return v, err
}

// CreateLinkedTicketInput is create_linked_ticket's input. State is
// Draft (the default) or Ready, as the Integration is configured.
type CreateLinkedTicketInput struct {
	Spec        Ref         `json:"spec"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	State       store.State `json:"state,omitempty"`
	Link        LinkInput   `json:"link"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// CreateLinkedTicket is the integration's create_linked_ticket(spec,
// title, description, state, link, full?). On the Project's Issues Spec
// while New issues need me is on, Code Central marks the Ticket awaiting
// triage (Spec code-central-279).
func (c *Core) CreateLinkedTicket(ctx context.Context, key string, in CreateLinkedTicketInput) (Written[TicketView], error) {
	if err := requireActor(ctx, "create_linked_ticket", Integration); err != nil {
		return Written[TicketView]{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return Written[TicketView]{}, invalid("title", "a Ticket needs a title")
	}
	state := in.State
	if state == "" {
		state = store.Draft
	}
	if state != store.Draft && state != store.Ready {
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
		t := store.Ticket{Number: w.NextNumber(key), Title: in.Title, State: state, Spec: specN, Link: l, Created: now, Updated: now, Description: in.Description,
			AwaitingTriage: awaitsTriage(w, key, specN)}
		if err := w.PutTicket(key, t); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, store.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}

// UpdateLinkedSpecInput is update_linked_spec's input. A field left out
// is unchanged.
type UpdateLinkedSpecInput struct {
	Spec  Ref     `json:"spec"`
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
	// Full asks for the Spec in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// UpdateLinkedSpec is the integration's update_linked_spec(spec, title?,
// body?, full?) on a Spec whose Link isn't detached, recorded in its
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
// external parent changed).
type UpdateLinkedTicketInput struct {
	Ticket      Ref     `json:"ticket"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Spec        *Ref    `json:"spec,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// UpdateLinkedTicket is the integration's update_linked_ticket(ticket,
// title?, description?, spec?, full?) on a Ticket whose Link isn't
// detached, recorded in its edits.
func (c *Core) UpdateLinkedTicket(ctx context.Context, key string, in UpdateLinkedTicketInput) (Written[TicketView], error) {
	if err := requireActor(ctx, "update_linked_ticket", Integration); err != nil {
		return Written[TicketView]{}, err
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return Written[TicketView]{}, invalid("title", "a Ticket needs a title")
	}
	return c.updateTicketIn(key, "update_linked_ticket", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		if err := ownedByLink(DisplayID(key, t.Number), t.Link); err != nil {
			return false, err
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
		return c.recordEdit(ctx, &t.Edits, prev), nil
	})
}

// ownedByLink refuses an Integration's update of an item its Link no
// longer owns.
func ownedByLink(id string, l store.Link) error {
	switch {
	case !l.Linked():
		return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is not linked, so Code Central owns its definition", id)}
	case l.Detached:
		return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s was unlinked from %s %s, so Code Central owns its definition now", id, l.Integration, l.ID)}
	}
	return nil
}

// recordEdit appends an edit by ctx's Actor when prev holds a change.
func (c *Core) recordEdit(ctx context.Context, edits *[]store.Edit, prev map[string]string) bool {
	if len(prev) == 0 {
		return false
	}
	*edits = append(*edits, store.Edit{Number: len(*edits) + 1, Actor: string(ActorOf(ctx)), Time: c.now(), Previous: prev})
	return true
}

// LinkedRow is a linked Spec or Ticket as the link operations return it.
// Kind is "spec" or "ticket"; State is set for a Ticket.
type LinkedRow struct {
	Kind  string      `json:"kind"`
	ID    string      `json:"id"`
	Title string      `json:"title"`
	State store.State `json:"state,omitempty"`
	Link  LinkView    `json:"link"`
}

// SetExternalStateInput is set_external_state's input.
type SetExternalStateInput struct {
	Target        Ref                 `json:"target"`
	ExternalState store.ExternalState `json:"external_state"`
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
	return c.updateLink(ctx, key, in.Target, func(l *store.Link) (map[string]string, error) {
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
// and lookups still find it. There is no re-link.
func (c *Core) Unlink(ctx context.Context, key string, in UnlinkInput) (LinkedRow, error) {
	if err := requireActor(ctx, "unlink", Human); err != nil {
		return LinkedRow{}, err
	}
	return c.updateLink(ctx, key, in.Target, func(l *store.Link) (map[string]string, error) {
		if l.Detached {
			return nil, nil
		}
		l.Detached = true
		return map[string]string{"detached": "false"}, nil
	})
}

// updateLink applies fn to the Link of the linked Spec or Ticket ref
// names (they share one counter). fn returns the edit's previous values,
// nil when nothing changed.
func (c *Core) updateLink(ctx context.Context, key string, ref Ref, fn func(l *store.Link) (map[string]string, error)) (LinkedRow, error) {
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
			if c.recordEdit(ctx, &s.Edits, prev) {
				s.Updated = c.now()
				if err := w.PutSpec(key, s); err != nil {
					return err
				}
			}
			row = specLinkedRow(key, s)
			return nil
		}
		if t, ok := w.Ticket(key, n); ok {
			if !t.Link.Linked() {
				return notLinked
			}
			prev, err := fn(&t.Link)
			if err != nil {
				return err
			}
			if c.recordEdit(ctx, &t.Edits, prev) {
				t.Updated = c.now()
				if err := w.PutTicket(key, t); err != nil {
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

func specLinkedRow(key string, s store.Spec) LinkedRow {
	return LinkedRow{Kind: "spec", ID: DisplayID(key, s.Number), Title: s.Title, Link: *linkView(s.Link)}
}

func ticketLinkedRow(key string, t store.Ticket) LinkedRow {
	return LinkedRow{Kind: "ticket", ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Link: *linkView(t.Link)}
}

// FindLinkedInput is find_linked's filters: the Integration, and
// optionally one external ID.
type FindLinkedInput struct {
	Integration string `json:"integration"`
	ID          string `json:"id,omitempty"`
}

// FindLinked is find_linked(integration, id?): every Spec and Ticket in
// the Project linked by that Integration, detached ones too, so an
// Integration knows what it has already pulled in. Rows are in number
// order.
func (c *Core) FindLinked(_ context.Context, key string, in FindLinkedInput) ([]LinkedRow, error) {
	if strings.TrimSpace(in.Integration) == "" {
		return nil, invalid("integration", "name the Integration whose links to find")
	}
	rows := []LinkedRow{}
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		match := func(l store.Link) bool {
			return l.Integration == in.Integration && (in.ID == "" || l.ID == in.ID)
		}
		byNumber := map[int]LinkedRow{}
		for _, s := range r.Specs(key) {
			if match(s.Link) {
				byNumber[s.Number] = specLinkedRow(key, s)
			}
		}
		for _, t := range r.Tickets(key) {
			if match(t.Link) {
				byNumber[t.Number] = ticketLinkedRow(key, t)
			}
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
// item isn't open, whose Link isn't detached and that isn't finished,
// and since when: its last external state change, or its creation.
func closedExternally(t store.Ticket) (time.Time, bool) {
	if !t.Link.Owned() || t.Link.ExternalState == store.ExternalOpen || finished(t.State) {
		return time.Time{}, false
	}
	for i := len(t.Edits) - 1; i >= 0; i-- {
		if _, ok := t.Edits[i].Previous["external_state"]; ok {
			return t.Edits[i].Time, true
		}
	}
	return t.Created, true
}
