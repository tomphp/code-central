package api

import "github.com/tomoram/code-central/domain"

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

// LinkInput is the Link an Integration creates an item with. The
// external state defaults to open.
type LinkInput struct {
	Integration   string               `json:"integration"`
	ID            string               `json:"id"`
	URL           string               `json:"url,omitempty"`
	ExternalState domain.ExternalState `json:"external_state,omitempty"`
}

// CreateLinkedSpecInput is create_linked_spec's input.
type CreateLinkedSpecInput struct {
	Title string    `json:"title"`
	Body  string    `json:"body,omitempty"`
	Link  LinkInput `json:"link"`
	// Full asks for the Spec in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
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

// UpdateLinkedSpecInput is update_linked_spec's input. A field left out
// is unchanged.
type UpdateLinkedSpecInput struct {
	Spec  Ref     `json:"spec" path:"n"`
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
	// Full asks for the Spec in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
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

// UnlinkInput is the human's unlink(target).
type UnlinkInput struct {
	Target Ref `json:"target"`
}

// FindLinkedInput is find_linked's filters: the Integration, and
// optionally one external ID.
type FindLinkedInput struct {
	Integration string `json:"integration"`
	ID          string `json:"id,omitempty"`
}
