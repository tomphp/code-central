package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// The Factory's Repositories (Spec code-central-848): it clones each of
// the Project's Repositories beside the Home Repository, adopts a
// matching clone, reports each clone, and holds new Claims while any is
// Repository missing.

// clone is the Factory's last clone report of the Repository name.
func clone(t *testing.T, h *factorytest.Harness, name string) *core.CloneView {
	t.Helper()
	p, err := h.Human().GetProject(context.Background(), h.Project)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(p.Repositories, func(r core.RepositoryView) bool { return r.Name == name })
	if i < 0 {
		t.Fatalf("the Project has no Repository %s", name)
	}
	return p.Repositories[i].Clone
}

// repositoryMissingItems are the Project's Repository missing items in
// Needs Me.
func repositoryMissingItems(t *testing.T, h *factorytest.Harness) []core.RepositoryMissingView {
	t.Helper()
	v, err := h.Human().NeedsMe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []core.RepositoryMissingView
	for _, g := range v.Groups {
		for _, row := range g.Tickets {
			if g.Reason == core.ReasonRepositoryMissing && row.Project == h.Project {
				out = append(out, *row.RepositoryMissing)
			}
		}
	}
	return out
}

// wantMissing checks Repository r is reported missing with a problem
// saying problem, in its clone and its Needs Me item.
func wantMissing(t *testing.T, h *factorytest.Harness, r *factorytest.Repository, problem string) {
	t.Helper()
	c := clone(t, h, r.Name)
	if c == nil || c.State != core.CloneMissing || c.Path != r.Path || !strings.Contains(c.Problem, problem) {
		t.Fatalf("%s's clone %+v, want missing at %s, its problem saying %q", r.Name, c, r.Path, problem)
	}
	items := repositoryMissingItems(t, h)
	if len(items) != 1 || items[0].Repository != r.Name || items[0].Path != r.Path || items[0].CloneURL != r.CloneURL || items[0].Problem != c.Problem || items[0].Fix == "" {
		t.Fatalf("Repository missing items %+v, want one for %s with its path, Clone URL, problem and fix", items, r.Name)
	}
}

// files is every name in dir, sorted.
func files(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestARepositoryAddedWhileTheFactoryRunsIsClonedOnTheNextPoll(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	r := h.Start()
	web := h.AddRepository("web")

	r.Poll()
	r.WantOutput("repository web: cloned " + web.CloneURL + " to " + web.Path)
	if got := h.GitIn(web.Path, "remote", "get-url", "origin"); got != web.CloneURL {
		t.Errorf("the clone's origin is %s, want %s", got, web.CloneURL)
	}
	if got, want := h.GitIn(web.Path, "rev-parse", "HEAD"), h.GitIn(web.CloneURL, "rev-parse", "main"); got != want {
		t.Errorf("the clone is at %s, want the remote's main, %s", got, want)
	}
	if c := clone(t, h, "web"); c == nil || c.State != core.CloneReady || c.Path != web.Path {
		t.Errorf("web's clone %+v, want ready at %s", c, web.Path)
	}
	if c := clone(t, h, h.Project); c == nil || c.State != core.CloneReady || c.Path != h.Repo {
		t.Errorf("the Home Repository's clone %+v, want ready at %s", c, h.Repo)
	}
	if items := repositoryMissingItems(t, h); len(items) != 0 {
		t.Errorf("Repository missing items %+v, want none", items)
	}

	id := h.ReadyTicket("Starts with every Repository cloned")
	r.Poll()
	h.WantState(id, store.InReview)
}

func TestTheCloneIsMadeUnderThePushRemotesName(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("upstream")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "upstream"})
	web := h.AddRepository("web")

	h.Start()
	if got := h.GitIn(web.Path, "remote"); got != "upstream" {
		t.Errorf("the clone's remotes are %q, want the Push remote's name, upstream, alone", got)
	}
}

