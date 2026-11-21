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

// A Ticket's Changed Repositories (Spec code-central-848, ADR 0046): with
// several Repositories the Factory reports them live from the branches
// while it holds the Ticket In Progress, and checks every finish against
// the branches, after the finishing turn and before landing.

// changed is tk's Changed Repositories as "name@commit", in order.
func changed(tk api.TicketView) []string {
	var out []string
	for _, r := range tk.Repositories {
		out = append(out, r.Name+"@"+r.Commit)
	}
	return out
}

func TestTheFactoryReportsTheChangedRepositoriesLiveThenTheFinishSettlesThem(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	apiRepo := h.AddRepository("api")
	id := h.ReadyTicket("Add an endpoint")
	h.Script(id, append(factorytest.CommitIn("api", "endpoint.go", "package api\n"),
		factorytest.Wait(),
		factorytest.WriteFile("client.go", "package acme\n"), factorytest.Git("add", "client.go"), factorytest.Git("commit", "-q", "-m", "Call it"),
		factorytest.FinishIn("Done.", true, "api"), factorytest.Result("success"))...)
	r := h.Start()

	apiHead := apiRepo.Tip(id)
	r.PollUntil("api's work to be reported live", func() bool { return slices.Equal(changed(h.Ticket(id)), []string{"api@" + apiHead}) })
	tk := h.WantState(id, domain.InProgress)
	rows, err := h.Server.Client.FindTickets(context.Background(), h.Project, api.FindTicketsInput{State: []domain.State{domain.InProgress}})
	if err != nil || len(rows) != 1 || !slices.Equal(rows[0].Repositories, []string{"api"}) {
		t.Errorf("rows %+v (%v), want the card naming api", rows, err)
	}
	if tk.Branch != "" {
		t.Errorf("branch %q, want none recorded until the finish", tk.Branch)
	}

	h.Continue(id)
	r.Settle()
	tk = h.WantState(id, domain.InReview)
	homeHead := h.Tip(id)
	if got := changed(tk); !slices.Equal(got, []string{"acme@" + homeHead, "api@" + apiHead}) {
		t.Fatalf("settled Changed Repositories %v, want the Home Repository's and api's heads", got)
	}
}

func TestAFinishThatLeavesOutARepositoryWithWorkFailsWithAFailureReport(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	apiRepo := h.AddRepository("api")
	id := h.ReadyTicket("Add an endpoint")
	h.Script(id, append(factorytest.CommitIn("api", "endpoint.go", "package api\n"),
		factorytest.Finish("Done."), factorytest.Result("success"))...)
	r := h.Start()

	tk := h.WantState(id, domain.Failed)
	if len(tk.FailureReports) != 1 {
		t.Fatalf("Failure Reports %+v, want one", tk.FailureReports)
	}
	fr := tk.FailureReports[0]
	head := apiRepo.Tip(id)[:12]
	if fr.Claim != 1 || !strings.Contains(fr.WhatBroke, "api is left out") || !strings.Contains(fr.WhereWorkStands, "api: branch "+id+" at "+head) {
		t.Fatalf("Failure Report %+v, want it on Claim 1 naming api left out at %s", fr, head)
	}
	r.WantOutput(id + ": the finish check failed: api is left out")
}

func TestAFinishNamingACommitThatIsntItsBranchsHeadFails(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.AddRepository("api")
	id := h.ReadyTicket("Add an endpoint")
	// The session commits again in api after finishing.
	h.Script(id, append(append(factorytest.CommitIn("api", "endpoint.go", "package api\n"),
		factorytest.FinishIn("Done.", false, "api")),
		append(factorytest.CommitIn("api", "late.go", "package api\n"), factorytest.Result("success"))...)...)
	h.Start()

	tk := h.WantState(id, domain.Failed)
	if fr := tk.FailureReports[0]; !strings.Contains(fr.WhatBroke, "isn't its branch "+id+"'s head") {
		t.Fatalf("Failure Report %+v, want it to say api's commit isn't its branch's head", fr)
	}
}

func TestTheFinishCheckRunsAgainBeforeLanding(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	apiRepo := h.AddRepository("api")
	id := h.ReadyTicket("Add an endpoint")
	// Finished outside the Factory, so no turn of its checked it.
	home := h.CommitOn(id, "client.go", "package acme\n")
	h.Finished(id, id, home)
	r := h.Start()
	apiRepo.CommitOn(id, "endpoint.go", "package api\n")
	base := h.Tip("main")
	h.Accept(id)

	r.Poll()

	tk := h.WantState(id, domain.Failed)
	if !strings.Contains(tk.FailureReports[0].WhatBroke, "api is left out") {
		t.Fatalf("Failure Report %+v, want it to name api", tk.FailureReports[0])
	}
	if got := h.Tip("main"); got != base {
		t.Fatalf("main moved to %s: nothing lands before the finish check passes", got)
	}
}

func TestAnInReviewTicketMergedByHandIsCheckedBeforeItLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	apiRepo := h.AddRepository("api")
	id := h.ReadyTicket("Add an endpoint")
	home := h.CommitOn(id, "client.go", "package acme\n")
	h.Finished(id, id, home)
	r := h.Start()
	apiRepo.CommitOn(id, "endpoint.go", "package api\n")
	h.MergeByHand(id, "main")

	r.Poll()

	h.WantState(id, domain.Failed)
	if strings.Contains(r.Stdout(), "merged into main by hand") {
		t.Fatalf("stdout %q, want it failed, not landed", r.Stdout())
	}
}

func TestAFinishMergedByHandPassesTheCheckOnceItsBranchIsGone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	apiRepo := h.AddRepository("api")
	id := h.ReadyTicket("Add an endpoint")
	h.Script(id, append(factorytest.CommitIn("api", "endpoint.go", "package api\n"),
		factorytest.WriteFile("client.go", "package acme\n"), factorytest.Git("add", "client.go"), factorytest.Git("commit", "-q", "-m", "Call it"),
		factorytest.FinishIn("Done.", true, "api"), factorytest.Result("success"))...)
	r := h.Start()
	h.WantState(id, domain.InReview)
	// The human merges the work by hand, and deletes api's branch.
	h.MergeByHand(id, "main")
	apiRepo.MergeByHand(id, "main")
	apiRepo.DiscardWorktree(apiRepo.Worktree(id), id)

	r.Poll()

	h.WantState(id, domain.Done)
	r.WantOutput(id + ": merged into main by hand\n")
}

func TestWithTheHomeRepositoryAloneAFinishIsNotChecked(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Add a client")
	h.Script(id, factorytest.WriteFile("client.go", "package acme\n"), factorytest.Git("add", "client.go"), factorytest.Git("commit", "-q", "-m", "Call it"),
		factorytest.Finish("Done."), factorytest.Result("success"))
	h.Start()

	tk := h.WantState(id, domain.InReview)
	if len(tk.FailureReports) != 0 || tk.Repositories != nil {
		t.Fatalf("ticket %+v, want no Failure Report and no Repositories shown", tk)
	}
}
