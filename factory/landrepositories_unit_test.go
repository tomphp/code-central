package factory

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/core"
)

// The landing step with a second Repository (Spec code-central-848, ADR
// 0049), on one fake repo per Repository: every Changed Repository's
// merge is checked before any base moves, and each lands in the landing
// order.

// withAPI adds the Repository api to the Project, after the Home
// Repository, with a fake repo of its own as its clone, and returns it.
func (w *landWorld) withAPI() *fakeRepo {
	w.t.Helper()
	if _, err := w.human.AddRepository(context.Background(), "acme", api.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"}); err != nil {
		w.t.Fatal(err)
	}
	repo := newFakeRepo("/work/api")
	apiRepo := &repository{name: "api", dir: "/work/api", git: repo, base: "main", worktrees: "/work/api.worktrees"}
	w.l.repos.byName["api"] = apiRepo
	w.l.repos.inOrder = []*repository{w.l.repos.home, apiRepo}
	return repo
}

// pollRepositories is a poll's landing step with the Repositories withAPI
// set up: as poll, but for making the clones, which takes real git. Each
// takes its Push remote from the Project, as syncing them does.
func (w *landWorld) pollRepositories() {
	w.t.Helper()
	ctx := context.Background()
	p, ok := w.l.landing(ctx)
	if !ok {
		w.t.Fatal("the landing step can't read the Project")
	}
	for _, v := range p.Repositories {
		if r := w.l.repos.named(v.Name); r != nil {
			r.listedAs(v)
		}
	}
	w.l.locateBases(ctx, p)
	w.l.land(ctx, p)
}

// acceptedInTwo makes an Accepted Ticket with a commit on its branch in
// the Home Repository and in api, finished with both, and returns its ID
// and its commit in api.
func (w *landWorld) acceptedInTwo(apiRepo *fakeRepo, title string) (id, apiWork string) {
	w.t.Helper()
	ctx := context.Background()
	id = w.ready(title)
	home := w.repo.commitOn(id, "Work on "+id)
	apiWork = apiRepo.commitOn(id, "Work on "+id+" in api")
	res, err := w.srv.Client.Claim(ctx, "acme", api.ClaimInput{Ticket: api.Ref(id)})
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.srv.Client.Finish(ctx, "acme", api.FinishInput{Ticket: api.Ref(id), ClaimID: res.ClaimID, Summary: "Done.", Branch: id, Commit: home,
		Repositories: []api.RepositoryWork{{Name: "api", Branch: id, Commit: apiWork}}}); err != nil {
		w.t.Fatal(err)
	}
	w.accept(id)
	return id, apiWork
}

func TestEveryRepositorysMergeIsCheckedBeforeAnyBaseMoves(t *testing.T) {
	w := newLandWorld(t)
	apiRepo := w.withAPI()
	id, apiWork := w.acceptedInTwo(apiRepo, "Add an endpoint")
	apiRepo.conflicts[apiWork] = []string{"endpoint.go"}
	homeMain, apiMain := w.repo.tip("refs/heads/main"), apiRepo.tip("refs/heads/main")

	w.pollRepositories()

	tk := w.ticket(id)
	if tk.State != domain.Ready || commentsSaying(tk, "- api: merging", "  - endpoint.go") != 1 {
		t.Fatalf("%s %s %+v, want it sent back naming api's conflict", id, tk.State, tk.Comments)
	}
	if w.repo.tip("refs/heads/main") != homeMain || apiRepo.tip("refs/heads/main") != apiMain {
		t.Fatal("a base moved: nothing lands while any Repository conflicts")
	}
}

func TestEachRepositoryLandsInTheLandingOrder(t *testing.T) {
	w := newLandWorld(t)
	apiRepo := w.withAPI()
	if _, err := w.human.SetLandingOrder(context.Background(), "acme", api.SetLandingOrderInput{Names: []string{"api", "acme"}}); err != nil {
		t.Fatal(err)
	}
	id, _ := w.acceptedInTwo(apiRepo, "Add an endpoint")

	w.pollRepositories()

	if tk := w.ticket(id); tk.State != domain.Done {
		t.Fatalf("%s %s, want Done once it has landed in both", id, tk.State)
	}
	out := w.out.String()
	apiAt := strings.Index(out, id+": landed on main at "+short(apiRepo.tip("refs/heads/main"))+" in Repository api")
	homeAt := strings.Index(out, id+": landed on main at "+short(w.repo.tip("refs/heads/main"))+"\n")
	if apiAt < 0 || homeAt < apiAt {
		t.Fatalf("stdout %q, want api landed before the Home Repository", out)
	}
}

