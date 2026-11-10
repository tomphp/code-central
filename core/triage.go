package core

import (
	"context"
	"fmt"
	"slices"
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
	OptionKeepAsDraft   = "keep_as_draft"
	OptionToRequirement = "to_requirement"
	OptionWontFix       = "wont_fix"
)

// triageActions are the triage actions in the order they are offered.
var triageActions = []string{OptionKeepAsDraft, OptionToRequirement, OptionWontFix}

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

// ToRequirementInput is the human's to_requirement(ticket, context?).
type ToRequirementInput struct {
	Ticket  Ref    `json:"ticket"`
	Context string `json:"context,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// contextHeading heads the human's context in a Requirement made by To
// Requirement.
const contextHeading = "## Context"

// requirementText is the text of the Requirement To Requirement makes of
// t: its title as a heading, then its description, then the context, if
// given, under its own heading.
func requirementText(t store.Ticket, extra string) string {
	parts := []string{"# " + t.Title}
	if d := strings.TrimSpace(t.Description); d != "" {
		parts = append(parts, d)
	}
	if extra != "" {
		parts = append(parts, contextHeading, extra)
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// ToRequirement is the human's to_requirement(ticket, context?): in one
// store update it makes an Incoming Issue a Requirement whose Spec has an
// outbound Link to the same issue, carrying over the comment keys
// written; detaches the Incoming Issue's Link; and Cancels it with the
// grilling Stage as its Replacement, the context as its comment, and its
// awaiting triage cleared. Its Move to Cancelled is via to_requirement,
// so the context comment is due on the issue (writeback.go). Its result
// is the Incoming Issue's, whose Replacement leads to the Requirement.
func (c *Core) ToRequirement(ctx context.Context, key string, in ToRequirementInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, OptionToRequirement); err != nil {
		return Written[TicketView]{}, err
	}
	var res Written[TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		t, err := resolveTicket(w, key, OptionToRequirement, "ticket", in.Ticket)
		if err != nil {
			return err
		}
		if why := whyNotIncoming(w, key, t); why != "" {
			return &Error{Code: ActionNotAllowed, Message: why}
		}
		if id, ok := linkedTo(w, key, t.Link, true); ok {
			return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s %s is already linked to Spec %s", t.Link.Integration, t.Link.ID, id)}
		}
		var before TicketView
		if !in.Full {
			before = c.ticketView(w, key, t)
		}
		extra := strings.TrimSpace(in.Context)
		link := t.Link
		link.Outbound, link.Written = true, slices.Clone(t.Link.Written)
		im, _, stageNs := c.requirementImport(w, key, store.Spec{Title: t.Title, Body: requirementText(t, extra), Link: link}, Stages, "")

		// The grilling Stage is new and waits on nothing, so as the
		// Replacement it can't make a cycle.
		now := c.now()
		if err := transition(ctx, &t, key, "cancel", "", now); err != nil {
			return err
		}
		t.Moves[len(t.Moves)-1].Via = OptionToRequirement
		t.Replacements = stageNs[:1]
		if extra != "" {
			addComment(&t, Human, now, 0, extra)
		}
		t.Link.Detached = true
		t.AwaitingTriage = false
		t.Updated = now
		im.Rewritten = []store.Ticket{t}
		if err := w.PutImport(key, im); err != nil {
			return err
		}
		res, err = acknowledge(before, c.ticketView(w, key, t), in.Full)
		return err
	})
	return res, err
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
