package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// Local-only Repositories (Spec code-central-879): a Repository with no
// Clone URL is never cloned. The Factory adopts the git repository at its
// local path with no remote check, and raises Repository missing when
// nothing usable is there. A Repository whose path is another's is
// Repository missing whatever its Clone URL.

// addLocalOnly adds Repository name to the Project with the local path
// dir and no Clone URL, leaving dir as it is.
func addLocalOnly(t *testing.T, h *factorytest.Harness, name, dir string) *factorytest.Repository {
	t.Helper()
	if _, err := h.Human().AddRepository(context.Background(), h.Project, api.AddRepositoryInput{Name: name, LocalPath: dir}); err != nil {
		t.Fatal(err)
	}
	return h.RepositoryAt(name, dir)
}

// editCloneURL has the human set Repository name's Clone URL to url.
func editCloneURL(t *testing.T, h *factorytest.Harness, name, url string) {
	t.Helper()
	if _, err := h.Human().EditRepository(context.Background(), h.Project, api.EditRepositoryInput{Name: name, CloneURL: &url}); err != nil {
		t.Fatal(err)
	}
}

// addUnrelatedOrigin gives the clone of r, a local-only Repository, a
// remote named origin, the Push remote's name, that isn't the
// Repository's: a repository of its own, other, whose main shares no
// history with r's. It returns other.
func addUnrelatedOrigin(h *factorytest.Harness, r *factorytest.Repository) *factorytest.Repository {
	other := h.MakeRepository("other")
	r.SetRemote("origin", other.CloneURL)
	return other
}

func TestALocalOnlyRepositoryIsAdoptedWithNoRemoteCheck(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	web := h.AddLocalRepository("web")
	head := web.Head(web.Path)

	r := h.Start()
	r.WantOutput("repository web: adopted the clone at " + web.Path)
	r.Poll()
	if c := clone(t, h, "web"); c == nil || c.State != api.CloneReady || c.Path != web.Path {
		t.Errorf("web's clone %+v, want ready at %s", c, web.Path)
	}
	if web.HasRemote("origin") {
		t.Errorf("web has a remote origin, want none added")
	}
	if got := web.Head(web.Path); got != head {
		t.Errorf("web is at %s, want it left at %s", got, head)
	}
}

func TestALocalOnlyRepositoryWithAnUnrelatedRemoteIsAdoptedAsItIs(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	web := h.AddLocalRepository("web")
	other := addUnrelatedOrigin(h, web)

	h.Start().Poll()
	if c := clone(t, h, "web"); c == nil || c.State != api.CloneReady {
		t.Errorf("web's clone %+v, want ready", c)
	}
	if got := web.RemoteURL("origin"); got != other.CloneURL {
		t.Errorf("web's origin is %s, want it left as %s", got, other.CloneURL)
	}
}

func TestALocalOnlyRepositoryWithNothingAtItsPathIsRepositoryMissingAndNeverCloned(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	web := addLocalOnly(t, h, "web", filepath.Join(filepath.Dir(h.Repo), "web"))
	id := h.ReadyTicket("Waits for every Repository")

	r := h.Start()
	r.WantOutput("warning: Repository web is missing: nothing at " + web.Path)
	r.Poll()
	wantMissing(t, h, web, "nothing at "+web.Path)
	if _, err := os.Lstat(web.Path); err == nil {
		t.Errorf("%s was made, want nothing cloned there", web.Path)
	}
	if s := h.Ticket(id).State; s != domain.Ready {
		t.Errorf("%s is %s, want it not claimed while web is missing", id, s)
	}
	if got := files(t, filepath.Dir(h.Repo)); !slices.Equal(got, []string{"repo", "repo.worktrees"}) {
		t.Errorf("the Factory left %v beside the Home Repository, want nothing of its own", got)
	}
}

func TestALocalOnlyRepositorysPathHoldingSomethingElseIsRepositoryMissing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	web := addLocalOnly(t, h, "web", filepath.Join(filepath.Dir(h.Repo), "web"))
	if err := os.MkdirAll(web.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web.Path, "plans.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h.Start().Poll()
	wantMissing(t, h, web, web.Path+" isn't a git repository")
	if got := files(t, web.Path); !slices.Equal(got, []string{"plans.txt"}) {
		t.Errorf("%s holds %v, want it untouched", web.Path, got)
	}
}

