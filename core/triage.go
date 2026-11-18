package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Triage (Spec code-central-279): an Incoming Issue is a linked Draft or
// Ready Ticket on an Integration's inbox Spec, the Issues Spec or the
// Azure Boards Spec (Spec code-central-894), whose Link isn't detached.
// With that Integration's New issues need me on, one it creates is
// awaiting triage, and waits in Needs Me as New issue until the human
// triages it. The triage actions are the human's, offered on every
// Incoming Issue, awaiting triage or not.

// TriageAction is a triage action the human may take on an Incoming Issue
// (Spec code-central-279), named for its operation.
type TriageAction string

// The triage actions.
const (
	OptionKeepAsDraft   TriageAction = "keep_as_draft"
	OptionToRequirement TriageAction = "to_requirement"
	OptionWontFix       TriageAction = "wont_fix"
)

// TriageActions are the triage actions in the order they are offered.
func TriageActions() []TriageAction {
	return []TriageAction{OptionKeepAsDraft, OptionToRequirement, OptionWontFix}
}

// awaitsTriage reports whether a Ticket created now on spec, in key, is
// awaiting triage: it is on an Integration's inbox Spec while that
// Integration's New issues need me is on.
func awaitsTriage(r store.Reader, key string, spec int) bool {
	in, ok := inboxOn(r, key, spec)
	return ok && in.newIssuesNeedMe
}

// whyNotIncoming says why t isn't an Incoming Issue, "" when it is.
func whyNotIncoming(r store.Reader, key string, t domain.Ticket) string {
	id := DisplayID(key, t.Number)
	_, onInbox := inboxOn(r, key, t.Spec)
	switch {
	case !onInbox || !t.Link.Linked():
		return fmt.Sprintf("%s is not an Incoming Issue: only a linked Ticket on the Project's Issues Spec or Azure Boards Spec is triaged", id)
	case t.Link.Detached:
		return fmt.Sprintf("%s was unlinked from %s %s, so it is no longer an Incoming Issue to triage", id, t.Link.Integration, t.Link.ID)
	case t.State != domain.Draft && t.State != domain.Ready:
		return fmt.Sprintf("%s is %s: only a Draft or Ready Incoming Issue is triaged", id, t.State)
	}
	return ""
}

// ticketTriageActions are the triage actions offered on t: every one on
// an Incoming Issue, none otherwise.
func ticketTriageActions(r store.Reader, key string, t domain.Ticket) []TriageAction {
	if whyNotIncoming(r, key, t) != "" {
		return nil
	}
	return TriageActions()
}

// triageNotes says what each triage action does to t, an Incoming Issue,
// as the UI shows it before the human confirms: what is written back
// depends on the Integration t is linked through, and Won't fix never
// changes a work item's State.
func triageNotes(t domain.Ticket) map[TriageAction]string {
	leaves := "leaves it here as it is"
	if t.AwaitingTriage {
		leaves += ", out of Needs Me,"
	}
	if t.Link.Integration == IntegrationAzureBoards {
		return map[TriageAction]string{
			OptionKeepAsDraft:   "Keep as Draft " + leaves + " and adds nothing to the work item in Azure Boards.",
			OptionToRequirement: "To Requirement makes a Requirement of the work item's title and body, Cancels this Ticket, and from then on only writes the work's progress to the work item.",
			OptionWontFix: "Won't fix Cancels this Ticket and tags the work item " + LabelWontFix + ", with your reason as a comment. " +
				"It leaves the work item's State as it is: move it in Azure DevOps yourself.",
		}
	}
	return map[TriageAction]string{
		OptionKeepAsDraft:   "Keep as Draft " + leaves + " and adds nothing to the issue on GitHub.",
		OptionToRequirement: "To Requirement makes a Requirement of the issue's title and body, Cancels this Ticket, and from then on only writes the work's progress to the issue.",
		OptionWontFix:       "Won't fix Cancels this Ticket and closes the issue on GitHub as not planned, with your reason as a comment.",
	}
}

// newIssue is when t became a New issue, if it is one: an Incoming Issue
// awaiting triage that isn't closed externally. Its time is its creation.
func newIssue(r store.Reader, key string, t domain.Ticket) (time.Time, bool) {
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
	if err := humanOnly(ctx, string(OptionKeepAsDraft)); err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, string(OptionKeepAsDraft), in.Ticket, in.Full, func(r store.Reader, t *domain.Ticket) (bool, error) {
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
	Ticket  Ref    `json:"ticket" path:"n"`
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
func requirementText(t domain.Ticket, extra string) string {
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
	if err := humanOnly(ctx, string(OptionToRequirement)); err != nil {
		return Written[TicketView]{}, err
	}
	var res Written[TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		t, err := resolveTicket(w, key, string(OptionToRequirement), "ticket", in.Ticket)
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
		im, _, stageNs := c.requirementImport(w, key, domain.Spec{Title: t.Title, Body: requirementText(t, extra), Link: link}, Stages(), "", false)

		// The grilling Stage is new and waits on nothing, so as the
		// Replacement it can't make a cycle.
		if err := c.triageCancel(ctx, key, &t, OptionToRequirement, extra); err != nil {
			return err
		}
		t.Replacements = stageNs[:1]
		t.Link.Detached = true
		t.Updated = c.now()
		im.Rewritten = []domain.Ticket{t}
		if err := w.PutImport(key, im); err != nil {
			return err
		}
		res, err = acknowledge(before, c.ticketView(w, key, t), in.Full)
		return err
	})
	return res, err
}

// triageCancel Cancels t, an Incoming Issue, via the triage action via,
// with comment, if any, as the human's comment, and clears its awaiting
// triage.
func (c *Core) triageCancel(ctx context.Context, key string, t *domain.Ticket, via TriageAction, comment string) error {
	now := c.now()
	if err := transition(ctx, t, key, "cancel", "", now); err != nil {
		return err
	}
	t.Moves[len(t.Moves)-1].Via = string(via)
	if comment = strings.TrimSpace(comment); comment != "" {
		addComment(t, Human, now, 0, comment)
	}
	t.AwaitingTriage = false
	return nil
}

// WontFixInput is the human's wont_fix(ticket, reason?).
type WontFixInput struct {
	Ticket Ref    `json:"ticket" path:"n"`
	Reason string `json:"reason,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// WontFix is the human's wont_fix(ticket, reason?): it Cancels an
// Incoming Issue as cancel does, with the reason as its comment, and
// clears its awaiting triage. Its Move to Cancelled is via wont_fix, so
// the code-central:wont-fix status label is due, with the reason, if
// given, as the wont-fix comment, and a GitHub issue is closed as not
// planned; a work item's State is left for the human (writeback.go). Its
// Link stays attached.
func (c *Core) WontFix(ctx context.Context, key string, in WontFixInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, string(OptionWontFix)); err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, string(OptionWontFix), in.Ticket, in.Full, func(r store.Reader, t *domain.Ticket) (bool, error) {
		if why := whyNotIncoming(r, key, *t); why != "" {
			return false, &Error{Code: ActionNotAllowed, Message: why}
		}
		if err := checkReplacements(r, key, *t, nil); err != nil {
			return false, err
		}
		return true, c.triageCancel(ctx, key, t, OptionWontFix, in.Reason)
	})
}
