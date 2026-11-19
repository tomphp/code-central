package workitemwatcher_test

import (
	"context"
	"fmt"
	"github.com/tomoram/code-central/domain"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakeazdo"
)

// agent is the context an agent's writes are made in.
var agent = core.WithActor(context.Background(), core.Agent)

// workItemPath is where work item id is read and patched, below the
// fake's root.
func workItemPath(id int) string {
	return fmt.Sprintf("%s/_apis/wit/workitems/%d", fakeazdo.CollectionPath, id)
}

// tags is the tags of work item id on the fake's board.
func (w *watching) tags(id int) string { return w.fake.WorkItem(id).Tags }

// patches are the JSON Patches the fake received since request from,
// each as its work item's path and body.
func (w *watching) patches(from int) []string {
	w.t.Helper()
	var out []string
	for _, r := range w.fake.Requests()[from:] {
		if r.Method == http.MethodPatch {
			out = append(out, r.Path+" "+r.Body)
		}
	}
	return out
}

// retagged is the JSON Patch that tests the work item's revision rev and
// makes its tags tags.
func retagged(id, rev int, tags string) string {
	return fmt.Sprintf(`%s [{"op":"test","path":"/rev","value":%d},{"op":"add","path":"/fields/System.Tags","value":%q}]`, workItemPath(id), rev, tags)
}

// wantNoStateChange checks no request to the fake ever touched a work
// item's State.
func (w *watching) wantNoStateChange() {
	w.t.Helper()
	for _, r := range w.fake.Requests() {
		if r.Method == http.MethodPatch && strings.Contains(r.Body, "System.State") {
			w.t.Fatalf("%s %s changes the State: %s", r.Method, r.Path, r.Body)
		}
	}
}

// toRequirement is the human's To Requirement on work item id's Ticket
// with context, returning the Requirement's Spec.
func (w *watching) toRequirement(id int, context string) string {
	w.t.Helper()
	ticket := w.ticketOf(id).ID
	if _, err := w.srv.Core.ToRequirement(human, "acme", core.ToRequirementInput{Ticket: core.Ref(ticket), Context: context}); err != nil {
		w.t.Fatal(err)
	}
	rows := w.linked()
	return rows[len(rows)-1].ID
}

// markdown is the texts of work item id's Discussion, which must all have
// been posted as Markdown.
func (w *watching) markdown(id int) []string {
	w.t.Helper()
	var out []string
	for _, c := range w.fake.Comments(id) {
		if c.Format != "markdown" {
			w.t.Fatalf("a comment was posted as %q, want markdown", c.Format)
		}
		out = append(out, c.Text)
	}
	return out
}

// comment is the comment key on target with text, as it is posted.
func (w *watching) comment(target, key, text string) string {
	return fmt.Sprintf("<!-- code-central %s %s -->\n%s\n\n[%s in Code Central](%s/?view=inbox&%s)", target, key, text, target, w.srv.URL, linkBack(target, key))
}

// linkBack is the query that opens target in the UI: a Requirement for
// the Requirement's comments, a Ticket for Won't fix.
func linkBack(target, key string) string {
	if key == core.CommentWontFix {
		return "ticket=" + target
	}
	return "list=requirements&requirement=" + target
}

// A Draft Incoming Issue gets code-central:draft beside the tags it
// already has, by a JSON Patch that tests the revision it read, and is
// left alone once it has it.
func TestADraftIncomingIssueIsTaggedDraftBesideItsOwnTags(t *testing.T) {
	w := newWatching(t)
	tagged := item(1)
	tagged.Rev, tagged.Tags = 7, "frontend; Needs Design"
	w.fake.PutWorkItems(tagged)
	w.fake.SavedQuery(queryID, 1)
	w.start()
	w.status(core.WatcherWatching)
	w.polled()
	if got := w.patches(0); !slices.Equal(got, []string{retagged(1, 7, "frontend; Needs Design; code-central:draft")}) {
		t.Fatalf("patches %q, want one adding the tag, testing revision 7", got)
	}
	w.polled()
	if got := w.patches(0); len(got) != 1 {
		t.Fatalf("patches %q, want none once the tag is on", got)
	}
	w.wantNoStateChange()
}

