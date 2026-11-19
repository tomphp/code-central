package issuewatcher_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/issuewatcher"
	"github.com/tomoram/code-central/watchers/watchertest"
)

// GitHub Issues from several repositories (code-central-858): the one
// Issue Watcher polls every listed repository into the one Issues Spec,
// and a repository that fails shows its own error while the others keep
// being watched.

// repositoryOf scripts GitHub's answer to reading name, a repository gh
// can triage.
func repositoryOf(h *watchertest.Harness, name string) {
	h.Answer(fakegh.Answer{Args: []string{"api", "repos/" + name},
		Stdout: `{"full_name":"` + name + `","has_issues":true,"permissions":` + triage + `}`})
}

// openIssuesOf scripts GitHub's answer to listing the open issues of
// repository, its issue numbers, on the first page.
func (w *watching) openIssuesOf(repository string, ns ...int) {
	w.t.Helper()
	var issues []issue
	for _, n := range ns {
		body := fmt.Sprintf("Body %d", n)
		issues = append(issues, issue{Number: n, Title: fmt.Sprintf("Issue %d", n), Body: &body, HTMLURL: fmt.Sprintf("https://github.com/%s/issues/%d", repository, n), State: "open"})
	}
	b, err := json.Marshal(append([]issue{}, issues...))
	if err != nil {
		w.t.Fatal(err)
	}
	q := url.Values{"state": {"open"}, "sort": {"created"}, "direction": {"asc"}, "per_page": {"100"}, "page": {strconv.Itoa(1)}}
	w.h.Answer(fakegh.Answer{Args: []string{"api", "repos/" + repository + "/issues?" + q.Encode()}, Stdout: string(b)})
}

// listing lists acme/web and acme/api as acme's repositories, both of
// which gh can triage.
func (w *watching) listing() {
	w.t.Helper()
	repositoryOf(w.h, "acme/api")
	if _, err := w.srv.Core.SetGitHubIssues(human, "acme", api.SetGitHubIssuesInput{On: true, Repositories: []string{"acme/web", "acme/api"}}); err != nil {
		w.t.Fatal(err)
	}
}

func TestIssuesFromTwoRepositoriesReachTheOneIssuesSpec(t *testing.T) {
	w := newWatching(t)
	w.listing()
	w.openIssues("", 1, openIssue(1))
	w.openIssuesOf("acme/api", 5)
	w.start(w.h.Bin)
	w.status(api.WatcherWatching)

	got := w.incoming()
	want := []string{issueLine(1), "acme/api#5 https://github.com/acme/api/issues/5 Issue 5: Body 5"}
	if !slices.Equal(got, want) {
		t.Fatalf("Incoming Issues %q, want %q", got, want)
	}
	watch := w.watch()
	if !slices.Equal(watch.Repositories, []string{"acme/web", "acme/api"}) {
		t.Fatalf("get_issue_watch repositories = %v, want both", watch.Repositories)
	}
	spec, err := w.srv.Core.GetSpec(human, "acme", api.GetSpecInput{Spec: api.Ref(watch.Spec)})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Title != "GitHub Issues" {
		t.Fatalf("Issues Spec titled %q, want GitHub Issues", spec.Title)
	}

	// An issue opened later in either repository arrives on the next poll.
	w.openIssuesOf("acme/api", 5, 6)
	w.polled()
	if got := w.incoming(); len(got) != 3 || !strings.HasPrefix(got[2], "acme/api#6 ") {
		t.Fatalf("Incoming Issues %q, want acme/api#6 added", got)
	}
}

