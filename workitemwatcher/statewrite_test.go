package workitemwatcher_test

import (
	"net/http"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/internal/fakeazdo"
)

// inProgress is work item 1 a Requirement mapped as mapped, its tag
// written, then its first Stage claimed, so In Progress is due.
func (w *watching) inProgress() string {
	w.t.Helper()
	spec := w.requirement(mapped)
	w.claimed(w.stages(spec)[0])
	return spec
}

// A work item already in the State due, in any case, is not patched: the
// phase is just recorded as written.
func TestAWorkItemAlreadyInTheStateIsNotPatched(t *testing.T) {
	w := newWatching(t)
	spec := w.inProgress()
	moved := w.fake.WorkItem(1)
	moved.State, moved.Rev = "committed", moved.Rev+1
	w.fake.PutWorkItems(moved)
	w.polledSettingNone("it is in the State already")
	if got := w.stateDue(spec); got != nil {
		t.Fatalf("State due %+v, want it recorded as written", got)
	}
}

// A State is set on a work item the query no longer matches, as on one
// it does.
func TestTheStateIsSetOnAWorkItemOutOfTheQuery(t *testing.T) {
	w := newWatching(t)
	w.inProgress()
	w.fake.SavedQuery(queryID)
	w.polledSetting("Committed")
}

// Under Hide Code Central, the State is still set, and no tag is.
func TestUnderHideCodeCentralTheStateIsStillSet(t *testing.T) {
	w := newWatching(t)
	w.setBoards(api.SetAzureBoardsInput{On: true, QueryURL: w.queryURL(queryID), HideCodeCentral: true, WorkItemStates: mapped})
	w.arrived(1)
	spec := w.toRequirement(1, "")
	w.polled()
	w.claimed(w.stages(spec)[0])
	w.polledSetting("Committed")
	if got := w.statePatches(0); len(got) != len(w.patches(0)) {
		t.Fatalf("patches %q, want only the State's", w.patches(0))
	}
}

// Nothing is written to a deleted work item, and the Work Item Watcher
// keeps watching.
func TestNothingIsWrittenToADeletedWorkItem(t *testing.T) {
	w := newWatching(t)
	w.inProgress()
	w.fake.DeleteWorkItem(1)
	w.polledSettingNone("the work item is deleted")
	w.status(api.WatcherWatching)
}

// A State patch whose test on the revision fails changes nothing, and
// the next poll reads the work item again and sets it, with no error.
func TestAStateRevisionConflictIsRetriedOnTheNextPoll(t *testing.T) {
	w := newWatching(t)
	w.inProgress()
	stale := fakeazdo.JSON("PATCH", workItemPath(1), http.StatusPreconditionFailed, map[string]string{"message": "The work item has been updated."})
	stale.Times = 1
	w.fake.Script(stale)
	w.polled()
	if got := w.state(1); got != "Approved" {
		t.Fatalf("State %q, want Approved: the conflict changed nothing", got)
	}
	w.polledSetting("Committed")
	if w.errorCount() != 0 {
		t.Fatal("a revision conflict went into an error, want it retried quietly")
	}
}

// A State Azure DevOps refuses leaves the work item as it was, and is
// due again on the next poll.
func TestARefusedStateIsLeftDue(t *testing.T) {
	w := newWatching(t)
	w.fake.RefuseState("Committed", "TF401320: Rule Error for field State. Error code: Required, InvalidListValue.")
	spec := w.inProgress()
	w.polled()
	if got := w.state(1); got != "Approved" {
		t.Fatalf("State %q, want Approved: the State was refused", got)
	}
	if got := w.stateDue(spec); got == nil || got.Name != "Committed" {
		t.Fatalf("State due %+v, want Committed still due", got)
	}
}