// To Requirement replaces the status tag, and only code-central: tags:
// every other one, in every other case, stays.
func TestTheStatusTagReplacesEveryOtherCodeCentralTagAndNoOtherTag(t *testing.T) {
	w := newWatching(t)
	tagged := item(1)
	tagged.Tags = "Code-Central:done; keep; code-central:draft; codecentral:keep; other"
	w.fake.PutWorkItems(tagged)
	w.fake.SavedQuery(queryID, 1)
	w.start()
	w.status(core.WatcherWatching)
	w.polled()
	if got := w.tags(1); got != "keep; code-central:draft; codecentral:keep; other" {
		t.Fatalf("tags %q, want the other code-central: tag removed, the rest in place", got)
	}
	w.toRequirement(1, "")
	w.polled()
	if got := w.tags(1); got != "keep; codecentral:keep; other; code-central:requirement" {
		t.Fatalf("tags %q, want code-central:requirement in draft's place", got)
	}
	w.wantNoStateChange()
}

// A work item with the tag already, in another case, isn't written.
func TestAWorkItemWithTheTagAlreadyIsNotPatched(t *testing.T) {
	w := newWatching(t)
	tagged := item(1)
	tagged.Tags = "Code-Central:Draft"
	w.fake.PutWorkItems(tagged)
	w.fake.SavedQuery(queryID, 1)
	w.start()
	w.status(core.WatcherWatching)
	w.polled()
	if got := w.patches(0); len(got) != 0 {
		t.Fatalf("patches %q, want none", got)
	}
}

// If the work item was edited after the Work Item Watcher read it, the
// test on its revision fails, nothing is changed, and the next poll
// reads it again and writes the tag.
func TestARevisionConflictIsRetriedOnTheNextPollFromTheNewRevision(t *testing.T) {
	w := newWatching(t)
	w.fake.PutWorkItems(item(1))
	w.fake.SavedQuery(queryID, 1)
	stale := fakeazdo.JSON("PATCH", workItemPath(1), http.StatusPreconditionFailed, map[string]string{"message": "The work item has been updated."})
	stale.Times = 1
	w.fake.Script(stale)
	w.start()
	w.status(core.WatcherWatching)
	w.polled()
	if got := w.tags(1); got != "" {
		t.Fatalf("tags %q, want none: the conflict changed nothing", got)
	}
	edited := item(1)
	edited.Rev = 2
	w.fake.PutWorkItems(edited)
	w.polled()
	if got := w.patches(0); len(got) != 2 || got[1] != retagged(1, 2, "code-central:draft") {
		t.Fatalf("patches %q, want a second testing the new revision", got)
	}
	if got := w.tags(1); got != "code-central:draft" {
		t.Fatalf("tags %q, want the tag on the next poll", got)
	}
	if w.errorCount() != 0 {
		t.Fatal("a revision conflict went into an error, want it retried quietly")
	}
}

// A PAT that can read but not write work items is an error with its fix,
// and the Work Item Watcher goes back to Watching once it is fixed.
func TestAPATThatCantWriteWorkItemsIsAnErrorWithItsFix(t *testing.T) {
	w := newWatching(t)
	w.fake.PutWorkItems(item(1))
	w.fake.SavedQuery(queryID, 1)
	w.failing("PATCH", workItemPath(1), http.StatusForbidden, "VS402625: Dependent rules failed")
	w.start()
	w.status(core.WatcherWatching)
	w.nextPoll()
	got := w.status(core.WatcherError)
	if got.Problem != core.ProblemPATScope || !strings.Contains(got.Message, "ADO_PAT") || !strings.Contains(got.Fix, "Work Items: Read & write") {
		t.Fatalf("status %+v, want pat_scope naming ADO_PAT, with the Read & write scope as the fix", got)
	}
	if strings.Contains(got.Message+got.Fix, pat) {
		t.Fatalf("status %+v shows the PAT", got)
	}
	w.nextPoll()
	w.nextPoll()
	w.status(core.WatcherError)
	w.nextPoll()
	w.status(core.WatcherWatching)
	if got := w.tags(1); got != "code-central:draft" {
		t.Fatalf("tags %q, want the tag once the PAT could write", got)
	}
}

