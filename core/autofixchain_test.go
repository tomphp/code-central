package core_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// Auto-fix chains its attempts (Spec code-central-1022): a later commit
// carrying the Tickets an open auto fix Ticket covers waits on it, the
// fix Ticket's own failed push is the next attempt, the chain stops at
// the Auto-fix limit, and cancelling hands the failure back.

// rideOnto reports commit pushed for each of ids, as the Factory does
// for Tickets the CI gate holds when it pushes a later commit.
func (f fixture) rideOnto(t *testing.T, commit string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		f.pushed(t, id, commit)
	}
}

// coveredWorld is itemWorld with Auto-fix on at limit, abc123 failed and
// acme-10 fixing it, and def456 failed carrying acme-5 and acme-8 with
// acme-6, covered by acme-10.
func coveredWorld(t *testing.T, limit int) fixture {
	t.Helper()
	f := itemWorld(t)
	f.autoFixOn(t, limit)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	f.moveTo(t, 6, domain.Accepted)
	f.rideOnto(t, "def456", "acme-5", "acme-8", "acme-6")
	f.ciRun(t, 2, "ci.yml", "def456", "acme-6", domain.CICompleted, domain.CIFailure)
	return f
}

func TestALaterCommitCarryingCoveredTicketsOpensNoSecondFixTicketAndStaysOutOfNeedsMe(t *testing.T) {
	f := coveredWorld(t, 3)

	f.noTicket(t, "acme-11")
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want def456 covered by acme-10", rows)
	}
	for _, id := range []string{"acme-5", "acme-8", "acme-6"} {
		tk := f.getTicket(t, id)
		if tk.NeedsMeReason != "" || tk.CIResult != api.CIFailed || tk.CICommit != "def456" {
			t.Errorf("%s: reason %q, result %q from %q, want failed at def456 and out of Needs Me", id, tk.NeedsMeReason, tk.CIResult, tk.CICommit)
		}
		want := &api.CIAutoFixView{Covering: &api.AutoFixCoveringView{FixTicket: "acme-10", Commit: "abc123"}}
		if !reflect.DeepEqual(tk.CIAutoFix, want) {
			t.Errorf("%s: Auto-fix %+v, want covered by acme-10 opened for abc123", id, tk.CIAutoFix)
		}
		if a, _ := f.core.AttentionOf(ctx, "acme", id); a.CIFailedCommit != "" || a.NeedsMe {
			t.Errorf("%s: attention %+v, want no Notification", id, a)
		}
	}
}

// fixPushFails accepts the auto fix Ticket fix and pushes commit for it,
// carrying riders, then reports its run id failed.
func (f fixture) fixPushFails(t *testing.T, fix int, id int64, commit string, riders ...string) {
	t.Helper()
	f.moveTo(t, fix, domain.Accepted)
	f.rideOnto(t, commit, append([]string{core.DisplayID("acme", fix)}, riders...)...)
	f.ciRun(t, id, "ci.yml", commit, core.DisplayID("acme", fix), domain.CICompleted, domain.CIFailure)
}

func TestTheFixTicketsOwnFailedPushAfterACoveredCommitIsAttemptTwo(t *testing.T) {
	f := coveredWorld(t, 3)
	f.fixPushFails(t, 10, 3, "ghi789", "acme-5", "acme-8", "acme-6")

	fix := f.getTicket(t, "acme-11")
	want := &api.AutoFixMarkView{Attempt: 2, Commit: "ghi789", Covers: []string{"acme-5", "acme-8", "acme-6", "acme-10"}, Previous: "acme-10"}
	if !reflect.DeepEqual(fix.AutoFix, want) {
		t.Fatalf("mark %+v, want %+v", fix.AutoFix, want)
	}
	f.noTicket(t, "acme-12")
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want ghi789 handled by acme-11", rows)
	}
	for _, id := range []string{"acme-5", "acme-6", "acme-10"} {
		got := f.getTicket(t, id).CIAutoFix
		if want := (&api.CIAutoFixView{Fixing: &api.AutoFixFixingView{FixTicket: "acme-11", Attempt: 2, Limit: 3}}); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: Auto-fix %+v, want acme-11 fixing it, attempt 2 of 3", id, got)
		}
	}
}

