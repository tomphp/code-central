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
