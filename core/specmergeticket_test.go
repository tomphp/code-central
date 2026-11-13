package core_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Spec merge Tickets (code-central-542): the Factory's open_spec_merge,
// through core's public operations.

const specMergeTitle = "Merge the base into the Spec Branch of acme-1"

// specMergeWorld is acme with a plain Spec acme-1 holding one Ready
// implementation Ticket, acme-2.
func specMergeWorld(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Offline mode")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "First", State: store.Ready})
	return f
}

func (f fixture) openSpecMerge(t *testing.T, conflicts ...string) core.OpenSpecMergeResult {
	t.Helper()
	res, err := f.core.OpenSpecMerge(ctx, "acme", core.OpenSpecMergeInput{Spec: "acme-1", Conflicts: conflicts, BaseTip: "aaa111", SpecBranchTip: "bbb222"})
	if err != nil {
		t.Fatalf("open_spec_merge: %v", err)
	}
	return res
}

func TestOpenSpecMergeOpensAReadyMarkedTicketWithTheConflictInItsDescription(t *testing.T) {
	f := specMergeWorld(t)

	res := f.openSpecMerge(t, "README", "go.mod")

	if res != (core.OpenSpecMergeResult{Ticket: "acme-3", Outcome: core.SpecMergeOpened}) {
		t.Fatalf("result %+v, want acme-3 opened", res)
	}
	tk := f.getTicket(t, "acme-3")
	want := "The Spec Branch spec/acme-1 conflicts with the base, main, in these files:\n\n- README\n- go.mod\n\n" +
		"The base's tip is aaa111 and the Spec Branch's tip is bbb222. Merge the base into this branch and resolve the conflicts.\n"
	if tk.Title != specMergeTitle || tk.State != store.Ready || tk.Spec.ID != "acme-1" || tk.Description != want || !tk.SpecMergeTicket {
		t.Errorf("opened %+v, want a Ready marked Ticket on acme-1 titled %q describing the conflict", tk, specMergeTitle)
	}
}

func TestOpenSpecMergeCommentsOnTheOpenMarkedTicketAndWritesNothingOnARepeat(t *testing.T) {
	f := specMergeWorld(t)
	f.openSpecMerge(t, "README")

	if res := f.openSpecMerge(t, "README"); res != (core.OpenSpecMergeResult{Ticket: "acme-3", Outcome: core.SpecMergeUnchanged}) {
		t.Errorf("the same conflict again gave %+v, want acme-3 unchanged", res)
	}
	if res := f.openSpecMerge(t, "go.mod"); res != (core.OpenSpecMergeResult{Ticket: "acme-3", Outcome: core.SpecMergeCommented}) {
		t.Errorf("a new conflict gave %+v, want acme-3 commented", res)
	}
	if res := f.openSpecMerge(t, "go.mod"); res.Outcome != core.SpecMergeUnchanged {
		t.Errorf("the new conflict again gave %+v, want unchanged", res)
	}
	tk := f.getTicket(t, "acme-3")
	if len(tk.Comments) != 1 || !strings.Contains(tk.Comments[0].Body, "- go.mod\n") {
		t.Errorf("comments %+v, want one naming go.mod", tk.Comments)
	}
	rows, _ := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{Title: specMergeTitle})
	if len(rows) != 1 {
		t.Errorf("Spec merge Tickets %+v, want one", rows)
	}
}

func TestOpenSpecMergeOpensAnotherOnceTheLastIsDone(t *testing.T) {
	f := specMergeWorld(t)
	f.openSpecMerge(t, "README")
	f.moveTo(t, 3, store.Done)

	if res := f.openSpecMerge(t, "README"); res != (core.OpenSpecMergeResult{Ticket: "acme-4", Outcome: core.SpecMergeOpened}) {
		t.Errorf("result %+v, want acme-4 opened", res)
	}
}

func TestOpenSpecMergeCommentsOnAnUnmarkedTicketWithItsTitleWithoutMarkingIt(t *testing.T) {
	f := specMergeWorld(t)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: specMergeTitle, Description: "Opened by an older Factory.\n", State: store.Ready})

	if res := f.openSpecMerge(t, "README"); res != (core.OpenSpecMergeResult{Ticket: "acme-3", Outcome: core.SpecMergeCommented}) {
		t.Fatalf("result %+v, want acme-3 commented", res)
	}
	if res := f.openSpecMerge(t, "README"); res.Outcome != core.SpecMergeUnchanged {
		t.Errorf("the same conflict again gave %+v, want unchanged", res)
	}
	tk := f.getTicket(t, "acme-3")
	if tk.SpecMergeTicket || len(tk.Comments) != 1 || !strings.Contains(tk.Comments[0].Body, "- README\n") {
		t.Errorf("legacy Ticket %+v, want it unmarked with one comment naming README", tk)
	}
}

func TestOpenSpecMergeRefusesALongLivedSpecAMapAndAFinishedSpec(t *testing.T) {
	for name, setup := range map[string]func(f fixture){
		"Long-lived": func(f fixture) { f.longLived(t, core.SpecView{ID: "acme-1"}) },
		"Map":        func(f fixture) { f.labelSpec(t, 1, core.MapLabel) },
		"Finished":   func(f fixture) { f.moveTo(t, 2, store.Done) },
	} {
		t.Run(name, func(t *testing.T) {
			f := specMergeWorld(t)
			setup(f)
			_, err := f.core.OpenSpecMerge(ctx, "acme", core.OpenSpecMergeInput{Spec: "acme-1", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"})
			wantCode(t, err, core.ActionNotAllowed, "")
			if rows, _ := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{Title: specMergeTitle}); len(rows) != 0 {
				t.Errorf("opened %+v", rows)
			}
		})
	}
}

func TestATicketCreatedWithTheSpecMergeTitleIsUnmarked(t *testing.T) {
	f := specMergeWorld(t)
	if tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: specMergeTitle, State: store.Ready}); tk.SpecMergeTicket {
		t.Errorf("create_ticket made %+v, want an ordinary Ticket", tk)
	}
}

// labelSpec gives Spec n the Label straight in the store, as only Code
// Central itself labels a Map.
func (f fixture) labelSpec(t *testing.T, n int, label string) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		s, _ := w.Spec("acme", n)
		s.Labels = append(s.Labels, label)
		return w.PutSpec("acme", s)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTheSpecMergeMarkSurvivesReopeningTheStore(t *testing.T) {
	f := reloaded(t, func(f fixture) {
		f.project(t, "acme")
		f.spec(t, "acme", "Offline mode")
		f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "First"})
		f.openSpecMerge(t, "README")
	})
	if !f.getTicket(t, "acme-3").SpecMergeTicket {
		t.Error("acme-3 lost its Spec merge mark")
	}
}
