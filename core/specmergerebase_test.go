package core_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// The Spec merge Ticket under Rebase (Spec code-central-1063, section
// "Conflicts and prompts"): with Rebase in force in any conflicting
// Repository, open_spec_merge titles the Ticket "Rebase the Spec Branch
// of <spec id> onto the base" with a body that says to rebase. The stored
// mark, not the title, makes it a Spec merge Ticket with the Merge
// results Auto-accept kind, and an open one takes comments under either
// title.

const specMergeRebaseTitle = "Rebase the Spec Branch of acme-1 onto the base"

// specMergeRebaseWorld is specMergeWorld under Merge with Land per Spec
// and Rebase on.
func specMergeRebaseWorld(t *testing.T) fixture {
	t.Helper()
	f := specMergeWorld(t)
	f.landing(t, domain.Landing{Method: domain.LandingMerge, Remote: "origin", LandPerSpec: true, Rebase: true})
	return f
}

func TestOpenSpecMergeTitlesTheTicketToRebaseWhenRebaseIsInForce(t *testing.T) {
	f := specMergeRebaseWorld(t)

	res := f.openSpecMerge(t, "README", "go.mod")

	if res != (core.OpenSpecMergeResult{Ticket: "acme-3", Outcome: core.SpecMergeOpened}) {
		t.Fatalf("result %+v, want acme-3 opened", res)
	}
	tk := f.getTicket(t, "acme-3")
	want := "The Spec Branch spec/acme-1 conflicts with the base, main, in these files:\n\n- README\n- go.mod\n\n" +
		"The base's tip is aaa111 and the Spec Branch's tip is bbb222. Rebase this branch onto the base and resolve the conflicts.\n"
	if tk.Title != specMergeRebaseTitle || tk.State != domain.Ready || tk.Spec.ID != "acme-1" || tk.Description != want || !tk.SpecMergeTicket {
		t.Errorf("opened %+v, want a Ready marked Ticket on acme-1 titled %q describing the conflict with\n%s", tk, specMergeRebaseTitle, want)
	}
}

func TestOpenSpecMergeTitlesTheTicketToRebaseWhenRebaseIsInForceInAnyConflictingRepository(t *testing.T) {
	// Rebase is on, but the Default Landing is Branch, where it isn't in
	// force; api's Own Landing is Merge, where it is.
	f := specMergeInTwo(t)
	f.landing(t, domain.Landing{Method: domain.LandingBranch, Remote: "origin", LandPerSpec: true, Rebase: true})
	f.editLanding(t, "api", core.RepositoryLandingMerge)
	home := core.SpecMergeConflict{Name: "acme", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"}
	api := core.SpecMergeConflict{Name: "api", Conflicts: []string{"endpoint.go"}, BaseTip: "ccc333", SpecBranchTip: "ddd444"}

	if _, err := f.core.OpenSpecMerge(ctx, "acme", core.OpenSpecMergeInput{Spec: "acme-1", Repositories: []core.SpecMergeConflict{home}}); err != nil {
		t.Fatal(err)
	}
	if tk := f.getTicket(t, "acme-3"); tk.Title != specMergeTitle || !strings.HasSuffix(tk.Description, "\nIn each, merge the base into this branch and resolve the conflicts.\n") {
		t.Errorf("a conflict only where Rebase isn't in force opened %q saying\n%s\nwant %q saying to merge", tk.Title, tk.Description, specMergeTitle)
	}
	f.moveTo(t, 3, domain.Done)

	if _, err := f.core.OpenSpecMerge(ctx, "acme", core.OpenSpecMergeInput{Spec: "acme-1", Repositories: []core.SpecMergeConflict{home, api}}); err != nil {
		t.Fatal(err)
	}
	tk := f.getTicket(t, "acme-4")
	want := "The Spec Branch spec/acme-1 conflicts with the base in these Repositories:\n\n" +
		"- acme: merging spec/acme-1 at bbb222 into main at aaa111 conflicts in these files:\n  - README\n" +
		"- api: merging spec/acme-1 at ddd444 into trunk at ccc333 conflicts in these files:\n  - endpoint.go\n\n" +
		"In each, rebase this branch onto its base and resolve the conflicts.\n"
	if tk.Title != specMergeRebaseTitle || tk.Description != want || !tk.SpecMergeTicket {
		t.Errorf("a conflict in api too opened %+v, want it marked and titled %q describing\n%s", tk, specMergeRebaseTitle, want)
	}
}

func TestTheRebaseTitledSpecMergeTicketKeepsTheMergeResultsAutoAcceptKind(t *testing.T) {
	f := specMergeRebaseWorld(t)
	f.openSpecMerge(t, "README")
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptMerge: true})
	f.finishTicket(t, "acme-3")
	autoAccepted(t, f.get(t, "acme-3"), domain.Done, domain.CauseProject)

	// An ordinary Ticket with the rebase title is Implementation results.
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: specMergeRebaseTitle, State: domain.Ready})
	f.finishTicket(t, "acme-4")
	if got := f.get(t, "acme-4"); got.State != domain.InReview {
		t.Fatalf("a Ticket titled like a rebase Spec merge Ticket is %s under Merge results, want In Review", got.State)
	}
}

func TestOpenSpecMergeCommentsOnTheOpenMarkedTicketWhateverItsTitle(t *testing.T) {
	// Opened under Merge, then Rebase is turned on: the open marked Ticket
	// keeps its title and takes the comment, which says to rebase.
	f := specMergeWorld(t)
	f.openSpecMerge(t, "README")
	f.landing(t, domain.Landing{Method: domain.LandingMerge, Remote: "origin", LandPerSpec: true, Rebase: true})

	if res := f.openSpecMerge(t, "go.mod"); res != (core.OpenSpecMergeResult{Ticket: "acme-3", Outcome: core.SpecMergeCommented}) {
		t.Fatalf("with Rebase on, a new conflict gave %+v, want acme-3 commented", res)
	}
	tk := f.getTicket(t, "acme-3")
	if tk.Title != specMergeTitle || len(tk.Comments) != 1 || !strings.Contains(tk.Comments[0].Body, "Rebase this branch onto the base") {
		t.Errorf("acme-3 is %+v, want it still titled %q with one comment saying to rebase", tk, specMergeTitle)
	}
	rows, _ := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{Spec: "acme-1", State: []domain.State{domain.Ready}})
	if len(rows) != 2 {
		t.Errorf("Ready Tickets on acme-1 %+v, want acme-2 and acme-3 alone", rows)
	}
}

func TestOpenSpecMergeCommentsOnAnUnmarkedTicketWithTheRebaseTitleWithoutMarkingIt(t *testing.T) {
	f := specMergeRebaseWorld(t)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: specMergeRebaseTitle, Description: "Opened by an older Factory.\n", State: domain.Ready})

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
