package core_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Auto-fix opening fix Tickets (Spec code-central-1022): with Auto-fix
// on, a CI failed item that offers New fix Ticket gets one fix Ticket
// on the CI Fixes Spec, opened by Code Central, and never reaches Needs
// Me.

// autoFixOn turns Auto-fix on in f with limit and the default Fix
// instructions.
func (f fixture) autoFixOn(t *testing.T, limit int) {
	t.Helper()
	if _, err := f.core.SetAutoFix(human, "acme", core.SetAutoFixInput{On: true, FixInstructions: domain.DefaultFixInstructions, Limit: limit}); err != nil {
		t.Fatalf("set_auto_fix: %v", err)
	}
}

func TestAutoFixOpensOneFixTicketForAnItemListingManyTicketsAndKeepsItOutOfNeedsMe(t *testing.T) {
	f := itemWorld(t)
	f.autoFixOn(t, 3)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)

	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want the item handled by Auto-fix", rows)
	}
	fix := f.getTicket(t, "acme-10")
	if fix.State != domain.Ready || fix.Spec.ID != "acme-9" || fix.Title != "Fix the CI failure of acme-5: First" {
		t.Fatalf("fix Ticket %q %s on %q, want Ready on the CI Fixes Spec acme-9 naming acme-5", fix.Title, fix.State, fix.Spec.ID)
	}
	if _, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "acme-11"}); err == nil {
		t.Fatal("acme-11 exists, want one fix Ticket for the item")
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		if tk := f.getTicket(t, id); tk.State != domain.Accepted || tk.NeedsMeReason != "" || tk.CIResult != core.CIFailed {
			t.Errorf("%s: %s, reason %q, result %q, want Accepted, failed and out of Needs Me", id, tk.State, tk.NeedsMeReason, tk.CIResult)
		}
	}
}

func TestAnAutoFixTicketCarriesItsMarkTheRunSummaryAndTheAttemptAndItsCommentIsCodeCentrals(t *testing.T) {
	f := itemWorld(t)
	f.autoFixOn(t, 3)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)

	fix := f.getTicket(t, "acme-10")
	want := &core.AutoFixMarkView{Attempt: 1, Commit: "abc123", Covers: []string{"acme-5", "acme-8"}}
	if !reflect.DeepEqual(fix.AutoFix, want) {
		t.Fatalf("mark %+v, want %+v", fix.AutoFix, want)
	}
	for _, part := range []string{
		"acme-5's CI failed: fix the failure forward",
		"CI failed at abc123:\n\n- [ci.yml](https://github.com/acme/web/actions/runs/1) (ci.yml)",
		"`gh run view 1 --log-failed`",
		domain.DefaultFixInstructions + "\n\nAuto-fix attempt 1 of 3.",
	} {
		if !strings.Contains(fix.Description, part) {
			t.Errorf("description lacks %q:\n%s", part, fix.Description)
		}
	}
	comments := f.getTicket(t, "acme-5").Comments
	if len(comments) != 1 || comments[0].Actor != string(core.Integration) ||
		comments[0].Body != "New fix Ticket: acme-10 fixes this CI failure forward; acme-5 stays Accepted and lands when a later commit passes CI." {
		t.Fatalf("acme-5 comments %+v, want the manual wording as integration", comments)
	}
	if n := len(f.getTicket(t, "acme-8").Comments); n != 0 {
		t.Fatalf("acme-8 has %d comments, want none", n)
	}
}

func TestAManualFixTicketHasNoMark(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	if _, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-5", Note: "ci.yml failed"}); err != nil {
		t.Fatal(err)
	}
	if fix := f.getTicket(t, "acme-10"); fix.AutoFix != nil {
		t.Fatalf("mark %+v, want none on a manual fix Ticket", fix.AutoFix)
	}
	if c := f.getTicket(t, "acme-5").Comments; len(c) != 1 || c[0].Actor != string(core.Human) {
		t.Fatalf("comments %+v, want the human's", c)
	}
}