// One failing repository shows its own error and fix while the others keep
// arriving, and fixing it brings the Issue Watcher back to Watching.
func TestOneFailingRepositoryShowsItsOwnErrorWhileTheOthersKeepWatching(t *testing.T) {
	w := newWatching(t)
	w.listing()
	w.h.Unreadable("acme/api")
	w.openIssues("", 1, openIssue(1))
	w.start(w.h.Bin)

	got := w.status(api.WatcherError)
	if len(got.Repositories) != 1 || got.Repositories[0].Repository != "acme/api" || got.Repositories[0].Problem != api.ProblemRepositoryUnreadable ||
		!strings.Contains(got.Repositories[0].Message, "can't read the repository acme/api") || got.Repositories[0].Fix == "" {
		t.Fatalf("status %+v, want acme/api's own problem and fix", got)
	}
	if got.Problem != api.ProblemRepositoryUnreadable || !strings.Contains(got.Message, "acme/api") || strings.Contains(got.Message, "acme/web") {
		t.Fatalf("status %+v, want the summary to name acme/api alone", got)
	}
	codecentraltest.Eventually(t, func() bool { return len(w.incoming()) == 1 }, "acme/web's issue didn't arrive while acme/api failed")
	wantIncoming(t, w, 1)

	// Still in error, it is one Notification, and the other repository
	// keeps being watched.
	w.openIssues("", 1, openIssue(1), openIssue(2))
	w.polled()
	w.status(api.WatcherError)
	wantIncoming(t, w, 1, 2)
	if n := w.errorCount(); n != 1 {
		t.Fatalf("went into an error %d times, want once", n)
	}

	// A second repository failing while in error sends no second one.
	w.h.Unreadable("acme/web")
	w.polled()
	if got := w.status(api.WatcherError); len(got.Repositories) != 2 {
		t.Fatalf("status %+v, want both repositories named", got)
	}
	if n := w.errorCount(); n != 1 {
		t.Fatalf("went into an error %d times, want still once", n)
	}

	repositoryOf(w.h, "acme/web")
	repositoryOf(w.h, "acme/api")
	w.openIssuesOf("acme/api", 5)
	w.polled()
	w.status(api.WatcherWatching)
	if got := w.incoming(); len(got) != 3 {
		t.Fatalf("Incoming Issues %q, want acme/api's arrived once it was fixed", got)
	}
}

// Each repository is checked on its own by check mode, and each failing
// one is answered with its own problem.
func TestCheckModeAnswersPerRepository(t *testing.T) {
	h := watchertest.New(t)
	repository(h, true, triage)
	h.Unreadable("acme/api")
	repositoryOf(h, "acme/site")

	r := issuewatcher.Check(ctx, api.IssuesCheckInput{Repositories: []string{"acme/web", "acme/api", "acme/site"}}, gh.Env{Environ: h.Environ(h.Bin), InstallPlaces: []string{}})
	if len(r.Repositories) != 1 || r.Repositories[0].Repository != "acme/api" || r.Problem != api.ProblemRepositoryUnreadable {
		t.Fatalf("check = %+v, want acme/api's problem alone", r)
	}
	r = issuewatcher.Check(ctx, api.IssuesCheckInput{Repositories: []string{"acme/web", "acme/site"}}, gh.Env{Environ: h.Environ(h.Bin), InstallPlaces: []string{}})
	if r.Problem != "" {
		t.Fatalf("check = %+v, want it to pass", r)
	}
	// One repository is answered as it always was.
	r = issuewatcher.Check(ctx, api.IssuesCheckInput{Repositories: []string{"acme/api"}}, gh.Env{Environ: h.Environ(h.Bin), InstallPlaces: []string{}})
	if r.Problem != api.ProblemRepositoryUnreadable || len(r.Repositories) != 0 {
		t.Fatalf("check = %+v, want a plain problem with no repositories", r)
	}
	// gh being logged out answers for every repository at once.
	h.LoggedOut()
	r = issuewatcher.Check(ctx, api.IssuesCheckInput{Repositories: []string{"acme/web", "acme/api"}}, gh.Env{Environ: h.Environ(h.Bin), InstallPlaces: []string{}})
	if r.Problem != api.ProblemLoggedOut || len(r.Repositories) != 0 {
		t.Fatalf("check = %+v, want logged out for them all", r)
	}
}

// A comment is posted on the issue of the repository it is linked to.
func TestWritesGoToTheRepositoryOfTheirIssue(t *testing.T) {
	w := newWatching(t)
	w.listing()
	w.openIssues("", 1, openIssue(1))
	w.openIssuesOf("acme/api", 5)
	w.start(w.h.Bin)
	w.status(api.WatcherWatching)

	var apiRepo string
	for _, i := range w.watch().Issues {
		if i.Link.ID == "acme/api#5" {
			apiRepo = i.Ticket
		}
	}
	if apiRepo == "" {
		t.Fatalf("Incoming Issues %q, want acme/api#5", w.incoming())
	}
	if _, err := w.srv.Core.WontFix(human, "acme", api.WontFixInput{Ticket: api.Ref(apiRepo), Reason: "Out of scope"}); err != nil {
		t.Fatal(err)
	}
	w.polled()
	codecentraltest.Eventually(t, func() bool {
		for _, c := range w.h.Calls() {
			if slices.Contains(c.Args, "repos/acme/api/issues/5/comments") && slices.Contains(c.Args, "POST") {
				return true
			}
		}
		return false
	}, "the Won't fix comment wasn't posted on acme/api's issue 5")
	for _, c := range w.h.Calls() {
		if slices.ContainsFunc(c.Args, func(a string) bool {
			return strings.Contains(a, "repos/acme/web/issues/1/comments") && slices.Contains(c.Args, "POST")
		}) {
			t.Fatalf("a comment was posted on acme/web's issue 1: %q", c.Args)
		}
	}
}
