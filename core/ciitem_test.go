package core_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// CI failed, CI cancelled and CI unavailable as one Needs Me item per
// pushed commit (code-central-432), through core's public operations,
// with one shared run per commit, as the real CI Watcher reports.

// itemWorld is landWorld under Merge + Push with CI Tracking, with acme-5
// (on Spec acme-1) and acme-8 (on Spec acme-7) Accepted a minute apart
// and abc123 pushed for both: the base's tip, containing both their work.
func itemWorld(t *testing.T) fixture {
	t.Helper()
	f := landWorld(t, false)
	f.landing(t, store.Landing{Method: store.LandingMerge, Push: true, Remote: "origin"})
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI = store.CI{Tracking: true, Provider: store.CIGitHubActions, Repository: "acme/web"}
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.spec(t, "acme", "Other")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-7", Title: "Third"})
	f.moveTo(t, 5, store.Accepted)
	f.clock.Advance(time.Minute)
	f.moveTo(t, 8, store.Accepted)
	f.pushed(t, "acme-5", "abc123")
	f.pushed(t, "acme-8", "abc123")
	return f
}

func notAllowed(t *testing.T, err error) {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.ActionNotAllowed {
		t.Fatalf("error %v, want action_not_allowed", err)
	}
}

// needsMe is Needs Me's rows, in order.
func (f fixture) needsMe(t *testing.T) []core.NeedsMeRow {
	t.Helper()
	v, err := f.core.NeedsMe(human)
	if err != nil {
		t.Fatal(err)
	}
	var rows []core.NeedsMeRow
	for _, g := range v.Groups {
		rows = append(rows, g.Tickets...)
	}
	return rows
}

// theItem is the one Needs Me row, which is a CI item with no Ticket.
func (f fixture) theItem(t *testing.T, reason core.NeedsMeReason) core.NeedsMeRow {
	t.Helper()
	rows := f.needsMe(t)
	if len(rows) != 1 || rows[0].Reason != reason || rows[0].ID != "" || rows[0].CIItem == nil {
		t.Fatalf("needs me %+v, want one %s item with no Ticket", rows, reason)
	}
	return rows[0]
}

func listed(v *core.CIItemView) []string {
	var ids []string
	for _, tk := range v.Tickets {
		ids = append(ids, tk.ID)
	}
	return ids
}

func TestCIFailedIsOneItemPerCommitListingTheTicketsWaitingOnItAcrossSpecs(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	row := f.theItem(t, core.ReasonCIFailed)
	if row.Project != "acme" || row.Title != "CI failed at abc123" || row.CIFailure == nil || row.CIFailure.Commit != "abc123" || row.CIResult != core.CIFailed {
		t.Fatalf("row %+v, want acme's CI failed at abc123 with its failure", row)
	}
	it := row.CIItem
	if it.Reason != core.ReasonCIFailed || it.Commit != "abc123" || !slices.Equal(listed(it), []string{"acme-5", "acme-8"}) {
		t.Fatalf("item %+v, want abc123 listing acme-5 then acme-8", it)
	}
	if it.Tickets[1].Spec != "acme-7" || it.Tickets[1].Title != "Third" {
		t.Fatalf("listed %+v, want acme-8 on acme-7 titled Third", it.Tickets[1])
	}
	if want := []string{"rerun_ci", "land_anyway", "new_fix_ticket"}; !slices.Equal(it.Options, want) {
		t.Fatalf("options %v, want %v with Send back to fix left out for two Tickets", it.Options, want)
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		tk := f.getTicket(t, id)
		if tk.NeedsMeReason != core.ReasonCIFailed || tk.CIResult != core.CIFailed || tk.CIItem == nil || tk.CIItem.Commit != "abc123" || !slices.Equal(tk.CIFailureOptions, it.Options) {
			t.Errorf("%s: reason %q, result %q, item %+v, options %v: want CI failed linking the item", id, tk.NeedsMeReason, tk.CIResult, tk.CIItem, tk.CIFailureOptions)
		}
	}
}

