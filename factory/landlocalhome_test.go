package factory_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// A local-only Home and per-Repository Push remotes (Spec
// code-central-890): the Factory reports the Home's Push remote, by the
// Home's own Push remote name, at start and whenever that name changes,
// an empty URL when its clone has none, which makes the Home local-only.
// Each Repository is pushed, fetched and gated by its own Landing Policy
// and Push remote, and no missing remote stops landing Project-wide.

// repositoryView is the Project's Repository name as the human reads it.
func repositoryView(t *testing.T, h *factorytest.Rig, name string) api.RepositoryView {
	t.Helper()
	p, err := h.Human().GetProject(context.Background(), h.Project)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(p.Repositories, func(r api.RepositoryView) bool { return r.Name == name })
	if i < 0 {
		t.Fatalf("the Project has no Repository %s", name)
	}
	return p.Repositories[i]
}

// setPushRemote has the human give the Repository name its own Push
// remote, or with remote "", have it use the Project's.
func setPushRemote(t *testing.T, h *factorytest.Rig, name, remote string) {
	t.Helper()
	if _, err := h.Human().EditRepository(context.Background(), h.Project, api.EditRepositoryInput{Name: name, PushRemote: &remote}); err != nil {
		t.Fatal(err)
	}
}

func TestUnderMergeAndPushAnUnpushedHomeLandsOnMergeWhileAPushedRepositoryIsPushedAndGated(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	apiRepo := h.AddRepository("api")
	mergeAndPush(h, "origin")
	trackCI(t, h)
	watchCIIn(t, h, "api", "acme/api")
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	if home := repositoryView(t, h, h.Project); !home.LocalOnly {
		t.Fatalf("Home %+v, want it local-only, its clone having no Push remote", home)
	}

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	homeMerge, apiPushed := h.Tip("main"), pushedIn(tk, "api")
	if landedIn(tk, h.Project) != homeMerge || pushedIn(tk, h.Project) != "" {
		t.Fatalf("Changed Repositories %+v, want the Home landed at its merge %s with nothing pushed", tk.Repositories, homeMerge)
	}
	if apiPushed == "" || apiPushed != apiRepo.Remote("origin").Tip("main") || landedIn(tk, "api") != "" {
		t.Fatalf("Changed Repositories %+v, want api's merge pushed to its origin and waiting on its CI", tk.Repositories)
	}
	r.WantOutput(id + ": pushed main to origin in Repository api\n")
	r.WantOutput(id + ": CI tracks main at " + apiPushed[:12] + " in Repository api")

	reportRunIn(t, h, "api", 7, id, apiPushed, domain.CICompleted, domain.CISuccess)
	r.Poll()

	if tk := h.WantState(id, domain.Done); landedIn(tk, "api") != apiPushed {
		t.Fatalf("Changed Repositories %+v, want api landed at its pushed commit that passed, %s", tk.Repositories, apiPushed)
	}
	if out := r.Stdout(); strings.Contains(out, "nothing lands") || strings.Contains(out, "doesn't have") {
		t.Errorf("stdout %q, want nothing stopped by the Home's missing remote", out)
	}
}

func TestEachRepositoryIsClonedPushedAndFetchedByItsOwnPushRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	home := h.AddRemote("origin")
	apiRepo := h.AddRepository("api")
	mergeAndPush(h, "origin")
	setPushRemote(t, h, "api", "upstream")
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	if !apiRepo.HasRemote("upstream") || apiRepo.HasRemote("origin") {
		t.Fatalf("api's clone has upstream %t and origin %t, want its own Push remote, upstream, alone", apiRepo.HasRemote("upstream"), apiRepo.HasRemote("origin"))
	}

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Done)
	homeMerge, apiMerge := h.Tip("main"), apiRepo.Tip("main")
	if got := home.Tip("main"); got != homeMerge {
		t.Errorf("the Home's origin's main is at %s, want its merge %s pushed", got, homeMerge)
	}
	if got := apiRepo.Remote("upstream").Tip("main"); got != apiMerge {
		t.Errorf("api's upstream's main is at %s, want its merge %s pushed", got, apiMerge)
	}
	if landedIn(tk, "api") != apiMerge || landedIn(tk, h.Project) != homeMerge {
		t.Fatalf("Changed Repositories %+v, want each landed at its pushed merge", tk.Repositories)
	}
	r.WantOutput(id + ": pushed main to origin\n")
	r.WantOutput(id + ": pushed main to upstream in Repository api\n")
}

