package api

import "github.com/tomoram/code-central/domain"

// AutoAcceptView is a Project's Auto-accept setting: on or off for
// every Auto-accept kind.
type AutoAcceptView map[domain.AutoAcceptKind]bool

// SetAutoAcceptInput is the human's set_auto_accept(kinds, full?).
type SetAutoAcceptInput struct {
	// Kinds is on or off per Auto-accept kind; a kind left out keeps its
	// value.
	Kinds map[domain.AutoAcceptKind]bool `json:"kinds"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
