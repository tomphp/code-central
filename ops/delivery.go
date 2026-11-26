package ops

import (
	"github.com/tomoram/code-central/api"
)

// The delivery loop: claiming, finishing and the human's review of the
// work, Blockers and Replacements, and keeping a Claim's session current.

// Claim is claim.
var Claim = &Definition[api.ClaimInput, api.ClaimResult]{
	Name: "claim", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/claim",
	Doc:  "Claim the next Ready Ticket (or the one named, or the next in a Spec); returns a Claim ID and the Ticket, or ticket null when none. Claiming a Ticket this session already holds, with its session_id, returns the same Claim.",
}

// Finish is finish.
var Finish = &Definition[api.FinishInput, api.FinishResult]{
	Name: "finish", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/finish",
	Doc:  "Finish a claimed Ticket into In Review with a summary: what was done, where (branch or commit; with several Repositories, the others' in repositories), and what to check. Returns an acknowledgement with the next step, or the Ticket with full.",
}

// Accept is the human's accept of a Ticket in review.
var Accept = &Definition[api.TicketInput, api.Written[api.TicketView]]{
	Name: "accept", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/accept",
	Doc:  "Accept a Ticket in review: Done, or Accepted when it has a branch to land.",
}

// Implement is the human's implement of a Spec Ticket recommended for it.
var Implement = &Definition[api.TicketInput, api.FinishResult]{
	Name: "implement", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/implement",
	Doc:  "Accept a spec Stage's recommendation to implement.",
}

// Reject is the human's reject of a Ticket in review.
var Reject = &Definition[api.RejectInput, api.Written[api.TicketView]]{
	Name: "reject", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/reject",
	Doc:  "Reject a Ticket in review with a comment, back to Ready.",
}

// SetSession is set_session.
var SetSession = &Definition[api.SetSessionInput, api.Written[api.TicketView]]{
	Name: "set_session", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/set_session",
	Doc:  "Record the session now working your Claim, and how to get back into it, so the human can resume it. A hook usually fills session_id in for you. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
}

// Release is the human's release of a Claim.
var Release = &Definition[api.ReleaseInput, api.Written[api.TicketView]]{
	Name: "release", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/release",
	Doc:  "Release a Claim: In Progress or Answered back to Ready.",
}

// Stop is the human's stop of a Factory Claim.
var Stop = &Definition[api.StopInput, api.Written[api.TicketView]]{
	Name: "stop", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/stop",
	Doc:  "Stop the Factory's session on an In Progress or Answered Ticket, keeping its State and Claim until resumed or released.",
}

// Resume is the human's resume of a Stopped Claim.
var Resume = &Definition[api.ResumeInput, api.Written[api.TicketView]]{
	Name: "resume", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/resume",
	Doc:  "Resume a Stopped Claim, so the Factory takes it up again.",
}

// AddBlocker is add_blocker.
var AddBlocker = &Definition[api.BlockerInput, api.Written[api.TicketView]]{
	Name: "add_blocker", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/add_blocker",
	Doc:  "Make a Ticket wait on another Ticket in this Project (any Spec) until that one is Done; a link that would form a cycle is rejected. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
}

// RemoveBlocker is remove_blocker.
var RemoveBlocker = &Definition[api.BlockerInput, api.Written[api.TicketView]]{
	Name: "remove_blocker", Audience: AgentAndHuman, Write: true,
	HTTP:         "POST /api/p/{key}/tickets/{n}/remove_blocker",
	Doc:          "Remove a Blocker from a Ticket. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
	Descriptions: map[string]string{"blocker": "The Blocker to remove."},
}

// Cancel is cancel.
var Cancel = &Definition[api.CancelInput, api.Written[api.TicketView]]{
	Name: "cancel", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/cancel",
	Doc:  "Cancel a Draft or Ready Ticket by naming the Replacements that take over its work; anything it blocked then waits on them. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
}
