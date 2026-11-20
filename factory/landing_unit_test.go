package factory

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/git"
)

// These tests drive a poll's landing step (the lander's poll) against a
// git.Fake repository and a real in-process Code Central, so each Landing Method's
// decisions run without git, a claude or the session part. What only
// real git shows (a merge-tree conflict, a push to a bare remote, a
// remote that is ahead or gone, a dirty base checkout) is tested through
// factorytest.

// landWorld is the landing part with a repository in a git.Fake, ready to land, and the
// Code Central it lands in. Its hooks are stubs, saying the Factory never
// stops and runs no turn, which a test may change.
type landWorld struct {
	t     *testing.T
	l     *lander
	repo  *testRepo
	fake  *git.Fake
	root  string // the folder holding every repository, and its worktrees
	top   string // the Home Repository's folder
	srv   *codecentraltest.Server
	human *client.Client
	out   *lineBuffer
	spec  string
}

// lineBuffer is the lander's output.
type lineBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *lineBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lineBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func newLandWorld(t *testing.T) *landWorld {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Client.CreateProject(context.Background(), api.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	human := client.New(srv.URL)
	human.Actor = api.Human
	out := &lineBuffer{}
	printf := func(format string, args ...any) { fmt.Fprintf(out, format+"\n", args...) }
	fake := git.NewFake(t)
	root := scratchDir(t)
	top := filepath.Join(root, "repo")
	repo := newTestRepo(t, fake, top)
	repos := newRepositories(printf)
	repos.setBase("main")
	repos.home = &repository{name: "acme", home: true, dir: top, git: repo, worktrees: top + ".worktrees"}
	repos.byName = map[string]*repository{"acme": repos.home}
	hooks := landerHooks{
		stopped:       func(context.Context) bool { return false },
		turnRunningOn: func(string) bool { return false },
	}
	l := newLander(repos, srv.Client, "acme", work{}, printf, hooks)
	return &landWorld{t: t, l: l, repo: repo, fake: fake, root: root, top: top, srv: srv, human: human, out: out}
}

// poll runs a poll's landing step.
func (w *landWorld) poll() { w.l.poll(context.Background()) }

// setLanding sets the Project's Landing Method as the human.
func (w *landWorld) setLanding(method domain.LandingMethod, push bool, remote string) {
	w.t.Helper()
	if _, err := w.human.SetLandingMethod(context.Background(), "acme", api.SetLandingMethodInput{Method: method, Push: push, Remote: remote}); err != nil {
		w.t.Fatal(err)
	}
}

// ready makes a Ready Ticket and returns its ID.
func (w *landWorld) ready(title string) string {
	w.t.Helper()
	ctx := context.Background()
	if w.spec == "" {
		s, err := w.srv.Client.CreateSpec(ctx, "acme", api.CreateSpecInput{Title: "Work"})
		if err != nil {
			w.t.Fatal(err)
		}
		w.spec = s.ID
		// Long-lived, so that its Tickets land one by one under the default Land per Spec.
		if _, err := w.srv.Client.AddLabels(ctx, "acme", api.LabelsInput{Target: api.Ref(s.ID), Labels: []string{domain.LongLivedLabel}}); err != nil {
			w.t.Fatal(err)
		}
	}
	tk, err := w.srv.Client.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: api.Ref(w.spec), Title: title, State: domain.Ready})
	if err != nil {
		w.t.Fatal(err)
	}
	return tk.ID
}

// finished claims the Ticket id and finishes it with branch and commit,
// so it is In Review.
func (w *landWorld) finished(id, branch, commit string) {
	w.t.Helper()
	ctx := context.Background()
	res, err := w.srv.Client.Claim(ctx, "acme", api.ClaimInput{Ticket: api.Ref(id)})
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.srv.Client.Finish(ctx, "acme", api.FinishInput{Ticket: api.Ref(id), ClaimID: res.ClaimID, Summary: "Done.", Branch: branch, Commit: commit}); err != nil {
		w.t.Fatal(err)
	}
}

