package factory

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The landing step with a second Repository (Spec code-central-848, ADR
// 0049), on one fake repo per Repository: every Changed Repository's
// merge is checked before any base moves, and each lands in the landing
// order.

// withAPI adds the Repository api to the Project, after the Home
// Repository, with a fake repo of its own as its clone, and returns it.
func (w *landWorld) withAPI() *fakeRepo {
	w.t.Helper()
	if _, err := w.human.AddRepository(context.Background(), "acme", core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"}); err != nil {
		w.t.Fatal(err)
	}
	repo := newFakeRepo("/work/api")
	api := &repository{name: "api", dir: "/work/api", git: repo, base: "main", worktrees: "/work/api.worktrees"}
	w.f.repositories["api"] = api
	w.f.inOrder = []*repository{w.f.home, api}
	return repo
}

// pollRepositories is a poll's landing step with the Repositories withAPI
// set up: as poll, but for making the clones, which takes real git.
func (w *landWorld) pollRepositories() {
	w.t.Helper()
	ctx := context.Background()
	p, ok := w.f.landing(ctx)
	if !ok {
		w.t.Fatal("the landing step can't read the Project")
	}
	w.f.locateBases(ctx, p)
	w.f.land(ctx, p)
}

// acceptedInTwo makes an Accepted Ticket with a commit on its branch in
// the Home Repository and in api, finished with both, and returns its ID
// and its commit in api.
func (w *landWorld) acceptedInTwo(api *fakeRepo, title string) (id, apiWork string) {
	w.t.Helper()
	ctx := context.Background()
	id = w.ready(title)
	home := w.repo.commitOn(id, "Work on "+id)
	apiWork = api.commitOn(id, "Work on "+id+" in api")
	res, err := w.srv.Client.Claim(ctx, "acme", core.ClaimInput{Ticket: core.Ref(id)})
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.srv.Client.Finish(ctx, "acme", core.FinishInput{Ticket: core.Ref(id), ClaimID: res.ClaimID, Summary: "Done.", Branch: id, Commit: home,
		Repositories: []core.RepositoryWork{{Name: "api", Branch: id, Commit: apiWork}}}); err != nil {
		w.t.Fatal(err)
	}
	w.accept(id)
	return id, apiWork
}

func TestEveryRepositorysMergeIsCheckedBeforeAnyBaseMoves(t *testing.T) {
	w := newLandWorld(t)
	api := w.withAPI()
	id, apiWork := w.acceptedInTwo(api, "Add an endpoint")
	api.conflicts[apiWork] = []string{"endpoint.go"}
	homeMain, apiMain := w.repo.tip("refs/heads/main"), api.tip("refs/heads/main")

	w.pollRepositories()

	tk := w.ticket(id)
	if tk.State != store.Ready || commentsSaying(tk, "- api: merging", "  - endpoint.go") != 1 {
		t.Fatalf("%s %s %+v, want it sent back naming api's conflict", id, tk.State, tk.Comments)
	}
	if w.repo.tip("refs/heads/main") != homeMain || api.tip("refs/heads/main") != apiMain {
		t.Fatal("a base moved: nothing lands while any Repository conflicts")
	}
}

func TestEachRepositoryLandsInTheLandingOrder(t *testing.T) {
	w := newLandWorld(t)
	api := w.withAPI()
	if _, err := w.human.SetLandingOrder(context.Background(), "acme", core.SetLandingOrderInput{Names: []string{"api", "acme"}}); err != nil {
		t.Fatal(err)
	}
	id, _ := w.acceptedInTwo(api, "Add an endpoint")

	w.pollRepositories()

	if tk := w.ticket(id); tk.State != store.Done {
		t.Fatalf("%s %s, want Done once it has landed in both", id, tk.State)
	}
	out := w.out.String()
	apiAt := strings.Index(out, id+": landed on main at "+short(api.tip("refs/heads/main"))+" in Repository api")
	homeAt := strings.Index(out, id+": landed on main at "+short(w.repo.tip("refs/heads/main"))+"\n")
	if apiAt < 0 || homeAt < apiAt {
		t.Fatalf("stdout %q, want api landed before the Home Repository", out)
	}
}

func TestARepositoryTheTicketHasLandedInHoldsNothingBackWithoutItsClone(t *testing.T) {
	w := newLandWorld(t)
	api := w.withAPI()
	id, apiWork := w.acceptedInTwo(api, "Add an endpoint")
	// Landed in api on an earlier poll, whose clone has gone since.
	if _, err := w.srv.Client.Land(context.Background(), "acme", core.LandInput{Ticket: core.Ref(id), Commit: apiWork, Repository: "api"}); err != nil {
		t.Fatal(err)
	}
	w.f.repositories["api"].problem, w.f.repositories["api"].git = "it isn't a git repository", nil

	w.pollRepositories()

	if tk := w.ticket(id); tk.State != store.Done {
		t.Fatalf("%s %s, want Done once it lands in the Home Repository", id, tk.State)
	}
}

// Land per Spec and Branch with a second Repository (ADR 0049).

// planSpec makes the world's Tickets from here on Spec-landed: on a
// plain Spec, held back from landing by a Draft tickets Stage until
// release cancels it.
func (w *landWorld) planSpec() (release func()) {
	w.t.Helper()
	ctx := context.Background()
	s, err := w.srv.Client.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Work"})
	if err != nil {
		w.t.Fatal(err)
	}
	w.spec = s.ID
	stage, err := w.srv.Client.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: core.Ref(s.ID), Title: "Tickets", State: store.Draft, Labels: []string{core.StageLabel("tickets")}})
	if err != nil {
		w.t.Fatal(err)
	}
	return func() {
		w.t.Helper()
		if _, err := w.human.Cancel(ctx, "acme", core.CancelInput{Ticket: core.Ref(stage.ID)}); err != nil {
			w.t.Fatal(err)
		}
	}
}