func TestAnItemListingOneTicketOffersSendBackUnlessItIsSpecLanded(t *testing.T) {
	f := itemWorld(t)
	f.moveTo(t, 8, store.Done)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	it := f.theItem(t, core.ReasonCIFailed).CIItem
	if want := []string{"rerun_ci", "land_anyway", "send_back", "new_fix_ticket"}; !slices.Equal(it.Options, want) || !slices.Equal(listed(it), []string{"acme-5"}) {
		t.Fatalf("item %+v, want acme-5 alone offered %v", it, want)
	}
	f.landing(t, store.Landing{Method: store.LandingMerge, Push: true, Remote: "origin", LandPerSpec: true})
	it = f.theItem(t, core.ReasonCIFailed).CIItem
	if want := []string{"rerun_ci", "land_anyway", "new_fix_ticket"}; !slices.Equal(it.Options, want) {
		t.Fatalf("options %v, want %v for a Spec-landed Ticket", it.Options, want)
	}
	_, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "acme-5", Note: "fix it"})
	notAllowed(t, err)
}

func TestSendBackIsRefusedWhileTheItemListsOtherTickets(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	for _, id := range []core.Ref{"acme-5", "acme-8"} {
		_, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: id, Note: "fix it"})
		notAllowed(t, err)
	}
	if tk := f.getTicket(t, "acme-5"); tk.State != store.Accepted {
		t.Fatalf("acme-5 is %s, want Accepted", tk.State)
	}
}

func TestLandAnywayOnAnyListedTicketIsStoredOnceForTheCommitAndLandsEveryListedTicket(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	if _, err := f.core.LandAnyway(human, "acme", core.TicketInput{Ticket: "acme-8"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		if tk := f.getTicket(t, id); !tk.LandAnyway || tk.NeedsMeReason != "" || tk.CIItem != nil {
			t.Errorf("%s: land anyway %v, reason %q, item %+v: want accepted with nothing waiting", id, tk.LandAnyway, tk.NeedsMeReason, tk.CIItem)
		}
	}
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want the item gone", rows)
	}
	f.store.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if len(p.CIResponses) != 1 || p.CIResponses[0].Commit != "abc123" || p.CIResponses[0].LandAnyway.IsZero() {
			t.Errorf("responses %+v, want one for abc123 with Land anyway", p.CIResponses)
		}
		for _, n := range []int{5, 8} {
			if tk, _ := r.Ticket("acme", n); !tk.PushedCommits[0].LandAnyway.IsZero() {
				t.Errorf("acme-%d's pushed commit carries Land anyway, want it on the Project only", n)
			}
		}
		return nil
	})
	// Choosing it again changes nothing.
	if _, err := f.core.LandAnyway(human, "acme", core.TicketInput{Ticket: "acme-5"}); err != nil {
		t.Fatal(err)
	}
}