func TestAutoFixStopsAtTheLimitAndHandsTheItemBackWithEveryAttemptLinked(t *testing.T) {
	f := coveredWorld(t, 2)
	f.fixPushFails(t, 10, 3, "ghi789", "acme-5", "acme-8", "acme-6")
	f.fixPushFails(t, 11, 4, "jkl000", "acme-10", "acme-5", "acme-8", "acme-6")

	f.noTicket(t, "acme-12")
	row := f.theItem(t, api.ReasonCIFailed)
	if !row.FocusItem || row.CIItem.Commit != "jkl000" {
		t.Fatalf("row %+v, want jkl000 back as a Focus Item", row)
	}
	if want := []api.CIFailureOption{api.OptionRerunCI, api.OptionLandAnyway, api.OptionNewFixTicket}; !reflect.DeepEqual(row.CIItem.Options, want) {
		t.Errorf("options %v, want %v", row.CIItem.Options, want)
	}
	stopped := &api.AutoFixStoppedView{Attempts: []string{"acme-10", "acme-11"}}
	if !reflect.DeepEqual(row.CIItem.AutoFixStopped, stopped) {
		t.Errorf("item's Auto-fix %+v, want stopped after acme-10 and acme-11", row.CIItem.AutoFixStopped)
	}
	for _, id := range []string{"acme-5", "acme-11"} {
		tk := f.getTicket(t, id)
		if got := tk.CIAutoFix; !reflect.DeepEqual(got, &api.CIAutoFixView{Stopped: stopped}) {
			t.Errorf("%s: Auto-fix %+v, want stopped after acme-10 and acme-11", id, got)
		}
		if a, _ := f.core.AttentionOf(ctx, "acme", id); a.CIFailedCommit != "jkl000" || !a.NeedsMe {
			t.Errorf("%s: attention %+v, want a Notification for jkl000", id, a)
		}
	}
}

func TestRaisingTheLimitLetsAutoFixActAgainOnAStoppedChain(t *testing.T) {
	f := coveredWorld(t, 1)
	f.fixPushFails(t, 10, 3, "ghi789", "acme-5", "acme-8", "acme-6")
	f.theItem(t, api.ReasonCIFailed)

	f.autoFixOn(t, 2)
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want ghi789 handled again", rows)
	}
	fix := f.getTicket(t, "acme-11")
	if fix.AutoFix == nil || fix.AutoFix.Attempt != 2 || fix.AutoFix.Previous != "acme-10" || !strings.HasSuffix(fix.Description, "Auto-fix attempt 2 of 2.") {
		t.Fatalf("acme-11 mark %+v, want attempt 2 of 2 after acme-10", fix.AutoFix)
	}
}

