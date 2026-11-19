package api

import "github.com/tomoram/code-central/domain"

// MoveInput is the human's move(ticket, to_state).
type MoveInput struct {
	Ticket  Ref          `json:"ticket" path:"n"`
	ToState domain.State `json:"to_state"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// RenameProjectInput is the human's rename_project(display_name).
type RenameProjectInput struct {
	DisplayName string `json:"display_name"`
}

// SetLandingMethodInput is the human's set_landing_method(method, push,
// remote, land_per_spec, rebase).
type SetLandingMethodInput struct {
	Method domain.LandingMethod `json:"method"`
	Push   bool                 `json:"push"`
	Remote string               `json:"remote"`
	// LandPerSpec, when left out, keeps the stored value.
	LandPerSpec *bool `json:"land_per_spec,omitempty"`
	// Rebase, when left out, keeps the stored value.
	Rebase *bool `json:"rebase,omitempty"`
	// PATEnv names the environment variable holding the Azure Repos PAT
	// under Pull Request; left out, it keeps the stored name.
	PATEnv string `json:"pat_env,omitempty"`
	// Base is the branch work is cut from and lands on; left out, it keeps
	// the stored one. Only its name is checked, never whether it exists.
	Base *string `json:"base,omitempty"`
}

// ProjectRow is one list_projects row: the Project and how many of its
// Tickets need the human.
type ProjectRow struct {
	ProjectView
	NeedsMe int `json:"needs_me"`
	// Unfinished counts the Tickets not yet Done or Cancelled; archiving
	// waits for it to reach zero.
	Unfinished int `json:"unfinished"`
}
