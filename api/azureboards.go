package api

import (
	"context"

	"github.com/tomoram/code-central/domain"
)

// AzureBoardsView is a Project's Azure Boards settings as every Project
// read returns them.
type AzureBoardsView struct {
	On bool `json:"on"`
	// Mode is how it picks its work items: saved_query or wiql.
	Mode domain.AzureBoardsMode `json:"mode"`
	// QueryURL is the Saved query's URL.
	QueryURL string `json:"query_url"`
	// ProjectURL is the Azure DevOps project a WIQL filter runs in, and
	// Where its WHERE clause.
	ProjectURL string `json:"project_url"`
	Where      string `json:"where"`
	// PATEnv is the name of the environment variable holding the Azure
	// DevOps PAT: the Project's CI PAT variable, ADO_PAT unless it names
	// another, unless Azure Boards names its own. The PAT itself is never
	// kept.
	PATEnv          string `json:"pat_env"`
	NewIssuesNeedMe bool   `json:"new_issues_need_me"`
	// HideCodeCentral is whether Write-back keeps Code Central out of what
	// it writes to a work item.
	HideCodeCentral bool `json:"hide_code_central"`
	// BranchFormat names the Spec Branch of a Requirement To Requirement
	// makes of a work item, with {id} for its ID, such as feat/{id};
	// empty for none.
	BranchFormat string `json:"branch_format"`
	// WorkItemStates are the work item States Write-back sets; a blank
	// one sets nothing.
	WorkItemStates WorkItemStates `json:"work_item_states"`
	// Spec is the Azure Boards Spec's ID, left out until Azure Boards is
	// first turned on.
	Spec string `json:"spec,omitempty"`
	// Watcher is the Work Item Watcher's status, as it last reported it.
	Watcher WatcherView `json:"watcher"`
}

// SetAzureBoardsInput is the human's set_azure_boards(on, mode, query_url,
// project_url, where, pat_env, new_issues_need_me, hide_code_central,
// branch_format, work_item_states, full?).
type SetAzureBoardsInput struct {
	On bool `json:"on"`
	// Mode is saved_query when left out.
	Mode domain.AzureBoardsMode `json:"mode"`
	// QueryURL is the Saved query's URL, as the browser shows it.
	QueryURL string `json:"query_url"`
	// ProjectURL is the Azure DevOps project a WIQL filter runs in, as its
	// URL, and Where the filter's WHERE clause, without the keyword.
	ProjectURL string `json:"project_url"`
	Where      string `json:"where"`
	// PATEnv names the environment variable holding the Azure DevOps PAT;
	// the Project's CI PAT variable when left out.
	PATEnv          string `json:"pat_env"`
	NewIssuesNeedMe bool   `json:"new_issues_need_me"`
	// HideCodeCentral keeps Code Central out of what Write-back writes to a
	// work item. It is no part of the Azure Boards check.
	HideCodeCentral bool `json:"hide_code_central"`
	// BranchFormat names the Spec Branch of a Requirement To Requirement
	// makes of a work item, with {id} for its ID; empty for none. It is
	// no part of the Azure Boards check.
	BranchFormat string `json:"branch_format"`
	// WorkItemStates are the work item States Write-back sets when the
	// linked work first reaches In Progress, Accepted and Done; a blank
	// one sets nothing. They are no part of the Azure Boards check.
	WorkItemStates WorkItemStates `json:"work_item_states"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// WorkItemStates are the State names of Azure Boards' Work Item States.
type WorkItemStates struct {
	InProgress string `json:"in_progress"`
	Accepted   string `json:"accepted"`
	Done       string `json:"done"`
}

// AzureBoardsCheckInput is what the Azure Boards check checks: that the
// PAT in the environment variable PATEnv can run the chosen Mode's query,
// the Saved query at QueryURL or the WHERE clause Where in the project at
// ProjectURL, and write to its work items. Only the chosen Mode's
// settings are given.
type AzureBoardsCheckInput struct {
	Mode       domain.AzureBoardsMode `json:"mode"`
	QueryURL   string                 `json:"query_url,omitempty"`
	ProjectURL string                 `json:"project_url,omitempty"`
	Where      string                 `json:"where,omitempty"`
	PATEnv     string                 `json:"pat_env"`
}

// AzureBoardsCheck runs the Azure Boards check. The service makes no
// external calls itself (ADR 0002, ADR 0004), so the check runs in the
// Work Item Watcher's process.
type AzureBoardsCheck func(ctx context.Context, in AzureBoardsCheckInput) WatcherCheckResult
