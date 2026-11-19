// The CI Watcher across Repositories (Spec code-central-848): it watches
// every watched Repository through its own CI Provider, reports each
// one's runs naming the Repository, and when one can't be read, names it
// in its error with the fix while it carries on watching the others.

package ciwatcher_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/internal/fakegh"
)

// pushedInTwo makes acme-2 an Accepted Ticket that changed acme and api,
// with home pushed in the Home Repository and api pushed in api, under
// Merge + Push, as the Factory would report them. api is added on GitHub,
// so watched by GitHub Actions on acme/api.
func (w *watching) pushedInTwo(home, apiRepo string) string {
	w.t.Helper()
	ctx := context.Background()
	human := api.WithActor(ctx, api.Human)
	c := w.srv.Core
	must(w.t, func() error {
		_, err := c.AddRepository(human, "acme", api.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
		return err
	})
	must(w.t, func() error {
		_, err := c.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: "origin"})
		return err
	})
	must(w.t, func() error { _, err := c.CreateSpec(ctx, "acme", api.CreateSpecInput{Title: "Login"}); return err })
	must(w.t, func() error {
		_, err := c.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: "1", Title: "Login form", State: domain.Ready})
		return err
	})
	res, err := c.Claim(ctx, "acme", api.ClaimInput{Ticket: "2"})
	if err != nil {
		w.t.Fatal(err)
	}
	must(w.t, func() error {
		_, err := c.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: res.ClaimID, Summary: "Done.", Branch: "acme-2", Commit: home,
			Repositories: []api.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: apiRepo}}})
		return err
	})
	must(w.t, func() error { _, err := c.Accept(human, "acme", api.TicketInput{Ticket: "2"}); return err })
	for repo, commit := range map[string]string{"": home, "api": apiRepo} {
		must(w.t, func() error {
			_, err := w.srv.Client.ReportPushedCommit(ctx, "acme", api.ReportPushedCommitInput{Ticket: "acme-2", Commit: commit, Repository: repo})
			return err
		})
	}
	return "acme-2"
}

// runsAnswerIn scripts GitHub's answer to listing commit's runs in the
// GitHub repository repo: one run, id, of ci.yml, concluded conclude.
func (w *watching) runsAnswerIn(repo, commit string, id int64, conclude string) {
	w.t.Helper()
	w.h.Answer(fakegh.Answer{Args: []string{"api", "repos/" + repo + "/actions/runs?head_sha=" + commit + "&per_page=100"},
		Stdout: fmt.Sprintf(`{"total_count":1,"workflow_runs":[{"id":%d,"run_attempt":1,"name":"CI","path":".github/workflows/ci.yml","head_branch":"main","head_sha":%q,`+
			`"status":"completed","conclusion":%q,"run_started_at":"2026-01-05T09:01:00Z","updated_at":"2026-01-05T09:05:00Z","html_url":"https://github.com/%s/actions/runs/%d"}]}`,
			id, commit, conclude, repo, id)})
	w.h.Answer(fakegh.Answer{Args: []string{"api", fmt.Sprintf("repos/%s/actions/runs/%d/attempts/1/jobs?per_page=100", repo, id)},
		Stdout: `{"jobs":[{"name":"test","status":"completed","conclusion":"` + conclude + `","html_url":"https://github.com/` + repo + `/job/1"}]}`})
}

// repositoryCI waits for ticket's CI result in the Changed Repository
// name to be want, and returns the Ticket.
func (w *watching) repositoryCI(ticket, name string, want api.CIResult) api.TicketView {
	w.t.Helper()
	var tk api.TicketView
	w.waitFor(fmt.Sprintf("%s's CI result in %s %q", ticket, name, want), func() bool {
		var err error
		if tk, err = w.srv.Client.GetTicket(context.Background(), "acme", api.Ref(ticket)); err != nil {
			w.t.Fatal(err)
		}
		i := slices.IndexFunc(tk.Repositories, func(r api.ChangedRepositoryView) bool { return r.Name == name })
		return i >= 0 && tk.Repositories[i].CIResult == want
	})
	return tk
}