// noTicket fails unless the Ticket id doesn't exist.
func (f fixture) noTicket(t *testing.T, id core.Ref) {
	t.Helper()
	if v, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: id}); err == nil {
		t.Fatalf("%s exists (%q), want no fix Ticket", id, v.Title)
	}
}

func TestAutoFixIsOffByDefaultSoTheItemWaitsInNeedsMe(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	f.theItem(t, core.ReasonCIFailed)
	f.noTicket(t, "acme-9")
}

func TestTurningAutoFixOnActsOnTheItemsAlreadyWaiting(t *testing.T) {
	f := itemWorld(t)
	f.moveTo(t, 6, domain.Accepted)
	f.pushed(t, "acme-6", "def456")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	f.ciRun(t, 2, "ci.yml", "def456", "acme-6", domain.CICompleted, domain.CIFailure)
	if rows := f.needsMe(t); len(rows) != 2 {
		t.Fatalf("needs me %+v, want two CI failed items", rows)
	}
	f.autoFixOn(t, 3)
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want both items handled", rows)
	}
	got := map[string][]string{}
	for _, id := range []string{"acme-10", "acme-11"} {
		fix := f.getTicket(t, id)
		got[fix.AutoFix.Commit] = fix.AutoFix.Covers
	}
	if want := map[string][]string{"abc123": {"acme-5", "acme-8"}, "def456": {"acme-6"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fix Tickets cover %v, want %v", got, want)
	}
	f.noTicket(t, "acme-12")
}

func TestAManualFixTicketLeavesTheCommitToTheHumanWhetherOpenDoneOrCancelled(t *testing.T) {
	for _, state := range []domain.State{domain.Ready, domain.Done, domain.Cancelled} {
		f := itemWorld(t)
		f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
		if _, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-5", Note: "ci.yml failed"}); err != nil {
			t.Fatal(err)
		}
		f.store.Update(func(w store.Writer) error {
			fix, _ := w.Ticket("acme", 10)
			fix.State = state
			return w.PutTicket("acme", fix)
		})
		f.autoFixOn(t, 3)
		f.noTicket(t, "acme-11")
		if state != domain.Ready {
			f.theItem(t, core.ReasonCIFailed)
		}
	}
}

func TestTurningAutoFixOffStopsItOpeningAndLeavesItsFixTicketsOpen(t *testing.T) {
	f := itemWorld(t)
	f.autoFixOn(t, 3)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	if _, err := f.core.SetAutoFix(human, "acme", core.SetAutoFixInput{FixInstructions: domain.DefaultFixInstructions, Limit: 3}); err != nil {
		t.Fatal(err)
	}
	if fix := f.getTicket(t, "acme-10"); fix.State != domain.Ready {
		t.Fatalf("fix Ticket %s, want it carrying on", fix.State)
	}
	f.moveTo(t, 6, domain.Accepted)
	f.pushed(t, "acme-6", "def456")
	f.ciRun(t, 2, "ci.yml", "def456", "acme-6", domain.CICompleted, domain.CIFailure)
	f.noTicket(t, "acme-11")
	if it := f.theItem(t, core.ReasonCIFailed).CIItem; it.Commit != "def456" {
		t.Fatalf("item %+v, want def456 waiting on the human", it)
	}
}

func TestAutoFixLeavesCICancelledAndCIUnavailableToTheHuman(t *testing.T) {
	f := itemWorld(t)
	f.autoFixOn(t, 3)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CICancelled)
	f.theItem(t, core.ReasonCICancelled)

	f = itemWorld(t)
	f.autoFixOn(t, 3)
	f.clock.Advance(core.CIUnavailableAfter + time.Minute)
	f.pushed(t, "acme-5", "def456")
	f.theItem(t, core.ReasonCIUnavailable)
	f.noTicket(t, "acme-9")
}