// reviewed makes an In Review Ticket whose branch, named after it, has
// one commit, finished with that commit, and returns its ID and the
// commit.
func (w *landWorld) reviewed(title string) (id, commit string) {
	w.t.Helper()
	id = w.ready(title)
	commit = w.repo.commitOn(id, "Work on "+id)
	w.finished(id, id, commit)
	return id, commit
}

// accepted is reviewed, then accepted by the human.
func (w *landWorld) accepted(title string) (id, commit string) {
	w.t.Helper()
	id, commit = w.reviewed(title)
	w.accept(id)
	return id, commit
}

func (w *landWorld) accept(id string) {
	w.t.Helper()
	if _, err := w.human.Accept(context.Background(), "acme", api.TicketInput{Ticket: api.Ref(id)}); err != nil {
		w.t.Fatal(err)
	}
}

func (w *landWorld) ticket(id string) api.TicketView {
	w.t.Helper()
	tk, err := w.srv.Client.GetTicket(context.Background(), "acme", api.Ref(id))
	if err != nil {
		w.t.Fatal(err)
	}
	return tk
}

// lines counts the factory's stdout lines containing s.
func (w *landWorld) lines(s string) int { return strings.Count(w.out.String(), s) }

// commentsSaying counts tk's comments that contain every part.
func commentsSaying(tk api.TicketView, parts ...string) int {
	n := 0
	for _, c := range tk.Comments {
		all := true
		for _, p := range parts {
			all = all && strings.Contains(c.Body, p)
		}
		if all {
			n++
		}
	}
	return n
}

// lastMove is tk's latest move.
func lastMove(tk api.TicketView) api.MoveView { return tk.Moves[len(tk.Moves)-1] }

func movedTo(tk api.TicketView, from, to domain.State) bool {
	for _, m := range tk.Moves {
		if m.From == from && m.To == to {
			return true
		}
	}
	return false
}

const (
	cantLandYet = "The Factory can't land this yet"
	cantPushYet = "The Factory can't push this Ticket's branch yet"
)

// Under Merge.

func TestAnAcceptedTicketAlreadyInTheBaseIsSimplyLanded(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, commit := w.accepted("Merged already")
	base := w.repo.mergeByHand(id)

	w.poll()
	tk := w.ticket(id)
	if tk.State != domain.Done || lastMove(tk).Commit != commit {
		t.Errorf("%s is %s with move %+v; want it landed at the finish commit %s", id, tk.State, lastMove(tk), commit)
	}
	if w.repo.tip("refs/heads/main") != base {
		t.Error("main moved, but the work was in it already")
	}
}

func TestACleanMergeBlockedByUncommittedChangesStaysAcceptedAndIsRetriedEachPoll(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, commit := w.accepted("Add a feature")
	feature := w.repo.fileOf(id)
	w.repo.makeDirty(w.top, feature)
	base := w.repo.tip("refs/heads/main")

	w.poll()
	w.poll()
	w.poll()
	tk := w.ticket(id)
	if tk.State != domain.Accepted || w.repo.tip("refs/heads/main") != base {
		t.Fatalf("%s is %s and main at %s; want it left Accepted with main unmoved", id, tk.State, w.repo.tip("refs/heads/main"))
	}
	if n := commentsSaying(tk, cantLandYet, "uncommitted changes in "+w.top, feature, "It stays Accepted"); n != 1 || commentsSaying(tk, cantLandYet) != 1 {
		t.Fatalf("want one comment naming %s over three polls, got %+v", feature, tk.Comments)
	}

	w.repo.makeClean(w.top, feature)
	w.poll()
	tk = w.ticket(id)
	merge := w.repo.tip("refs/heads/main")
	if tk.State != domain.Done || lastMove(tk).Commit != merge {
		t.Fatalf("%s is %s with move %+v; want it landed at the merge %s", id, tk.State, lastMove(tk), merge)
	}
	if !w.repo.IsAncestor(commit, merge) || !w.repo.IsAncestor(base, merge) {
		t.Errorf("main's tip %s doesn't hold both the base %s and the work %s", merge, base, commit)
	}
}

