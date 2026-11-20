package api

// IssueWatch is get_issue_watch: everything the Issue Watcher needs on
// each poll, so it keeps no state of its own. The gh path Option is read
// on every poll, so changing it needs no restart.
type IssueWatch struct {
	Project string `json:"project"`
	On      bool   `json:"on"`
	// Repositories are the GitHub repositories to poll, each as owner/name.
	Repositories []string `json:"repositories"`
	// Label is the filter label; empty lets every issue in.
	Label  string `json:"label"`
	GHPath string `json:"gh_path"`
	// Spec is the Issues Spec's ID, left out until GitHub Issues is first
	// turned on.
	Spec string `json:"spec,omitempty"`
	// Issues are the Incoming Issues, in number order, so the Issue
	// Watcher sees which issues it has and which have changed.
	Issues []IncomingIssue `json:"issues"`
	// Writes are the items, Incoming Issues and outbound Specs, whose
	// GitHub issues have writes due, in number order (writeback.go).
	Writes []IssueWrites `json:"writes"`
}

// IncomingIssue is a Ticket on an Integration's inbox Spec, linked
// through it, that isn't finished and whose Link isn't detached, as
// get_issue_watch and get_work_item_watch list it.
type IncomingIssue struct {
	Ticket      string   `json:"ticket"`
	Link        LinkView `json:"link"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
}

// WatchInput is the input of the Issue Watcher's and the Work Item Watcher's
// read, which takes nothing.
type WatchInput struct{}
