package core_test

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The Needs Me count is the number of things in Needs Me as the human
// sees it (Spec code-central-568), given for all Projects and per
// Project.

func wantCounts(t *testing.T, f fixture, count int, counts map[string]int) {
	t.Helper()
	v, err := f.core.NeedsMe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Count != count || !maps.Equal(v.Counts, counts) {
		t.Fatalf("count %d, counts %v, want %d, %v", v.Count, v.Counts, count, counts)
	}
}

func TestARequirementsStageTicketsCountOnceAsTheRequirement(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Toggle", State: store.Ready}) // acme-6
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", State: store.Ready})   // acme-7
	for _, n := range []int{3, 4, 5, 6, 7} {
		f.setState(t, n, store.InReview)
	}
	if rows := f.needsMe(t); len(rows) != 5 {
		t.Fatalf("needs_me has %d rows, want the 3 Stage Tickets, the build Ticket and the plain one", len(rows))
	}
	// The Requirement once, its build Ticket and the plain Ticket.
	wantCounts(t, f, 3, map[string]int{"acme": 3})
}

func TestACIItemListingSeveralTicketsCountsOnce(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	f.theItem(t, core.ReasonCIFailed)
	wantCounts(t, f, 1, map[string]int{"acme": 1})
}

func TestBaseRowsCountOnceEach(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "a", Remote: "b"})
	wantCounts(t, f, 1, map[string]int{"acme": 1})
	f.reportBase(t, core.ReportBaseInput{State: core.BaseMissing})
	wantCounts(t, f, 1, map[string]int{"acme": 1})
	f.spec(t, "acme", "Login")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", State: store.Ready})
	f.setState(t, 3, store.InReview)
	wantCounts(t, f, 2, map[string]int{"acme": 2})
}

func TestCountsAddUpToCountAndLeaveOutAProjectWithNothingWaiting(t *testing.T) {
	f := needsMeWorld(t)
	f.project(t, "quiet")
	v, err := f.core.NeedsMe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0
	for _, n := range v.Counts {
		sum += n
	}
	if sum != v.Count || v.Count != 7 {
		t.Fatalf("counts %v sum to %d, count %d, want 7 for both", v.Counts, sum, v.Count)
	}
	if _, ok := v.Counts["quiet"]; ok {
		t.Fatalf("counts %v include a Project with nothing waiting", v.Counts)
	}
	if v.Counts["acme"] == 0 || v.Counts["web"] == 0 {
		t.Fatalf("counts %v, want acme and web", v.Counts)
	}
}

func TestNeedsMeIsEmptyCountsNoProject(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	wantCounts(t, f, 0, map[string]int{})
}

func TestFindTicketsRowsCarryTheReasonGetTicketGives(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	// acme-5 and acme-8 are listed by the CI item.
	for _, id := range []string{"acme-5", "acme-8"} {
		wantEverywhere(t, f, "acme", id, core.ReasonCIFailed)
	}
	rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{State: store.States})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if tk := f.getTicket(t, r.ID); tk.NeedsMeReason != r.NeedsMeReason {
			t.Errorf("%s: row reason %q, get_ticket %q", r.ID, r.NeedsMeReason, tk.NeedsMeReason)
		}
	}
}

func TestTurningAReasonOffMarksItsRowsNotFocusItemsAndChangesNothingElse(t *testing.T) {
	f := needsMeWorld(t)
	focus := func() map[core.NeedsMeReason][]bool {
		out := map[core.NeedsMeReason][]bool{}
		for _, row := range f.needsMe(t) {
			if row.Project == "acme" {
				out[row.Reason] = append(out[row.Reason], row.FocusItem)
			}
		}
		return out
	}
	for reason, flags := range focus() {
		if slices.Contains(flags, false) {
			t.Fatalf("%s has a row that isn't a Focus Item while every reason is on", reason)
		}
	}
	reasons := func() map[string]core.NeedsMeReason {
		rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{State: store.States})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]core.NeedsMeReason{}
		for _, r := range rows {
			out[r.ID] = r.NeedsMeReason
		}
		return out
	}
	before, beforeView, beforeReasons, beforeAttention := f.needsMe(t), mustNeedsMe(t, f), reasons(), f.core.Attention(ctx)

	if _, err := f.core.SetFocusKinds(human, "acme", core.SetFocusKindsInput{Off: []core.NeedsMeReason{core.ReasonAnswered}}); err != nil {
		t.Fatal(err)
	}
	got := focus()
	if flags := got[core.ReasonAnswered]; len(flags) == 0 || slices.Contains(flags, true) {
		t.Fatalf("Answered rows' focus_item %v, want all false", flags)
	}
	for reason, flags := range got {
		if reason != core.ReasonAnswered && slices.Contains(flags, false) {
			t.Fatalf("%s rows' focus_item %v, want true: only Answered is off", reason, flags)
		}
	}
	if v := mustNeedsMe(t, f); v.Count != beforeView.Count || !maps.Equal(v.Counts, beforeView.Counts) || len(f.needsMe(t)) != len(before) {
		t.Errorf("count %d %v, was %d %v: Focus Kinds filter nothing", v.Count, v.Counts, beforeView.Count, beforeView.Counts)
	}
	if !maps.Equal(reasons(), beforeReasons) {
		t.Errorf("find_tickets reasons changed with Focus Kinds")
	}
	if !reflect.DeepEqual(f.core.Attention(ctx), beforeAttention) {
		t.Errorf("Attention changed with Focus Kinds")
	}
	// Other Projects' rows are unaffected, and find_tickets rows carry no
	// focus_item at all.
	for _, row := range f.needsMe(t) {
		if row.Project == "web" && !row.FocusItem {
			t.Errorf("web row %+v lost its Focus Item flag", row)
		}
	}
}

func mustNeedsMe(t *testing.T, f fixture) core.NeedsMeView {
	t.Helper()
	v, err := f.core.NeedsMe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