func TestALocalOnlyRepositoryInsideAnotherIsRepositoryMissing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	nested := filepath.Join(h.Repo, "web")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	web := addLocalOnly(t, h, "web", nested)

	h.Start().Poll()
	wantMissing(t, h, web, "inside the git repository at")
}

func TestALocalOnlyRepositoryGetsAWorktreeAndLandsOnceMerged(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	apiRepo := h.AddLocalRepository("api")
	id := inAPI(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	wantOnBranch(t, h, id, apiRepo.Worktree(id))
	if got := h.SubjectIn(apiRepo.Path, apiRepo.Tip(id)); got != "Change endpoint.go" {
		t.Errorf("api's branch %s ends with %q, want the session's commit", id, got)
	}

	h.Accept(id)
	r.Poll()
	tk := h.WantState(id, domain.Done)
	if merge := apiRepo.Tip("main"); landedIn(tk, "api") != merge {
		t.Errorf("Changed Repositories %+v, want api landed at its merge %s", tk.Repositories, merge)
	}
}

func TestACloneURLAddedLaterIsCheckedAgainstTheExistingClone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	web := h.MakeRepository("web")
	other := h.MakeRepository("other")
	h.CloneByHand(web.CloneURL, web.Path)
	addLocalOnly(t, h, "web", web.Path)
	r := h.Start()
	r.Poll()
	if c := clone(t, h, "web"); c == nil || c.State != api.CloneReady {
		t.Fatalf("web's clone %+v, want ready", c)
	}

	editCloneURL(t, h, "web", web.CloneURL)
	r.Poll()
	if c := clone(t, h, "web"); c == nil || c.State != api.CloneReady {
		t.Fatalf("web's clone %+v, want ready with the Clone URL its origin is", c)
	}

	editCloneURL(t, h, "web", other.CloneURL)
	r.Poll()
	wantMissing(t, h, &factorytest.Repository{Name: "web", CloneURL: other.CloneURL, Path: web.Path}, "origin is "+web.CloneURL)
	if got := web.RemoteURL("origin"); got != web.CloneURL {
		t.Errorf("web's origin is %s, want it never rewritten", got)
	}
}

func TestARepositoryAtAnEarlierRepositorysPathIsRepositoryMissing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	web := h.AddRepository("web")
	// Before the Factory's first report Code Central can't see that web's
	// default path is api's local path.
	apiRepo := addLocalOnly(t, h, "api", web.Path)

	r := h.Start()
	r.WantOutput("warning: Repository api is missing: " + web.Path + " is Repository web's clone")
	r.Poll()
	wantMissing(t, h, apiRepo, web.Path+" is Repository web's clone")
	if c := clone(t, h, "web"); c == nil || c.State != api.CloneReady {
		t.Errorf("web's clone %+v, want ready", c)
	}
}

func TestARepositoryWithACloneURLAtAnEarlierRepositorysPathIsNeitherClonedNorAdopted(t *testing.T) {
	t.Parallel()
	// Real git: the site's path holds a repository with no commit, which the
	// Fake git has no way to make.
	h := factorytest.New(t, factorytest.RealGit())
	site := h.MakeRepository("site")
	// web, local-only, is at site's default path, which Code Central
	// can't see before the Factory's first report.
	h.GitIn(filepath.Dir(h.Repo), "init", "-q", "-b", "main", site.Path)
	web := addLocalOnly(t, h, "web", site.Path)
	if _, err := h.Human().AddRepository(context.Background(), h.Project, api.AddRepositoryInput{Name: "site", CloneURL: site.CloneURL}); err != nil {
		t.Fatal(err)
	}

	h.Start().Poll()
	wantMissing(t, h, site, site.Path+" is Repository web's clone")
	if web.HasRemote("origin") {
		t.Errorf("web has a remote origin, want none added")
	}
	if got := files(t, web.Path); !slices.Equal(got, []string{".git"}) {
		t.Errorf("%s holds %v, want nothing cloned into it", web.Path, got)
	}
}

func TestARepositoryAtTheHomeRepositorysPathIsRepositoryMissing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	web := addLocalOnly(t, h, "web", h.Repo)
	// First in the landing order, it still can't take the Home's path.
	setLandingOrder(t, h, "web", h.Project)

	h.Start().Poll()
	wantMissing(t, h, web, h.Repo+" is Repository "+h.Project+"'s clone")
}