func TestEveryRepositorysSpecBranchIsCheckedBeforeAnyBaseMoves(t *testing.T) {
	w := newLandWorld(t)
	api := w.withAPI()
	release := w.planSpec()
	id, _ := w.acceptedInTwo(api, "Add an endpoint")
	w.pollRepositories()
	branch := "refs/heads/" + specBranch(w.spec)
	if w.repo.tip(branch) == "" || api.tip(branch) == "" {
		t.Fatalf("%s wasn't merged into its Spec Branch in both:\n%s", id, w.out)
	}
	// api's Spec Branch conflicts with its base.
	api.conflicts[api.tip(branch)] = []string{"endpoint.go"}
	homeMain, apiMain := w.repo.tip("refs/heads/main"), api.tip("refs/heads/main")
	release()

	w.pollRepositories()

	if w.repo.tip("refs/heads/main") != homeMain || api.tip("refs/heads/main") != apiMain {
		t.Fatal("a base moved: nothing lands while any Repository's Spec Branch conflicts")
	}
	if tk := w.ticket(id); tk.State != store.Accepted {
		t.Fatalf("%s is %s, want it still Accepted", id, tk.State)
	}
	rows, err := w.srv.Client.FindTickets(context.Background(), "acme", core.FindTicketsInput{Title: core.SpecMergeTitle(w.spec)})
	if err != nil || len(rows) != 1 {
		t.Fatalf("Spec merge Tickets %+v (%v), want one", rows, err)
	}
	if fix := w.ticket(rows[0].ID); !strings.Contains(fix.Description, "- api: merging spec/"+w.spec+" at "+api.tip(branch)+" into main at "+apiMain+" conflicts in these files:\n  - endpoint.go\n") {
		t.Fatalf("Spec merge Ticket %q, want it to name api's conflict", fix.Description)
	}
}

func TestASpecLandsWithTheSameMergeSpecCommitInEachRepository(t *testing.T) {
	w := newLandWorld(t)
	api := w.withAPI()
	release := w.planSpec()
	release()
	id, apiWork := w.acceptedInTwo(api, "Add an endpoint")

	w.pollRepositories()

	tk := w.ticket(id)
	homeMerge, apiMerge := w.repo.tip("refs/heads/main"), api.tip("refs/heads/main")
	want := "Merge spec " + w.spec + ": Work"
	if tk.State != store.Done || w.repo.messageOf(homeMerge) != want || api.messageOf(apiMerge) != want || !api.isAncestor(apiWork, apiMerge) {
		t.Fatalf("%s is %s; main is at %q and %q, want it Done, landed by the Spec's merge in each", id, tk.State, w.repo.messageOf(homeMerge), api.messageOf(apiMerge))
	}
}

func TestATicketIsMergedIntoItsSpecBranchInEveryRepositoryOrNone(t *testing.T) {
	w := newLandWorld(t)
	api := w.withAPI()
	w.planSpec()
	id := w.ready("Add an endpoint")
	home := w.repo.commitOn(id, "Work on "+id)
	apiWork := api.commitOn(id, "Work on "+id+" in api")
	api.conflicts[apiWork] = []string{"endpoint.go"}
	res, err := w.srv.Client.Claim(context.Background(), "acme", core.ClaimInput{Ticket: core.Ref(id)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.srv.Client.Finish(context.Background(), "acme", core.FinishInput{Ticket: core.Ref(id), ClaimID: res.ClaimID, Summary: "Done.", Branch: id, Commit: home,
		Repositories: []core.RepositoryWork{{Name: "api", Branch: id, Commit: apiWork}}}); err != nil {
		t.Fatal(err)
	}
	w.accept(id)

	w.pollRepositories()

	tk := w.ticket(id)
	if tk.State != store.Ready || commentsSaying(tk, "merging it into the Spec Branch conflicts", "- api: merging", "  - endpoint.go") != 1 {
		t.Fatalf("%s %s %+v, want it sent back naming api's conflict with its Spec Branch", id, tk.State, tk.Comments)
	}
	if spec := w.repo.tip("refs/heads/" + specBranch(w.spec)); w.repo.isAncestor(home, spec) {
		t.Fatal("the Home Repository's Spec Branch holds the Ticket's work: a Ticket conflicting in any Repository is merged into none")
	}
}

func TestUnderBranchATicketLandsInEachRepositoryOnceMergedThereByHand(t *testing.T) {
	w := newLandWorld(t)
	w.setLanding(store.LandingBranch, false, "")
	api := w.withAPI()
	id, _ := w.acceptedInTwo(api, "Add an endpoint")

	api.mergeByHand(id)
	w.pollRepositories()

	tk := w.ticket(id)
	if tk.State != store.Accepted || !tk.PartlyLanded {
		t.Fatalf("%s is %s, partly landed %v, want it Partly landed once merged in api alone", id, tk.State, tk.PartlyLanded)
	}

	w.repo.mergeByHand(id)
	w.pollRepositories()

	if tk := w.ticket(id); tk.State != store.Done {
		t.Fatalf("%s is %s, want Done once merged in both", id, tk.State)
	}
}