func TestABaseThatIsNotCheckedOutIsMovedByRef(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.detach()
	id, commit := w.accepted("Add a feature")

	w.poll()
	merge := w.repo.tip("refs/heads/main")
	if tk := w.ticket(id); tk.State != domain.Done || lastMove(tk).Commit != merge {
		t.Fatalf("%s is %s; want it landed at main's new tip %s", id, tk.State, merge)
	}
	if !w.repo.IsAncestor(commit, merge) {
		t.Errorf("main's tip %s doesn't hold the work %s", merge, commit)
	}
}

func TestAnInReviewTicketMergedByHandIsLanded(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, commit := w.reviewed("Merged by hand")
	w.repo.mergeByHand(id)

	w.poll()
	tk := w.ticket(id)
	if m := lastMove(tk); tk.State != domain.Done || m.From != domain.InReview || m.Commit != commit {
		t.Errorf("%s is %s with move %+v; want it landed from In Review with the finish commit %s", id, tk.State, m, commit)
	}
	if movedTo(tk, domain.InReview, domain.Accepted) {
		t.Error("the Factory accepted the Ticket")
	}
}

func TestAnInReviewTicketNotMergedIsLeftForTheHuman(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, _ := w.reviewed("Waiting for review")
	base := w.repo.tip("refs/heads/main")

	w.poll()
	if s := w.ticket(id).State; s != domain.InReview {
		t.Errorf("%s is %s, want it left In Review", id, s)
	}
	if w.repo.tip("refs/heads/main") != base {
		t.Error("main moved for an In Review Ticket")
	}
}

func TestATicketMergedWhileInAnotherStateGetsOnlyAWarningLine(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, commit := w.reviewed("Rejected, then merged")
	if _, err := w.human.Reject(context.Background(), "acme", api.RejectInput{Ticket: api.Ref(id), Comment: "Not yet."}); err != nil {
		t.Fatal(err)
	}
	w.repo.mergeByHand(id)

	w.poll()
	w.poll()
	tk := w.ticket(id)
	if got := w.lines("warning: " + id + " is " + string(tk.State) + " but its finish commit " + short(commit) + " is already in main"); got != 1 {
		t.Errorf("want one warning line over two polls, got %d in %q", got, w.out)
	}
	if tk.State == domain.Done {
		t.Errorf("%s was landed from a State that can't land", id)
	}
}

// hookedToSession gives w's lander the hooks of a session part with no
// turn running, and returns it, for a test to pause or drain.
func (w *landWorld) hookedToSession() *factory {
	f := newFactory(Env{Stdout: io.Discard}, options{parallel: 1})
	w.l.hooks = f.hooks()
	return f
}

func TestAPausedFactoryDoesNotLand(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, _ := w.accepted("Add a feature")
	f := w.hookedToSession()
	f.pause = &pause{reason: "rate_limit"}

	w.poll()
	if s := w.ticket(id).State; s != domain.Accepted {
		t.Errorf("%s is %s; nothing lands during an outage", id, s)
	}
}

func TestADrainingFactoryDoesNotLand(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, _ := w.accepted("Add a feature")
	f := w.hookedToSession()
	close(f.drainCh)

	w.poll()
	if s := w.ticket(id).State; s != domain.Accepted {
		t.Errorf("%s is %s; nothing lands while draining", id, s)
	}
}

func TestAFactoryThatStopsMidPollLandsNothingMore(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	first, _ := w.accepted("Accepted first")
	w.srv.Clock.Advance(time.Minute)
	second, _ := w.accepted("Accepted second")
	// Halted, draining or paused once the first has landed.
	w.l.hooks.stopped = func(context.Context) bool { return w.ticket(first).State == domain.Done }

	w.poll()
	if s := w.ticket(first).State; s != domain.Done {
		t.Fatalf("%s is %s; want it landed before the stop", first, s)
	}
	if s := w.ticket(second).State; s != domain.Accepted {
		t.Errorf("%s is %s; nothing more lands once the Factory stops mid-poll", second, s)
	}
}