func TestARepositoryTheTicketHasLandedInHoldsNothingBackWithoutItsClone(t *testing.T) {
	w := newLandWorld(t)
	apiRepo := w.withAPI()
	id, apiWork := w.acceptedInTwo(apiRepo, "Add an endpoint")
	// Landed in api on an earlier poll, whose clone has gone since.
	if _, err := w.srv.Client.Land(context.Background(), "acme", api.LandInput{Ticket: api.Ref(id), Commit: apiWork, Repository: "api"}); err != nil {
		t.Fatal(err)
	}
	w.l.repos.byName["api"].problem, w.l.repos.byName["api"].git = "it isn't a git repository", nil

	w.pollRepositories()

	if tk := w.ticket(id); tk.State != domain.Done {
		t.Fatalf("%s %s, want Done once it lands in the Home Repository", id, tk.State)
	}
}

// Land per Spec and Branch with a second Repository (ADR 0049).

// holdSpec makes the world's Tickets from here on Spec-landed: on a
// plain Spec, held back from landing by a Draft tickets Stage until
// release cancels it.
func (w *landWorld) holdSpec() (release func()) {
	w.t.Helper()
	ctx := context.Background()
	s, err := w.srv.Client.CreateSpec(ctx, "acme", api.CreateSpecInput{Title: "Work"})
	if err != nil {
		w.t.Fatal(err)
	}
	w.spec = s.ID
	stage, err := w.srv.Client.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: api.Ref(s.ID), Title: "Tickets", State: domain.Draft, Labels: []string{core.StageLabel("tickets")}})
	if err != nil {
		w.t.Fatal(err)
	}
	return func() {
		w.t.Helper()
		if _, err := w.human.Cancel(ctx, "acme", api.CancelInput{Ticket: api.Ref(stage.ID)}); err != nil {
			w.t.Fatal(err)
		}
	}
}

func TestEveryRepositorysSpecBranchIsCheckedBeforeAnyBaseMoves(t *testing.T) {
	w := newLandWorld(t)
	apiRepo := w.withAPI()
	release := w.holdSpec()
	id, _ := w.acceptedInTwo(apiRepo, "Add an endpoint")
	w.pollRepositories()
	branch := "refs/heads/" + domain.SpecBranch(w.spec, nil)
	if w.repo.tip(branch) == "" || apiRepo.tip(branch) == "" {
		t.Fatalf("%s wasn't merged into its Spec Branch in both:\n%s", id, w.out)
	}
	// api's Spec Branch conflicts with its base.
	apiRepo.conflicts[apiRepo.tip(branch)] = []string{"endpoint.go"}
	homeMain, apiMain := w.repo.tip("refs/heads/main"), apiRepo.tip("refs/heads/main")
	release()

	w.pollRepositories()

	if w.repo.tip("refs/heads/main") != homeMain || apiRepo.tip("refs/heads/main") != apiMain {
		t.Fatal("a base moved: nothing lands while any Repository's Spec Branch conflicts")
	}
	if tk := w.ticket(id); tk.State != domain.Accepted {
		t.Fatalf("%s is %s, want it still Accepted", id, tk.State)
	}
	rows, err := w.srv.Client.FindTickets(context.Background(), "acme", api.FindTicketsInput{Title: api.SpecMergeTitle(w.spec)})
	if err != nil || len(rows) != 1 {
		t.Fatalf("Spec merge Tickets %+v (%v), want one", rows, err)
	}
	if fix := w.ticket(rows[0].ID); !strings.Contains(fix.Description, "- api: merging spec/"+w.spec+" at "+apiRepo.tip(branch)+" into main at "+apiMain+" conflicts in these files:\n  - endpoint.go\n") {
		t.Fatalf("Spec merge Ticket %q, want it to name api's conflict", fix.Description)
	}
}

func TestASpecLandsWithTheSameMergeSpecCommitInEachRepository(t *testing.T) {
	w := newLandWorld(t)
	apiRepo := w.withAPI()
	release := w.holdSpec()
	release()
	id, apiWork := w.acceptedInTwo(apiRepo, "Add an endpoint")

	w.pollRepositories()

	tk := w.ticket(id)
	homeMerge, apiMerge := w.repo.tip("refs/heads/main"), apiRepo.tip("refs/heads/main")
	want := "Merge spec " + w.spec + ": Work"
	if tk.State != domain.Done || w.repo.messageOf(homeMerge) != want || apiRepo.messageOf(apiMerge) != want || !apiRepo.isAncestor(apiWork, apiMerge) {
		t.Fatalf("%s is %s; main is at %q and %q, want it Done, landed by the Spec's merge in each", id, tk.State, w.repo.messageOf(homeMerge), apiRepo.messageOf(apiMerge))
	}
}

