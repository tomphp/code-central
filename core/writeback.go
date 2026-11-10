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
	// LabelRequirement is on an issue To Requirement made a Requirement
	// of, while the Requirement's Stages aren't all Done.
	LabelRequirement = StatusLabelPrefix + "requirement"
	// LabelWontFix is on an issue the human chose Won't fix for.
	LabelWontFix = StatusLabelPrefix + "wont-fix"
)

// The keys of the comments written to an issue.
const (
	// CommentContext is the key of the comment with the context the human
	// gave To Requirement.
	CommentContext = "context"
	// CommentWontFix is the key of the comment with the human's Won't fix
	// reason.
	CommentWontFix = "wont-fix"
)

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

// The UI's query parameters that open the item a comment links back to:
// a Ticket, or a Requirement in the Requirements list.
const (
	ticketLink      = "ticket="
	requirementLink = "list=requirements&requirement="
)

// issueComment is the comment key on target, with text, as it is posted.
// Its link back opens target in the UI by open, ticketLink or
// requirementLink.
func (c *Core) issueComment(target, open, key, text string) IssueComment {
	link := fmt.Sprintf("[%s in Code Central](%s/?view=inbox&%s%s)", target, c.uiURL, open, url.QueryEscape(target))
	return IssueComment{Key: key, Body: IssueMarker(target, key) + "\n" + strings.TrimSpace(text) + "\n\n" + link}
}

// triageComment is the comment the human's triage action via gave when
// it Cancelled t, if it ever did: empty when it gave none. latest
// reports whether that Cancel is still t's last Move.
func triageComment(t store.Ticket, via string) (comment string, latest, ok bool) {
	for i, m := range slices.Backward(t.Moves) {
		if m.To != store.Cancelled || m.Via != via {
			continue
		}
		latest = i == len(t.Moves)-1 && t.State == store.Cancelled
		for _, cm := range slices.Backward(t.Comments) {
			if cm.Time.Equal(m.Time) && cm.Actor == string(Human) {
				return cm.Body, latest, true
			}
		}
		return "", latest, true
	}
	return "", false, false
}

// ticketIssueWrites is what is due on the issue t, a Ticket on key's
// Issues Spec, is linked to; ok is false when nothing is.
func (c *Core) ticketIssueWrites(key string, t store.Ticket) (w IssueWrites, ok bool) {
	if !t.Link.Attached() {
		return IssueWrites{}, false
	}
	id := DisplayID(key, t.Number)
	w = IssueWrites{Target: id, Link: *linkView(t.Link), Comments: []IssueComment{}}
	if reason, latest, _ := triageComment(t, OptionWontFix); latest {
		w.Label, w.Close = LabelWontFix, CloseNotPlanned
		if strings.TrimSpace(reason) != "" && !slices.Contains(t.Link.Written, CommentWontFix) {
			w.Comments = append(w.Comments, c.issueComment(id, ticketLink, CommentWontFix, reason))
		}
	} else if t.State == store.Draft || t.State == store.Ready {
		w.Label = LabelDraft
	}
	return dueWhile(t.Link.ExternalState, w)
}

// specIssueWrites is what is due on the issue s, a Spec with an outbound
// Link that isn't detached, is linked to; ok is false when nothing is.
// The context comment is due when To Requirement was given context, and
// the issue should carry code-central:requirement while the
// Requirement's Stages aren't all Done.
func (c *Core) specIssueWrites(r store.Reader, key string, s store.Spec) (w IssueWrites, ok bool) {
	if !s.Link.Attached() || !s.Link.Outbound {
		return IssueWrites{}, false
	}
	id := DisplayID(key, s.Number)
	w = IssueWrites{Target: id, Link: *linkView(s.Link), Comments: []IssueComment{}}
	if extra := requirementContext(r, key, s.Link); extra != "" && !slices.Contains(s.Link.Written, CommentContext) {
		w.Comments = append(w.Comments, c.issueComment(id, requirementLink, CommentContext, extra))
	}
	if slices.ContainsFunc(specTickets(r, key, s.Number), func(t store.Ticket) bool {
		return StageOf(t.Labels) != "" && t.State != store.Done
	}) {
		w.Label = LabelRequirement
	}
	return dueWhile(s.Link.ExternalState, w)
}

// requirementContext is the context the human gave To Requirement for
// the issue l links to: the comment to_requirement made on its Incoming
// Issue, empty when it gave none. It stays due if the Incoming Issue is
// moved on later.
func requirementContext(r store.Reader, key string, l store.Link) string {
	for _, t := range r.Tickets(key) {
		if t.Link.Integration != l.Integration || t.Link.ID != l.ID {
			continue
		}
		if extra, _, ok := triageComment(t, OptionToRequirement); ok {
			return extra
		}
	}
	return ""
}

// dueWhile is w, the writes due on an issue last seen as state: its label
// and closing are due only while it is open, and nothing is due on a
// deleted issue. ok is false when nothing is due.
func dueWhile(state store.ExternalState, w IssueWrites) (IssueWrites, bool) {
	if state != store.ExternalOpen {
		w.Label, w.Close = "", ""
	}
	if state == store.ExternalDeleted {
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
