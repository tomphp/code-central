package factory_test

import (
	"context"
	"github.com/tomoram/code-central/domain"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Pull requests across Repositories (Spec code-central-848, ADR 0050).

// acceptedIn is a Ticket of h that changed both the Home Repository and
// api (inTwo), In Review then Accepted and polled once, with the Factory
// running.
func acceptedIn(t *testing.T, h *factorytest.Harness) (id string, r *factorytest.Running) {
	t.Helper()
	id = inTwo(h, "Add an endpoint")
	r = h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	return id, r
}

// pullRequestWatch is what the Pull Request Watcher reads for h's Project.
func pullRequestWatch(t *testing.T, h *factorytest.Harness) core.PullRequestWatch {
	t.Helper()
	w, err := h.Server.Client.GetPullRequestWatch(context.Background(), h.Project)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// pushedWorkIn is the work the Factory has reported pushed in repository,
// for the Pull Request Watcher to open pull requests on, by branch.
func pushedWorkIn(t *testing.T, h *factorytest.Harness, repository string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, work := range pullRequestWatch(t, h).Work {
		if work.Repository == repository {
			out[work.Branch] = work.Commit
		}
	}
	return out
}

// wantPushedIn checks the Factory has reported ticket's branch in
// repository as pushed: the Pull Request Watcher would open a pull request
// there. It returns the pushed commit.
func wantPushedIn(t *testing.T, h *factorytest.Harness, repository, ticket string) string {
	t.Helper()
	work := pushedWorkIn(t, h, repository)
	commit, ok := work[ticket]
	if !ok {
		t.Fatalf("the pushed work in %s isn't reported: the Watcher's list there is %v", repository, work)
	}
	return commit
}

// reportPullRequestIn is the Pull Request Watcher reporting ticket's pull
// request in repository, as merged at commit.
func reportPullRequestIn(t *testing.T, h *factorytest.Harness, repository, branch string, number int, commit string) {
	t.Helper()
	in := core.ReportPullRequestInput{Repository: repository, Branch: branch, Number: number, Link: "https://example.test/pull/1", State: domain.PRMerged, MergeCommit: commit}
	if _, err := h.Server.Client.ReportPullRequest(context.Background(), h.Project, in); err != nil {
		t.Fatal(err)
	}
}

// landedIn is tk's landed commit in repository, "" when it hasn't landed
// there.
func landedIn(tk core.TicketView, repository string) string {
	for _, r := range tk.Repositories {
		if r.Name == repository && r.Landed != nil {
			return r.Landed.Commit
		}
	}
	return ""
}

func TestUnderPullRequestEachChangedRepositoryIsPushedAndLandsWhenItsPullRequestMergesInAnyOrder(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	api := h.AddRepository("api")
	pullRequestOn(h, "origin")
	id, r := acceptedIn(t, h)

	r.WantOutput(id + ": pushed " + id + " to origin in Repository api")
	apiHead := wantPushedIn(t, h, "api", id)
	homeHead := wantPushedIn(t, h, h.Project, id)
	if got := h.GitIn(api.Path, "rev-parse", "origin/"+id); got != apiHead {
		t.Errorf("api's remote %s is at %q, want %s", id, got, apiHead)
	}

	// The human merges api's pull request first, on the host.
	h.GitIn(api.Path, "push", "-q", "origin", id+":refs/heads/main")
	reportPullRequestIn(t, h, "api", id, 4, apiHead)
	r.Poll()
	tk := h.WantState(id, domain.Accepted)
	if !tk.PartlyLanded || landedIn(tk, "api") != apiHead || landedIn(tk, h.Project) != "" {
		t.Fatalf("after api merged: partly landed %v, api at %q, home at %q", tk.PartlyLanded, landedIn(tk, "api"), landedIn(tk, h.Project))
	}

	// Then the Home Repository's.
	h.Git("push", "-q", home.Path, id+":refs/heads/main")
	reportPullRequestIn(t, h, "", id, 7, homeHead)
	r.Poll()
	tk = h.WantState(id, domain.Done)
	if landedIn(tk, h.Project) != homeHead || landedIn(tk, "api") != apiHead {
		t.Errorf("landed home at %q and api at %q, want each at its merge commit", landedIn(tk, h.Project), landedIn(tk, "api"))
	}
}

func TestSentBackAfterASiblingMergedTheNextClaimStartsThatRepositorysBranchAtItsBaseTip(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api := h.AddRepository("api")
	pullRequestOn(h, "origin")
	id, r := acceptedIn(t, h)
	apiHead := wantPushedIn(t, h, "api", id)
	// api's pull request is squash-merged: the host's commit has the
	// work but not its history.
	squash := api.CommitOn("squash", "endpoint.go", "package api // squashed\n")
	h.GitIn(api.Path, "push", "-q", "origin", "squash:refs/heads/main")
	reportPullRequestIn(t, h, "api", id, 4, squash)
	r.Poll()
	if tk := h.WantState(id, domain.Accepted); landedIn(tk, "api") != squash {
		t.Fatalf("api landed at %q, want %s", landedIn(tk, "api"), squash)
	}

	if _, err := h.Human().SendBack(context.Background(), h.Project, core.SendBackInput{Ticket: core.Ref(id), Note: "Redo the client."}); err != nil {
		t.Fatal(err)
	}
	h.Script(id, factorytest.WriteFile("client.go", "package acme // v2\n"), factorytest.Git("add", "client.go"), factorytest.Git("commit", "-q", "-m", "Redo it"),
		factorytest.FinishIn("Redone.", true), factorytest.Result("success"))
	r.Poll()
	tk := h.WantState(id, domain.InReview)

	if got := h.GitIn(api.Path, "rev-parse", id); got == apiHead || got != squash {
		t.Errorf("api's %s is at %q, want moved from %s to the Base tip %s", id, got, apiHead, squash)
	}
	r.WantOutput(id + ": its work in Repository api landed, so " + id + " moved to main's tip")
	if landedIn(tk, "api") != squash {
		t.Errorf("api's landed commit %q, want %s kept", landedIn(tk, "api"), squash)
	}
	if len(tk.FailureReports) != 0 {
		t.Errorf("failure reports %+v, want the finish check passed", tk.FailureReports)
	}
}

// A broken base in one Repository holds back only the work touching it,
// under Pull Request as under Merge.
func TestUnderPullRequestATicketInAnotherRepositoryLandsWhileTheHomeRepositorysBaseIsMissing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api := h.AddRepository("api")
	pullRequestOn(h, "origin")
	id := inAPI(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	apiHead := wantPushedIn(t, h, "api", id)
	// The Home Repository's Base becomes a branch that exists nowhere. The
	// Factory has reported its remote's path, so the change needs the Code
	// Host's URL reported again first.
	pullRequestOn(h, "origin")
	release := "release"
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingPullRequest, Remote: "origin", Base: &release})
	h.GitIn(api.Path, "push", "-q", "origin", id+":refs/heads/main")
	reportPullRequestIn(t, h, "api", id, 4, apiHead)

	r.Poll()

	if tk := h.WantState(id, domain.Done); landedIn(tk, "api") != apiHead {
		t.Errorf("api landed at %q, want its merge commit %s", landedIn(tk, "api"), apiHead)
	}
}
