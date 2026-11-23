package workitemwatcher_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// mapped is the Work Item States the tests map, unless they say otherwise.
var mapped = api.WorkItemStates{InProgress: "Committed", Accepted: "Resolved", Done: "Done"}

// setStates is the human saving states as acme's Work Item States.
func (w *watching) setStates(states api.WorkItemStates) {
	w.t.Helper()
	w.setBoards(api.SetAzureBoardsInput{On: true, QueryURL: w.queryURL(queryID), WorkItemStates: states})
}

// state is work item id's State on the fake's board.
func (w *watching) state(id int) string { return w.fake.WorkItem(id).State }

// statePatches are the JSON Patches since request from that set a State.
func (w *watching) statePatches(from int) []string {
	var out []string
	for _, p := range w.patches(from) {
		if strings.Contains(p, "System.State") {
			out = append(out, p)
		}
	}
	return out
}

// restated is the JSON Patch that tests work item id's revision rev and
// sets its State to state.
func restated(id, rev int, state string) string {
	return fmt.Sprintf(`%s [{"op":"test","path":"/rev","value":%d},{"op":"add","path":"/fields/System.State","value":%q}]`, workItemPath(id), rev, state)
}

// polledSetting is a poll that must set work item 1's State to state by
// one JSON Patch testing the revision it was at, and nothing else.
func (w *watching) polledSetting(state string) {
	w.t.Helper()
	from := len(w.fake.Requests())
	w.polled()
	rev := w.fake.WorkItem(1).Rev - 1
	if got := w.statePatches(from); len(got) != 1 || got[0] != restated(1, rev, state) {
		w.t.Fatalf("State patches %q, want one to %s testing revision %d", got, state, rev)
	}
	if got := w.state(1); got != state {
		w.t.Fatalf("State %q, want %s", got, state)
	}
}

// polledSettingNone is a poll that must set no State.
func (w *watching) polledSettingNone(why string) {
	w.t.Helper()
	from := len(w.fake.Requests())
	w.polled()
	if got := w.statePatches(from); len(got) != 0 {
		w.t.Fatalf("State patches %q, want none: %s", got, why)
	}
}

// stateDue is the State due on Requirement spec's work item, nil when none is.
func (w *watching) stateDue(spec string) *api.WorkItemStateWrite {
	for _, due := range w.watch().Writes {
		if due.Target == spec {
			return due.State
		}
	}
	return nil
}

// claimed is ticket claimed by the agent, so In Progress.
func (w *watching) claimed(ticket string) {
	w.t.Helper()
	if _, err := w.srv.Core.Claim(agent, "acme", api.ClaimInput{Ticket: api.Ref(ticket)}); err != nil {
		w.t.Fatal(err)
	}
}

// requirement is work item 1 arrived with states mapped and taken To
// Requirement, then polled, so work from then on is after the mapping,
// returning its Spec.
func (w *watching) requirement(states api.WorkItemStates) string {
	w.t.Helper()
	w.setStates(states)
	w.arrived(1)
	spec := w.toRequirement(1, "")
	w.polled()
	return spec
}

// builtOut is every Stage of Requirement spec worked Done, with an
// implementation Ticket made before its Spec Stage, which it returns.
func (w *watching) builtOut(spec string) string {
	w.t.Helper()
	stages := w.stages(spec)
	w.workedDone(stages[0], "", "")
	w.workedDone(stages[1], "", "")
	fix := w.implementation(spec, "Fix it")
	w.workedDone(stages[2], "", "")
	return fix
}

// acceptedToLand is implementation ticket worked, finished on its branch
// and accepted, so Accepted until it lands.
func (w *watching) acceptedToLand(ticket string) {
	w.t.Helper()
	c, err := w.srv.Core.Claim(agent, "acme", api.ClaimInput{Ticket: api.Ref(ticket)})
	if err == nil {
		_, err = w.srv.Core.Finish(agent, "acme", api.FinishInput{Ticket: api.Ref(ticket), ClaimID: c.ClaimID, Summary: "Finished.", Branch: ticket})
	}
	if err == nil {
		_, err = w.srv.Core.Accept(human, "acme", api.TicketInput{Ticket: api.Ref(ticket)})
	}
	if err != nil {
		w.t.Fatal(err)
	}
}

// landed is Accepted ticket landed in commit.
func (w *watching) landed(ticket, commit string) {
	w.t.Helper()
	if _, err := w.srv.Core.Land(agent, "acme", api.LandInput{Ticket: api.Ref(ticket), Commit: commit}); err != nil {
		w.t.Fatal(err)
	}
}

