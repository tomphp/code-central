package factory_test

import (
	"context"
	"github.com/tomoram/code-central/domain"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Mixed Landings (Spec code-central-890): each of a Ticket's Changed
// Repositories lands by its own Landing, a Spec lands in each Repository
// by that Repository's, and a change of one Repository's Own Landing with
// work in flight applies from the next landing step there alone.

// setLanding has the human give the Repository name the Own Landing
// landing, or with core.RepositoryLandingDefault, have it follow the
// Default Landing.
func setLanding(t *testing.T, h *factorytest.Harness, name string, landing core.RepositoryLanding) {
	t.Helper()
	if _, err := h.Human().EditRepository(context.Background(), h.Project, core.EditRepositoryInput{Name: name, Landing: &landing}); err != nil {
		t.Fatal(err)
	}
}

// pullRequestBesideAMergedHome has every Repository but the Home, which
// has an Own Landing of Merge, follow a Default Landing of Pull Request.
// (An Own Landing of Pull Request needs a Code Host's Clone URL, which the
// harness's Repositories don't have.)
func pullRequestBesideAMergedHome(t *testing.T, h *factorytest.Harness) {
	t.Helper()
	setLanding(t, h, h.Project, core.RepositoryLandingMerge)
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingPullRequest, Remote: "origin"})
}

func TestATicketLandsByMergeInTheHomeAndByItsPullRequestInARepositoryUnderPullRequest(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	pullRequestBesideAMergedHome(t, h)
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	apiBase := h.GitIn(api.Path, "rev-parse", "main")

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	homeMerge := h.Git("rev-parse", "main")
	if !tk.PartlyLanded || landedIn(tk, h.Project) != homeMerge || landedIn(tk, "api") != "" {
		t.Fatalf("partly landed %v %+v, want it merged and landed in the Home at %s alone", tk.PartlyLanded, tk.Repositories, homeMerge)
	}
	apiHead := wantPushedIn(t, h, "api", id)
	if got := api.Remote("origin").Git("rev-parse", id); got != apiHead {
		t.Errorf("api's origin has %s at %q, want its work %s pushed for its pull request", id, got, apiHead)
	}
	if got := h.GitIn(api.Path, "rev-parse", "main"); got != apiBase {
		t.Errorf("api's main moved to %s: under Pull Request the Factory merges nothing there", got)
	}
	if work := pushedWorkIn(t, h, h.Project); len(work) != 0 {
		t.Errorf("the Watcher's work in the Home is %v, want none: the Home lands by Merge, with no pull request", work)
	}
	if w := pullRequestWatch(t, h); !w.On || len(w.Repositories) != 1 || w.Repositories[0].Name != "api" {
		t.Errorf("the Pull Request Watcher's watch is on %v for %+v, want it on for api alone", w.On, w.Repositories)
	}
	r.WantOutput(id + ": pushed " + id + " to origin in Repository api\n")

	// The human merges api's pull request on the host.
	h.GitIn(api.Path, "push", "-q", "origin", id+":refs/heads/main")
	reportPullRequestIn(t, h, "api", id, 4, apiHead)
	r.Poll()

	tk = h.WantState(id, domain.Done)
	if landedIn(tk, "api") != apiHead || landedIn(tk, h.Project) != homeMerge {
		t.Fatalf("Changed Repositories %+v, want api landed at its pull request's merge %s and the Home's landing kept", tk.Repositories, apiHead)
	}
}

func TestAConflictInTheHomeSendsTheTicketBackBeforeItsBranchIsPushedForAPullRequestElsewhere(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	pullRequestBesideAMergedHome(t, h)
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	// The Home's base moves on, conflicting with the Ticket's work there.
	h.CommitOn("main", "client.go", "package other\n")
	// Hold the next Claim's turn, so the Ticket stays as send_back left it.
	h.Script(id, factorytest.Wait())

	h.Accept(id)
	r.Poll()

	if tk := h.Ticket(id); !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s wasn't sent back: %+v", id, tk.Moves)
	}
	if got := api.Remote("origin").Git("branch", "--list", id); got != "" {
		t.Errorf("api's origin holds %q, want nothing pushed: every Repository is checked before the Ticket lands in any", got)
	}
	if work := pushedWorkIn(t, h, "api"); len(work) != 0 {
		t.Errorf("the Watcher's work in api is %v, want none", work)
	}
}