func TestRerunOnAListedTicketReRunsTheCommitOnceForEveryListedTicket(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	if _, err := f.core.RerunCI(human, "acme", core.TicketInput{Ticket: "acme-5"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		if tk := f.getTicket(t, id); tk.CIResult != core.CIRunning || tk.NeedsMeReason != "" {
			t.Errorf("%s: result %q, reason %q, want running with no reason", id, tk.CIResult, tk.NeedsMeReason)
		}
	}
	w, err := f.core.GetCIWatch(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Reruns) != 1 || w.Reruns[0].Commit != "abc123" || w.Reruns[0].ID != 1 {
		t.Fatalf("reruns %+v, want run 1 on abc123 asked for once", w.Reruns)
	}
	// The new attempt passing lands both.
	f.ciAttempt(t, 1, 2, "ci.yml", "abc123", "acme-8", store.CICompleted, store.CISuccess)
	for _, id := range []string{"acme-5", "acme-8"} {
		if got := f.getTicket(t, id).CIResult; got != core.CIPassed {
			t.Errorf("%s: result %q, want passed", id, got)
		}
	}
}

func TestNewFixTicketOpensOneReadyTicketOnTheCIFixesSpecAndLeavesTheListedTicketsWaiting(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	if _, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-8", Note: "ci.yml failed"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		if tk := f.getTicket(t, id); tk.State != store.Accepted || tk.LandAnyway || tk.NeedsMeReason != "" || tk.CIResult != core.CIFailed {
			t.Errorf("%s: %s, land anyway %v, reason %q, result %q, want Accepted, failed and out of Needs Me", id, tk.State, tk.LandAnyway, tk.NeedsMeReason, tk.CIResult)
		}
	}
	fix := f.getTicket(t, "acme-10")
	if fix.State != store.Ready || fix.Spec.ID != "acme-9" {
		t.Fatalf("fix Ticket %s on %q, want Ready on the CI Fixes Spec acme-9", fix.State, fix.Spec.ID)
	}
	spec, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-9"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Title != "CI Fixes" || !slices.Contains(spec.Labels, core.LongLivedLabel) {
		t.Fatalf("CI Fixes Spec %q with labels %v, want it long-lived", spec.Title, spec.Labels)
	}
	f.store.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if len(p.CIResponses) != 1 || p.CIResponses[0].FixTicket != 10 || p.CIFixesSpec != 9 {
			t.Errorf("responses %+v, CI Fixes Spec %d, want the fix Ticket stored once on abc123's response", p.CIResponses, p.CIFixesSpec)
		}
		return nil
	})
	if n := len(f.getTicket(t, "acme-8").Comments); n != 1 {
		t.Fatalf("acme-8 has %d comments, want the one naming the fix Ticket", n)
	}
	if n := len(f.getTicket(t, "acme-5").Comments); n != 0 {
		t.Fatalf("acme-5 has %d comments, want none", n)
	}
	// The item stays out of Needs Me while the fix Ticket is open.
	if _, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-5", Note: "again"}); err == nil {
		t.Fatal("a second fix Ticket for the same commit was opened, want it refused while the first is open")
	}
}

func TestTheCIFailedItemReturnsWhenItsFixTicketIsCancelledOrDoneWhileTheCommitStillFails(t *testing.T) {
	for _, state := range []store.State{store.Cancelled, store.Done} {
		f := itemWorld(t)
		f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
		if _, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-8", Note: "ci.yml failed"}); err != nil {
			t.Fatal(err)
		}
		f.store.Update(func(w store.Writer) error {
			fix, _ := w.Ticket("acme", 10)
			fix.State = state
			return w.PutTicket("acme", fix)
		})
		if got := f.getTicket(t, "acme-5").NeedsMeReason; got != core.ReasonCIFailed {
			t.Errorf("fix %s: reason %q, want the item back while the commit still fails", state, got)
		}
	}
}

func TestCICancelledIsOneItemPerCommitWithNoNewFixTicket(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CICancelled)
	it := f.theItem(t, core.ReasonCICancelled).CIItem
	if want := []string{"rerun_ci", "land_anyway"}; !slices.Equal(it.Options, want) || !slices.Equal(listed(it), []string{"acme-5", "acme-8"}) {
		t.Fatalf("item %+v, want acme-5 and acme-8 offered %v", it, want)
	}
}

func TestCIUnavailableIsOneItemPerCommitWithNoOptions(t *testing.T) {
	f := itemWorld(t)
	f.clock.Advance(core.CIUnavailableAfter + time.Minute)
	row := f.theItem(t, core.ReasonCIUnavailable)
	if row.CIUnavailable == nil || row.CIUnavailable.Cause != core.CIWatcherNotWatching || row.CIResult != core.CIRunning {
		t.Fatalf("row %+v, want the CI Watcher not watching, with the commit running", row)
	}
	if it := row.CIItem; !slices.Equal(listed(it), []string{"acme-5", "acme-8"}) || len(it.Options) != 0 {
		t.Fatalf("item %+v, want acme-5 and acme-8 with no options", it)
	}
	if tk := f.getTicket(t, "acme-8"); tk.NeedsMeReason != core.ReasonCIUnavailable || tk.CIItem == nil || tk.CIFailureOptions != nil {
		t.Fatalf("acme-8: reason %q, item %+v, options %v", tk.NeedsMeReason, tk.CIItem, tk.CIFailureOptions)
	}
}

