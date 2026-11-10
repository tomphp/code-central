package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// Triage (Spec code-central-279): an Incoming Issue is a Draft or Ready
// Ticket on its Project's Issues Spec whose Link isn't detached. With New
// issues need me on, one the Integration creates is awaiting triage, and
// waits in Needs Me as New issue until the human triages it. The triage
// actions are the human's, offered on every Incoming Issue, awaiting
// triage or not.

// The triage actions, each an operation's name.
const (
	OptionKeepAsDraft = "keep_as_draft"
	OptionWontFix     = "wont_fix"
)

// triageActions are the triage actions in the order they are offered.
var triageActions = []string{OptionKeepAsDraft, OptionWontFix}

// issuesSpec is the number of key's Issues Spec, 0 when it has none.
func issuesSpec(r store.Reader, key string) int {
	p, _ := r.Project(key)
	return p.GitHubIssues.Spec
}

// awaitsTriage reports whether a Ticket created now on spec, in key, is
// awaiting triage: it is on the Issues Spec while New issues need me is
// on.
func awaitsTriage(r store.Reader, key string, spec int) bool {
	p, _ := r.Project(key)
	return p.GitHubIssues.NewIssuesNeedMe && spec == issuesSpec(r, key)
}

// whyNotIncoming says why t isn't an Incoming Issue, "" when it is.
func whyNotIncoming(r store.Reader, key string, t store.Ticket) string {
	id := DisplayID(key, t.Number)
	switch spec := issuesSpec(r, key); {
	case spec == 0 || t.Spec != spec || !t.Link.Linked():
		return fmt.Sprintf("%s is not an Incoming Issue: only a linked Ticket on the Project's Issues Spec is triaged", id)
	case t.Link.Detached:
		return fmt.Sprintf("%s was unlinked from %s %s, so it is no longer an Incoming Issue to triage", id, t.Link.Integration, t.Link.ID)
	case t.State != store.Draft && t.State != store.Ready:
		return fmt.Sprintf("%s is %s: only a Draft or Ready Incoming Issue is triaged", id, t.State)
	}
	return ""
}

// ticketTriageActions are the triage actions offered on t: every one on
// an Incoming Issue, none otherwise.
func ticketTriageActions(r store.Reader, key string, t store.Ticket) []string {
	if whyNotIncoming(r, key, t) != "" {
		return nil
	}
	return triageActions
}

// newIssue is when t became a New issue, if it is one: an Incoming Issue
// awaiting triage that isn't closed externally. Its time is its creation.
func newIssue(r store.Reader, key string, t store.Ticket) (time.Time, bool) {
	if !t.AwaitingTriage || whyNotIncoming(r, key, t) != "" {
		return time.Time{}, false
	}
	if _, closed := closedExternally(t); closed {
		return time.Time{}, false
	}
	return t.Created, true
}

// KeepAsDraft is the human's keep_as_draft(ticket): it clears an
// Incoming Issue's awaiting triage, so it leaves Needs Me and the New
// issue reason never comes back for it. It writes nothing to GitHub.
// Keeping one that isn't awaiting triage changes nothing.
func (c *Core) KeepAsDraft(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, OptionKeepAsDraft); err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, OptionKeepAsDraft, in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		if why := whyNotIncoming(r, key, *t); why != "" {
			return false, &Error{Code: ActionNotAllowed, Message: why}
		}
		if !t.AwaitingTriage {
			return false, nil
		}
		t.AwaitingTriage = false
		return true, nil
	})
}

// WontFixInput is the human's wont_fix(ticket, reason?).
type WontFixInput struct {
	Ticket Ref    `json:"ticket"`
	Reason string `json:"reason,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// WontFix is the human's wont_fix(ticket, reason?): it Cancels an
// Incoming Issue as cancel does, with the reason as its comment, and
// clears its awaiting triage. Its Move to Cancelled is via wont_fix, so
// closing its issue as not planned is due, with the reason, if given, as
// the wont-fix comment (writeback.go). Its Link stays attached.
func (c *Core) WontFix(ctx context.Context, key string, in WontFixInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, OptionWontFix); err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, OptionWontFix, in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		if why := whyNotIncoming(r, key, *t); why != "" {
			return false, &Error{Code: ActionNotAllowed, Message: why}
		}
		before := *t
		now := c.now()
		if err := transition(ctx, t, key, "cancel", "", now); err != nil {
			return false, err
		}
		if err := checkReplacements(r, key, before, nil); err != nil {
			return false, err
		}
		t.Moves[len(t.Moves)-1].Via = OptionWontFix
		if reason := strings.TrimSpace(in.Reason); reason != "" {
			addComment(t, Human, now, 0, reason)
		}
		t.AwaitingTriage = false
		return true, nil
	})
}