func TestAnExistingMatchingCloneIsAdoptedAsItIs(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	web := h.MakeRepository("web")
	h.GitIn(filepath.Dir(web.Path), "clone", "-q", web.CloneURL, web.Path)
	// The clone's own URL differs from the Clone URL only in its .git and
	// a trailing slash, and it holds work of its own.
	h.GitIn(web.Path, "remote", "set-url", "origin", strings.TrimSuffix(web.CloneURL, ".git")+"/")
	if err := os.WriteFile(filepath.Join(web.Path, "notes.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.GitIn(web.Path, "checkout", "-q", "-b", "mine")
	head := h.GitIn(web.Path, "rev-parse", "HEAD")
	if _, err := h.Human().AddRepository(context.Background(), h.Project, core.AddRepositoryInput{Name: "web", CloneURL: web.CloneURL}); err != nil {
		t.Fatal(err)
	}

	r := h.Start()
	r.WantOutput("repository web: adopted the clone at " + web.Path)
	// The Factory's first Heartbeat follows its first poll, so its first
	// report stands from the second.
	r.Poll()
	if c := clone(t, h, "web"); c == nil || c.State != core.CloneReady || c.Path != web.Path {
		t.Errorf("web's clone %+v, want ready at %s", c, web.Path)
	}
	if got := h.GitIn(web.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != "mine" {
		t.Errorf("the clone is on %s, want it left on mine", got)
	}
	if got := h.GitIn(web.Path, "rev-parse", "HEAD"); got != head {
		t.Errorf("the clone is at %s, want it left at %s", got, head)
	}
	if got := h.GitIn(web.Path, "status", "--porcelain"); got != "?? notes.txt" {
		t.Errorf("the clone's status is %q, want its own notes.txt left alone", got)
	}
}

func TestARepositoryAtItsLocalPathIsAdopted(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	web := h.MakeRepository("web")
	elsewhere := filepath.Join(filepath.Dir(filepath.Dir(h.Repo)), "src", "web")
	h.GitIn(filepath.Dir(h.Repo), "clone", "-q", web.CloneURL, elsewhere)
	if _, err := h.Human().AddRepository(context.Background(), h.Project, core.AddRepositoryInput{Name: "web", CloneURL: web.CloneURL, LocalPath: elsewhere}); err != nil {
		t.Fatal(err)
	}

	h.Start().Poll()
	if c := clone(t, h, "web"); c == nil || c.State != core.CloneReady || c.Path != elsewhere {
		t.Errorf("web's clone %+v, want ready at its local path %s", c, elsewhere)
	}
	if _, err := os.Stat(web.Path); err == nil {
		t.Errorf("%s was made, want the local path used instead", web.Path)
	}
}

func TestAPathHoldingSomethingElseIsRepositoryMissingAndHoldsClaimsUntilMovedAside(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	web := h.MakeRepository("web")
	if err := os.MkdirAll(web.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web.Path, "plans.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Human().AddRepository(context.Background(), h.Project, core.AddRepositoryInput{Name: "web", CloneURL: web.CloneURL}); err != nil {
		t.Fatal(err)
	}
	id := h.ReadyTicket("Waits for every Repository")

	r := h.Start()
	r.WantOutput("warning: Repository web is missing: " + web.Path + " isn't a git repository")
	r.Poll()
	wantMissing(t, h, web, "isn't a git repository")
	if s := h.Ticket(id).State; s != store.Ready {
		t.Errorf("%s is %s, want it not claimed while web is missing", id, s)
	}
	if got := files(t, web.Path); !slices.Equal(got, []string{"plans.txt"}) {
		t.Errorf("%s holds %v, want it untouched", web.Path, got)
	}

	if err := os.Rename(web.Path, web.Path+".aside"); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	h.WantState(id, store.InReview)
	if c := clone(t, h, "web"); c == nil || c.State != core.CloneReady {
		t.Errorf("web's clone %+v, want ready once cloned", c)
	}
	if items := repositoryMissingItems(t, h); len(items) != 0 {
		t.Errorf("Repository missing items %+v, want none once web is cloned", items)
	}
}

func TestACloneOfAnotherRepositoryIsRepositoryMissing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	web := h.MakeRepository("web")
	other := h.MakeRepository("other")
	h.GitIn(filepath.Dir(web.Path), "clone", "-q", other.CloneURL, web.Path)
	head := h.GitIn(web.Path, "rev-parse", "HEAD")
	if _, err := h.Human().AddRepository(context.Background(), h.Project, core.AddRepositoryInput{Name: "web", CloneURL: web.CloneURL}); err != nil {
		t.Fatal(err)
	}

	h.Start().Poll()
	wantMissing(t, h, web, "origin is "+other.CloneURL)
	if got := h.GitIn(web.Path, "rev-parse", "HEAD"); got != head {
		t.Errorf("the clone is at %s, want it untouched at %s", got, head)
	}
	if got := h.GitIn(web.Path, "remote", "get-url", "origin"); got != other.CloneURL {
		t.Errorf("the clone's origin is %s, want it untouched", got)
	}
}

func TestAFailedCloneIsRepositoryMissing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	gone := filepath.Join(filepath.Dir(filepath.Dir(h.Repo)), "repositories", "gone.git")
	if _, err := h.Human().AddRepository(context.Background(), h.Project, core.AddRepositoryInput{Name: "gone", CloneURL: gone}); err != nil {
		t.Fatal(err)
	}
	id := h.ReadyTicket("Waits for every Repository")

	r := h.Start()
	r.Poll()
	wantMissing(t, h, &factorytest.Repository{Name: "gone", CloneURL: gone, Path: filepath.Join(filepath.Dir(h.Repo), "gone")}, "git clone failed")
	if s := h.Ticket(id).State; s != store.Ready {
		t.Errorf("%s is %s, want it not claimed while gone is missing", id, s)
	}
	if got := files(t, filepath.Dir(h.Repo)); !slices.Equal(got, []string{"repo", "repo.worktrees"}) {
		t.Errorf("a failed clone left %v beside the Home Repository, want nothing of its own", got)
	}
}

func TestBackToTheHomeRepositoryAloneTheCloneReportIsCleared(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	web := h.AddRepository("web")
	r := h.Start()
	r.Poll()
	if c := clone(t, h, "web"); c == nil {
		t.Fatal("web's clone isn't reported")
	}
	if _, err := h.Human().RemoveRepository(context.Background(), h.Project, core.RemoveRepositoryInput{Name: "web"}); err != nil {
		t.Fatal(err)
	}
	r.Poll()

	// Added again, web has no clone report until the Factory's next poll.
	if _, err := h.Human().AddRepository(context.Background(), h.Project, core.AddRepositoryInput{Name: "web", CloneURL: web.CloneURL}); err != nil {
		t.Fatal(err)
	}
	if c := clone(t, h, "web"); c != nil {
		t.Errorf("web's clone %+v before the Factory has looked again, want none", c)
	}
}

func TestARemovedRepositorysCloneIsLeftOnDisk(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	web := h.AddRepository("web")
	r := h.Start()
	if _, err := h.Human().RemoveRepository(context.Background(), h.Project, core.RemoveRepositoryInput{Name: "web"}); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	if got := h.GitIn(web.Path, "remote", "get-url", "origin"); got != web.CloneURL {
		t.Errorf("the removed Repository's clone has origin %q, want it left as it was", got)
	}
}
