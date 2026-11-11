//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/internal/fakegh"
)

const (
	ghRepo   = "repos/acme/web"
	ghRemote = "git@github.com:acme/web.git"
)

// pullRequestWorld is a world under Pull Request on a fake GitHub: the
// remote origin is a bare repository the test merges into by hand, as the
// host would, and the fake gh answers for acme/web.
type pullRequestWorld struct {
	*world
	gh          string // the fake gh's folder
	landPerSpec bool
}

// startPullRequestWorld starts the world and switches its Project to Pull
// Request before any implementation Ticket exists. The Factory reports the
// bare repository's path as the Push remote URL, which names no Code Host,
// so the test reports the GitHub URL instead, as a real clone's would be.
func startPullRequestWorld(t *testing.T, requirement string, landPerSpec bool) *pullRequestWorld {
	t.Helper()
	ghData := t.TempDir()
	t.Setenv("FAKE_GH_DIR", ghData)
	if err := fakegh.Script(ghData, fakegh.Answer{Args: []string{"api", ghRepo},
		Stdout: `{"full_name":"acme/web","permissions":{"push":true,"pull":true}}`}); err != nil {
		t.Fatal(err)
	}
	w := &pullRequestWorld{world: startWorldWith(t, requirement, true), gh: ghData, landPerSpec: landPerSpec}
	gh, _ := json.Marshal(map[string]string{"gh_path": fakeGHBin})
	w.svc.human(t, "/api/set_options", string(gh))
	codecentraltest.Eventually(t, func() bool { return w.pushRemoteURL(t) != "" }, "the Factory didn't report the Push remote")
	w.svc.human(t, "/api/p/acme/report_push_remote", `{"url":"`+ghRemote+`","base":"main"}`)
	w.method(t, "pull_request")
	w.watcherState(t, "watching")
	return w
}

func (w *pullRequestWorld) pushRemoteURL(t *testing.T) string {
	t.Helper()
	resp, err := http.Get(w.svc.url + "/api/projects/acme")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var p struct {
		CI struct {
			URL string `json:"push_remote_url"`
		} `json:"ci"`
	}
	json.NewDecoder(resp.Body).Decode(&p)
	return p.CI.URL
}

func (w *pullRequestWorld) method(t *testing.T, m string) {
	t.Helper()
	w.svc.human(t, "/api/p/acme/set_landing_method",
		fmt.Sprintf(`{"method":%q,"push":false,"remote":"origin","land_per_spec":%t}`, m, w.landPerSpec))
}

func (w *pullRequestWorld) watcherState(t *testing.T, state string) {
	t.Helper()
	w.svc.watcherState(t, "landing", state)
}

// repoll makes the Pull Request Watcher poll now, instead of in up to two
// minutes: it is stopped and started again by switching to Branch, which
// leaves an Accepted Ticket waiting, and back. It returns once done holds,
// trying again as the Factory and the watcher race.
func (w *pullRequestWorld) repoll(t *testing.T, done func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("%s didn't happen", what)
		}
		w.method(t, "branch")
		w.method(t, "pull_request")
		time.Sleep(2 * time.Second)
	}
}

// pushedTickets is the Tickets of the pushed work Code Central has the
// Pull Request Watcher watch.
func (w *pullRequestWorld) pushedTickets(t *testing.T) []string {
	t.Helper()
	resp, err := http.Get(w.svc.url + "/api/p/acme/pull-request-watch")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var watch struct {
		Work []struct {
			Tickets []string `json:"tickets"`
		} `json:"work"`
	}
	json.NewDecoder(resp.Body).Decode(&watch)
	var out []string
	for _, w := range watch.Work {
		out = append(out, w.Tickets...)
	}
	return out
}

// answerList scripts what fake GitHub lists for pull requests from branch
// into main.
func (w *pullRequestWorld) answerList(t *testing.T, branch string, prs ...string) {
	t.Helper()
	path := ghRepo + "/pulls?state=all&sort=created&direction=desc&per_page=100&head=" +
		url.QueryEscape("acme:"+branch) + "&base=main"
	if err := fakegh.Script(w.gh, fakegh.Answer{Args: []string{"api", path}, Stdout: "[" + strings.Join(prs, ",") + "]"}); err != nil {
		t.Fatal(err)
	}
}

