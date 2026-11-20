package api

import "time"

// EditView is one entry in a Spec's or Ticket's edit history. Previous
// holds the old value of each field the edit changed.
type EditView struct {
	Number   int               `json:"number"`
	Actor    string            `json:"actor"`
	Time     time.Time         `json:"time"`
	Previous map[string]string `json:"previous"`
}

// EditTicketInput is edit_ticket's input. A field left out is unchanged.
type EditTicketInput struct {
	Ticket      Ref     `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	Title       *string `json:"title,omitempty" desc:"The new title."`
	Description *string `json:"description,omitempty" desc:"The new description in Markdown, replacing the old one whole."`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// EditSpecInput is edit_spec's input. A field left out is unchanged; a
// body given replaces the old one whole.
type EditSpecInput struct {
	Spec  Ref     `json:"spec" path:"n" desc:"The Spec." required:"true"`
	Title *string `json:"title,omitempty" desc:"The new title."`
	Body  *string `json:"body,omitempty" desc:"The new body in Markdown, replacing the old one whole."`
	// Full asks for the Spec in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Spec instead of the acknowledgement."`
}

// TicketInput names one Ticket, for actions that need nothing else.
type TicketInput struct {
	Ticket Ref `json:"ticket" path:"n" desc:"The Ticket." required:"true" cli:"<id>"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// CommentInput is comment's input.
type CommentInput struct {
	Ticket Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	Body   string `json:"body" desc:"The comment in Markdown." required:"true"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// DeferInput is defer's input; the reason is required.
type DeferInput struct {
	Ticket Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	Reason string `json:"reason" desc:"Why it isn't ready." required:"true"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// RecordKind says whether a record is a Spec or a Ticket.
type RecordKind string

// The kinds of record.
const (
	RecordSpec   RecordKind = "spec"
	RecordTicket RecordKind = "ticket"
)

// RecordKinds are the kinds of record.
func RecordKinds() []RecordKind { return []RecordKind{RecordSpec, RecordTicket} }

// TargetView is a Spec's or Ticket's Labels and Tags, as the Label and
// Tag operations return them.
type TargetView struct {
	ID     string            `json:"id"`
	Kind   RecordKind        `json:"kind"`
	Labels []string          `json:"labels"`
	Tags   map[string]string `json:"tags"`
}

// LabelsInput is add_labels' and remove_labels' input. Each Label is
// added or removed by name; the rest of the list is left alone, so two
// writers don't overwrite each other.
type LabelsInput struct {
	Target Ref `json:"target" desc:"The Spec or Ticket." required:"true" cli:"<id>"`
	// Labels to add or remove; the operation's definition words it.
	Labels []string `json:"labels" required:"true" cli:"<label>..."`
}

// SetTagsInput is set_tags' input.
type SetTagsInput struct {
	Target Ref               `json:"target" desc:"The Spec or Ticket." required:"true" cli:"<id>"`
	Tags   map[string]string `json:"tags" desc:"Tags to set, as {key: value}." required:"true" cli:"<key>=<value>..."`
}

// RemoveTagsInput is remove_tags' input.
type RemoveTagsInput struct {
	Target Ref      `json:"target" desc:"The Spec or Ticket." required:"true" cli:"<id>"`
	Keys   []string `json:"keys" desc:"Tag keys to remove." required:"true" cli:"<key>..."`
}

// CommentView is one comment on a Ticket.
type CommentView struct {
	Number int       `json:"number"`
	Actor  string    `json:"actor"`
	Time   time.Time `json:"time"`
	Claim  int       `json:"claim,omitempty"` // the Claim a required note was made under
	Body   string    `json:"body"`
}