// The first Stage of a Requirement going In Progress sets its work item's
// mapped In Progress State, once, by a JSON Patch that tests the revision.
func TestARequirementsFirstStageInProgressSetsTheInProgressState(t *testing.T) {
	w := newWatching(t)
	spec := w.requirement(mapped)
	w.polledSettingNone("no work has started")
	w.claimed(w.stages(spec)[0])
	w.polledSetting("Committed")
	w.polledSettingNone("it is set once")
	if w.errorCount() != 0 {
		t.Fatal("the Work Item Watcher went into an error")
	}
}

// Its work Accepted, then landed with the Spec Finished, sets the
// Accepted State, then the Done State, each once: a send-back, or a
// teammate moving the work item, never has a phase's State set again.
func TestARequirementsWorkSetsEachPhasesStateOnce(t *testing.T) {
	w := newWatching(t)
	spec := w.requirement(mapped)
	fix := w.builtOut(spec)
	w.polledSetting("Committed")
	w.acceptedToLand(fix)
	w.polledSetting("Resolved")

	moved := w.fake.WorkItem(1)
	moved.State, moved.Rev = "Active", moved.Rev+1
	w.fake.PutWorkItems(moved)
	if _, err := w.srv.Core.SendBack(human, "acme", api.SendBackInput{Ticket: api.Ref(fix), Note: "CI fails."}); err != nil {
		t.Fatal(err)
	}
	w.polledSettingNone("a send-back sets nothing")
	w.acceptedToLand(fix)
	w.polledSettingNone("Accepted was set once already")
	if got := w.state(1); got != "Active" {
		t.Fatalf("State %q, want the teammate's Active kept", got)
	}

	w.landed(fix, "abc123")
	w.polledSetting("Done")
	w.polledSettingNone("Done was set once")
	if got := w.tags(1); got != "code-central:done" {
		t.Fatalf("tags %q, want done with the Done State", got)
	}
}

// Work that reaches several phases between two polls has only the
// furthest one's State set, and the nearer ones recorded as written, so
// none of them is ever set.
func TestOnlyTheFurthestPhaseReachedIsSet(t *testing.T) {
	w := newWatching(t)
	spec := w.requirement(mapped)
	fix := w.builtOut(spec)
	w.acceptedToLand(fix)
	w.landed(fix, "abc123")
	want := &api.WorkItemStateWrite{Name: "Done", Key: domain.StateKeyDone, Nearer: []string{domain.StateKeyInProgress, domain.StateKeyAccepted}}
	if got := w.stateDue(spec); got == nil || got.Name != want.Name || got.Key != want.Key || !slices.Equal(got.Nearer, want.Nearer) {
		t.Fatalf("State due %+v, want %+v", got, want)
	}
	w.polledSetting("Done")
	if got := w.stateDue(spec); got != nil {
		t.Fatalf("State due %+v after it was set, want none", got)
	}
}

// A phase mapped to no State is passed over: nothing is set for it, and
// a further phase's State still is.
func TestAPhaseWithABlankMappingSetsNothing(t *testing.T) {
	w := newWatching(t)
	spec := w.requirement(api.WorkItemStates{Accepted: "Resolved"})
	fix := w.builtOut(spec)
	w.polledSettingNone("In Progress maps to no State")
	w.acceptedToLand(fix)
	w.polledSetting("Resolved")
	w.landed(fix, "abc123")
	w.polledSettingNone("Done maps to no State")
}

// A Requirement Finished with nothing landed, its implementation
// cancelled, is won't fix: no State is set for it.
func TestARequirementFinishedWithNothingLandedSetsNothing(t *testing.T) {
	w := newWatching(t)
	spec := w.requirement(api.WorkItemStates{Accepted: "Resolved", Done: "Done"})
	fix := w.builtOut(spec)
	if _, err := w.srv.Core.Cancel(human, "acme", api.CancelInput{Ticket: api.Ref(fix)}); err != nil {
		t.Fatal(err)
	}
	w.polledSettingNone("nothing landed")
	if got := w.tags(1); got != "code-central:wont-fix" {
		t.Fatalf("tags %q, want won't fix", got)
	}
}

// Work that reached a phase before its State was mapped is never moved
// for it: only a phase reached from then on is.
func TestWorkThatReachedAPhaseBeforeItWasMappedIsNotMoved(t *testing.T) {
	w := newWatching(t)
	spec := w.requirement(api.WorkItemStates{})
	fix := w.builtOut(spec)
	w.polled()
	w.setStates(mapped)
	w.polledSettingNone("In Progress was reached before it was mapped")
	w.acceptedToLand(fix)
	w.polledSetting("Resolved")
}
