package api

import "github.com/tomoram/code-central/domain"

// WaitResult is a Ticket's State when a Wait returns, with its last move.
type WaitResult struct {
	Ticket   string       `json:"ticket"`
	State    domain.State `json:"state"`
	LastMove *MoveView    `json:"last_move,omitempty"`
}