// Won't fix posts the reason and tags the work item code-central:wont-fix,
// and never moves its State.
func TestWontFixCommentsAndTagsTheWorkItemAndNeverChangesItsState(t *testing.T) {
	w := newWatching(t)
	w.arrived(1)
	ticket := w.ticketOf(1).ID
	if _, err := w.srv.Core.WontFix(human, "acme", core.WontFixInput{Ticket: core.Ref(ticket), Reason: "Not for us."}); err != nil {
		t.Fatal(err)
	}
	w.polled()
	if got, want := w.markdown(1), []string{w.comment(ticket, core.CommentWontFix, "Not for us.")}; !slices.Equal(got, want) {
		t.Fatalf("comments %q, want %q", got, want)
	}
	if got := w.tags(1); got != "code-central:wont-fix" {
		t.Fatalf("tags %q, want code-central:wont-fix", got)
	}
	if got := w.fake.WorkItem(1).State; got != "Approved" {
		t.Fatalf("State %q, want Approved: left for the human", got)
	}
	w.wantNoStateChange()
	from := len(w.fake.Requests())
	w.polled()
	if got := w.patches(from); len(got) != 0 || len(w.fake.Comments(1)) != 1 {
		t.Fatalf("patches %q and %d comments after the next poll, want nothing more written", got, len(w.fake.Comments(1)))
	}
}

// Won't fix without a reason only tags.
func TestWontFixWithoutAReasonOnlyTags(t *testing.T) {
	w := newWatching(t)
	w.arrived(1)
	if _, err := w.srv.Core.WontFix(human, "acme", core.WontFixInput{Ticket: core.Ref(w.ticketOf(1).ID)}); err != nil {
		t.Fatal(err)
	}
	w.polled()
	if got := w.fake.Comments(1); len(got) != 0 || w.tags(1) != "code-central:wont-fix" {
		t.Fatalf("comments %+v, tags %q, want the tag alone", got, w.tags(1))
	}
}

// A comment whose marker is already on the work item, posted before a
// crash that lost its report, is not posted again, and is reported
// written, so it is never due again.
func TestACommentAlreadyOnTheWorkItemIsNotPostedAgainAndIsReportedWritten(t *testing.T) {
	w := newWatching(t)
	w.arrived(1)
	spec := w.toRequirement(1, "Seen on every login.")
	posted := w.comment(spec, core.CommentContext, "Seen on every login.")
	w.fake.PutComment(1, "A human's note.")
	w.fake.PutComment(1, posted)
	w.polled()
	if got := w.fake.Comments(1); len(got) != 2 {
		t.Fatalf("comments %+v, want the two there already: the marked one not posted again", got)
	}
	for _, due := range w.watch().Writes {
		if len(due.Comments) != 0 {
			t.Fatalf("writes due %+v, want the comment reported written", due)
		}
	}
}

// A requirement's progress is posted as it comes due: the context, the
// grilling resolution and Spec body, each landed Ticket, and the final
// summary, each once, in order; the tag follows the work and the work
// item is never closed.
func TestARequirementsProgressIsPostedOnceAsMarkdownAndTheTagFollowsIt(t *testing.T) {
	w := newWatching(t)
	w.arrived(1)
	spec := w.toRequirement(1, "Seen on every login.")
	stages := w.stages(spec)
	w.polled()
	if got, want := w.markdown(1), []string{w.comment(spec, "context", "Seen on every login.")}; !slices.Equal(got, want) {
		t.Fatalf("comments %q, want the context once", got)
	}
	w.polled()
	if len(w.fake.Comments(1)) != 1 {
		t.Fatalf("comments %q after another poll, want the context only once", w.markdown(1))
	}

	w.workedDone(stages[0], "Use Postgres.", "")
	w.workedDone(stages[1], "", "")
	w.polled()
	got := w.markdown(1)
	if len(got) != 3 || got[1] != w.comment(spec, "grilling", "Use Postgres.") || !strings.HasPrefix(got[2], "<!-- code-central "+spec+" spec -->\n# ") {
		t.Fatalf("comments %q, want the grilling resolution, then the Spec's body", got)
	}

	fix := w.implementation(spec, "Fix it")
	w.workedDone(stages[2], "", "")
	w.polled()
	if got := w.tags(1); got != "code-central:building" {
		t.Fatalf("tags %q, want building once the Stages are Done", got)
	}

	w.workedDone(fix, "", "abc123")
	w.polled()
	got = w.markdown(1)
	landed := w.comment(spec, "landed:"+fix, fmt.Sprintf("Landed **Fix it** (%s) in abc123.", fix))
	closed := w.comment(spec, "closed", fmt.Sprintf("Finished. What landed:\n\n- **Fix it** (%s) in abc123", fix))
	if len(got) != 5 || got[3] != landed || got[4] != closed {
		t.Fatalf("comments %q, want the landed Ticket, then the final summary", got)
	}
	if got := w.tags(1); got != "code-central:done" {
		t.Fatalf("tags %q, want done", got)
	}
	if due := w.watch().Writes; len(due) != 1 || len(due[0].Comments) != 0 {
		t.Fatalf("writes due after = %+v, want only the tag", due)
	}
	if got := w.fake.WorkItem(1).State; got != "Approved" {
		t.Fatalf("State %q, want Approved: never changed", got)
	}
	w.wantNoStateChange()
}

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