// answerOpen scripts what fake GitHub answers to opening a pull request.
func (w *pullRequestWorld) answerOpen(t *testing.T, number int) {
	t.Helper()
	if err := fakegh.Script(w.gh, fakegh.Answer{Args: []string{"api", ghRepo + "/pulls", "--method", "POST"},
		Stdout: githubPR(number, "open", "")}); err != nil {
		t.Fatal(err)
	}
}

// opened is the pull requests opened so far, each as the gh call's args.
func (w *pullRequestWorld) opened(t *testing.T) []string {
	t.Helper()
	calls, err := fakegh.Calls(w.gh)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range calls {
		a := strings.Join(c.Args, "\x00")
		if strings.HasPrefix(a, "api\x00"+ghRepo+"/pulls\x00--method\x00POST") {
			out = append(out, a)
		}
	}
	return out
}

// githubPR is a pull request as GitHub's REST API gives it.
func githubPR(n int, state, extra string) string {
	if extra != "" {
		extra = "," + extra
	}
	return fmt.Sprintf(`{"number":%d,"html_url":"https://github.com/acme/web/pull/%d","state":%q%s}`, n, n, state, extra)
}

// squashMerge merges branch into the remote's main as one commit, as the
// host's Squash and merge does, and returns that commit.
func (w *pullRequestWorld) squashMerge(t *testing.T, branch, message string) string {
	t.Helper()
	remote := w.inRepo("remote", "get-url", "origin")
	clone := filepath.Join(t.TempDir(), "host")
	w.inRepo("clone", "-q", remote, clone)
	git := func(args ...string) string {
		t.Helper()
		return runGit(t, clone, args...)
	}
	git("config", "user.name", "GitHub")
	git("config", "user.email", "noreply@github.com")
	git("merge", "--squash", "origin/"+branch)
	git("commit", "-q", "-m", message)
	git("push", "-q", "origin", "main")
	return git("rev-parse", "HEAD")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(gitBinary(t), args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// merged is a pull request GitHub reports merged at sha.
func merged(n int, sha string) string {
	return githubPR(n, "closed", `"closed_at":"2099-01-01T00:00:00Z","merged_at":"2099-01-01T00:00:00Z","merge_commit_sha":"`+sha+`"`)
}

// One Ticket under Pull Request, from accept to Done: accepting pushes its
// branch, the Pull Request Watcher opens a pull request into the base with
// the Ticket's summary, the host squash-merges it, and the Factory fetches
// the base and lands the Ticket at the host's merge commit.
func TestATicketGoesFromAcceptToAPullRequestToItsMergeToDone(t *testing.T) {
	w := startPullRequestWorld(t, "Greet the user\n\nSay hello on start.", false)
	for _, id := range []string{"acme-2", "acme-3", "acme-4"} {
		w.waitFor(id, "In Review")
		w.accept(id)
		w.waitFor(id, "Done")
	}
	w.waitFor("acme-5", "In Review")
	w.answerList(t, "acme-5")
	w.answerOpen(t, 7)

	// Accepting pushes the Ticket's branch to the remote, and the watcher
	// opens the pull request for it.
	w.accept("acme-5")
	codecentraltest.Eventually(t, func() bool { return w.inRepo("ls-remote", "origin", "refs/heads/acme-5") != "" },
		"the Factory didn't push acme-5")
	w.repoll(t, func() bool { return len(w.opened(t)) > 0 }, "opening a pull request")
	opened := w.opened(t)[0]
	for _, want := range []string{"title=Say hello", "head=acme-5", "base=main", "Wrote hello.txt.", "acme-5"} {
		if !strings.Contains(opened, want) {
			t.Errorf("the pull request was opened with %q, which lacks %q", opened, want)
		}
	}
	if got := w.ticket("acme-5").State; got != "Accepted" {
		t.Fatalf("acme-5 is %s with its pull request open, want Accepted", got)
	}

	// The host squash-merges it. The Ticket lands at that commit, though it
	// isn't the Ticket's own and the base never held the Ticket's commit.
	sha := w.squashMerge(t, "acme-5", "Say hello (#7)")
	w.answerList(t, "acme-5", merged(7, sha))
	w.repoll(t, func() bool { return w.ticket("acme-5").State == "Done" }, "landing acme-5")
	done := w.ticket("acme-5")
	if m := done.Moves[len(done.Moves)-1]; m.Commit != sha {
		t.Errorf("acme-5 landed at %q, want the host's merge commit %s", m.Commit, sha)
	}
	if got := w.inRepo("rev-parse", "main"); got != sha {
		t.Errorf("main is at %s, want the host's merge commit %s", got, sha)
	}
	if b, err := os.ReadFile(filepath.Join(w.repo, "hello.txt")); err != nil || string(b) != "hello\n" {
		t.Errorf("hello.txt on main: %q, %v", b, err)
	}
	if n := len(w.opened(t)); n != 1 {
		t.Errorf("opened %d pull requests, want one", n)
	}
	w.stop(t)
}

// A two-Ticket Spec under Land per Spec and Pull Request goes through one
// pull request: the Spec Branch is pushed once the Spec is ready, the
// watcher opens one pull request for it with a section per Ticket, and
// both Tickets land at its merge commit.
func TestATwoTicketSpecGoesThroughOnePullRequest(t *testing.T) {
	w := startPullRequestWorld(t, twoTicketSpec+"\n\nSay hello, then goodbye.", true)
	for _, id := range []string{"acme-2", "acme-3", "acme-4"} {
		w.waitFor(id, "In Review")
		w.accept(id)
		w.waitFor(id, "Done")
	}
	w.answerList(t, "spec/acme-1")
	w.answerOpen(t, 9)

	// Accepting the first Ticket merges it into the Spec Branch locally; no
	// pull request until the whole Spec is ready.
	w.waitFor("acme-5", "In Review")
	w.accept("acme-5")
	w.waitFor("acme-6", "In Review")
	if got := w.inRepo("ls-remote", "origin", "refs/heads/spec/acme-1"); got != "" {
		t.Fatalf("the Spec Branch was pushed before the Spec was ready: %s", got)
	}
	if n := len(w.opened(t)); n != 0 {
		t.Fatalf("opened %d pull requests before the Spec was ready", n)
	}
	w.accept("acme-6")
	codecentraltest.Eventually(t, func() bool { return w.inRepo("ls-remote", "origin", "refs/heads/spec/acme-1") != "" },
		"the Factory didn't push the Spec Branch")
	// Both Tickets have their push reported, or the pull request would open
	// without the second.
	codecentraltest.Eventually(t, func() bool { return len(w.pushedTickets(t)) == 2 }, "the Factory didn't report the push for both Tickets")
	w.repoll(t, func() bool { return len(w.opened(t)) > 0 }, "opening a pull request")
	opened := w.opened(t)
	if len(opened) != 1 {
		t.Fatalf("opened %d pull requests, want one: %q", len(opened), opened)
	}
	for _, want := range []string{"head=spec/acme-1", "base=main", "acme-1", "acme-5", "acme-6", "Say hello", "Say goodbye"} {
		if !strings.Contains(opened[0], want) {
			t.Errorf("the pull request was opened with %q, which lacks %q", strings.ReplaceAll(opened[0], "\x00", " "), want)
		}
	}
	for _, id := range []string{"acme-5", "acme-6"} {
		if got := w.ticket(id).State; got != "Accepted" {
			t.Errorf("%s is %s while the pull request is open, want Accepted", id, got)
		}
	}

	// Merged: both land at the host's one merge commit.
	sha := w.squashMerge(t, "spec/acme-1", "Greet and part (#9)")
	w.answerList(t, "spec/acme-1", merged(9, sha))
	w.repoll(t, func() bool { return w.ticket("acme-5").State == "Done" && w.ticket("acme-6").State == "Done" }, "landing the Spec")
	for _, id := range []string{"acme-5", "acme-6"} {
		d := w.ticket(id)
		if m := d.Moves[len(d.Moves)-1]; m.Commit != sha {
			t.Errorf("%s landed at %q, want the host's merge commit %s", id, m.Commit, sha)
		}
	}
	for _, f := range []string{"hello.txt", "goodbye.txt"} {
		if _, err := os.Stat(filepath.Join(w.repo, f)); err != nil {
			t.Errorf("%s isn't on main: %v", f, err)
		}
	}
	if n := len(w.opened(t)); n != 1 {
		t.Errorf("opened %d pull requests, want one", n)
	}
	w.stop(t)
}