func TestARepositoryWhoseCloneLacksItsOwnPushRemoteIsMissingAndHoldsBackOnlyItsTickets(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.AddRemote("origin")
	apiRepo := h.AddRepository("api")
	mergeAndPush(h, "origin")
	both := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(both, domain.InReview)

	// Its clone has origin alone.
	setPushRemote(t, h, "api", "upstream")
	home, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.Accept(both)
	r.Poll()

	wantMissing(t, h, apiRepo, "has no remote upstream, the Push remote")
	h.WantState(home, domain.Done)
	h.WantState(both, domain.Accepted)
	r.WantOutput(both + ": waits to land: Repository api has no clone")
	if out := r.Stdout(); strings.Contains(out, "nothing lands until") {
		t.Errorf("stdout %q, want no Project-wide stop", out)
	}
}

func TestTheFactoryReportsAHomeWithNoPushRemoteAtStartAndOneAddedLaterAtTheNextStart(t *testing.T) {
	t.Parallel()
	// Real git: the Home's origin is a github.com URL, a remote outside the
	// Fake git.
	h := factorytest.New(t, factorytest.RealGit())
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: "origin"})
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	h.WantState(id, domain.Done)
	if home := repositoryView(t, h, h.Project); !home.LocalOnly || home.CloneURL != "" {
		t.Fatalf("Home %+v, want it local-only: its clone has no Push remote", home)
	}

	// Added while running: reported at the next start.
	h.AddRemote("origin")
	h.SetRemote("origin", "git@github.com:acme/web.git")
	r.Poll()
	if home := repositoryView(t, h, h.Project); !home.LocalOnly {
		t.Fatalf("Home %+v, want it local-only until the next start", home)
	}
	r.Stop()

	h.Start()
	if home := repositoryView(t, h, h.Project); home.LocalOnly || home.CloneURL != "git@github.com:acme/web.git" {
		t.Fatalf("Home %+v, want it pushed, at origin's URL", home)
	}
}

func TestAChangedHomePushRemoteNameIsReportedAndLandingGoesOn(t *testing.T) {
	t.Parallel()
	// Real git: the Home's origin is a github.com URL, a remote outside the
	// Fake git.
	h := factorytest.New(t, factorytest.RealGit())
	h.AddRemote("origin")
	h.SetRemote("origin", "git@github.com:acme/web.git")
	r := h.Start()
	if home := repositoryView(t, h, h.Project); home.LocalOnly {
		t.Fatalf("Home %+v, want it pushed", home)
	}

	setPushRemote(t, h, h.Project, "nowhere")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r.Poll()

	if home := repositoryView(t, h, h.Project); !home.LocalOnly {
		t.Fatalf("Home %+v, want it local-only: its clone has no remote nowhere", home)
	}
	h.WantState(id, domain.Done)

	setPushRemote(t, h, h.Project, "")
	r.Poll()
	if home := repositoryView(t, h, h.Project); home.LocalOnly || home.CloneURL != "git@github.com:acme/web.git" {
		t.Fatalf("Home %+v, want it pushed again, at origin's URL", home)
	}
}

func TestALocalOnlyHomeUnderPullRequestIsSaidOnceAndHoldsBackOnlyTheTicketsLandingThere(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	apiRepo := h.AddRepository("api")
	// While its Push remote was last reported, the Home may land by Pull
	// Request; its clone has none now.
	pullRequestOn(h, "origin")
	both := inTwo(h, "Add an endpoint")
	only := inAPI(h, "Add another endpoint")
	r := h.Start()
	h.WantState(both, domain.InReview)
	h.WantState(only, domain.InReview)
	if home := repositoryView(t, h, h.Project); !home.LocalOnlyUnderPullRequest {
		t.Fatalf("Home %+v, want it local-only under Pull Request", home)
	}

	h.Accept(both)
	h.Accept(only)
	r.Poll()
	r.Poll()

	h.WantState(both, domain.Accepted)
	if apiRepo.Remote("origin").Exists(both) {
		t.Errorf("api's origin holds %s, want nothing of it pushed while the Home holds it back", both)
	}
	if pushed := wantPushedIn(t, h, "api", only); pushed != apiRepo.Tip(only) {
		t.Errorf("%s's pushed commit in api is %s, want its branch's tip", only, pushed)
	}
	r.WantOutput(both + ": waits to land: it is local-only under Pull Request\n")
	said := "warning: Repository " + h.Project + " is local-only under Pull Request"
	if got := strings.Count(r.Stdout(), said); got != 1 {
		t.Errorf("stdout %q says %q %d times, want once", r.Stdout(), said, got)
	}
	if out := r.Stdout(); strings.Contains(out, "can't fetch") {
		t.Errorf("stdout %q, want the local-only Home never fetched", out)
	}
}