func TestAcceptedTicketsLandOldestAcceptFirst(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	second, _ := w.reviewed("Accepted second")
	first, _ := w.reviewed("Accepted first")
	w.accept(first)
	w.srv.Clock.Advance(time.Minute)
	w.accept(second)

	w.poll()
	out := w.out.String()
	if i, j := strings.Index(out, first+": landed"), strings.Index(out, second+": landed"); i < 0 || j < 0 || i > j {
		t.Errorf("want %s landed before %s, got %q", first, second, out)
	}
}

func TestUnderMergeAnAcceptedTicketsWorktreeIsKeptUntilItLands(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, _ := w.accepted("Add a feature")
	wt := w.l.repos.worktree(id)
	w.repo.addWorktree(wt, id)
	// Uncommitted changes in the base checkout hold the merge, so the
	// Ticket is still Accepted when the sweep runs.
	feature := w.repo.fileOf(id)
	w.repo.makeDirty(w.top, feature)

	w.poll()
	if s := w.ticket(id).State; s != domain.Accepted {
		t.Fatalf("%s is %s, want it held Accepted", id, s)
	}
	if !w.repo.hasWorktree(wt) {
		t.Error("the worktree of an Accepted Ticket was removed under Merge")
	}

	w.repo.makeClean(w.top, feature)
	w.poll()
	if s := w.ticket(id).State; s != domain.Done {
		t.Fatalf("%s is %s, want it landed", id, s)
	}
	if w.repo.hasWorktree(wt) {
		t.Error("the clean worktree of a landed Ticket is still there")
	}
}

// Under Branch.

func TestUnderBranchATicketFinishedWithoutACommitLandsAtItsBranchTip(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.setLanding(domain.LandingBranch, false, "")
	id := w.ready("No commit given")
	tip := w.repo.commitOn(id, "Work")
	w.finished(id, id, "")
	w.accept(id)
	w.repo.mergeByHand(id)

	w.poll()
	if tk := w.ticket(id); tk.State != domain.Done || lastMove(tk).Commit != tip {
		t.Errorf("%s is %s with move %+v; want it landed at the branch tip %s", id, tk.State, lastMove(tk), tip)
	}
}

func TestUnderBranchATicketWhoseWorkIsNotInTheRepositoryIsWarnedAboutOnce(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.setLanding(domain.LandingBranch, false, "")
	id := w.ready("Branch gone")
	w.finished(id, "gone", "")
	w.accept(id)

	w.poll()
	w.poll()
	if got := w.lines("warning: " + id + " is Accepted under the Branch Landing Method, but its branch gone isn't in this repository"); got != 1 {
		t.Errorf("want one warning over two polls, got %d in %q", got, w.out)
	}
	if s := w.ticket(id).State; s != domain.Accepted {
		t.Errorf("%s is %s, want it left Accepted", id, s)
	}
}

func TestUnderBranchAConflictingBranchIsNeverSentBack(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.setLanding(domain.LandingBranch, false, "")
	id := w.ready("Change the readme")
	w.finished(id, id, w.repo.commitFileOn(id, "Work on "+id, "README", "The branch's readme\n"))
	w.accept(id)
	// The base changes the same line.
	w.repo.commitFileOn("main", "Change the readme in the base", "README", "The base's readme\n")
	base := w.repo.tip("refs/heads/main")

	w.poll()
	tk := w.ticket(id)
	if tk.State != domain.Accepted || movedTo(tk, domain.Accepted, domain.Ready) {
		t.Errorf("%s is %s with moves %+v; want it left Accepted, never sent back", id, tk.State, tk.Moves)
	}
	if w.repo.tip("refs/heads/main") != base {
		t.Error("main moved under the Branch Landing Method")
	}
	if commentsSaying(tk, "Factory") != 0 {
		t.Errorf("want no comment from the Factory, got %+v", tk.Comments)
	}
}

