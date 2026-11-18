package ops

import "github.com/tomoram/code-central/core"

// The human's GitHub Issues, Azure Boards and Models settings, and the
// triage actions on an incoming issue.

// SetGitHubIssues is the human's set_github_issues (Spec code-central-279).
var SetGitHubIssues = &Definition[core.SetGitHubIssuesInput, core.Written[core.ProjectView]]{
	Name: "set_github_issues", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_github_issues",
	Doc:  "Set the Project's GitHub Issues: on or off, its repositories, filter label and New issues need me.",
	Call: (*core.Core).SetGitHubIssues,
}

// KeepAsDraft is the human's keep_as_draft (code-central-287).
var KeepAsDraft = &Definition[core.TicketInput, core.Written[core.TicketView]]{
	Name: "keep_as_draft", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/keep_as_draft",
	Doc:  "Keep an Incoming Issue as a Draft Ticket.",
	Call: (*core.Core).KeepAsDraft,
}

// ToRequirement is the human's to_requirement (code-central-290).
var ToRequirement = &Definition[core.ToRequirementInput, core.Written[core.TicketView]]{
	Name: "to_requirement", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/to_requirement",
	Doc:  "Turn an Incoming Issue into a Requirement.",
	Call: (*core.Core).ToRequirement,
}

// WontFix is the human's wont_fix (code-central-289).
var WontFix = &Definition[core.WontFixInput, core.Written[core.TicketView]]{
	Name: "wont_fix", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/wont_fix",
	Doc:  "Cancel an Incoming Issue as won't fix.",
	Call: (*core.Core).WontFix,
}

// SetAzureBoards is the human's set_azure_boards (Spec code-central-894).
var SetAzureBoards = &Definition[core.SetAzureBoardsInput, core.Written[core.ProjectView]]{
	Name: "set_azure_boards", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_azure_boards",
	Doc:  "Set the Project's Azure Boards: on or off, its Mode and each Mode's settings.",
	Call: (*core.Core).SetAzureBoards,
}

// SetModels is the human's set_models (Spec code-central-301): the model
// and effort per kind of work.
var SetModels = &Definition[core.SetModelsInput, core.Written[core.ProjectView]]{
	Name: "set_models", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_models",
	Doc:  "Set the Models section of the Project's Options: the model and effort per kind of work.",
	Call: (*core.Core).SetModels,
}
