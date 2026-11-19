package api

// CIAutoFixView is what Auto-fix does about a Ticket's CI failure in one
// Repository, worked out in Go for the UI to format (ADR 0063): the auto
// fix Ticket fixing it, the one covering it, or the chain Auto-fix
// stopped at the limit. One of them is set.
type CIAutoFixView struct {
	Fixing   *AutoFixFixingView   `json:"fixing,omitempty"`
	Covering *AutoFixCoveringView `json:"covering,omitempty"`
	Stopped  *AutoFixStoppedView  `json:"stopped,omitempty"`
}

// AutoFixFixingView is the auto fix Ticket fixing the failure of a
// Ticket's latest pushed commit: attempt Attempt of the Auto-fix limit,
// Limit.
type AutoFixFixingView struct {
	FixTicket string `json:"fix_ticket"`
	Attempt   int    `json:"attempt"`
	Limit     int    `json:"limit"`
}

// AutoFixCoveringView is the open auto fix Ticket covering a Ticket whose
// later commit carries the failure it fixes, and the commit it was opened
// for.
type AutoFixCoveringView struct {
	FixTicket string `json:"fix_ticket"`
	Commit    string `json:"commit"`
}

// AutoFixStoppedView is the chain Auto-fix stopped at the limit on a CI
// failed item, which comes back to the human: "Auto-fix stopped after N
// attempts", Attempts being the N auto fix Tickets, attempt 1 first.
type AutoFixStoppedView struct {
	Attempts []string `json:"attempts"`
}