func TestUnderBranchADirtyWorktreeOfAnAcceptedTicketIsWarnedAboutAndLeft(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.setLanding(domain.LandingBranch, false, "")
	id, _ := w.accepted("Add a feature")
	wt := w.l.repos.worktree(id)
	w.repo.addWorktree(wt, id)
	w.repo.makeDirty(wt, "notes.txt")

	w.poll()
	w.poll()
	if got := w.lines("warning: " + id + " is Accepted but its worktree " + wt + " has uncommitted changes: left in place"); got != 1 {
		t.Errorf("want one warning over two polls, got %d in %q", got, w.out)
	}
	if !w.repo.hasWorktree(wt) {
		t.Error("the dirty worktree was removed")
	}

	// Cleaned, it is removed, and warned of again should it get dirty.
	w.repo.makeClean(wt, "notes.txt")
	w.poll()
	if w.repo.hasWorktree(wt) {
		t.Error("the clean worktree of an Accepted Ticket is still there under Branch")
	}
}

func TestAChangeOfLandingMethodAppliesFromTheNextPoll(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.setLanding(domain.LandingBranch, false, "")
	first, _ := w.accepted("Left as a branch")
	base := w.repo.tip("refs/heads/main")

	w.poll()
	w.poll()
	if s := w.ticket(first).State; s != domain.Accepted || w.repo.tip("refs/heads/main") != base {
		t.Fatalf("%s is %s and main moved; want it left under Branch", first, s)
	}
	if got := w.lines(first + ": Accepted under the Branch Landing Method: merge " + first + " into main and the Factory lands it"); got != 1 {
		t.Errorf("want the Branch line once over two polls, got %q", w.out)
	}

	// Branch to Merge: the Accepted Ticket not merged by hand is merged.
	w.setLanding(domain.LandingMerge, false, "")
	w.poll()
	if s := w.ticket(first).State; s != domain.Done {
		t.Fatalf("%s is %s, want it merged and landed under Merge", first, s)
	}
	if got := w.repo.messageOf(w.repo.tip("refs/heads/main")); got != "Merge "+first+": Left as a branch" {
		t.Errorf("main's tip is %q, want the Factory's merge of %s", got, first)
	}

	// Merge to Branch: the next accept is left alone.
	w.setLanding(domain.LandingBranch, false, "")
	second, _ := w.accepted("Left as a branch too")
	base = w.repo.tip("refs/heads/main")
	w.poll()
	w.poll()
	if s := w.ticket(second).State; s != domain.Accepted || w.repo.tip("refs/heads/main") != base {
		t.Errorf("%s is %s and main moved; want it left under Branch", second, s)
	}
}

// With Push.

func TestUnderMergeAndPushAFailedPushIsCommentedOnceAndRetried(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.repo.setRefusing("origin", true)
	w.setLanding(domain.LandingMerge, true, "origin")
	id, _ := w.accepted("Add a feature")

	w.poll()
	w.poll()
	tk := w.ticket(id)
	if tk.State != domain.Accepted {
		t.Fatalf("%s is %s, want it held Accepted until the push succeeds", id, tk.State)
	}
	merge := w.repo.tip("refs/heads/main")
	if got := w.repo.messageOf(merge); got != "Merge "+id+": Add a feature" {
		t.Errorf("main's tip is %q, want the Factory's merge", got)
	}
	if n := commentsSaying(tk, cantLandYet); n != 1 || commentsSaying(tk, "couldn't push main to origin", "pre-receive hook declined", "It stays Accepted") != 1 {
		t.Errorf("want one comment quoting the failed push, got %+v", tk.Comments)
	}
	if got := w.lines(id + ": can't land yet: couldn't push main to origin"); got != 1 {
		t.Errorf("want one can't-land line over two polls, got %d in %q", got, w.out)
	}

	w.repo.setRefusing("origin", false)
	w.poll()
	tk = w.ticket(id)
	if tk.State != domain.Done {
		t.Fatalf("%s is %s, want it landed once the push succeeds", id, tk.State)
	}
	if got := w.repo.remoteTip("origin", "main"); got != merge {
		t.Errorf("the remote's main is at %q, want the merge %s", got, merge)
	}
	if n := commentsSaying(tk, cantLandYet); n != 1 {
		t.Errorf("want still one comment, got %d", n)
	}
}

