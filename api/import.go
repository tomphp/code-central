package api

import "github.com/tomoram/code-central/domain"

// ImportInput is import's input.
type ImportInput struct {
	Payload ImportPayload `json:"payload" required:"true"`
	// DryRun runs every check and returns the plan, writing nothing.
	DryRun bool `json:"dry_run,omitempty" desc:"Check everything and return the plan, writing nothing."`
}

// ImportPayload is a whole .scratch/ folder: one Spec per feature folder,
// and every Ticket in it. Items refer to each other by source path.
type ImportPayload struct {
	Specs   []ImportSpec   `json:"specs" desc:"One Spec per feature folder." required:"true"`
	Tickets []ImportTicket `json:"tickets" desc:"Every Ticket, in folder then file-number order."`
}

// ImportSpec is one feature folder. Source becomes the Spec's source Tag.
type ImportSpec struct {
	Source string            `json:"source" desc:"The spec.md, map.md or folder path from the repository root." required:"true"`
	Title  string            `json:"title" desc:"The Spec's title." required:"true"`
	Body   string            `json:"body,omitempty" desc:"The Spec's body, in Markdown."`
	Labels []string          `json:"labels,omitempty" desc:"Labels."`
	Tags   map[string]string `json:"tags,omitempty" desc:"Tags as {key: value}; source is set from source."`
}

// ImportTicket is one Scratch file. Spec is its Spec's source; BlockedBy
// and Replacements are other Tickets' sources in the same payload. State
// defaults to Draft; Comment, when given, is stored as one comment.
type ImportTicket struct {
	Source       string            `json:"source" desc:"The file's path from the repository root." required:"true"`
	Spec         string            `json:"spec" desc:"Its Spec's source." required:"true"`
	Title        string            `json:"title" desc:"The Ticket's title." required:"true"`
	State        domain.State      `json:"state,omitempty" desc:"Defaults to Draft." enum:"Draft,Ready,Done,Cancelled"`
	Description  string            `json:"description,omitempty" desc:"The description, in Markdown, without the header lines that became State, Blockers, Labels or Tags."`
	Labels       []string          `json:"labels,omitempty" desc:"Labels."`
	Tags         map[string]string `json:"tags,omitempty" desc:"Tags as {key: value}; source is set from source."`
	BlockedBy    []string          `json:"blocked_by,omitempty" desc:"Sources of the Tickets it waits on, in this payload."`
	Replacements []string          `json:"replacements,omitempty" desc:"For a Cancelled Ticket, sources of the Tickets that take over its work."`
	Comment      string            `json:"comment,omitempty" desc:"A reason to keep as a comment (wontfix, Deferred)."`
}

// ImportResult is the plan (on a dry run, with provisional IDs) or what
// was created, grouped by Spec, with a count per State.
type ImportResult struct {
	DryRun      bool                 `json:"dry_run"`
	Specs       []ImportedSpec       `json:"specs"`
	StateCounts map[domain.State]int `json:"state_counts"`
	NextStep    string               `json:"next_step"`
}

// ImportedSpec is one Spec of an Import's plan or result.
type ImportedSpec struct {
	Source  string            `json:"source"`
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Labels  []string          `json:"labels"`
	Tags    map[string]string `json:"tags"`
	Tickets []ImportedTicket  `json:"tickets"`
}

// ImportedTicket is one Ticket of an Import's plan or result.
type ImportedTicket struct {
	Source       string            `json:"source"`
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	State        domain.State      `json:"state"`
	Labels       []string          `json:"labels"`
	Tags         map[string]string `json:"tags"`
	BlockedBy    []string          `json:"blocked_by"`
	Replacements []string          `json:"replacements"`
}

// ImportGuideView is get_import_guide's result.
type ImportGuideView struct {
	Guide string `json:"guide"`
}