func TestTwoCommitsAreTwoItems(t *testing.T) {
	f := itemWorld(t)
	f.moveTo(t, 6, store.Accepted)
	f.pushed(t, "acme-6", "def456")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	f.ciRun(t, 2, "ci.yml", "def456", "acme-6", store.CICompleted, store.CIFailure)
	rows := f.needsMe(t)
	if len(rows) != 2 || rows[0].CIItem == nil || rows[1].CIItem == nil {
		t.Fatalf("needs me %+v, want two CI failed items", rows)
	}
	if a, b := listed(rows[0].CIItem), listed(rows[1].CIItem); !slices.Equal(a, []string{"acme-5", "acme-8"}) || !slices.Equal(b, []string{"acme-6"}) {
		t.Fatalf("items list %v and %v, want acme-5 and acme-8, then acme-6", a, b)
	}
}

func TestAResponseStoredOnATicketsPushedCommitBeforeIsStillHonoured(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	err := f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 5)
		tk.PushedCommits[0].LandAnyway = f.clock.Now()
		return w.PutTicket("acme", tk)
	})
	if err != nil {
		t.Fatal(err)
	}
	if tk := f.getTicket(t, "acme-5"); !tk.LandAnyway || tk.NeedsMeReason != "" {
		t.Fatalf("acme-5: land anyway %v, reason %q, want its own Land anyway honoured", tk.LandAnyway, tk.NeedsMeReason)
	}
	if it := f.theItem(t, core.ReasonCIFailed).CIItem; !slices.Equal(listed(it), []string{"acme-8"}) {
		t.Fatalf("item lists %v, want acme-8 alone", listed(it))
	}
	err = f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 8)
		tk.PushedCommits[0].Rerun = f.clock.Now()
		tk.PushedCommits[0].Reruns = []store.CIRunAttempt{{ID: 1, Attempt: 1}}
		return w.PutTicket("acme", tk)
	})
	if err != nil {
		t.Fatal(err)
	}
	if tk := f.getTicket(t, "acme-8"); tk.CIResult != core.CIRunning || tk.NeedsMeReason != "" {
		t.Fatalf("acme-8: result %q, reason %q, want running on its own Re-run request", tk.CIResult, tk.NeedsMeReason)
	}
	w, err := f.core.GetCIWatch(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Reruns) != 1 || w.Reruns[0].Ticket != "acme-8" {
		t.Fatalf("reruns %+v, want acme-8's", w.Reruns)
	}
}

func TestALaterCommitPassingClearsTheItemWithNoComment(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	f.theItem(t, core.ReasonCIFailed)
	f.pushed(t, "acme-5", "def456")
	f.pushed(t, "acme-8", "def456")
	f.ciRun(t, 2, "ci.yml", "def456", "acme-5", store.CICompleted, store.CISuccess)
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want nothing waiting", rows)
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		tk := f.getTicket(t, id)
		if tk.CIResult != core.CIPassed || tk.CICommit != "def456" || tk.NeedsMeReason != "" || len(tk.Comments) != 0 {
			t.Errorf("%s: result %q from %s, reason %q, %d comments: want passed at def456 with no comment", id, tk.CIResult, tk.CICommit, tk.NeedsMeReason, len(tk.Comments))
		}
	}
}

func TestLandAnywayAppliesToThatCommitOnly(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	if _, err := f.core.LandAnyway(human, "acme", core.TicketInput{Ticket: "acme-5"}); err != nil {
		t.Fatal(err)
	}
	f.pushed(t, "acme-5", "def456")
	f.pushed(t, "acme-8", "def456")
	f.ciRun(t, 2, "ci.yml", "def456", "acme-5", store.CICompleted, store.CIFailure)
	if it := f.theItem(t, core.ReasonCIFailed).CIItem; it.Commit != "def456" || !slices.Equal(listed(it), []string{"acme-5", "acme-8"}) {
		t.Fatalf("item %+v, want def456 listing both again", it)
	}
	if tk := f.getTicket(t, "acme-5"); tk.LandAnyway {
		t.Fatal("acme-5 reads Land anyway for def456, want abc123's only")
	}
}