func TestUnderMergeAndPushTheBaseIsNotPushedAgainOnceTheRemoteHasIt(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.setLanding(domain.LandingMerge, true, "origin")
	id, _ := w.reviewed("Merged and pushed by hand")
	w.repo.mergeByHand(id)
	if _, err := w.repo.Push(context.Background(), "origin", "main", ""); err != nil {
		t.Fatal(err)
	}

	w.poll()
	if s := w.ticket(id).State; s != domain.Done {
		t.Fatalf("%s is %s, want it landed", id, s)
	}
	if w.repo.pushes.Load() != 1 || w.lines("pushed main") != 0 {
		t.Errorf("pushed %d times (%q); want the base left as the remote has it", w.repo.pushes.Load(), w.out)
	}
}

func TestUnderMergeAndPushAnInReviewTicketMergedByHandLandsOnceTheBaseIsPushed(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.repo.setRefusing("origin", true)
	w.setLanding(domain.LandingMerge, true, "origin")
	id, commit := w.reviewed("Merged by hand")
	w.repo.mergeByHand(id)

	w.poll()
	tk := w.ticket(id)
	if tk.State != domain.InReview {
		t.Fatalf("%s is %s, want it held In Review until the base is pushed", id, tk.State)
	}
	if commentsSaying(tk, "couldn't push main to origin", "It stays In Review") != 1 {
		t.Errorf("want a comment on the failed push, got %+v", tk.Comments)
	}

	w.repo.setRefusing("origin", false)
	w.poll()
	tk = w.ticket(id)
	if m := lastMove(tk); tk.State != domain.Done || m.From != domain.InReview || m.Commit != commit {
		t.Errorf("%s is %s with move %+v; want it landed from In Review with the finish commit %s", id, tk.State, m, commit)
	}
	if got, want := w.repo.remoteTip("origin", "main"), w.repo.tip("refs/heads/main"); got != want {
		t.Errorf("the remote's main is at %q, want %s", got, want)
	}
	if got := w.lines(id + ": pushed main to origin"); got != 1 {
		t.Errorf("want one pushed line, got %q", w.out)
	}
}

// homeLocalOnly reports whether the Project lists the Home as local-only.
func (w *landWorld) homeLocalOnly() bool {
	w.t.Helper()
	p, err := w.human.GetProject(context.Background(), "acme")
	if err != nil {
		w.t.Fatal(err)
	}
	return p.Repositories[0].LocalOnly
}

func TestAMissingHomePushRemoteIsReportedAsLocalOnlyAndStopsNothing(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.setLanding(domain.LandingMerge, true, "nowhere")
	id, _ := w.accepted("Add a feature")
	reviewed, _ := w.reviewed("Merged by hand")
	w.repo.mergeByHand(reviewed)

	w.poll()

	if !w.homeLocalOnly() {
		t.Fatal("the Home isn't local-only, though its clone has no remote nowhere")
	}
	for _, tk := range []string{id, reviewed} {
		if s := w.ticket(tk).State; s != domain.Done {
			t.Errorf("%s is %s, want it landed on merge, with nothing pushed", tk, s)
		}
	}
	if got := w.repo.remoteTip("origin", "main"); got != "" || strings.Contains(w.out.String(), "nothing lands") {
		t.Errorf("origin's main is at %q and the lines say %q, want nothing pushed and nothing stopped", got, w.out)
	}

	// Renamed to one its clone has: reported, and pushed again.
	w.setLanding(domain.LandingMerge, true, "origin")
	again, _ := w.accepted("Add another feature")
	w.poll()
	if w.homeLocalOnly() {
		t.Fatal("the Home is local-only, though its clone has origin")
	}
	if s := w.ticket(again).State; s != domain.Done || w.repo.remoteTip("origin", "main") != w.repo.tip("refs/heads/main") {
		t.Errorf("%s is %s and origin's main at %q, want it landed with main pushed", again, s, w.repo.remoteTip("origin", "main"))
	}

	// Gone while the Home is pushed: reported once more.
	w.repo.removeRemote("origin")
	w.poll()
	if !w.homeLocalOnly() {
		t.Fatal("the Home isn't local-only, though origin has gone from its clone")
	}
}