func TestATicketIsMergedIntoItsSpecBranchInEveryRepositoryOrNone(t *testing.T) {
	w := newLandWorld(t)
	apiRepo := w.withAPI()
	w.holdSpec()
	id, apiWork := w.acceptedInTwo(apiRepo, "Add an endpoint")
	home := w.repo.tip("refs/heads/" + id)
	apiRepo.conflicts[apiWork] = []string{"endpoint.go"}

	w.pollRepositories()

	tk := w.ticket(id)
	if tk.State != domain.Ready || commentsSaying(tk, "merging it into the Spec Branch conflicts", "- api: merging", "  - endpoint.go") != 1 {
		t.Fatalf("%s %s %+v, want it sent back naming api's conflict with its Spec Branch", id, tk.State, tk.Comments)
	}
	if spec := w.repo.tip("refs/heads/" + domain.SpecBranch(w.spec, nil)); w.repo.isAncestor(home, spec) {
		t.Fatal("the Home Repository's Spec Branch holds the Ticket's work: a Ticket conflicting in any Repository is merged into none")
	}
}

func TestUnderBranchATicketLandsInEachRepositoryOnceMergedThereByHand(t *testing.T) {
	w := newLandWorld(t)
	w.setLanding(domain.LandingBranch, false, "")
	apiRepo := w.withAPI()
	id, _ := w.acceptedInTwo(apiRepo, "Add an endpoint")

	apiRepo.mergeByHand(id)
	w.pollRepositories()

	tk := w.ticket(id)
	if tk.State != domain.Accepted || !tk.PartlyLanded {
		t.Fatalf("%s is %s, partly landed %v, want it Partly landed once merged in api alone", id, tk.State, tk.PartlyLanded)
	}

	w.repo.mergeByHand(id)
	w.pollRepositories()

	if tk := w.ticket(id); tk.State != domain.Done {
		t.Fatalf("%s is %s, want Done once merged in both", id, tk.State)
	}
}

func TestARefusedPushIsTriedAgainThoughAnotherRepositorysSpecBranchNowConflicts(t *testing.T) {
	w := newLandWorld(t)
	apiRepo := w.withAPI()
	w.repo.addRemote("origin")
	apiRepo.addRemote("origin")
	w.setLanding(domain.LandingMerge, true, "origin")
	release := w.holdSpec()
	id, _ := w.acceptedInTwo(apiRepo, "Add an endpoint")
	w.pollRepositories()
	w.repo.setBroken("origin", true)
	release()
	w.pollRepositories()
	homeMerge, branch := w.repo.tip("refs/heads/main"), "refs/heads/"+domain.SpecBranch(w.spec, nil)
	if w.repo.messageOf(homeMerge) != "Merge spec "+w.spec+": Work" || apiRepo.messageOf(apiRepo.tip("refs/heads/main")) == "Merge spec "+w.spec+": Work" {
		t.Fatalf("want the Spec merged in the Home Repository, its push refused, and api not merged yet:\n%s", w.out)
	}
	// Meanwhile api's Spec Branch comes to conflict with its base.
	apiRepo.conflicts[apiRepo.tip(branch)] = []string{"endpoint.go"}
	w.repo.setBroken("origin", false)

	w.pollRepositories()

	if got := w.repo.remoteTip("origin", "main"); got != homeMerge {
		t.Fatalf("the Home Repository's remote main is at %q, want its merge %s pushed: a merge made already is still pushed", got, homeMerge)
	}
	if tk := w.ticket(id); !tk.PartlyLanded {
		t.Fatalf("%s %+v, want it landed in the Home Repository alone", id, tk.Repositories)
	}
	if rows, _ := w.srv.Client.FindTickets(context.Background(), "acme", api.FindTicketsInput{Title: api.SpecMergeTitle(w.spec)}); len(rows) != 1 {
		t.Fatalf("Spec merge Tickets %+v, want one for api's conflict", rows)
	}
}

func TestUnderPullRequestASpecTouchingARepositoryWithNoCloneIsPushedNowhere(t *testing.T) {
	w := newLandWorld(t)
	apiRepo := w.withAPI()
	w.repo.addRemote("origin")
	apiRepo.addRemote("origin")
	if _, err := w.srv.Client.ReportPushRemote(context.Background(), "acme", api.ReportPushRemoteInput{URL: "https://github.com/acme/app.git", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	w.setLanding(domain.LandingPullRequest, false, "origin")
	release := w.holdSpec()
	w.acceptedInTwo(apiRepo, "Add an endpoint")
	w.pollRepositories()
	if w.repo.tip("refs/heads/"+domain.SpecBranch(w.spec, nil)) == "" {
		t.Fatalf("not merged into the Spec Branch:\n%s", w.out)
	}
	// api's clone has gone since.
	w.l.repos.byName["api"].problem, w.l.repos.byName["api"].git = "it isn't a git repository", nil
	release()

	w.pollRepositories()

	if got := w.repo.remoteTip("origin", domain.SpecBranch(w.spec, nil)); got != "" {
		t.Fatalf("the Home Repository's Spec Branch was pushed at %s, though api's can't be checked", got)
	}
}
