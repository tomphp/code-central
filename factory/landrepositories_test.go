package factory_test

import (
	"context"
	"github.com/tomoram/code-central/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Landing a Ticket across its Changed Repositories under Merge (Spec
// code-central-848, ADR 0049): the Factory checks the merge in every
// Repository before it lands any, then lands each in the landing order,
// merging, advancing the base and pushing in turn. A failed push leaves
// the Ticket Partly landed, to be tried again, and a broken base in one
// Repository holds back only the work touching it.

// inTwo makes a Ready Ticket whose session commits in the Home Repository
// and in api and finishes with both, and returns its ID.
func inTwo(h *factorytest.Harness, title string) string {
	id := h.ReadyTicket(title)
	h.Script(id, append(factorytest.CommitIn("api", "endpoint.go", "package api\n"),
		factorytest.WriteFile("client.go", "package acme\n"), factorytest.Git("add", "client.go"), factorytest.Git("commit", "-q", "-m", "Call it"),
		factorytest.FinishIn("Done.", true, "api"), factorytest.Result("success"))...)
	return id
}

// inAPI makes a Ready Ticket whose session commits in api alone and
// finishes with it, and returns its ID.
func inAPI(h *factorytest.Harness, title string) string {
	id := h.ReadyTicket(title)
	h.Script(id, append(factorytest.CommitIn("api", "endpoint.go", "package api\n"), factorytest.FinishIn("Done.", false, "api"), factorytest.Result("success"))...)
	return id
}

// setLandingOrder has the human put the Repositories in names' order.
func setLandingOrder(t *testing.T, h *factorytest.Harness, names ...string) {
	t.Helper()
	if _, err := h.Human().SetLandingOrder(context.Background(), h.Project, core.SetLandingOrderInput{Names: names}); err != nil {
		t.Fatal(err)
	}
}

func TestATwoRepositoryTicketLandsInEachInTheLandingOrder(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	api := h.AddRepository("api")
	mergeAndPush(h, "origin")
	setLandingOrder(t, h, "api", h.Project)
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Done)
	homeMerge, apiMerge := h.Git("rev-parse", "main"), h.GitIn(api.Path, "rev-parse", "main")
	if got := home.Git("rev-parse", "main"); got != homeMerge {
		t.Errorf("the Home Repository's remote main is at %s, want its merge %s", got, homeMerge)
	}
	if got := api.Remote("origin").Git("rev-parse", "main"); got != apiMerge {
		t.Errorf("api's remote main is at %s, want its merge %s", got, apiMerge)
	}
	if landedIn(tk, "api") != apiMerge || landedIn(tk, h.Project) != homeMerge {
		t.Fatalf("Changed Repositories %+v, want each landed at its merge", tk.Repositories)
	}
	out := r.Stdout()
	first := strings.Index(out, id+": landed on main at "+apiMerge[:12]+" in Repository api\n")
	second := strings.Index(out, id+": landed on main at "+homeMerge[:12]+"\n")
	if first < 0 || second < first {
		t.Fatalf("stdout %q, want api landed first, then the Home Repository: the landing order", out)
	}
}

func TestAConflictInEitherRepositoryLandsNeither(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	// api, landing after the Home Repository, conflicts.
	apiBase := api.CommitOn("main", "endpoint.go", "package other\n")
	homeBase := h.Git("rev-parse", "main")
	// Hold the next Claim's turn, so the Ticket stays as send_back left it.
	h.Script(id, factorytest.Wait())

	h.Accept(id)
	r.Poll()

	tk := h.Ticket(id)
	if !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s wasn't sent back: %+v", id, tk.Moves)
	}
	if got := h.Git("rev-parse", "main"); got != homeBase {
		t.Errorf("the Home Repository's main moved to %s: nothing lands when api conflicts", got)
	}
	if got := h.GitIn(api.Path, "rev-parse", "main"); got != apiBase {
		t.Errorf("api's main moved to %s", got)
	}
	if !commented(tk, "- api: merging", "into main at "+apiBase[:12], "  - endpoint.go") || commented(tk, "- "+h.Project+": merging") {
		t.Fatalf("comments %+v, want the send-back note to name api, its base and its file, and not the Home Repository", tk.Comments)
	}
	r.WantOutput(id + ": conflicts with the base in api: sent back to Ready")
}