func TestUnderBranchAndPushTheBranchReachesTheRemoteOnAcceptAndNotWhileInReview(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.setLanding(domain.LandingBranch, true, "origin")
	id, commit := w.reviewed("Add a feature")
	base := w.repo.tip("refs/heads/main")

	w.poll()
	if got := w.repo.remoteTip("origin", id); got != "" {
		t.Fatalf("the remote has %s at %s while it is In Review", id, got)
	}

	w.accept(id)
	w.poll()
	if got := w.repo.remoteTip("origin", id); got != commit {
		t.Errorf("the remote's %s is at %q, want the finish commit %s", id, got, commit)
	}
	if s := w.ticket(id).State; s != domain.Accepted {
		t.Errorf("%s is %s, want it left Accepted", id, s)
	}
	if w.repo.tip("refs/heads/main") != base {
		t.Error("main moved under the Branch Landing Method")
	}
	if got := w.repo.remoteTip("origin", "main"); got != "" {
		t.Errorf("the base was pushed under Branch: %s", got)
	}

	w.poll()
	if got := w.lines(id + ": pushed " + id + " to origin"); got != 1 || w.repo.pushes.Load() != 1 {
		t.Errorf("want the branch pushed once, got %d lines and %d pushes in %q", got, w.repo.pushes.Load(), w.out)
	}

	w.repo.mergeByHand(id)
	w.poll()
	if s := w.ticket(id).State; s != domain.Done {
		t.Errorf("%s is %s, want it landed once merged by hand", id, s)
	}
}

func TestUnderBranchAndPushAFailedPushIsCommentedOnceAndRetried(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.repo.setRefusing("origin", true)
	w.setLanding(domain.LandingBranch, true, "origin")
	id, commit := w.accepted("Add a feature")

	w.poll()
	w.poll()
	tk := w.ticket(id)
	if tk.State != domain.Accepted {
		t.Fatalf("%s is %s, want it left Accepted", id, tk.State)
	}
	if n := commentsSaying(tk, cantPushYet); n != 1 || commentsSaying(tk, "couldn't push "+id+" to origin", "It stays Accepted") != 1 {
		t.Errorf("want one comment quoting the failed push, got %+v", tk.Comments)
	}
	if got := w.lines(id + ": can't push yet: couldn't push " + id + " to origin"); got != 1 {
		t.Errorf("want one can't-push line over two polls, got %d in %q", got, w.out)
	}

	w.repo.setRefusing("origin", false)
	w.poll()
	if got := w.repo.remoteTip("origin", id); got != commit {
		t.Errorf("the remote's %s is at %q, want the finish commit %s", id, got, commit)
	}
	tk = w.ticket(id)
	if tk.State != domain.Accepted || commentsSaying(tk, cantPushYet) != 1 {
		t.Errorf("%s is %s with comments %+v; want it Accepted with still one comment", id, tk.State, tk.Comments)
	}
	if got := w.lines(id + ": pushed " + id + " to origin"); got != 1 {
		t.Errorf("want one pushed line, got %q", w.out)
	}
}

// The sweep, under any Landing Method.

func TestTheSweepRemovesOnlyCleanWorktreesOfFinishedTickets(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	landed, _ := w.accepted("Landed")
	review, _ := w.reviewed("Still in review")
	for _, id := range []string{landed, review} {
		w.repo.addWorktree(w.l.repos.worktree(id), id)
	}
	stray := filepath.Join(w.top+".worktrees", "acme-999")
	w.repo.addWorktree(stray, "acme-999")
	elsewhere := filepath.Join(w.root, "elsewhere")
	w.repo.addWorktree(elsewhere, "other")

	w.poll()
	if w.repo.hasWorktree(w.l.repos.worktree(landed)) {
		t.Errorf("the clean worktree of the landed %s is still there", landed)
	}
	for _, kept := range []string{w.l.repos.worktree(review), stray, elsewhere} {
		if !w.repo.hasWorktree(kept) {
			t.Errorf("the worktree %s was removed", kept)
		}
	}
	if got := w.lines(landed + ": Done: removed its worktree"); got != 1 {
		t.Errorf("want one removed line, got %q", w.out)
	}
}

