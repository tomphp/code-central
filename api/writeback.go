package api

// IssueClose is how an issue is closed.
type IssueClose string

// The ways an issue is closed, as GitHub names them.
const (
	CloseCompleted  IssueClose = "completed"
	CloseNotPlanned IssueClose = "not_planned"
)

// IssueWrites is an item whose issue has GitHub writes due, as
// get_issue_watch lists it.
type IssueWrites struct {
	// Target is the item, such as acme-5.
	Target string   `json:"target"`
	Link   LinkView `json:"link"`
	// Comments are the comments due, in the order to post them.
	Comments []IssueComment `json:"comments"`
	// Label is the status label the issue should carry; empty asks for
	// none to be changed.
	Label string `json:"label,omitempty"`
	// Close is how the issue should be closed; empty when it shouldn't.
	Close IssueClose `json:"close,omitempty"`
}

// IssueComment is a comment due on an issue: its key, stable for the
// item, and its Markdown body ready to post, which carries the hidden
// marker IssueMarker gives and ends with a link back to the item. Under
// Hide Code Central it carries the marker OpaqueMarker gives instead, and
// no link back.
type IssueComment struct {
	Key  string `json:"key"`
	Body string `json:"body"`
}

// ReportIssueWrittenInput is report_issue_written's input: the linked
// item and the key of the comment written to its issue.
type ReportIssueWrittenInput struct {
	Target Ref    `json:"target"`
	Key    string `json:"key"`
}