func TestARefusedPushLeavesTheTicketPartlyLandedAndIsTriedAgain(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api := h.AddRepository("api")
	mergeAndPush(h, "origin")
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	remote := api.Remote("origin")
	remote.RefusePushes()

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	if !tk.PartlyLanded || landedIn(tk, h.Project) == "" || landedIn(tk, "api") != "" {
		t.Fatalf("acme-2 partly landed %v %+v, want it landed in the Home Repository alone", tk.PartlyLanded, tk.Repositories)
	}
	if note := tk.Comments[len(tk.Comments)-1].Body; !strings.HasPrefix(note, "The Factory can't land this in api yet: couldn't push main to origin") {
		t.Fatalf("comment %q, want it to say api's push was refused", note)
	}
	if tk.Repositories[1].NotLanded != "couldn't push main to origin" {
		t.Fatalf("api %+v, want why it hasn't landed", tk.Repositories[1])
	}
	merge := h.GitIn(api.Path, "rev-parse", "main")

	// Tried again, without merging again or moving the Home Repository.
	homeMain := h.Git("rev-parse", "main")
	remote.AcceptPushes()
	r.Poll()

	tk = h.WantState(id, domain.Done)
	if landedIn(tk, "api") == "" || remote.Git("rev-parse", "main") != merge || h.Git("rev-parse", "main") != homeMain {
		t.Fatalf("Changed Repositories %+v, want api landed, its first merge %s pushed", tk.Repositories, merge)
	}
}

func TestABrokenBaseInOneRepositoryHoldsBackOnlyTheTicketsTouchingIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRepository("api")
	homeOnly := h.ReadyTicket("Change the client")
	h.Script(homeOnly, factorytest.WriteFile("client.go", "package acme\n"), factorytest.Git("add", "client.go"), factorytest.Git("commit", "-q", "-m", "Change it"),
		factorytest.FinishIn("Done.", true), factorytest.Result("success"))
	both := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(homeOnly, domain.InReview)
	h.WantState(both, domain.InReview)
	h.Accept(homeOnly)
	h.Accept(both)
	// api's Base becomes a branch that exists nowhere.
	release := "release"
	if _, err := h.Human().EditRepository(context.Background(), h.Project, core.EditRepositoryInput{Name: "api", Base: &release}); err != nil {
		t.Fatal(err)
	}

	r.Poll()

	h.WantState(homeOnly, domain.Done)
	h.WantState(both, domain.Accepted)
	r.WantOutput(both + ": waits to land: its base, release, is missing in Repository api")
	rows := h.NeedsMeItems(core.ReasonBaseMissing)
	if len(rows) != 1 || rows[0].BaseMissing.Repository != "api" || rows[0].BaseMissing.Base != "release" {
		t.Fatalf("Base missing items %+v, want one naming api's release", rows)
	}
}

func TestAHandMergeInOneRepositoryLandsWhileTheHomeRepositorysBaseIsMissing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	id := inAPI(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	setBase(h, "release")
	h.GitIn(api.Path, "merge", "-q", "--no-ff", "-m", "Merge "+id, id)

	r.Poll()

	h.WantState(id, domain.Done)
	r.WantOutput(id + ": merged into main by hand in Repository api")
}

func TestADivergedBaseInOneRepositoryHoldsBackOnlyItsWork(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api := h.AddRepository("api")
	mergeAndPush(h, "origin")
	homeOnly := h.ReadyTicket("Change the client")
	h.Script(homeOnly, factorytest.WriteFile("client.go", "package acme\n"), factorytest.Git("add", "client.go"), factorytest.Git("commit", "-q", "-m", "Change it"),
		factorytest.FinishIn("Done.", true), factorytest.Result("success"))
	r := h.Start()
	h.WantState(homeOnly, domain.InReview)
	// api's remote and its clone's main each gain a commit the other
	// lacks, and they conflict.
	api.CommitOn("main", "README.md", "ours\n")
	theirs := pushElsewhere(t, h, api, "README.md", "theirs\n")

	h.Accept(homeOnly)
	r.Poll()

	h.WantState(homeOnly, domain.Done)
	rows := h.NeedsMeItems(core.ReasonBaseDiverged)
	if len(rows) != 1 || rows[0].BaseDiverged.Repository != "api" || rows[0].BaseDiverged.Remote != theirs {
		t.Fatalf("Base diverged items %+v, want one naming api, its remote at %s", rows, theirs)
	}
	r.WantOutput("warning: the base, main, in Repository api has diverged from the remote's")
}

