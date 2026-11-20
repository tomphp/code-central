package api

import (
	"fmt"

	"github.com/tomoram/code-central/domain"
)

// NotLandedView is an Accepted but not landed item's Repositories, while
// the Project has several: those its Ticket has landed in, and those it
// hasn't, each with why, in landing order.
type NotLandedView struct {
	Landed    []string                  `json:"landed"`
	NotLanded []NotLandedRepositoryView `json:"not_landed"`
}

// NotLandedRepositoryView is a Changed Repository a Ticket hasn't landed
// in, and why.
type NotLandedRepositoryView struct {
	Name string `json:"name"`
	// Reason is why: its broken base, or what the Factory last said about
	// it since the Ticket was accepted; left out when there's none to
	// give.
	Reason string `json:"reason,omitempty"`
}

// CantLandLead starts the Factory's comment that it can't land a Ticket's
// work in a Repository yet.
const CantLandLead = "The Factory can't land this in "

// CantLandComment is the Factory's comment that it can't land a Ticket's
// work in the Repository name yet, and why, while the Ticket stays in
// state. Core reads why's first line back as the Repository's reason.
func CantLandComment(name, why string, state domain.State) string {
	return fmt.Sprintf("%s%s yet: %s\n\nIt stays %s and the Factory tries again on every poll.", CantLandLead, name, why, state)
}