// workedDone is ticket claimed by the agent, commented on with comment if
// it isn't empty, finished, and accepted to Done, or with a commit,
// landed with it.
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
			in.Recommendation = domain.RecommendSplit
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

// Under Hide Code Central, the Watcher posts each comment in the hidden
// form and makes no tag call: the code-central: tag already on the work
// item, and the human's own, stay as they are.
func TestUnderHideCodeCentralTheHiddenCommentIsPostedAndNoTagIsWritten(t *testing.T) {
	w := newWatching(t)
	w.setBoards(core.SetAzureBoardsInput{On: true, QueryURL: w.queryURL(queryID), HideCodeCentral: true})
	tagged := item(1)
	tagged.Tags = "frontend; code-central:draft"
	w.fake.PutWorkItems(tagged)
	w.fake.SavedQuery(queryID, 1)
	w.start()
	w.status(core.WatcherWatching)
	wantIncoming(t, w, 1)
	spec := w.toRequirement(1, "  Seen on every login.  ")
	w.polled()
	if got, want := w.markdown(1), []string{core.OpaqueMarker(spec, core.CommentContext) + "\nSeen on every login."}; !slices.Equal(got, want) {
		t.Fatalf("comments %q, want %q", got, want)
	}
	if got := w.patches(0); len(got) != 0 {
		t.Fatalf("patches %q, want no tag call", got)
	}
	if got := w.tags(1); got != "frontend; code-central:draft" {
		t.Fatalf("tags %q, want them left as they were", got)
	}
	if due := w.watch().Writes; len(due) != 0 {
		t.Fatalf("writes due after = %+v, want the comment reported written", due)
	}
	w.wantNoStateChange()
}

// A comment is taken as posted if the work item has either its branded
// or its opaque marker, whatever the setting, so a crash around a change
// of the setting never posts it twice.
func TestEitherMarkerOnTheWorkItemStopsTheCommentBeingPostedAgain(t *testing.T) {
	for _, hide := range []bool{false, true} {
		for name, marker := range map[string]func(target, key string) string{"branded": core.IssueMarker, "opaque": core.OpaqueMarker} {
			t.Run(fmt.Sprintf("%s marker, hidden %t", name, hide), func(t *testing.T) {
				w := newWatching(t)
				w.setBoards(core.SetAzureBoardsInput{On: true, QueryURL: w.queryURL(queryID), HideCodeCentral: hide})
				w.arrived(1)
				spec := w.toRequirement(1, "Seen on every login.")
				w.fake.PutComment(1, marker(spec, core.CommentContext)+"\nSeen on every login.")
				w.polled()
				if got := w.fake.Comments(1); len(got) != 1 {
					t.Fatalf("comments %+v, want the one there already, not posted again", got)
				}
				for _, due := range w.watch().Writes {
					if len(due.Comments) != 0 {
						t.Fatalf("writes due %+v, want the comment reported written", due)
					}
				}
			})
		}
	}
}
