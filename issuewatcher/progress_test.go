package issuewatcher_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// agent is the context an agent's writes are made in.
var agent = core.WithActor(context.Background(), core.Agent)

// stages are the Stage Tickets of Requirement spec, in Stage order.
func (w *watching) stages(spec string) []string {
	w.t.Helper()
	sp, err := w.srv.Core.GetSpec(context.Background(), "acme", core.GetSpecInput{Spec: core.Ref(spec)})
	if err != nil {
		w.t.Fatal(err)
	}
	var out []string
	for _, s := range sp.Requirement.Stages {
		out = append(out, s.Ticket)
	}
	return out
}

// workedDone is ticket claimed by the agent, commented on with comment if it
// isn't empty, finished, and accepted to Done, or with a commit, landed
// with it.
func (w *watching) workedDone(ticket, comment, commit string) {
	w.t.Helper()
	c, err := w.srv.Core.Claim(agent, "acme", core.ClaimInput{Ticket: core.Ref(ticket)})
	if err == nil && comment != "" {
		_, err = w.srv.Core.Comment(agent, "acme", core.CommentInput{Ticket: core.Ref(ticket), Body: comment})
	}
	branch := ""
	if commit != "" {
		branch = ticket
	}
	if err == nil {
		in := core.FinishInput{Ticket: core.Ref(ticket), ClaimID: c.ClaimID, Summary: "Finished " + ticket + ".", Branch: branch}
		if core.StageOf(c.Ticket.Labels) == "spec" {
			in.Recommendation = store.RecommendSplit
		}
		_, err = w.srv.Core.Finish(agent, "acme", in)
	}
	if err == nil {
		_, err = w.srv.Core.Accept(human, "acme", core.TicketInput{Ticket: core.Ref(ticket)})
	}
	if err == nil && commit != "" {
		_, err = w.srv.Core.Land(agent, "acme", core.LandInput{Ticket: core.Ref(ticket), Commit: commit})
	}
	if err != nil {
		w.t.Fatal(err)
	}
}

// implementation is the agent's Ready implementation Ticket title on
// spec, returning its ID.
func (w *watching) implementation(spec, title string) string {
	w.t.Helper()
	out, err := w.srv.Core.CreateTicket(agent, "acme", core.CreateTicketInput{Spec: core.Ref(spec), Title: title, State: "Ready", Full: true})
	if err != nil {
		w.t.Fatal(err)
	}
	return out.Record.ID
}

// posted is the write posting the comment key for spec with text.
func (w *watching) posted(spec, key, text string) string {
	return fmt.Sprintf("POST repos/acme/web/issues/1/comments body=<!-- code-central %s %s -->\n%s\n\n[%s in Code Central](%s/?view=inbox&list=requirements&requirement=%s)",
		spec, key, text, spec, w.srv.URL, spec)
}

// relabelled is the writes that move issue 1's status label from one to
// another.
func relabelled(from, to string) []string {
	return []string{"POST repos/acme/web/issues/1/labels labels[]=" + to, "DELETE repos/acme/web/issues/1/labels/" + from}
}

// As the Requirement goes on, each comment is posted once, in order, the
// label follows requirement, building and done, and the issue closes as
// completed with the list of what landed.
func TestARequirementsProgressIsWrittenBackAndItsIssueClosesAsCompleted(t *testing.T) {
	w := newWatching(t)
	spec := w.toRequirement(w.arrivedFirst(labelled(1, "code-central:draft")), "")
	stages := w.stages(spec)
	w.polled()
	w.openIssues("", 1, labelled(1, "code-central:requirement"))

	w.workedDone(stages[0], "Use Postgres.", "")
	w.workedDone(stages[1], "", "")
	from := len(w.h.Calls())
	w.polled()
	w.wantWrites(from,
		w.posted(spec, "grilling", "Use Postgres."),
		w.posted(spec, "spec", "# Issue 1\n\nBody 1"))

	fix := w.implementation(spec, "Fix it")
	test := w.implementation(spec, "Test it")
	w.workedDone(stages[2], "", "")
	from = len(w.h.Calls())
	w.polled()
	w.wantWrites(from, relabelled("code-central:requirement", "code-central:building")...)
	w.openIssues("", 1, labelled(1, "code-central:building"))

	w.workedDone(fix, "", "abc123")
	from = len(w.h.Calls())
	w.polled()
	w.wantWrites(from, w.posted(spec, "landed:"+fix, fmt.Sprintf("Landed **Fix it** (%s) in abc123.", fix)))

	w.workedDone(test, "", "def456")
	from = len(w.h.Calls())
	w.polled()
	w.wantWrites(from, append(append([]string{
		w.posted(spec, "landed:"+test, fmt.Sprintf("Landed **Test it** (%s) in def456.", test)),
		w.posted(spec, "closed", fmt.Sprintf("Finished. What landed:\n\n- **Fix it** (%s) in abc123\n- **Test it** (%s) in def456", fix, test)),
	}, relabelled("code-central:building", "code-central:done")...),
		"PATCH repos/acme/web/issues/1 state=closed state_reason=completed")...)

	// Nothing more is due, so nothing is posted twice.
	if due := w.writesDue(); len(due) != 0 {
		t.Fatalf("writes due after = %+v, want none", due)
	}
	from = len(w.h.Calls())
	w.polled()
	w.wantWrites(from)
}

// A Spec Finished with nothing landed closes its issue as not planned,
// with code-central:wont-fix.
func TestARequirementFinishedWithNothingLandedClosesItsIssueAsNotPlanned(t *testing.T) {
	w := newWatching(t)
	spec := w.toRequirement(w.arrivedFirst(labelled(1, "code-central:requirement")), "")
	for _, s := range w.stages(spec) {
		if _, err := w.srv.Core.Move(human, "acme", core.MoveInput{Ticket: core.Ref(s), ToState: "Cancelled"}); err != nil {
			t.Fatal(err)
		}
	}
	from := len(w.h.Calls())
	w.polled()
	w.wantWrites(from, append(relabelled("code-central:requirement", "code-central:wont-fix"),
		"PATCH repos/acme/web/issues/1 state=closed state_reason=not_planned")...)
}
