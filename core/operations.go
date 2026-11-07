package core

// Audience says which front doors may expose an operation.
type Audience int

const (
	// AgentAndHuman operations are exposed by MCP, the CLI and HTTP.
	AgentAndHuman Audience = iota
	// HumanOnly operations are exposed over HTTP only.
	HumanOnly
)

// Op describes one named operation.
type Op struct {
	Name     string
	Audience Audience
	Write    bool
}

// Operations is the one list of named operations. Every front door builds
// its routes, tools or commands from it, and the cross-door suite checks
// each door against it. Add a new operation here first.
var Operations = []Op{
	{Name: "create_project", Audience: HumanOnly, Write: true},
	{Name: "get_project", Audience: HumanOnly},
	{Name: "create_spec", Audience: AgentAndHuman, Write: true},
	{Name: "create_ticket", Audience: AgentAndHuman, Write: true},
	{Name: "get_ticket", Audience: AgentAndHuman},
	{Name: "find_tickets", Audience: AgentAndHuman},
	// Defining work.
	{Name: "edit_spec", Audience: AgentAndHuman, Write: true},
	{Name: "edit_ticket", Audience: AgentAndHuman, Write: true},
	{Name: "add_labels", Audience: AgentAndHuman, Write: true},
	{Name: "remove_labels", Audience: AgentAndHuman, Write: true},
	{Name: "set_tags", Audience: AgentAndHuman, Write: true},
	{Name: "remove_tags", Audience: AgentAndHuman, Write: true},
	{Name: "comment", Audience: AgentAndHuman, Write: true},
	{Name: "approve", Audience: AgentAndHuman, Write: true},
	{Name: "defer", Audience: AgentAndHuman, Write: true},

	// The delivery loop (ticket 25).
	{Name: "claim", Audience: AgentAndHuman, Write: true},
	{Name: "finish", Audience: AgentAndHuman, Write: true},
	{Name: "accept", Audience: HumanOnly, Write: true},
	{Name: "reject", Audience: HumanOnly, Write: true},

	// Silent Claims and sessions (ticket 31).
	{Name: "set_session", Audience: AgentAndHuman, Write: true},
	{Name: "release", Audience: HumanOnly, Write: true},
	// Blockers, Replacements and finding work (ticket 28).
	{Name: "add_blocker", Audience: AgentAndHuman, Write: true},
	{Name: "remove_blocker", Audience: AgentAndHuman, Write: true},
	{Name: "cancel", Audience: AgentAndHuman, Write: true},
	{Name: "get_spec", Audience: AgentAndHuman},
	{Name: "find_specs", Audience: AgentAndHuman},

	// Failure Reports and giving up (ticket 30).
	{Name: "report_failure", Audience: AgentAndHuman, Write: true},
	{Name: "give_up", Audience: AgentAndHuman, Write: true},
	{Name: "send_back", Audience: HumanOnly, Write: true},
	{Name: "to_draft", Audience: HumanOnly, Write: true},

	// The published workflow (ticket 26).
	{Name: "get_workflow", Audience: AgentAndHuman},
}

// Operation looks an operation up by name.
func Operation(name string) (Op, bool) {
	for _, op := range Operations {
		if op.Name == name {
			return op, true
		}
	}
	return Op{}, false
}