func TestASpecLandsByMergeInTheHomeAndByItsPullRequestInARepositoryUnderPullRequest(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, id := specInTwo(h)
	pullRequestBesideAMergedHome(t, h)
	branch := "spec/" + h.SpecID()
	r := h.Start()
	h.WantState(id, domain.InReview)
	apiBase := h.GitIn(api.Path, "rev-parse", "main")

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	homeMerge := h.Git("rev-parse", "main")
	if got, want := subject(h, h.Repo, homeMerge), "Merge spec "+h.SpecID()+": Work"; got != want {
		t.Errorf("the Home's base is at %q, want the Spec Branch merged: %q", got, want)
	}
	if !tk.PartlyLanded || landedIn(tk, h.Project) != homeMerge || landedIn(tk, "api") != "" {
		t.Fatalf("partly landed %v %+v, want it landed in the Home alone, at the Spec's merge %s", tk.PartlyLanded, tk.Repositories, homeMerge)
	}
	apiTip := wantPushedIn(t, h, "api", branch)
	if got := api.Remote("origin").Git("rev-parse", branch); got != apiTip || apiTip != h.GitIn(api.Path, "rev-parse", branch) {
		t.Errorf("api's origin has %s at %q, want its Spec Branch %s pushed for the Spec's pull request", branch, got, apiTip)
	}
	if got := h.GitIn(api.Path, "rev-parse", "main"); got != apiBase {
		t.Errorf("api's main moved to %s: under Pull Request the Factory merges nothing there", got)
	}
	if work := pushedWorkIn(t, h, h.Project); len(work) != 0 {
		t.Errorf("the Watcher's work in the Home is %v, want none: the Home lands by Merge, with no pull request", work)
	}
	r.WantOutput(h.SpecID() + ": pushed " + branch + " to origin in Repository api\n")

	// The human merges the Spec's pull request in api.
	h.GitIn(api.Path, "push", "-q", "origin", branch+":refs/heads/main")
	reportPullRequestIn(t, h, "api", branch, 4, apiTip)
	r.Poll()

	tk = h.WantState(id, domain.Done)
	if landedIn(tk, "api") != apiTip || landedIn(tk, h.Project) != homeMerge {
		t.Fatalf("Changed Repositories %+v, want api landed at its pull request's merge %s and the Home's landing kept", tk.Repositories, apiTip)
	}
}

func TestUnderBranchInARepositoryASpecsTicketsLandThereOneByOneWhileTheSpecMergesInTheHome(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, both := specInTwo(h)
	setLanding(t, h, "api", core.RepositoryLandingBranch)
	// Its own file, so its commit in api is never the other's.
	only := h.ReadyTicket("Add another endpoint")
	h.Script(only, append(factorytest.CommitIn("api", "other.go", "package api\n"), factorytest.FinishIn("Done.", false, "api"), factorytest.Result("success"))...)
	r := h.Start()
	h.WantState(both, domain.InReview)
	h.WantState(only, domain.InReview)
	apiBase := h.GitIn(api.Path, "rev-parse", "main")

	h.Accept(both)
	h.Accept(only)
	r.Poll()

	// The Ticket in api alone doesn't hold the Spec back: under Branch,
	// Land per Spec has no effect there.
	tk := h.WantState(both, domain.Accepted)
	homeMerge := h.Git("rev-parse", "main")
	if got, want := subject(h, h.Repo, homeMerge), "Merge spec "+h.SpecID()+": Work"; got != want {
		t.Errorf("the Home's base is at %q, want the Spec Branch merged: %q", got, want)
	}
	if !tk.PartlyLanded || landedIn(tk, h.Project) != homeMerge || landedIn(tk, "api") != "" {
		t.Fatalf("partly landed %v %+v, want %s landed in the Home alone, at the Spec's merge %s", tk.PartlyLanded, tk.Repositories, both, homeMerge)
	}
	if got := h.GitIn(api.Path, "rev-parse", "main"); got != apiBase {
		t.Errorf("api's main moved to %s: under Branch the human merges there", got)
	}
	r.WantOutput(both + ": Accepted under the Branch Landing Method: merge " + both + " into main in Repository api and the Factory lands it\n")
	r.WantOutput(only + ": Accepted under the Branch Landing Method: merge " + only + " into main in Repository api and the Factory lands it\n")

	// The human merges the api-only Ticket there: it lands on its own.
	h.GitIn(api.Path, "merge", "-q", "--no-ff", "-m", "Merge "+only, only)
	r.Poll()
	if tk := h.WantState(only, domain.Done); landedIn(tk, "api") != h.GitIn(api.Path, "rev-parse", "refs/heads/"+only) {
		t.Fatalf("Changed Repositories %+v, want %s landed in api at its work", tk.Repositories, only)
	}
	h.WantState(both, domain.Accepted)

	// Then the other.
	h.GitIn(api.Path, "merge", "-q", "--no-ff", "-m", "Merge "+both, both)
	r.Poll()
	tk = h.WantState(both, domain.Done)
	if landedIn(tk, "api") != h.GitIn(api.Path, "rev-parse", "refs/heads/"+both) || landedIn(tk, h.Project) != homeMerge {
		t.Fatalf("Changed Repositories %+v, want %s landed in api at its work and the Home's landing kept", tk.Repositories, both)
	}
}

