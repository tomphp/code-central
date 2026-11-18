// Package ops defines each of Code Central's operations once (ADR 0063).
// It sits between core and the front doors: it imports core and nothing
// from a transport, and httpapi, mcp, client and cli import it. A
// Definition carries an operation's name, Audience, write flag, HTTP route,
// the Core call that serves it, its one-line documentation and the
// behaviour that varies by door; each door derives its route, tool, schema,
// client call or command from it, so adding an operation means one
// definition, one Core method and one cross-door case.
//
// All lists every operation. While the migration runs (Spec code-central-928),
// an operation not yet converted is listed by its Op alone and the doors'
// legacy maps still serve it; the migration guard in server checks that each
// operation is defined exactly one way.
package ops

import (
	"context"
	"reflect"
	"strings"

	"github.com/tomoram/code-central/core"
)

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

// Op is what every door knows of an operation: its name, its Audience and
// whether it writes.
type Op struct {
	Name     string
	Audience Audience
	Write    bool
}

// Definition defines one operation over its input and result types. Core's
// input type is the single source of each field's wire shape and its
// documentation, through struct tags:
//
//	desc:"…"        the MCP schema description of the field
//	required:"true" the MCP schema lists the field in required, in field order
//	enum:"A,B"      an explicit enum in the MCP schema
//	minItems:"n"    a list's minItems in the MCP schema
//	path:"n"        the field the HTTP path's {n} binds to; the path wins
//	query:"name"    a GET's query parameter where it differs from the JSON name
//	mcp:"-"         left out of the MCP schema
//	cli:"…"         the CLI flag name where it differs, and the positional argument
//
// A json:"-" field is never on any door, except through a query tag, which
// names a GET parameter the JSON body never carries.
type Definition[In, Out any] struct {
	Name     string
	Audience Audience
	Write    bool
	// HTTP is the method and path pattern in net/http ServeMux syntax, as
	// "POST /api/p/{key}/tickets/{n}/comment". {key} is the Project key.
	HTTP string
	// Doc is the one-line documentation: the MCP tool description and the
	// CLI help text.
	Doc string
	// Descriptions overrides a field's desc tag by JSON name, for an input
	// type shared by operations whose wording differs.
	Descriptions map[string]string
	// Call serves the operation: usually the Core method as a method
	// expression, such as (*core.Core).CreateSpec.
	Call func(c *core.Core, ctx context.Context, key string, in In) (Out, error)
	// HTTPDefaults, when set, is applied to the input the HTTP door decoded,
	// after the path is bound and before the call: the HTTP API's defaults
	// where they differ from the agent doors'.
	HTTPDefaults func(in *In)
}

// Operation is a Definition with its types erased, as Definitions lists
// them for the doors.
type Operation interface {
	Op() Op
	// Route is the HTTP method and path pattern.
	Route() (method, pattern string)
	// Description is the one-line documentation.
	Description() string
	// Input is the input type, In.
	Input() reflect.Type
	// NewInput is a new zero input, as *In, for the doors to decode into.
	NewInput() any
	// FieldDescription is the field's description override, by JSON name.
	FieldDescription(name string) (string, bool)
	// ApplyHTTPDefaults applies the HTTP defaults to in, a *In.
	ApplyHTTPDefaults(in any)
	// Invoke calls the operation with in, a *In.
	Invoke(ctx context.Context, c *core.Core, key string, in any) (any, error)
}

func (d *Definition[In, Out]) Op() Op { return Op{Name: d.Name, Audience: d.Audience, Write: d.Write} }

func (d *Definition[In, Out]) Route() (method, pattern string) {
	method, pattern, _ = strings.Cut(d.HTTP, " ")
	return method, pattern
}

func (d *Definition[In, Out]) Description() string { return d.Doc }

func (d *Definition[In, Out]) Input() reflect.Type { return reflect.TypeFor[In]() }

func (d *Definition[In, Out]) NewInput() any { return new(In) }

func (d *Definition[In, Out]) FieldDescription(name string) (string, bool) {
	desc, ok := d.Descriptions[name]
	return desc, ok
}

