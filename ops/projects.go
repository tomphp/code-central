package ops

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
)

// Projects, the published workflow, Needs Me, moving by hand and Options.

// CreateProject is the human's create_project. It is on no Project, so
// its path has no {key}.
var CreateProject = &Definition[api.CreateProjectInput, api.ProjectView]{
	Name: "create_project", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/projects/create_project",
	Doc:  "Create a Project.",
	Call: func(c *core.Core, ctx context.Context, _ string, in api.CreateProjectInput) (api.ProjectView, error) {
		return c.CreateProject(ctx, in)
	},
}

// GetProject is get_project.
var GetProject = &Definition[struct{}, api.ProjectView]{
	Name: "get_project", Audience: HumanOnly,
	HTTP: "GET /api/projects/{key}",
	Doc:  "Get a Project.",
	Call: func(c *core.Core, ctx context.Context, key string, _ struct{}) (api.ProjectView, error) {
		return c.GetProject(ctx, key)
	},
}

// SetProjectMode is set_project_mode (ticket 41).
var SetProjectMode = &Definition[api.SetProjectModeInput, api.ProjectView]{
	Name: "set_project_mode", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/projects/{key}/set_project_mode",
	Doc:  "Set the Project's mode, which decides the teaching files init writes.",
	Call: (*core.Core).SetProjectMode,
}

// GetWorkflow is get_workflow (ticket 26). The workflow is the same for
// every Project, so its path has no {key}; the MCP door checks the Project
// itself, as it does for every operation without one.
var GetWorkflow = &Definition[struct{}, api.WorkflowView]{
	Name: "get_workflow", Audience: AgentAndHuman,
	HTTP: "GET /api/workflow",
	Doc:  "Get the whole workflow (every State, action, who may take it, what it requires) and how to work a Ticket; call it before working one.",
	Call: func(c *core.Core, ctx context.Context, _ string, _ struct{}) (api.WorkflowView, error) {
		return c.GetWorkflow(ctx)
	},
}

// NeedsMe is needs_me: what waits on the human across every Project.
var NeedsMe = &Definition[struct{}, api.NeedsMeView]{
	Name: "needs_me", Audience: HumanOnly,
	HTTP: "GET /api/needs-me",
	Doc:  "Get what waits on the human across every Project.",
	Call: func(c *core.Core, ctx context.Context, _ string, _ struct{}) (api.NeedsMeView, error) {
		return c.NeedsMe(ctx)
	},
}

// ListProjects is list_projects.
var ListProjects = &Definition[struct{}, []api.ProjectRow]{
	Name: "list_projects", Audience: HumanOnly,
	HTTP: "GET /api/projects",
	Doc:  "List the Projects.",
	Call: func(c *core.Core, ctx context.Context, _ string, _ struct{}) ([]api.ProjectRow, error) {
		return c.ListProjects(ctx)
	},
}

// Move is the human's move(to_state).
var Move = &Definition[api.MoveInput, api.Written[api.TicketView]]{
	Name: "move", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/move",
	Doc:  "Move a Ticket to any State but its own, Needs Answer and Failed, as the human.",
	Call: (*core.Core).Move,
}

// RenameProject is the human's rename_project.
var RenameProject = &Definition[api.RenameProjectInput, api.ProjectView]{
	Name: "rename_project", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/rename_project",
	Doc:  "Rename a Project.",
	Call: (*core.Core).RenameProject,
}

// ArchiveProject is the human's archive_project (ticket 39). It takes an
// empty JSON body.
var ArchiveProject = &Definition[struct{}, api.ProjectView]{
	Name: "archive_project", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/archive_project",
	Doc:  "Archive a Project, which then refuses every write.",
	Call: func(c *core.Core, ctx context.Context, key string, _ struct{}) (api.ProjectView, error) {
		return c.ArchiveProject(ctx, key)
	},
}

// UnarchiveProject is the human's unarchive_project.
var UnarchiveProject = &Definition[struct{}, api.ProjectView]{
	Name: "unarchive_project", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/unarchive_project",
	Doc:  "Unarchive a Project.",
	Call: func(c *core.Core, ctx context.Context, key string, _ struct{}) (api.ProjectView, error) {
		return c.UnarchiveProject(ctx, key)
	},
}

// SetSubagentExploration is set_subagent_exploration (code-central-441).
var SetSubagentExploration = &Definition[api.SetSubagentExplorationInput, api.Written[api.ProjectView]]{
	Name: "set_subagent_exploration", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_subagent_exploration",
	Doc:  "Set whether a Stage Guide tells agents to explore with subagents.",
	Call: (*core.Core).SetSubagentExploration,
}

// SetModelAssignment is set_model_assignment.
var SetModelAssignment = &Definition[api.SetModelAssignmentInput, api.Written[api.ProjectView]]{
	Name: "set_model_assignment", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_model_assignment",
	Doc:  "Set which model works each kind of Ticket.",
	Call: (*core.Core).SetModelAssignment,
}

// SetAutoAccept is set_auto_accept (code-central-536): the Project's own
// setting and the human's alone.
var SetAutoAccept = &Definition[api.SetAutoAcceptInput, api.Written[api.ProjectView]]{
	Name: "set_auto_accept", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_auto_accept",
	Doc:  "Set whether the Project accepts finished work by itself.",
	Call: (*core.Core).SetAutoAccept,
}

// SetFocusKinds is set_focus_kinds (Spec code-central-550): the human's
// choice of which Needs Me reasons are Focus Items.
var SetFocusKinds = &Definition[api.SetFocusKindsInput, api.Written[api.ProjectView]]{
	Name: "set_focus_kinds", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_focus_kinds",
	Doc:  "Set which Needs Me reasons are Focus Items for the Project.",
	Call: (*core.Core).SetFocusKinds,
}

// GetOptions is get_options (Spec code-central-160): the human's global
// preferences, on no Project.
var GetOptions = &Definition[struct{}, api.OptionsView]{
	Name: "get_options", Audience: HumanOnly,
	HTTP: "GET /api/options",
	Doc:  "Get the Options.",
	Call: func(c *core.Core, ctx context.Context, _ string, _ struct{}) (api.OptionsView, error) {
		return c.GetOptions(ctx)
	},
}

// GetStorage is get_storage (Spec code-central-1032): the Storage tab's
// figures, on no Project; its project query narrows the counts to one.
var GetStorage = &Definition[api.GetStorageInput, api.StorageView]{
	Name: "get_storage", Audience: HumanOnly,
	HTTP: "GET /api/storage",
	Doc:  "Get where the Store is and its size, each Project's count of Specs, Tickets and Usage days, the process's memory and the services registered on the Store.",
	Call: func(c *core.Core, ctx context.Context, _ string, in api.GetStorageInput) (api.StorageView, error) {
		return c.GetStorage(ctx, in)
	},
}

// SetOptions is the human's set_options; each Option left out stays.
var SetOptions = &Definition[api.SetOptionsInput, api.OptionsView]{
	Name: "set_options", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/set_options",
	Doc:  "Change the Options.",
	Call: func(c *core.Core, ctx context.Context, _ string, in api.SetOptionsInput) (api.OptionsView, error) {
		return c.SetOptions(ctx, in)
	},
}
