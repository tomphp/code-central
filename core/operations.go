package core

// Audience says which front doors may expose an operation.
type Audience int

const (
	// AgentAndHuman operations are exposed by MCP, the CLI and HTTP.
	AgentAndHuman Audience = iota
	// HumanOnly operations are exposed over HTTP only.
	HumanOnly
	// IntegrationOnly operations are an Integration's, over HTTP only.
	IntegrationOnly
	// FactoryOnly operations are the Factory's and the CI Watcher's own,
	// over HTTP only: agents in a session get no MCP tool or CLI command
	// for them.
	FactoryOnly
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
	{Name: "implement", Audience: HumanOnly, Write: true},
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
	// The agent sends Accepted work back on a conflict (ticket 72).
	{Name: "send_back", Audience: AgentAndHuman, Write: true},
	{Name: "to_draft", Audience: HumanOnly, Write: true},

	// init's teaching files and modes (ticket 41).
	{Name: "set_project_mode", Audience: HumanOnly, Write: true},

	// The published workflow (ticket 26).
	{Name: "get_workflow", Audience: AgentAndHuman},

	// Rounds and Questions (ticket 29).
	{Name: "post_round", Audience: AgentAndHuman, Write: true},
	{Name: "pick_up_answers", Audience: AgentAndHuman, Write: true},
	{Name: "answer", Audience: HumanOnly, Write: true},
	{Name: "accept_recommended", Audience: HumanOnly, Write: true},
	{Name: "dismiss_question", Audience: HumanOnly, Write: true},
	// The roll-up of a Spec's Questions is HTTP-only, for the UI.
	{Name: "get_spec_questions", Audience: HumanOnly},

	// Needs Me, moving by hand and Projects (ticket 32).
	{Name: "needs_me", Audience: HumanOnly},
	{Name: "list_projects", Audience: HumanOnly},
	{Name: "move", Audience: HumanOnly, Write: true},
	{Name: "rename_project", Audience: HumanOnly, Write: true},
	// The Landing Method (code-central-173).
	{Name: "set_landing_method", Audience: HumanOnly, Write: true},

	// The Integration contract (ticket 42).
	{Name: "create_linked_spec", Audience: IntegrationOnly, Write: true},
	{Name: "create_linked_ticket", Audience: IntegrationOnly, Write: true},
	{Name: "update_linked_spec", Audience: IntegrationOnly, Write: true},
	{Name: "update_linked_ticket", Audience: IntegrationOnly, Write: true},
	{Name: "set_external_state", Audience: IntegrationOnly, Write: true},
	{Name: "find_linked", Audience: IntegrationOnly},
	{Name: "unlink", Audience: HumanOnly, Write: true},

	// Scratch Import (ticket 40).
	{Name: "get_import_guide", Audience: AgentAndHuman},
	{Name: "import", Audience: AgentAndHuman, Write: true},

	// Finished work and Archived Projects (ticket 39).
	{Name: "archive_project", Audience: HumanOnly, Write: true},
	{Name: "unarchive_project", Audience: HumanOnly, Write: true},

	// Requirements and the local Factory (Spec 66).
	{Name: "submit_requirement", Audience: AgentAndHuman, Write: true},
	{Name: "land", Audience: AgentAndHuman, Write: true},
	{Name: "get_stage_guide", Audience: AgentAndHuman},

	// Maps and Prototypes (Spec code-central-213).
	{Name: "submit_map", Audience: AgentAndHuman, Write: true},
	{Name: "submit_prototype", Audience: AgentAndHuman, Write: true},

	// Options, the human's global preferences (Spec code-central-160).
	// HTTP only: no MCP tool and no CLI command.
	{Name: "get_options", Audience: HumanOnly},
	{Name: "set_options", Audience: HumanOnly, Write: true},

	// CI Tracking (Spec code-central-247).
	{Name: "set_ci_tracking", Audience: HumanOnly, Write: true},
	{Name: "report_push_remote", Audience: FactoryOnly, Write: true},
	{Name: "add_ci_rule", Audience: HumanOnly, Write: true},
	{Name: "edit_ci_rule", Audience: HumanOnly, Write: true},
	{Name: "remove_ci_rule", Audience: HumanOnly, Write: true},
	// The CI Watcher's own (code-central-253).
	{Name: "get_ci_watch", Audience: FactoryOnly},
	{Name: "report_ci_watcher_status", Audience: FactoryOnly, Write: true},
	// Tracking a pushed Ticket's CI Runs (code-central-256).
	{Name: "report_pushed_commit", Audience: FactoryOnly, Write: true},
	{Name: "report_ci_runs", Audience: FactoryOnly, Write: true},
	// Land per Spec (code-central-371).
	{Name: "report_spec_merge", Audience: FactoryOnly, Write: true},
	// CI failed and its failure options (code-central-261).
	{Name: "land_anyway", Audience: HumanOnly, Write: true},
	// Re-run failed jobs (code-central-265).
	{Name: "rerun_ci", Audience: HumanOnly, Write: true},
	// Send back to fix and New fix Ticket (code-central-264).
	{Name: "new_fix_ticket", Audience: HumanOnly, Write: true},

	// GitHub Issues (Spec code-central-279).
	{Name: "set_github_issues", Audience: HumanOnly, Write: true},
	// The Issue Watcher's own (code-central-285).
	{Name: "get_issue_watch", Audience: IntegrationOnly},
	{Name: "report_issue_watcher_status", Audience: IntegrationOnly, Write: true},
	// New issue and the triage actions (code-central-287).
	{Name: "keep_as_draft", Audience: HumanOnly, Write: true},
	// Write-backs and Won't fix (code-central-289).
	{Name: "report_issue_written", Audience: IntegrationOnly, Write: true},
	{Name: "wont_fix", Audience: HumanOnly, Write: true},
	// To Requirement and the outbound Link (code-central-290).
	{Name: "to_requirement", Audience: HumanOnly, Write: true},
	// The Model Choice (Spec code-central-301).
	{Name: "get_model_choice", Audience: FactoryOnly},
	{Name: "report_model_choice", Audience: FactoryOnly, Write: true},
	// The Models section of Project Options (Spec code-central-301).
	{Name: "set_models", Audience: HumanOnly, Write: true},
	// The Reported Models (code-central-311).
	{Name: "report_models", Audience: FactoryOnly, Write: true},
	{Name: "report_heartbeat", Audience: FactoryOnly, Write: true},
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
