package api

// CIItemView is a CI item as a Needs Me row and each listed Ticket's
// view carry it: its reason, the pushed commit, the Tickets waiting on
// it, accepted earliest first, and the failure options the human may
// choose, each an operation's name, which act on every one of them;
// none while CI unavailable.
type CIItemView struct {
	Reason NeedsMeReason `json:"reason"`
	// Repository is the Repository Name the commit was pushed in, left
	// out while the Project has the Home Repository alone. Each option
	// acts on that Repository only.
	Repository string            `json:"repository,omitempty"`
	Commit     string            `json:"commit"`
	Tickets    []CIItemTicket    `json:"tickets"`
	Options    []CIFailureOption `json:"options"`
	// NewFixNote is the note New fix Ticket starts from, the Run summary
	// and the Project's Fix instructions; SendBackNote is the note Send
	// back to fix starts from, the Run summary alone. Each is left out
	// while its option isn't offered, and the human edits either before
	// choosing it.
	NewFixNote   string `json:"new_fix_note,omitempty"`
	SendBackNote string `json:"send_back_note,omitempty"`
	// AutoFixStopped is the "Auto-fix stopped after N attempts" line
	// while Auto-fix has stopped at the limit on the item, which waits on
	// the human again; left out otherwise.
	AutoFixStopped *AutoFixStoppedView `json:"auto_fix_stopped,omitempty"`
}

// CIItemTicket is one Ticket a CI item lists, with its Spec.
type CIItemTicket struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Spec  string `json:"spec"`
}
