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

// Land per Spec across Repositories (Spec code-central-848, ADR 0049): a
// Spec Branch per Changed Repository, all checked before any lands, one
// Spec merge Ticket per Spec naming each conflicting Repository, and the
// same "Merge spec" commit in each Repository in the landing order.

// specInTwo is a Requirement Spec with the Repository api and a Ready
// Ticket whose session commits in both (inTwo), and returns the Ticket.
func specInTwo(h *factorytest.Harness) (api *factorytest.Repository, id string) {
	h.MakeRequirement()
	api = h.AddRepository("api")
	return api, inTwo(h, "Add an endpoint")
}

// subject is the subject of commit in the repository at dir.
func subject(h *factorytest.Harness, dir, commit string) string {
	return h.GitIn(dir, "log", "-1", "--format=%s", commit)
}

func TestATwoRepositorySpecLandsWithTheSameMergeSpecCommitInEachInTheLandingOrder(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	api, id := specInTwo(h)
	mergeAndPush(h, "origin")
	setLandingOrder(t, h, "api", h.Project)
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Done)
	homeMerge, apiMerge := h.Git("rev-parse", "main"), h.GitIn(api.Path, "rev-parse", "main")
	want := "Merge spec " + h.SpecID() + ": Work"
	if got := subject(h, h.Repo, homeMerge); got != want {
		t.Errorf("the Home Repository's merge is %q, want %q", got, want)
	}
	if got := subject(h, api.Path, apiMerge); got != want {
		t.Errorf("api's merge is %q, want %q", got, want)
	}
	if home.Git("rev-parse", "main") != homeMerge || api.Remote("origin").Git("rev-parse", "main") != apiMerge {
		t.Error("a merge wasn't pushed to its Repository's remote")
	}
	if landedIn(tk, "api") != apiMerge || landedIn(tk, h.Project) != homeMerge {
		t.Fatalf("Changed Repositories %+v, want each landed at its merge", tk.Repositories)
	}
	out := r.Stdout()
	first := strings.Index(out, h.SpecID()+": merged spec/"+h.SpecID()+" into main at "+apiMerge[:12]+" in Repository api\n")
	second := strings.Index(out, h.SpecID()+": merged spec/"+h.SpecID()+" into main at "+homeMerge[:12]+"\n")
	if first < 0 || second < first {
		t.Fatalf("stdout %q, want the Spec merged in api first, then the Home Repository: the landing order", out)
	}
	r.WantOutput(id + ": merged into spec/" + h.SpecID() + " at ")
}

func TestASpecConflictInAnyRepositoryOpensOneSpecMergeTicketNamingEachAndNothingLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, id := specInTwo(h)
	release := holdTicketsStage(t, h)
	fix := "acme-5"
	h.Script(fix, factorytest.Wait())
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	r.WantOutput(id + ": merged into spec/" + h.SpecID() + " at ")
	// Each base moves on, conflicting with its Spec Branch.
	homeBase := h.CommitOn("main", "client.go", "package other\n")
	apiBase := api.CommitOn("main", "endpoint.go", "package other\n")

	release()
	r.Poll()

	h.WantState(id, domain.Accepted)
	if h.Git("rev-parse", "main") != homeBase || h.GitIn(api.Path, "rev-parse", "main") != apiBase {
		t.Fatal("a base moved: nothing lands while any Repository's Spec Branch conflicts")
	}
	spec := "spec/" + h.SpecID()
	homeTip, apiTip := h.Git("rev-parse", spec), h.GitIn(api.Path, "rev-parse", spec)
	tk := h.Ticket(fix)
	if !tk.SpecMergeTicket || tk.Title != core.SpecMergeTitle(h.SpecID()) {
		t.Fatalf("%s %+v, want the Spec merge Ticket", fix, tk)
	}
	for _, line := range []string{
		"- " + h.Project + ": merging " + spec + " at " + homeTip + " into main at " + homeBase + " conflicts in these files:\n  - client.go\n",
		"- api: merging " + spec + " at " + apiTip + " into main at " + apiBase + " conflicts in these files:\n  - endpoint.go\n",
	} {
		if !strings.Contains(tk.Description, line) {
			t.Errorf("description %q, want it to say %q", tk.Description, line)
		}
	}
	r.WantOutput(h.SpecID() + ": " + spec + " conflicts with the base in " + h.Project + ", api: opened " + fix)
}