func TestEachRepositorysBaseIsKeptLevelWithItsOwnRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api := h.AddRepository("api")
	mergeAndPush(h, "origin")
	r := h.Start()
	theirs := pushElsewhere(t, h, api, "other.go", "package api\n")

	r.Poll()

	if got := h.GitIn(api.Path, "rev-parse", "main"); got != theirs {
		t.Fatalf("api's main is at %s, want it fast-forwarded to its remote's %s", got, theirs)
	}
	r.WantOutput("main: fast-forwarded to origin/main at " + theirs[:12] + " in Repository api")
}

// pushElsewhere commits file with content on main in a clone of api of
// its own and pushes it to api's remote, as someone else would, and
// returns the commit.
func pushElsewhere(t *testing.T, h *factorytest.Harness, api *factorytest.Repository, file, content string) string {
	t.Helper()
	other := t.TempDir()
	h.GitIn(other, "clone", "-q", api.CloneURL, ".")
	if err := os.WriteFile(filepath.Join(other, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h.GitIn(other, "add", file)
	h.GitIn(other, "commit", "-q", "-m", "Theirs: "+file)
	h.GitIn(other, "push", "-q", "origin", "main")
	return h.GitIn(other, "rev-parse", "HEAD")
}

// The CI gate per Repository (Spec code-central-848, ADR 0034): under
// Merge + Push with CI Tracking on, each Repository lands once its own
// CI result is passed, at a pushed commit there holding the work; one
// Not watched lands once pushed.

// watchCIIn has the human set the Repository name's CI to GitHub Actions
// on repository, or with repository "", to Not watched.
func watchCIIn(t *testing.T, h *factorytest.Harness, name, repository string) {
	t.Helper()
	ci := core.RepositoryCI{Repository: repository, NotWatched: repository == ""}
	if _, err := h.Human().SetRepositoryCI(context.Background(), h.Project, core.SetRepositoryCIInput{Name: name, RepositoryCI: ci}); err != nil {
		t.Fatal(err)
	}
}

// pushedIn is tk's latest pushed commit in the Repository name, or "".
func pushedIn(tk core.TicketView, name string) string {
	for _, r := range tk.Repositories {
		if r.Name == name && r.PushedCommit != nil {
			return r.PushedCommit.Commit
		}
	}
	return ""
}

// gatedInTwo is a harness under Merge + Push to origin with CI Tracking
// on and the Repository api, its CI watched on acme/api unless notWatched,
// with a Ticket in both Accepted and polled once, so its merge is pushed
// in each. It returns the Ticket.
func gatedInTwo(t *testing.T, notWatched bool) (h *factorytest.Harness, api *factorytest.Repository, r *factorytest.Running, id string) {
	t.Helper()
	h = factorytest.New(t)
	h.AddRemote("origin")
	api = h.AddRepository("api")
	mergeAndPush(h, "origin")
	trackCI(t, h)
	if notWatched {
		watchCIIn(t, h, "api", "")
	} else {
		watchCIIn(t, h, "api", "acme/api")
	}
	id, r = acceptedIn(t, h)
	return h, api, r, id
}

func TestUnderTheCIGateEachRepositoryLandsOnceItsOwnCIResultPasses(t *testing.T) {
	t.Parallel()
	h, api, r, id := gatedInTwo(t, false)

	tk := h.WantState(id, domain.Accepted)
	homePushed, apiPushed := pushedIn(tk, h.Project), pushedIn(tk, "api")
	if homePushed == "" || apiPushed == "" || apiPushed != api.Remote("origin").Git("rev-parse", "main") {
		t.Fatalf("Changed Repositories %+v, want each merge pushed and reported, api's at its remote's main", tk.Repositories)
	}
	if tk.PartlyLanded {
		t.Fatalf("Changed Repositories %+v, want it landed nowhere before any CI passes", tk.Repositories)
	}
	r.WantOutput(id + ": CI tracks main at " + apiPushed[:12] + " in Repository api")

	reportRunIn(t, h, "api", 7, id, apiPushed, domain.CICompleted, domain.CISuccess)
	r.Poll()

	tk = h.WantState(id, domain.Accepted)
	if landedIn(tk, "api") != apiPushed || !tk.PartlyLanded || tk.Repositories[0].Landed != nil {
		t.Fatalf("Changed Repositories %+v, want api landed at its pushed commit that passed, %s, while the Home Repository waits on its CI", tk.Repositories, apiPushed)
	}
	// A later tip of api's base is no business of a Ticket landed there.
	pushElsewhere(t, h, api, "other.go", "package api\n")
	r.Poll()
	if tk := h.Ticket(id); pushedIn(tk, "api") != apiPushed {
		t.Fatalf("api's pushed commit is %q, want it left at %s: the Ticket has landed there", pushedIn(tk, "api"), apiPushed)
	}

	reportRun(t, h, 1, id, homePushed, domain.CICompleted, domain.CISuccess)
	r.Poll()

	tk = h.WantState(id, domain.Done)
	if landedIn(tk, h.Project) != homePushed || landedIn(tk, "api") != apiPushed {
		t.Fatalf("Changed Repositories %+v, want each landed at its pushed commit that passed", tk.Repositories)
	}
}

func TestUnderTheCIGateARedBuildInARepositoryHoldsBackOnlyItsLanding(t *testing.T) {
	t.Parallel()
	h, _, r, id := gatedInTwo(t, false)
	tk := h.Ticket(id)
	homePushed, apiPushed := pushedIn(tk, h.Project), pushedIn(tk, "api")

	reportRunIn(t, h, "api", 7, id, apiPushed, domain.CICompleted, domain.CIFailure)
	reportRun(t, h, 1, id, homePushed, domain.CICompleted, domain.CISuccess)
	r.Poll()

	tk = h.WantState(id, domain.Accepted)
	if landedIn(tk, h.Project) != homePushed || landedIn(tk, "api") != "" {
		t.Fatalf("Changed Repositories %+v, want the Home Repository landed and api held back by its red build", tk.Repositories)
	}
	r.WantOutput(id + ": CI failed at " + apiPushed[:12] + " in Repository api, so it stays Accepted")
}

func TestUnderTheCIGateANotWatchedRepositoryLandsOncePushed(t *testing.T) {
	t.Parallel()
	h, api, r, id := gatedInTwo(t, true)

	tk := h.WantState(id, domain.Accepted)
	apiMerge := api.Remote("origin").Git("rev-parse", "main")
	if landedIn(tk, "api") != apiMerge || tk.Repositories[0].Landed != nil || pushedIn(tk, "api") != "" {
		t.Fatalf("Changed Repositories %+v, want api landed at its pushed merge %s, with no pushed commit for CI, and the Home Repository waiting on its CI", tk.Repositories, apiMerge)
	}

	reportRun(t, h, 1, id, pushedIn(tk, h.Project), domain.CICompleted, domain.CISuccess)
	r.Poll()
	h.WantState(id, domain.Done)
}

func TestUnderTheCIGateALaterTipOfARepositorysBaseIsReportedToTheTicketsWaitingThere(t *testing.T) {
	t.Parallel()
	h, api, r, id := gatedInTwo(t, false)
	tip := pushElsewhere(t, h, api, "other.go", "package api\n")

	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	if pushedIn(tk, "api") != tip {
		t.Fatalf("api's pushed commit is %q, want its remote's later tip %s, which holds the work", pushedIn(tk, "api"), tip)
	}
	reportRunIn(t, h, "api", 8, id, tip, domain.CICompleted, domain.CISuccess)
	r.Poll()
	if tk := h.Ticket(id); landedIn(tk, "api") != tip {
		t.Fatalf("Changed Repositories %+v, want api landed at the tip that passed, %s", tk.Repositories, tip)
	}
}

func TestUnderTheCIGateLandAnywayOnARepositorysRedBuildLandsItThere(t *testing.T) {
	t.Parallel()
	h, _, r, id := gatedInTwo(t, false)
	apiPushed := pushedIn(h.Ticket(id), "api")
	reportRunIn(t, h, "api", 7, id, apiPushed, domain.CICompleted, domain.CIFailure)
	r.Poll()

	if _, err := h.Server.Core.LandAnyway(core.WithActor(context.Background(), core.Human), h.Project, core.CIResponseInput{Ticket: core.Ref(id), Repository: "api"}); err != nil {
		t.Fatal(err)
	}
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	if landedIn(tk, "api") != apiPushed || tk.Repositories[0].Landed != nil {
		t.Fatalf("Changed Repositories %+v, want api landed anyway at %s, and the Home Repository still waiting on its own CI", tk.Repositories, apiPushed)
	}
	r.WantOutput(id + ": landing anyway at " + apiPushed[:12] + " in Repository api, as you chose, though its CI failed")
}