func TestTheCIWatcherReportsEachRepositorysRunsThroughItsOwnTarget(t *testing.T) {
	w := newWatching(t)
	id := w.pushedInTwo("abc123", "def456")
	w.runsAnswerIn("acme/web", "abc123", 11, "success")
	// The same run ID in another repository is another run.
	w.runsAnswerIn("acme/api", "def456", 11, "failure")
	w.start(w.h.Bin)

	w.repositoryCI(id, "acme", api.CIPassed)
	tk := w.repositoryCI(id, "api", api.CIFailed)
	if tk.CIResult != api.CIFailed || len(tk.CIRuns) != 2 {
		t.Fatalf("CI result %q with runs %+v, want failed with both Repositories' runs", tk.CIResult, tk.CIRuns)
	}
	if apiRepo := tk.CIRuns[1]; apiRepo.Repository != "api" || apiRepo.Commit != "def456" || apiRepo.URL != "https://github.com/acme/api/actions/runs/11" {
		t.Fatalf("api's run %+v, want it reported in api", apiRepo)
	}
	if got := w.status(api.WatcherWatching); len(got.Repositories) != 0 {
		t.Fatalf("status %+v, want Watching with no problems", got)
	}
}

func TestAnUnreadableRepositoryIsNamedInTheErrorWhileTheOthersAreStillWatched(t *testing.T) {
	w := newWatching(t)
	id := w.pushedInTwo("abc123", "def456")
	w.runsAnswerIn("acme/web", "abc123", 11, "success")
	w.h.Unreadable("acme/api")
	w.start(w.h.Bin)

	got := w.status(api.WatcherError)
	if len(got.Repositories) != 1 || got.Repositories[0].Repository != "api" || got.Repositories[0].Problem != api.ProblemRepositoryUnreadable ||
		!strings.Contains(got.Repositories[0].Message, "acme/api") || got.Repositories[0].Fix == "" {
		t.Fatalf("status %+v, want in error about api alone, with its fix", got)
	}
	// The Home Repository is still watched.
	w.repositoryCI(id, "acme", api.CIPassed)
	if n := w.listed("repos/acme/api/actions"); n != 0 {
		t.Fatalf("listed api's runs %d times, want none while it can't be read", n)
	}
}

func TestANotWatchedRepositoryIsNeverListed(t *testing.T) {
	w := newWatching(t)
	id := w.pushedInTwo("abc123", "def456")
	human := api.WithActor(context.Background(), api.Human)
	must(t, func() error {
		_, err := w.srv.Core.SetRepositoryCI(human, "acme", api.SetRepositoryCIInput{Name: "api", RepositoryCI: api.RepositoryCI{NotWatched: true}})
		return err
	})
	w.runsAnswerIn("acme/web", "abc123", 11, "success")
	w.start(w.h.Bin)
	tk := w.repositoryCI(id, "acme", api.CIPassed)
	if tk.CIResult != api.CIPassed {
		t.Fatalf("CI result %q, want passed: api is Not watched", tk.CIResult)
	}
	w.status(api.WatcherWatching)
	for _, c := range w.h.Calls() {
		if slices.ContainsFunc(c.Args, func(a string) bool { return strings.Contains(a, "acme/api") }) {
			t.Fatalf("gh called on api, which is Not watched: %v", c.Args)
		}
	}
}

// The Home Repository may be Not watched too (Spec code-central-890):
// the CI Watcher then never lists its runs, and still watches the others.
func TestANotWatchedHomeIsNeverListedWhileTheOthersAreWatched(t *testing.T) {
	w := newWatching(t)
	id := w.pushedInTwo("abc123", "def456")
	human := api.WithActor(context.Background(), api.Human)
	must(t, func() error {
		_, err := w.srv.Core.SetRepositoryCI(human, "acme", api.SetRepositoryCIInput{Name: "acme", RepositoryCI: api.RepositoryCI{NotWatched: true}})
		return err
	})
	w.runsAnswerIn("acme/api", "def456", 11, "success")
	w.start(w.h.Bin)
	tk := w.repositoryCI(id, "api", api.CIPassed)
	if tk.CIResult != api.CIPassed {
		t.Fatalf("CI result %q, want passed: the Home is Not watched", tk.CIResult)
	}
	w.status(api.WatcherWatching)
	for _, c := range w.h.Calls() {
		if slices.ContainsFunc(c.Args, func(a string) bool { return strings.Contains(a, "acme/web") }) {
			t.Fatalf("gh called on the Home's acme/web, which is Not watched: %v", c.Args)
		}
	}
}