func (d *Definition[In, Out]) ApplyHTTPDefaults(in any) {
	if d.HTTPDefaults != nil {
		d.HTTPDefaults(in.(*In))
	}
}

func (d *Definition[In, Out]) Invoke(ctx context.Context, c *core.Core, key string, in any) (any, error) {
	return d.Call(c, ctx, key, *in.(*In))
}

// entry is one row of the list: a converted operation by its definition,
// an unconverted one by its Op alone.
type entry struct {
	op  Op
	def Operation
}

func legacy(op Op) entry        { return entry{op: op} }
func defined(d Operation) entry { return entry{op: d.Op(), def: d} }

// All is the one list of operations, in order. Every front door builds its
// routes, tools or commands from it, and the cross-door suite checks each
// door against it.
func All() []Op {
	out := make([]Op, len(all))
	for i, e := range all {
		out[i] = e.op
	}
	return out
}

// Definitions are the operations with a Definition, in All's order.
func Definitions() []Operation {
	var out []Operation
	for _, e := range all {
		if e.def != nil {
			out = append(out, e.def)
		}
	}
	return out
}

// Lookup finds an operation by name.
func Lookup(name string) (Op, bool) {
	for _, e := range all {
		if e.op.Name == name {
			return e.op, true
		}
	}
	return Op{}, false
}

// DefinitionOf is the named operation's Definition, when it has one.
func DefinitionOf(name string) (Operation, bool) {
	for _, e := range all {
		if e.op.Name == name && e.def != nil {
			return e.def, true
		}
	}
	return nil, false
}

