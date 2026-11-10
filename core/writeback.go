package core

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Write-backs (Spec code-central-279): Code Central works out from the
// work's state, never from events, what each linked issue should have,
// and get_issue_watch lists it; the Issue Watcher makes it so. A comment
// is written once: report_issue_written records its key on the item's
// Link. Labels and closing need no record, as the Issue Watcher changes
// only what differs. Nothing is ever due from Rounds, answers, Failure
// Reports, Transcripts or Claims.

// StatusLabelPrefix starts every status label, so the Issue Watcher
// knows which of an issue's labels are Code Central's.
const StatusLabelPrefix = "code-central:"

// The status labels an issue carries, one at a time.
const (
	// LabelDraft is on an Incoming Issue, a Draft or Ready Ticket on the
	// Issues Spec that no triage action has taken off it.
	LabelDraft = StatusLabelPrefix + "draft"
	// LabelWontFix is on an issue the human chose Won't fix for.
	LabelWontFix = StatusLabelPrefix + "wont-fix"
)

// CommentWontFix is the key of the comment with the human's Won't fix
// reason.
const CommentWontFix = "wont-fix"

// IssueClose is how an issue is closed.
type IssueClose string

// The ways an issue is closed, as GitHub names them.
const (
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
// marker IssueMarker gives and ends with a link back to the item.
type IssueComment struct {
	Key  string `json:"key"`
	Body string `json:"body"`
}

// IssueMarker is the hidden HTML comment in the comment key posted for
// target, so the Issue Watcher finds it on the issue before posting it
// again after a crash.
func IssueMarker(target, key string) string {
	return fmt.Sprintf("<!-- code-central %s %s -->", target, key)
}

// SetUIURL makes url, such as http://localhost:7733, the UI's address
// that a comment links back to. Call it before serving; without it,
// the link is relative to the UI.
func (c *Core) SetUIURL(url string) { c.uiURL = strings.TrimRight(url, "/") }

// issueComment is the comment key on target, with text, as it is posted.
func (c *Core) issueComment(target, key, text string) IssueComment {
	link := fmt.Sprintf("[%s in Code Central](%s/?view=inbox&ticket=%s)", target, c.uiURL, url.QueryEscape(target))
	return IssueComment{Key: key, Body: IssueMarker(target, key) + "\n" + strings.TrimSpace(text) + "\n\n" + link}
}

// wontFixReason is t's Won't fix reason, if the human chose Won't fix
// for it: the comment its wont_fix made, empty when it gave none.
func wontFixReason(t store.Ticket) (reason string, ok bool) {
	if t.State != store.Cancelled || len(t.Moves) == 0 {
		return "", false
	}
	m := t.Moves[len(t.Moves)-1]
	if m.To != store.Cancelled || m.Via != OptionWontFix {
		return "", false
	}
	for _, cm := range slices.Backward(t.Comments) {
		if cm.Time.Equal(m.Time) && cm.Actor == string(Human) {
			return cm.Body, true
		}
	}
	return "", true
}

// ticketIssueWrites is what is due on the issue t, a Ticket on key's
// Issues Spec, is linked to; ok is false when nothing is. Its label and
// closing are due only while the issue was last seen open, and nothing
// is due on a deleted issue.
func (c *Core) ticketIssueWrites(key string, t store.Ticket) (w IssueWrites, ok bool) {
	if !t.Link.Owned() {
		return IssueWrites{}, false
	}
	id := DisplayID(key, t.Number)
	w = IssueWrites{Target: id, Link: *linkView(t.Link), Comments: []IssueComment{}}
	if reason, ok := wontFixReason(t); ok {
		w.Label, w.Close = LabelWontFix, CloseNotPlanned
		if strings.TrimSpace(reason) != "" && !slices.Contains(t.Link.Written, CommentWontFix) {
			w.Comments = append(w.Comments, c.issueComment(id, CommentWontFix, reason))
		}
	} else if t.State == store.Draft || t.State == store.Ready {
		w.Label = LabelDraft
	}
	if t.Link.ExternalState != store.ExternalOpen {
		w.Label, w.Close = "", ""
	}
	if t.Link.ExternalState == store.ExternalDeleted {
		w.Comments = []IssueComment{}
	}
	return w, len(w.Comments) > 0 || w.Label != "" || w.Close != ""
}

// ReportIssueWrittenInput is report_issue_written's input: the linked
// item and the key of the comment written to its issue.
type ReportIssueWrittenInput struct {
	Target Ref    `json:"target"`
	Key    string `json:"key"`
}

// ReportIssueWritten is the Issue Watcher's report_issue_written(target,
// key), accepted from the integration Actor only: the comment key is
// written to target's issue, so it is never due again. Reporting it
// again changes nothing.
func (c *Core) ReportIssueWritten(ctx context.Context, key string, in ReportIssueWrittenInput) (LinkedRow, error) {
	if err := requireActor(ctx, "report_issue_written", Integration); err != nil {
		return LinkedRow{}, err
	}
	if strings.TrimSpace(in.Key) == "" {
		return LinkedRow{}, invalid("key", "name the key of the comment written")
	}
	return c.updateLink(ctx, key, in.Target, func(l *store.Link) (map[string]string, error) {
		if slices.Contains(l.Written, in.Key) {
			return nil, nil
		}
		l.Written = append(l.Written, in.Key)
		return map[string]string{}, nil
	})
}