func TestAPushedCommitWhoseReportFailedIsReportedAsItsOwnNotALaterTicketsPush(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.setLanding(domain.LandingMerge, true, "origin")
	if _, err := w.human.SetCITracking(context.Background(), "acme", api.SetCITrackingInput{Tracking: true, Repository: "acme/web"}); err != nil {
		t.Fatal(err)
	}
	first, _ := w.accepted("First")
	second, _ := w.accepted("Second")
	// Code Central refuses the first Ticket's report for one poll.
	var refusing atomic.Bool
	refusing.Store(true)
	target, err := url.Parse(w.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	proxy := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if refusing.Load() && strings.HasSuffix(r.URL.Path, "/"+first+"/report_pushed_commit") {
			http.Error(rw, "down", http.StatusServiceUnavailable)
			return
		}
		rp.ServeHTTP(rw, r)
	}))
	t.Cleanup(proxy.Close)
	w.l.client = client.New(proxy.URL)

	w.poll()
	parents, err := w.repo.ParentsOf(w.repo.tip("refs/heads/main"))
	if err != nil {
		t.Fatal(err)
	}
	firstMerge := parents[0]
	if tk := w.ticket(first); tk.State != domain.Accepted || tk.PushedCommit != nil {
		t.Fatalf("%s is %s with pushed commit %+v, want it Accepted with its report refused", first, tk.State, tk.PushedCommit)
	}
	if tk := w.ticket(second); tk.PushedCommit == nil || tk.PushedCommit.Commit != w.repo.remoteTip("origin", "main") {
		t.Fatalf("%s's pushed commit %+v, want the remote's main", second, tk.PushedCommit)
	}

	refusing.Store(false)
	w.poll()
	if tk := w.ticket(first); tk.State != domain.Accepted || tk.PushedCommit == nil || tk.PushedCommit.Commit != firstMerge {
		t.Fatalf("%s is %s with pushed commit %+v, want it Accepted with its own merge %s, not the later push", first, tk.State, tk.PushedCommit, firstMerge)
	}
}

// Under Merge with Land per Spec.

func TestASpecWhoseLastTicketIsAcceptedMidPollWaitsForItsMergeIntoTheSpecBranch(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	s, err := w.srv.Client.CreateSpec(context.Background(), "acme", api.CreateSpecInput{Title: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	w.spec = s.ID // not long-lived: its Tickets land together
	first, _ := w.accepted("First")
	second, commit := w.reviewed("Second")
	w.poll()
	if w.lines(first+": merged into spec/") != 1 {
		t.Fatalf("%s wasn't merged into the Spec Branch:\n%s", first, w.out)
	}
	base := w.repo.tip("refs/heads/main")

	// Accepted after the poll read its Accepted Tickets, as the human can
	// at any moment: the Spec is ready, but the Spec Branch lacks it.
	w.accept(second)
	p, ok := w.l.landing(context.Background())
	if !ok {
		t.Fatal("can't read the Project")
	}
	w.l.landSpecs(context.Background(), p)
	if got := w.repo.tip("refs/heads/main"); got != base {
		t.Fatalf("the Spec landed at %s without %s's work", got, second)
	}

	w.poll()
	merge := w.repo.tip("refs/heads/main")
	if !w.repo.IsAncestor(commit, merge) {
		t.Errorf("main's tip %s doesn't hold %s's work %s", merge, second, commit)
	}
	for _, id := range []string{first, second} {
		if tk := w.ticket(id); tk.State != domain.Done || lastMove(tk).Commit != merge {
			t.Errorf("%s is %s with move %+v; want it landed at the Spec's merge %s", id, tk.State, lastMove(tk), merge)
		}
	}
}