func TestAChangedOwnLandingWithWorkInFlightAppliesFromTheNextLandingStepThereAlone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	api := h.AddRepository("api")
	mergeAndPush(h, "origin")
	setLanding(t, h, "api", core.RepositoryLandingBranch)
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	apiBase := h.GitIn(api.Path, "rev-parse", "main")

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	homeMerge := h.Git("rev-parse", "main")
	if !tk.PartlyLanded || landedIn(tk, h.Project) != homeMerge || landedIn(tk, "api") != "" {
		t.Fatalf("partly landed %v %+v, want it merged and landed in the Home alone, at %s", tk.PartlyLanded, tk.Repositories, homeMerge)
	}
	if got := home.Git("rev-parse", "main"); got != homeMerge {
		t.Errorf("the Home's origin's main is at %s, want its merge %s pushed under Merge + Push", got, homeMerge)
	}
	if got := h.GitIn(api.Path, "rev-parse", "main"); got != apiBase {
		t.Fatalf("api's main moved to %s: under Branch the human merges there", got)
	}

	// api follows the Default Landing, Merge + Push, from now on.
	setLanding(t, h, "api", core.RepositoryLandingDefault)
	r.Poll()

	tk = h.WantState(id, domain.Done)
	apiMerge := h.GitIn(api.Path, "rev-parse", "main")
	if got, want := subject(h, api.Path, apiMerge), "Merge "+id+": Add an endpoint"; got != want || landedIn(tk, "api") != apiMerge {
		t.Fatalf("api's main is at %q, landed at %q; want the Factory's merge %q, landed there", got, landedIn(tk, "api"), want)
	}
	if got := api.Remote("origin").Git("rev-parse", "main"); got != apiMerge {
		t.Errorf("api's origin's main is at %s, want its merge %s pushed", got, apiMerge)
	}
	if landedIn(tk, h.Project) != homeMerge || h.Git("rev-parse", "main") != homeMerge {
		t.Fatalf("Changed Repositories %+v, want the Home's landing at %s kept", tk.Repositories, homeMerge)
	}
}

func TestASpecBranchConflictInTheHomeHoldsBackTheSpecsPullRequestElsewhere(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, id := specInTwo(h)
	pullRequestBesideAMergedHome(t, h)
	release := holdTicketsStage(t, h)
	fix := "acme-5"
	h.Script(fix, factorytest.Wait())
	branch := "spec/" + h.SpecID()
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	r.WantOutput(id + ": merged into " + branch + " at ")
	// The Home's base moves on, conflicting with its Spec Branch there.
	homeBase := h.CommitOn("main", "client.go", "package other\n")

	release()
	r.Poll()

	h.WantState(id, domain.Accepted)
	if got := h.Git("rev-parse", "main"); got != homeBase {
		t.Errorf("the Home's main moved to %s: nothing lands while any Repository's Spec Branch conflicts", got)
	}
	if got := api.Remote("origin").Git("branch", "--list", branch); got != "" {
		t.Errorf("api's origin holds %q, want no Spec Branch pushed for a pull request while the Home's conflicts", got)
	}
	if work := pushedWorkIn(t, h, "api"); len(work) != 0 {
		t.Errorf("the Watcher's work in api is %v, want none", work)
	}
	if tk := h.Ticket(fix); !tk.SpecMergeTicket {
		t.Fatalf("%s %+v, want the Spec merge Ticket", fix, tk)
	}
	r.WantOutput(h.SpecID() + ": " + branch + " conflicts with the base in " + h.Project + ": opened " + fix)
}

func TestARepositoryGivenALandingWithLandPerSpecWhileASpecsTicketWaitsMergesItIntoItsSpecBranchThere(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, id := specInTwo(h)
	setLanding(t, h, "api", core.RepositoryLandingBranch)
	release := holdTicketsStage(t, h)
	branch := "spec/" + h.SpecID()
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	// Merged into the Home's Spec Branch alone: under Branch in api the
	// human merges it.
	r.WantOutput(id + ": merged into " + branch + " at ")
	apiWork := h.GitIn(api.Path, "rev-parse", "refs/heads/"+id)

	// api follows the Default Landing, Merge with Land per Spec, from now
	// on: the Ticket goes through its Spec Branch there too.
	setLanding(t, h, "api", core.RepositoryLandingDefault)
	r.Poll()
	r.WantOutput(id + ": merged into " + branch + " at ")
	r.WantOutput(" in Repository api\n")

	release()
	r.Poll()

	tk := h.WantState(id, domain.Done)
	apiMerge := h.GitIn(api.Path, "rev-parse", "main")
	if got, want := subject(h, api.Path, apiMerge), "Merge spec "+h.SpecID()+": Work"; got != want || landedIn(tk, "api") != apiMerge {
		t.Fatalf("api's main is at %q, landed at %q; want the Spec Branch merged there, %q, landed there", got, landedIn(tk, "api"), want)
	}
	if !strings.Contains(h.GitIn(api.Path, "branch", "--contains", apiWork, "--list", "main"), "main") {
		t.Fatalf("api's main doesn't hold %s's work there, %s", id, apiWork)
	}
}
