package api

import "github.com/tomoram/code-central/domain"

// WorkItemWatch is get_work_item_watch: everything the Work Item Watcher
// needs on each poll, so it keeps no state of its own.
type WorkItemWatch struct {
	Project string `json:"project"`
	On      bool   `json:"on"`
	// Mode is how it picks its work items: saved_query or wiql.
	Mode domain.AzureBoardsMode `json:"mode"`
	// QueryURL is the Saved query's URL.
	QueryURL string `json:"query_url"`
	// ProjectURL is the Azure DevOps project a WIQL filter runs in, and
	// Where its WHERE clause.
	ProjectURL string `json:"project_url"`
	Where      string `json:"where"`
	// PATEnv is the name of the environment variable holding the Azure
	// DevOps PAT.
	PATEnv string `json:"pat_env"`
	// Spec is the Azure Boards Spec's ID, left out until Azure Boards is
	// first turned on.
	Spec string `json:"spec,omitempty"`
	// Issues are the Incoming Issues on the Azure Boards Spec, in number
	// order, so the Watcher sees which work items it has and which have
	// changed.
	Issues []IncomingIssue `json:"issues"`
	// Linked is every azure-boards Link ID in the Project, detached and
	// outbound ones too, once each, so the Watcher tells a new work item
	// from one already taken off the inbox.
	Linked []string `json:"linked"`
	// Writes are the items, Incoming Issues and outbound Specs, whose work
	// items have writes due, in number order (writeback.go).
	Writes []WorkItemWrites `json:"writes"`
}

// WorkItemWrites is an item whose work item has writes due, as
// get_work_item_watch lists it: what is due on a GitHub issue, with the
// status label as the work item's tag of the same name, and never a
// close. Its State is set only as Work Item States maps it.
type WorkItemWrites struct {
	// Target is the item, such as acme-5.
	Target string   `json:"target"`
	Link   LinkView `json:"link"`
	// Comments are the comments due, in the order to post them.
	Comments []IssueComment `json:"comments"`
	// Tag is the status tag the work item should carry, in place of any
	// other code-central: tag; empty asks for none to be changed, as it
	// always is under Hide Code Central.
	Tag string `json:"tag,omitempty"`
	// State is the work item State due, left out when none is.
	State *WorkItemStateWrite `json:"state,omitempty"`
}

// WorkItemStateWrite is the work item State due for the furthest phase
// the linked work reached: the State to set, then the Written keys to
// report with report_issue_written once it is set.
type WorkItemStateWrite struct {
	Name string `json:"name"`
	// Key is the phase's Written key, such as state:done.
	Key string `json:"key"`
	// Nearer are the nearer phases' Written keys, recorded as written
	// with it, so none of them is due again.
	Nearer []string `json:"nearer"`
}