// all is the list. Add a new operation here first, by its definition.
var all = []entry{
	legacy(Op{Name: "create_project", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "get_project", Audience: HumanOnly}),
	defined(CreateSpec),
	defined(CreateTicket),
	defined(GetTicket),
	defined(FindTickets),
	// Defining work.
	defined(EditSpec),
	defined(EditTicket),
	defined(AddLabels),
	defined(RemoveLabels),
	defined(SetTags),
	defined(RemoveTags),
	defined(Comment),
	defined(Approve),
	defined(Defer),

	// The delivery loop (ticket 25).
	legacy(Op{Name: "claim", Audience: AgentAndHuman, Write: true}),
	legacy(Op{Name: "finish", Audience: AgentAndHuman, Write: true}),
	legacy(Op{Name: "accept", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "implement", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "reject", Audience: HumanOnly, Write: true}),

	// Silent Claims and sessions (ticket 31).
	legacy(Op{Name: "set_session", Audience: AgentAndHuman, Write: true}),
	legacy(Op{Name: "release", Audience: HumanOnly, Write: true}),
	// Blockers, Replacements and finding work (ticket 28).
	legacy(Op{Name: "add_blocker", Audience: AgentAndHuman, Write: true}),
	legacy(Op{Name: "remove_blocker", Audience: AgentAndHuman, Write: true}),
	legacy(Op{Name: "cancel", Audience: AgentAndHuman, Write: true}),
	defined(GetSpec),
	defined(FindSpecs),

	// Failure Reports and giving up (ticket 30).
	defined(ReportFailure),
	defined(GiveUp),
	// The agent sends Accepted work back on a conflict (ticket 72).
	defined(SendBack),
	defined(ToDraft),

	// init's teaching files and modes (ticket 41).
	legacy(Op{Name: "set_project_mode", Audience: HumanOnly, Write: true}),

	// The published workflow (ticket 26).
	legacy(Op{Name: "get_workflow", Audience: AgentAndHuman}),

	// Rounds and Questions (ticket 29).
	defined(PostRound),
	defined(PickUpAnswers),
	defined(Answer),
	defined(AcceptRecommended),
	defined(DismissQuestion),
	// The roll-up of a Spec's Questions is HTTP-only, for the UI.
	defined(GetSpecQuestions),

	// Needs Me, moving by hand and Projects (ticket 32).
	legacy(Op{Name: "needs_me", Audience: HumanOnly}),
	legacy(Op{Name: "list_projects", Audience: HumanOnly}),
	legacy(Op{Name: "move", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "rename_project", Audience: HumanOnly, Write: true}),
	// The Landing Method (code-central-173).
	legacy(Op{Name: "set_landing_method", Audience: HumanOnly, Write: true}),
	// Subagent Exploration (code-central-441).
	legacy(Op{Name: "set_subagent_exploration", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "set_model_assignment", Audience: HumanOnly, Write: true}),
	// Auto-accept (code-central-536): the human's alone, over HTTP only.
	legacy(Op{Name: "set_auto_accept", Audience: HumanOnly, Write: true}),
	// Focus Kinds (Spec code-central-550).
	legacy(Op{Name: "set_focus_kinds", Audience: HumanOnly, Write: true}),
	// Repositories (Spec code-central-848): the human's alone, over HTTP
	// and the CLI's repository command, never MCP.
	legacy(Op{Name: "add_repository", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "edit_repository", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "remove_repository", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "set_landing_order", Audience: HumanOnly, Write: true}),

	// The Integration contract (ticket 42).
	legacy(Op{Name: "create_linked_spec", Audience: IntegrationOnly, Write: true}),
	legacy(Op{Name: "create_linked_ticket", Audience: IntegrationOnly, Write: true}),
	legacy(Op{Name: "update_linked_spec", Audience: IntegrationOnly, Write: true}),
	legacy(Op{Name: "update_linked_ticket", Audience: IntegrationOnly, Write: true}),
	legacy(Op{Name: "set_external_state", Audience: IntegrationOnly, Write: true}),
	legacy(Op{Name: "find_linked", Audience: IntegrationOnly}),
	legacy(Op{Name: "unlink", Audience: HumanOnly, Write: true}),

	// Scratch Import (ticket 40).
	legacy(Op{Name: "get_import_guide", Audience: AgentAndHuman}),
	legacy(Op{Name: "import", Audience: AgentAndHuman, Write: true}),

	// Finished work and Archived Projects (ticket 39).
	legacy(Op{Name: "archive_project", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "unarchive_project", Audience: HumanOnly, Write: true}),

	// Requirements and the local Factory (Spec 66).
	legacy(Op{Name: "submit_requirement", Audience: AgentAndHuman, Write: true}),
	legacy(Op{Name: "land", Audience: AgentAndHuman, Write: true}),
	legacy(Op{Name: "get_stage_guide", Audience: AgentAndHuman}),

	// Maps and Prototypes (Spec code-central-213).
	legacy(Op{Name: "submit_map", Audience: AgentAndHuman, Write: true}),
	legacy(Op{Name: "submit_prototype", Audience: AgentAndHuman, Write: true}),
	legacy(Op{Name: "submit_small_change", Audience: AgentAndHuman, Write: true}),
	// Retros (Spec code-central-646).
	legacy(Op{Name: "submit_retro", Audience: AgentAndHuman, Write: true}),

	// Options, the human's global preferences (Spec code-central-160).
	// HTTP only: no MCP tool and no CLI command.
	legacy(Op{Name: "get_options", Audience: HumanOnly}),
	legacy(Op{Name: "set_options", Audience: HumanOnly, Write: true}),

	// CI Tracking (Spec code-central-247).
	legacy(Op{Name: "set_ci_tracking", Audience: HumanOnly, Write: true}),
	// A Repository's CI entry (Spec code-central-848).
	legacy(Op{Name: "set_repository_ci", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "report_push_remote", Audience: FactoryOnly, Write: true}),
	legacy(Op{Name: "add_ci_rule", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "edit_ci_rule", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "remove_ci_rule", Audience: HumanOnly, Write: true}),
	// The CI Watcher's own (code-central-253).
	legacy(Op{Name: "get_ci_watch", Audience: FactoryOnly}),
	legacy(Op{Name: "report_ci_watcher_status", Audience: FactoryOnly, Write: true}),
	// Tracking a pushed Ticket's CI Runs (code-central-256).
	legacy(Op{Name: "report_pushed_commit", Audience: FactoryOnly, Write: true}),
	// The Pull Request Watcher's own (code-central-388).
	legacy(Op{Name: "get_pull_request_watch", Audience: FactoryOnly}),
	legacy(Op{Name: "report_pull_request", Audience: FactoryOnly, Write: true}),
	legacy(Op{Name: "report_pull_request_watcher_status", Audience: FactoryOnly, Write: true}),
	legacy(Op{Name: "report_ci_runs", Audience: FactoryOnly, Write: true}),
	// Land per Spec (code-central-371).
	legacy(Op{Name: "report_spec_merge", Audience: FactoryOnly, Write: true}),
	// Spec merge Tickets (code-central-542).
	legacy(Op{Name: "open_spec_merge", Audience: FactoryOnly, Write: true}),
	// CI failed and its failure options (code-central-261).
	legacy(Op{Name: "land_anyway", Audience: HumanOnly, Write: true}),
	// Re-run failed jobs (code-central-265).
	legacy(Op{Name: "rerun_ci", Audience: HumanOnly, Write: true}),
	// Send back to fix and New fix Ticket (code-central-264).
	legacy(Op{Name: "new_fix_ticket", Audience: HumanOnly, Write: true}),

	// GitHub Issues (Spec code-central-279).
	legacy(Op{Name: "set_github_issues", Audience: HumanOnly, Write: true}),
	// The Issue Watcher's own (code-central-285).
	legacy(Op{Name: "get_issue_watch", Audience: IntegrationOnly}),
	legacy(Op{Name: "report_issue_watcher_status", Audience: IntegrationOnly, Write: true}),
	// New issue and the triage actions (code-central-287).
	legacy(Op{Name: "keep_as_draft", Audience: HumanOnly, Write: true}),
	// Write-backs and Won't fix (code-central-289).
	legacy(Op{Name: "report_issue_written", Audience: IntegrationOnly, Write: true}),
	legacy(Op{Name: "wont_fix", Audience: HumanOnly, Write: true}),
	// To Requirement and the outbound Link (code-central-290).
	legacy(Op{Name: "to_requirement", Audience: HumanOnly, Write: true}),
	// Azure Boards (Spec code-central-894).
	legacy(Op{Name: "set_azure_boards", Audience: HumanOnly, Write: true}),
	// The Work Item Watcher's own; it reports each comment written with
	// report_issue_written.
	legacy(Op{Name: "get_work_item_watch", Audience: IntegrationOnly}),
	legacy(Op{Name: "report_work_item_watcher_status", Audience: IntegrationOnly, Write: true}),
	// The Model Choice (Spec code-central-301).
	legacy(Op{Name: "get_model_choice", Audience: FactoryOnly}),
	legacy(Op{Name: "report_model_choice", Audience: FactoryOnly, Write: true}),
	// The Models section of Project Options (Spec code-central-301).
	legacy(Op{Name: "set_models", Audience: HumanOnly, Write: true}),
	// The Reported Models (code-central-311).
	legacy(Op{Name: "report_models", Audience: FactoryOnly, Write: true}),
	legacy(Op{Name: "report_heartbeat", Audience: FactoryOnly, Write: true}),
	// The Factory's base under Pull Request (code-central-394).
	legacy(Op{Name: "report_base", Audience: FactoryOnly, Write: true}),
	// The Factory's clones of the Repositories (Spec code-central-848).
	legacy(Op{Name: "report_repositories", Audience: FactoryOnly, Write: true}),
	// A Ticket's Changed Repositories, live, and the Factory's finish
	// check (Spec code-central-848).
	legacy(Op{Name: "report_changed_repositories", Audience: FactoryOnly, Write: true}),
	legacy(Op{Name: "fail_finish", Audience: FactoryOnly, Write: true}),

	// Chat (Spec code-central-633): the human's Conversation and the
	// Factory's long-poll for its turns. Chat writes change only memory.
	legacy(Op{Name: "get_chat", Audience: HumanOnly}),
	legacy(Op{Name: "start_conversation", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "send_chat_message", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "stop_chat_reply", Audience: HumanOnly, Write: true}),
	legacy(Op{Name: "next_chat_work", Audience: FactoryOnly}),

	// The Factory's wake (Spec code-central-743): a long-poll that ends
	// when a write its poll acts on reaches the Project.
	legacy(Op{Name: "next_factory_wake", Audience: FactoryOnly}),
}