func TestAutoFixLeavesAClosedPullRequestToTheHuman(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.withCITracking(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: domain.PRClosed})
	f.autoFixOn(t, 3)
	wantReason(t, f.get(t, "2"), core.ReasonPullRequestClosed)
	f.noTicket(t, "acme-3")
}

func TestAutoFixOpensNothingForACIRulesFailure(t *testing.T) {
	f := itemWorld(t)
	f.autoFixOn(t, 3)
	if _, err := f.core.AddCIRule(human, "acme", core.CIRuleInput{Workflow: "nightly.yml", Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	started := f.clock.Now()
	run := core.CIRunView{ID: 7, Attempt: 1, Workflow: "nightly.yml", Name: "Nightly", Branch: "main", Commit: "fff000",
		Status: domain.CICompleted, Conclusion: domain.CIFailure, Started: &started, Finished: &started, URL: "https://github.com/acme/web/actions/runs/7", Rule: "acme-9"}
	if _, err := f.core.ReportCIRuns(ctx, "acme", core.ReportCIRunsInput{Runs: []core.CIRunView{run}}); err != nil {
		t.Fatal(err)
	}
	f.noTicket(t, "acme-10")
}

func TestConcurrentReportsOfTheSameFailureOpenOneFixTicket(t *testing.T) {
	f := itemWorld(t)
	f.autoFixOn(t, 3)
	started := f.clock.Now()
	run := core.CIRunView{ID: 1, Attempt: 1, Workflow: "ci.yml", Name: "ci.yml", Branch: "main", Commit: "abc123",
		Status: domain.CICompleted, Conclusion: domain.CIFailure, Started: &started, Finished: &started, URL: "https://github.com/acme/web/actions/runs/1", Ticket: "acme-5"}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := f.core.ReportCIRuns(ctx, "acme", core.ReportCIRunsInput{Runs: []core.CIRunView{run}}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if fix := f.getTicket(t, "acme-10"); fix.AutoFix == nil || fix.AutoFix.Commit != "abc123" {
		t.Fatalf("acme-10 mark %+v, want the auto fix Ticket for abc123", fix.AutoFix)
	}
	f.noTicket(t, "acme-11")
	if n := len(f.getTicket(t, "acme-5").Comments); n != 1 {
		t.Fatalf("acme-5 has %d comments, want one", n)
	}
}

func TestAnAutoFixTicketLeftUnrecordedByACrashIsRecordedNotOpenedAgain(t *testing.T) {
	f := itemWorld(t)
	// A crash between the fix Ticket's write and its response's leaves
	// acme-9, marked for abc123, that no response names.
	err := f.store.Update(func(w store.Writer) error {
		return w.PutTicket("acme", domain.Ticket{Number: w.NextNumber("acme"), Title: "Fix the CI failure of acme-5: First", State: domain.Ready, Spec: 1,
			Created: f.clock.Now(), Updated: f.clock.Now(), AutoFix: &domain.AutoFixMark{Attempt: 1, Commit: "abc123", Covers: []int{5, 8}}})
	})
	if err != nil {
		t.Fatal(err)
	}
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	f.autoFixOn(t, 3)

	f.noTicket(t, "acme-10")
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want the item handled by acme-9", rows)
	}
	f.store.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if len(p.CIResponses) != 1 || p.CIResponses[0].Commit != "abc123" || p.CIResponses[0].FixTicket != 9 {
			t.Errorf("responses %+v, want acme-9 recorded on abc123's", p.CIResponses)
		}
		return nil
	})
	if c := f.getTicket(t, "acme-5").Comments; len(c) != 1 || !strings.HasPrefix(c[0].Body, "New fix Ticket: acme-9 fixes") || c[0].Actor != string(core.Integration) {
		t.Fatalf("acme-5 comments %+v, want Auto-fix's naming acme-9", c)
	}
}
