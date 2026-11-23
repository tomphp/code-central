package workitemwatcher_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/internal/fakeazdo"
)

// ruleError is Azure DevOps refusing a State by a workflow rule.
const ruleError = "TF401320: Rule Error for field State. Error code: Required, InvalidListValue."

// requirements is work items 1 and 2 arrived with mapped, each taken To
// Requirement, then polled, returning their Specs.
func (w *watching) requirements() (one, two string) {
	w.t.Helper()
	w.setStates(mapped)
	w.arrived(1, 2)
	one, two = w.toRequirement(1, ""), w.toRequirement(2, "")
	w.polled()
	return one, two
}

// refused waits for the Work Item Watcher's state_refused error, which
// must name each of names, and returns it.
func (w *watching) refused(names ...string) api.WatcherView {
	w.t.Helper()
	got := w.status(api.WatcherError)
	if got.Problem != api.ProblemStateRefused {
		w.t.Fatalf("status %+v, want state_refused", got)
	}
	for _, name := range names {
		if !strings.Contains(got.Message, name) {
			w.t.Fatalf("message %q, want it to name %s", got.Message, name)
		}
	}
	return got
}

// A State Azure DevOps refuses leaves the work item as it was, and is an
// error naming the work item, the State and Azure DevOps' reason, with
// its fix. The comments and tag are still written, and the State is
// tried again each poll, still in error, with one Notification.
func TestARefusedStateIsAnErrorNamingTheWorkItemTheStateAndWhy(t *testing.T) {
	w := newWatching(t)
	w.fake.RefuseState("Done", ruleError)
	spec := w.requirement(mapped)
	w.workedDone(w.builtOut(spec), "", "abc123")
	w.polled()
	got := w.refused("work item 1", spec, "Done", ruleError)
	for _, fix := range []string{"Correct or blank Done in Work Item States", "Azure DevOps"} {
		if !strings.Contains(got.Fix, fix) {
			t.Fatalf("fix %q, want it to say %s", got.Fix, fix)
		}
	}
	if got := w.state(1); got != "Approved" {
		t.Fatalf("State %q, want Approved: the State was refused", got)
	}
	if got := w.tags(1); got != "code-central:done" {
		t.Fatalf("tags %q, want done written", got)
	}
	if got := w.markdown(1); !strings.Contains(got[len(got)-1], "Finished. What landed") {
		t.Fatalf("comments %q, want the final summary written", got)
	}
	if got := w.stateDue(spec); got == nil || got.Name != "Done" {
		t.Fatalf("State due %+v, want Done still due", got)
	}

	for range 2 {
		from := len(w.fake.Requests())
		w.polled()
		if got := w.statePatches(from); len(got) != 1 {
			t.Fatalf("State patches %q, want the refused State tried again", got)
		}
		w.refused()
	}
	if n := w.errorCount(); n != 1 {
		t.Fatalf("went into an error %d times, want once, for one Notification", n)
	}
}

// The error clears on the first poll with no refusal: the State was set,
// or the work item was moved to it already.
func TestARefusedStateClearsOnThePollThatRefusesNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		fix  func(w *watching)
	}{
		{"allowed in Azure DevOps", func(w *watching) { w.fake.AllowState("Committed") }},
		{"moved to it by a teammate", func(w *watching) {
			moved := w.fake.WorkItem(1)
			moved.State, moved.Rev = "Committed", moved.Rev+1
			w.fake.PutWorkItems(moved)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWatching(t)
			w.fake.RefuseState("Committed", ruleError)
			spec := w.inProgress()
			w.polled()
			w.refused()
			c.fix(w)
			w.polled()
			w.status(api.WatcherWatching)
			if got := w.state(1); got != "Committed" {
				t.Fatalf("State %q, want Committed", got)
			}
			if got := w.stateDue(spec); got != nil {
				t.Fatalf("State due %+v, want none", got)
			}
		})
	}
}