func TestATicketConflictingWithItsSpecBranchInOneRepositoryIsMergedIntoNeither(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, first := specInTwo(h)
	release := holdTicketsStage(t, h)
	defer release()
	// Cut from the same Spec Branch tips, it writes api's endpoint.go too.
	second := h.ReadyTicket("Change the endpoint")
	h.Script(second, append(factorytest.CommitIn("api", "endpoint.go", "package other\n"),
		factorytest.WriteFile("other.go", "package acme\n"), factorytest.Git("add", "other.go"), factorytest.Git("commit", "-q", "-m", "Other"),
		factorytest.FinishIn("Done.", true, "api"), factorytest.Result("success"))...)
	r := h.Start()
	h.WantState(first, domain.InReview)
	h.WantState(second, domain.InReview)
	h.Accept(first)
	r.Poll()
	spec := "spec/" + h.SpecID()
	homeTip, apiTip := h.Git("rev-parse", spec), h.GitIn(api.Path, "rev-parse", spec)
	// Hold the next Claim's turn, so the Ticket stays as send_back left it.
	h.Script(second, factorytest.Wait())

	h.Accept(second)
	r.Poll()

	tk := h.Ticket(second)
	if !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s wasn't sent back: %+v", second, tk.Moves)
	}
	if h.Git("rev-parse", spec) != homeTip || h.GitIn(api.Path, "rev-parse", spec) != apiTip {
		t.Fatal("a Spec Branch moved: a Ticket conflicting in any Repository is merged into none")
	}
	if !commented(tk, "- api: merging", "into "+spec+" at "+apiTip[:12], "  - endpoint.go") || commented(tk, "- "+h.Project+": merging") {
		t.Fatalf("comments %+v, want the send-back note to name api's Spec Branch and file, and not the Home Repository", tk.Comments)
	}
	r.WantOutput(second + ": conflicts with the Spec Branch in api: sent back to Ready")
}

func TestARefusedPushLeavesASpecsTicketPartlyLandedAndIsTriedAgain(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api, id := specInTwo(h)
	mergeAndPush(h, "origin")
	r := h.Start()
	h.WantState(id, domain.InReview)
	remote := api.Remote("origin")
	remote.RefusePushes()

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	if !tk.PartlyLanded || landedIn(tk, h.Project) == "" || landedIn(tk, "api") != "" {
		t.Fatalf("partly landed %v %+v, want it landed in the Home Repository alone", tk.PartlyLanded, tk.Repositories)
	}
	if !commented(tk, "The Factory can't land this in api yet: couldn't push main to origin") {
		t.Fatalf("comments %+v, want one saying api's push was refused", tk.Comments)
	}
	merge := h.GitIn(api.Path, "rev-parse", "main")
	homeMain := h.Git("rev-parse", "main")

	remote.AcceptPushes()
	r.Poll()

	// Its work is in api's base, so it lands there through the plain path,
	// as with the Home Repository alone, without merging again.
	tk = h.WantState(id, domain.Done)
	if landedIn(tk, "api") == "" || remote.Git("rev-parse", "main") != merge || h.Git("rev-parse", "main") != homeMain {
		t.Fatalf("Changed Repositories %+v, want api landed and its Spec's merge %s pushed, and the Home Repository left as it was", tk.Repositories, merge)
	}
	if got := subject(h, api.Path, merge); got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("api's merge is %q, want the Spec's", got)
	}
}

func TestOnceTheSpecMergeTicketIsAcceptedATwoRepositorySpecLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, id := specInTwo(h)
	release := holdTicketsStage(t, h)
	// The Spec merge Ticket merges api's base into its branch there.
	fix := "acme-5"
	h.Script(fix, factorytest.InRepository("api", factorytest.Git("merge", "-q", "-X", "theirs", "main")), factorytest.FinishIn("Merged the base.", false, "api"), factorytest.Result("success"))
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	api.CommitOn("main", "endpoint.go", "package other\n")

	release()
	r.Poll()
	h.WantState(fix, domain.InReview)
	if _, err := h.Human().Accept(context.Background(), h.Project, core.TicketInput{Ticket: core.Ref(fix)}); err != nil {
		t.Fatal(err)
	}
	r.Poll()

	h.WantState(id, domain.Done)
	h.WantState(fix, domain.Done)
}