func TestLoweringTheLimitAppliesOnTheNextFailure(t *testing.T) {
	f := coveredWorld(t, 3)
	f.autoFixOn(t, 1)
	if fix := f.getTicket(t, "acme-10"); fix.State != domain.Ready {
		t.Fatalf("acme-10 %s, want it carrying on", fix.State)
	}
	if got, want := f.getTicket(t, "acme-5").CIAutoFix, (&api.CIAutoFixView{Covering: &api.AutoFixCoveringView{FixTicket: "acme-10", Commit: "abc123"}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("acme-5 Auto-fix %+v, want still covered by acme-10", got)
	}

	f.fixPushFails(t, 10, 3, "ghi789", "acme-5", "acme-8", "acme-6")
	f.noTicket(t, "acme-11")
	if it := f.theItem(t, api.ReasonCIFailed).CIItem; !reflect.DeepEqual(it.AutoFixStopped, &api.AutoFixStoppedView{Attempts: []string{"acme-10"}}) {
		t.Fatalf("item's Auto-fix %+v, want stopped after acme-10", it.AutoFixStopped)
	}
}

func TestCancellingAnAutoFixTicketHandsItsFailureBackIncludingTheCoveredTicketsLaterCommits(t *testing.T) {
	f := coveredWorld(t, 3)
	if _, err := f.core.Cancel(human, "acme", api.CancelInput{Ticket: "acme-10"}); err != nil {
		t.Fatal(err)
	}
	f.noTicket(t, "acme-11")
	if it := f.theItem(t, api.ReasonCIFailed).CIItem; it.Commit != "def456" || it.AutoFixStopped != nil {
		t.Fatalf("item %+v, want def456 back with the human and no stopped line", it)
	}
	if a, _ := f.core.AttentionOf(ctx, "acme", "acme-5"); a.CIFailedCommit != "def456" {
		t.Fatalf("acme-5 attention %+v, want a Notification for def456", a)
	}

	// acme-5 and acme-8 ride onto a still later commit, which fails too.
	f.rideOnto(t, "ghi789", "acme-5", "acme-8", "acme-6")
	f.ciRun(t, 3, "ci.yml", "ghi789", "acme-5", domain.CICompleted, domain.CIFailure)
	f.noTicket(t, "acme-11")
	if it := f.theItem(t, api.ReasonCIFailed).CIItem; it.Commit != "ghi789" {
		t.Fatalf("item %+v, want ghi789 with the human", it)
	}
	if v := f.getTicket(t, "acme-5").CIAutoFix; v != nil {
		t.Fatalf("acme-5 Auto-fix %+v, want none once handed back", v)
	}
}

func TestAnAutoFixTicketDoneWhileItsCommitStillFailsHandsItBack(t *testing.T) {
	f := itemWorld(t)
	f.autoFixOn(t, 3)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	f.moveTo(t, 10, domain.Done)
	f.autoFixOn(t, 3)

	f.noTicket(t, "acme-11")
	if it := f.theItem(t, api.ReasonCIFailed).CIItem; it.Commit != "abc123" || it.AutoFixStopped != nil {
		t.Fatalf("item %+v, want abc123 back with the human", it)
	}
	if v := f.getTicket(t, "acme-5").CIAutoFix; v != nil {
		t.Fatalf("acme-5 Auto-fix %+v, want none once handed back", v)
	}
}

func TestEachFailureHasItsOwnAttemptBudget(t *testing.T) {
	f := itemWorld(t)
	f.autoFixOn(t, 1)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	f.fixPushFails(t, 10, 2, "def456", "acme-5", "acme-8")
	if it := f.theItem(t, api.ReasonCIFailed).CIItem; it.Commit != "def456" || it.AutoFixStopped == nil {
		t.Fatalf("item %+v, want def456 stopped", it)
	}

	// acme-6's own failure, carrying none of that chain's Tickets.
	f.moveTo(t, 6, domain.Accepted)
	f.pushed(t, "acme-6", "fff000")
	f.ciRun(t, 3, "ci.yml", "fff000", "acme-6", domain.CICompleted, domain.CIFailure)
	fix := f.getTicket(t, "acme-11")
	if want := (&api.AutoFixMarkView{Attempt: 1, Commit: "fff000", Covers: []string{"acme-6"}}); !reflect.DeepEqual(fix.AutoFix, want) {
		t.Fatalf("acme-11 mark %+v, want %+v", fix.AutoFix, want)
	}
}

func TestEachTicketOnACommitTwoChainsCoverNamesTheAutoFixTicketCoveringIt(t *testing.T) {
	f := itemWorld(t)
	f.autoFixOn(t, 3)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	f.moveTo(t, 6, domain.Accepted)
	f.pushed(t, "acme-6", "fff000")
	f.ciRun(t, 2, "ci.yml", "fff000", "acme-6", domain.CICompleted, domain.CIFailure)
	// acme-10 covers acme-5 and acme-8, acme-11 covers acme-6, and all
	// three ride onto def456.
	f.rideOnto(t, "def456", "acme-5", "acme-8", "acme-6")
	f.ciRun(t, 3, "ci.yml", "def456", "acme-6", domain.CICompleted, domain.CIFailure)

	f.noTicket(t, "acme-12")
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want def456 covered", rows)
	}
	for id, want := range map[string]api.AutoFixCoveringView{
		"acme-5": {FixTicket: "acme-10", Commit: "abc123"},
		"acme-8": {FixTicket: "acme-10", Commit: "abc123"},
		"acme-6": {FixTicket: "acme-11", Commit: "fff000"},
	} {
		if got := f.getTicket(t, id).CIAutoFix; got == nil || got.Covering == nil || *got.Covering != want {
			t.Errorf("%s: Auto-fix %+v, want covered by %+v", id, got, want)
		}
	}
}

func TestEachRepositoryHasItsOwnAutoFixChainAndLine(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.autoFixOn(t, 3)
	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	// acme-10 fixes acme@abc123; acme-5 failing in api is no later commit
	// carrying its failure.
	f.ciRunIn(t, "api", 2, "def456", "acme-5", domain.CICompleted, domain.CIFailure)
	if m := f.getTicket(t, "acme-11").AutoFix; m == nil || m.Repository != "api" || m.Attempt != 1 {
		t.Fatalf("acme-11 mark %+v, want attempt 1 in api", m)
	}
	// acme-5 rides onto a later Home commit, still carrying abc123's
	// failure.
	f.pushed(t, "acme-5", "ghi789")
	f.ciRun(t, 3, "ci.yml", "ghi789", "acme-5", domain.CICompleted, domain.CIFailure)
	f.noTicket(t, "acme-12")

	v := f.getTicket(t, "acme-5")
	home := &api.CIAutoFixView{Covering: &api.AutoFixCoveringView{FixTicket: "acme-10", Commit: "abc123"}}
	if got := changedIn(t, v, "acme").CIAutoFix; !reflect.DeepEqual(got, home) || !reflect.DeepEqual(v.CIAutoFix, home) {
		t.Errorf("Home Auto-fix %+v, Ticket's %+v, want covered by acme-10", got, v.CIAutoFix)
	}
	apiRepo := &api.CIAutoFixView{Fixing: &api.AutoFixFixingView{FixTicket: "acme-11", Attempt: 1, Limit: 3}}
	if got := changedIn(t, v, "api").CIAutoFix; !reflect.DeepEqual(got, apiRepo) {
		t.Errorf("api Auto-fix %+v, want acme-11 fixing it", got)
	}
}