// Correcting or blanking the refused mapping clears the error at once,
// and the next poll sets the corrected State, or none.
func TestChangingTheRefusedMappingClearsTheErrorAtOnce(t *testing.T) {
	for _, c := range []struct {
		name   string
		states api.WorkItemStates
		want   string
	}{
		{"corrected", api.WorkItemStates{InProgress: "Active", Accepted: mapped.Accepted, Done: mapped.Done}, "Active"},
		{"blanked", api.WorkItemStates{Accepted: mapped.Accepted, Done: mapped.Done}, "Approved"},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWatching(t)
			w.fake.RefuseState("Committed", ruleError)
			w.inProgress()
			w.polled()
			w.refused()
			w.setStates(c.states)
			if got := w.settings().Watcher; got.State == api.WatcherError {
				t.Fatalf("status %+v once the mapping changed, want the error cleared", got)
			}
			w.polled()
			w.status(api.WatcherWatching)
			if got := w.state(1); got != c.want {
				t.Fatalf("State %q, want %s", got, c.want)
			}
		})
	}
}

// A refused State doesn't stop the other work items' States being set,
// and the error names only the work item refused.
func TestARefusedStateLeavesOtherWorkItemsStatesSet(t *testing.T) {
	w := newWatching(t)
	w.fake.RefuseState("Committed", ruleError)
	one, two := w.requirements()
	w.claimed(w.stages(one)[0])
	fix := w.builtOut(two)
	w.acceptedToLand(fix)
	w.polled()
	got := w.refused("work item 1", "Committed")
	if strings.Contains(got.Message, "work item 2") {
		t.Fatalf("message %q names work item 2, whose State was set", got.Message)
	}
	if got := w.state(2); got != "Resolved" {
		t.Fatalf("work item 2's State %q, want Resolved", got)
	}
}

// Each work item whose State is refused is named.
func TestEachRefusedWorkItemIsNamed(t *testing.T) {
	w := newWatching(t)
	w.fake.RefuseState("Committed", ruleError)
	one, two := w.requirements()
	w.claimed(w.stages(one)[0])
	w.claimed(w.stages(two)[0])
	w.polled()
	w.refused("work item 1", one, "work item 2", two)
}

// A rate limit or a missing scope later in the poll that refused a State
// keeps its own status: Watching, rate-limited, or pat_scope.
func TestARateLimitOrMissingScopeAfterARefusalKeepsItsOwnStatus(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer fakeazdo.Answer
		want   func(w *watching)
	}{
		{"rate limit", fakeazdo.JSON("PATCH", workItemPath(2), http.StatusTooManyRequests, map[string]string{"message": "slow down"}).WithHeader("Retry-After", "600"),
			func(w *watching) {
				w.rateLimitedUntil(w.srv.Clock.Now().Add(10 * time.Minute))
				if n := w.errorCount(); n != 0 {
					w.t.Fatalf("went into an error %d times, want none", n)
				}
			}},
		{"missing scope", fakeazdo.JSON("PATCH", workItemPath(2), http.StatusForbidden, map[string]string{"message": "VS402625"}),
			func(w *watching) {
				if got := w.status(api.WatcherError); got.Problem != api.ProblemPATScope {
					w.t.Fatalf("status %+v, want pat_scope", got)
				}
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWatching(t)
			w.fake.RefuseState("Committed", ruleError)
			one, two := w.requirements()
			w.claimed(w.stages(one)[0])
			w.claimed(w.stages(two)[0])
			c.answer.Times = 1
			w.fake.Script(c.answer)
			from := len(w.fake.Requests())
			w.polled()
			if got := w.statePatches(from); len(got) != 2 || !strings.HasPrefix(got[0], workItemPath(1)+" ") || !strings.HasPrefix(got[1], workItemPath(2)+" ") {
				t.Fatalf("State patches %q, want work item 1's refused before work item 2's write failed", got)
			}
			c.want(w)
		})
	}
}