func TestUnderTheCIGateASpecStuckInOneRepositoryLandsThereOnceUnstuck(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api, id := specInTwo(h)
	mergeAndPush(h, "origin")
	trackCI(t, h)
	watchCIIn(t, h, "api", "acme/api")
	r := h.Start()
	h.WantState(id, domain.InReview)
	// An untracked file in api's clone is in the way of its base's move.
	inTheWay := filepath.Join(api.Path, "endpoint.go")
	if err := os.WriteFile(inTheWay, []byte("package mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	if pushedIn(tk, h.Project) == "" || pushedIn(tk, "api") != "" || !tk.WaitingForSpec {
		t.Fatalf("waiting for its Spec %v, %+v: want the Spec's merge pushed in the Home Repository alone, and the Ticket still waiting for its Spec in api", tk.WaitingForSpec, tk.Repositories)
	}
	if !commented(tk, "The Factory can't land this in api yet: uncommitted changes in "+api.Path) {
		t.Fatalf("comments %+v, want one saying what is in api's way", tk.Comments)
	}

	if err := os.Remove(inTheWay); err != nil {
		t.Fatal(err)
	}
	r.Poll()

	tk = h.WantState(id, domain.Accepted)
	apiPushed := pushedIn(tk, "api")
	if apiPushed == "" || subject(h, api.Path, apiPushed) != "Merge spec "+h.SpecID()+": Work" {
		t.Fatalf("api's pushed commit %q, want the Spec's merge there", apiPushed)
	}
	reportRunIn(t, h, "api", 7, id, apiPushed, domain.CICompleted, domain.CISuccess)
	reportRun(t, h, 1, id, pushedIn(tk, h.Project), domain.CICompleted, domain.CISuccess)
	r.Poll()

	tk = h.WantState(id, domain.Done)
	if landedIn(tk, "api") != apiPushed {
		t.Fatalf("Changed Repositories %+v, want api landed at its pushed merge that passed, %s", tk.Repositories, apiPushed)
	}
}

func TestUnderPullRequestATwoRepositorySpecBranchIsPushedInEachAndLandsAsEachMerges(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	api, id := specInTwo(h)
	pullRequestOn(h, "origin")
	branch := "spec/" + h.SpecID()
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	r.WantOutput(h.SpecID() + ": pushed " + branch + " to origin in Repository api")
	apiTip, homeTip := wantPushedIn(t, h, "api", branch), wantPushedIn(t, h, h.Project, branch)
	if got := api.Remote("origin").Git("rev-parse", branch); got != apiTip || apiTip != h.GitIn(api.Path, "rev-parse", branch) {
		t.Errorf("api's remote %s is at %q, want its Spec Branch %s", branch, got, apiTip)
	}
	if got := home.Git("rev-parse", branch); got != homeTip {
		t.Errorf("the Home Repository's remote %s is at %q, want %s", branch, got, homeTip)
	}
	if got := api.Remote("origin").Git("branch", "--list", id); got != "" {
		t.Errorf("api's remote has the Ticket's own branch %q: only the Spec Branch is pushed", got)
	}

	// The human merges api's pull request first, on the host.
	h.GitIn(api.Path, "push", "-q", "origin", branch+":refs/heads/main")
	reportPullRequestIn(t, h, "api", branch, 4, apiTip)
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	if !tk.PartlyLanded || landedIn(tk, "api") != apiTip || landedIn(tk, h.Project) != "" {
		t.Fatalf("partly landed %v %+v, want it landed in api alone, at its merge", tk.PartlyLanded, tk.Repositories)
	}

	// Then the Home Repository's.
	h.Git("push", "-q", home.Path, branch+":refs/heads/main")
	reportPullRequestIn(t, h, "", branch, 7, homeTip)
	r.Poll()

	tk = h.WantState(id, domain.Done)
	if landedIn(tk, h.Project) != homeTip {
		t.Fatalf("Changed Repositories %+v, want the Home Repository landed at its merge %s", tk.Repositories, homeTip)
	}
}

func TestADivergedBaseInOneRepositoryHoldsBackASpecTouchingIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api, id := specInTwo(h)
	mergeAndPush(h, "origin")
	release := holdTicketsStage(t, h)
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	r.WantOutput(id + ": merged into spec/" + h.SpecID() + " at ")
	homeMain := h.Git("rev-parse", "main")
	// api's remote and its clone's main each gain a commit the other
	// lacks, and they conflict.
	api.CommitOn("main", "README.md", "ours\n")
	pushElsewhere(t, h, api, "README.md", "theirs\n")

	release()
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	if got := h.Git("rev-parse", "main"); got != homeMain || tk.PartlyLanded {
		t.Fatalf("the Home Repository's main is at %s, partly landed %v: a Spec touching a diverged base lands nowhere", got, tk.PartlyLanded)
	}
	if rows := h.NeedsMeItems(core.ReasonBaseDiverged); len(rows) != 1 || rows[0].BaseDiverged.Repository != "api" {
		t.Fatalf("Base diverged items %+v, want one naming api", rows)
	}
}

func TestUnderPullRequestASpecConflictInOneRepositoryPushesNoSpecBranch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	api, id := specInTwo(h)
	pullRequestOn(h, "origin")
	release := holdTicketsStage(t, h)
	fix := "acme-5"
	h.Script(fix, factorytest.Wait())
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	r.WantOutput(id + ": merged into spec/" + h.SpecID() + " at ")
	// api's base moves on, on its host, conflicting with its Spec Branch.
	pushElsewhere(t, h, api, "endpoint.go", "package other\n")

	release()
	r.Poll()

	branch := "spec/" + h.SpecID()
	if got := home.Git("branch", "--list", branch); got != "" {
		t.Errorf("the Home Repository's remote has %q: no Spec Branch is pushed while one conflicts", got)
	}
	if got := api.Remote("origin").Git("branch", "--list", branch); got != "" {
		t.Errorf("api's remote has %q", got)
	}
	if tk := h.Ticket(fix); !tk.SpecMergeTicket || !strings.Contains(tk.Description, "- api: merging "+branch) {
		t.Fatalf("%s %+v, want the Spec merge Ticket naming api's conflict", fix, tk)
	}
}
