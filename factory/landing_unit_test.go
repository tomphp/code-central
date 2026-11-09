package factory

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// These tests drive a poll's landing step (landAndSweep) against a fake
// repo and a real in-process Code Central, so each Landing Method's
// decisions run without git or a claude. What only real git shows (a
// merge-tree conflict, a push to a bare remote, a remote that is ahead
// or gone, a dirty base checkout) is tested through factorytest.

// landWorld is a factory with a fake repo, ready to land, and the Code
// Central it lands in.
type landWorld struct {
	t     *testing.T
	f     *factory
	repo  *fakeRepo
	srv   *codecentraltest.Server
	human *client.Client
	out   *lineBuffer
	spec  string
}

// lineBuffer is the factory's stdout.
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

const fakeTop = "/work/repo"

func newLandWorld(t *testing.T) *landWorld {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	human := client.New(srv.URL)
	human.Actor = core.Human
	out := &lineBuffer{}
	repo := newFakeRepo(fakeTop)
	f := newFactory(Env{Stdout: out, Now: time.Now}, options{parallel: 1})
	f.repo, f.repository, f.project, f.base, f.client = fakeTop, repo, "acme", "main", srv.Client
	f.worktrees = fakeTop + ".worktrees"
	return &landWorld{t: t, f: f, repo: repo, srv: srv, human: human, out: out}
}

// poll runs a poll's landing step.
func (w *landWorld) poll() { w.f.landAndSweep(context.Background()) }

// setLanding sets the Project's Landing Method as the human.
func (w *landWorld) setLanding(method store.LandingMethod, push bool, remote string) {
	w.t.Helper()
	if _, err := w.human.SetLandingMethod(context.Background(), "acme", core.SetLandingMethodInput{Method: method, Push: push, Remote: remote}); err != nil {
		w.t.Fatal(err)
	}
}

// ready makes a Ready Ticket and returns its ID.
func (w *landWorld) ready(title string) string {
	w.t.Helper()
	ctx := context.Background()
	if w.spec == "" {
		s, err := w.srv.Client.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Work"})
		if err != nil {
			w.t.Fatal(err)
		}
		w.spec = s.ID
	}
	tk, err := w.srv.Client.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: core.Ref(w.spec), Title: title, State: store.Ready})
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
	res, err := w.srv.Client.Claim(ctx, "acme", core.ClaimInput{Ticket: core.Ref(id)})
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.srv.Client.Finish(ctx, "acme", core.FinishInput{Ticket: core.Ref(id), ClaimID: res.ClaimID, Summary: "Done.", Branch: branch, Commit: commit}); err != nil {
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
	if _, err := w.human.Accept(context.Background(), "acme", core.TicketInput{Ticket: core.Ref(id)}); err != nil {
		w.t.Fatal(err)
	}
}

func (w *landWorld) ticket(id string) core.TicketView {
	w.t.Helper()
	tk, err := w.srv.Client.GetTicket(context.Background(), "acme", core.Ref(id))
	if err != nil {
		w.t.Fatal(err)
	}
	return tk
}

// lines counts the factory's stdout lines containing s.
func (w *landWorld) lines(s string) int { return strings.Count(w.out.String(), s) }

// commentsSaying counts tk's comments that contain every part.
func commentsSaying(tk core.TicketView, parts ...string) int {
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
func lastMove(tk core.TicketView) core.MoveView { return tk.Moves[len(tk.Moves)-1] }

func movedTo(tk core.TicketView, from, to store.State) bool {
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
	if tk.State != store.Done || lastMove(tk).Commit != commit {
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
	w.repo.setDirty(fakeTop, " M feature.txt")
	base := w.repo.tip("refs/heads/main")

	w.poll()
	w.poll()
	w.poll()
	tk := w.ticket(id)
	if tk.State != store.Accepted || w.repo.tip("refs/heads/main") != base {
		t.Fatalf("%s is %s and main at %s; want it left Accepted with main unmoved", id, tk.State, w.repo.tip("refs/heads/main"))
	}
	if n := commentsSaying(tk, cantLandYet, "uncommitted changes in "+fakeTop, "feature.txt", "It stays Accepted"); n != 1 || commentsSaying(tk, cantLandYet) != 1 {
		t.Fatalf("want one comment naming feature.txt over three polls, got %+v", tk.Comments)
	}

	w.repo.setDirty(fakeTop, "")
	w.poll()
	tk = w.ticket(id)
	merge := w.repo.tip("refs/heads/main")
	if tk.State != store.Done || lastMove(tk).Commit != merge {
		t.Fatalf("%s is %s with move %+v; want it landed at the merge %s", id, tk.State, lastMove(tk), merge)
	}
	if !w.repo.isAncestor(commit, merge) || !w.repo.isAncestor(base, merge) {
		t.Errorf("main's tip %s doesn't hold both the base %s and the work %s", merge, base, commit)
	}
}

func TestABaseThatIsNotCheckedOutIsMovedByRef(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.list[0].branch = "elsewhere"
	id, commit := w.accepted("Add a feature")

	w.poll()
	merge := w.repo.tip("refs/heads/main")
	if tk := w.ticket(id); tk.State != store.Done || lastMove(tk).Commit != merge {
		t.Fatalf("%s is %s; want it landed at main's new tip %s", id, tk.State, merge)
	}
	if !w.repo.isAncestor(commit, merge) {
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
	if m := lastMove(tk); tk.State != store.Done || m.From != store.InReview || m.Commit != commit {
		t.Errorf("%s is %s with move %+v; want it landed from In Review with the finish commit %s", id, tk.State, m, commit)
	}
	if movedTo(tk, store.InReview, store.Accepted) {
		t.Error("the Factory accepted the Ticket")
	}
}

func TestAnInReviewTicketNotMergedIsLeftForTheHuman(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, _ := w.reviewed("Waiting for review")
	base := w.repo.tip("refs/heads/main")

	w.poll()
	if s := w.ticket(id).State; s != store.InReview {
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
	if _, err := w.human.Reject(context.Background(), "acme", core.RejectInput{Ticket: core.Ref(id), Comment: "Not yet."}); err != nil {
		t.Fatal(err)
	}
	w.repo.mergeByHand(id)

	w.poll()
	w.poll()
	tk := w.ticket(id)
	if got := w.lines("warning: " + id + " is " + string(tk.State) + " but its finish commit " + short(commit) + " is already in main"); got != 1 {
		t.Errorf("want one warning line over two polls, got %d in %q", got, w.out)
	}
	if tk.State == store.Done {
		t.Errorf("%s was landed from a State that can't land", id)
	}
}

func TestAPausedFactoryDoesNotLand(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, _ := w.accepted("Add a feature")
	w.f.pause = &pause{reason: "rate_limit"}

	w.poll()
	if s := w.ticket(id).State; s != store.Accepted {
		t.Errorf("%s is %s; nothing lands during an outage", id, s)
	}
}

func TestADrainingFactoryDoesNotLand(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	id, _ := w.accepted("Add a feature")
	close(w.f.drainCh)

	w.poll()
	if s := w.ticket(id).State; s != store.Accepted {
		t.Errorf("%s is %s; nothing lands while draining", id, s)
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
	wt := w.f.worktree(id)
	w.repo.addWorktree(wt, id)
	// Uncommitted changes in the base checkout hold the merge, so the
	// Ticket is still Accepted when the sweep runs.
	w.repo.setDirty(fakeTop, " M feature.txt")

	w.poll()
	if s := w.ticket(id).State; s != store.Accepted {
		t.Fatalf("%s is %s, want it held Accepted", id, s)
	}
	if !w.repo.hasWorktree(wt) {
		t.Error("the worktree of an Accepted Ticket was removed under Merge")
	}

	w.repo.setDirty(fakeTop, "")
	w.poll()
	if s := w.ticket(id).State; s != store.Done {
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
	w.setLanding(store.LandingBranch, false, "")
	id := w.ready("No commit given")
	tip := w.repo.commitOn(id, "Work")
	w.finished(id, id, "")
	w.accept(id)
	w.repo.mergeByHand(id)

	w.poll()
	if tk := w.ticket(id); tk.State != store.Done || lastMove(tk).Commit != tip {
		t.Errorf("%s is %s with move %+v; want it landed at the branch tip %s", id, tk.State, lastMove(tk), tip)
	}
}

func TestUnderBranchATicketWhoseWorkIsNotInTheRepositoryIsWarnedAboutOnce(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.setLanding(store.LandingBranch, false, "")
	id := w.ready("Branch gone")
	w.finished(id, "gone", "")
	w.accept(id)

	w.poll()
	w.poll()
	if got := w.lines("warning: " + id + " is Accepted under the Branch Landing Method, but its branch gone isn't in this repository"); got != 1 {
		t.Errorf("want one warning over two polls, got %d in %q", got, w.out)
	}
	if s := w.ticket(id).State; s != store.Accepted {
		t.Errorf("%s is %s, want it left Accepted", id, s)
	}
}

func TestUnderBranchAConflictingBranchIsNeverSentBack(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.setLanding(store.LandingBranch, false, "")
	id, commit := w.accepted("Change the readme")
	w.repo.conflicts[commit] = []string{"README"}
	base := w.repo.tip("refs/heads/main")

	w.poll()
	tk := w.ticket(id)
	if tk.State != store.Accepted || movedTo(tk, store.Accepted, store.Ready) {
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
	w.setLanding(store.LandingBranch, false, "")
	id, _ := w.accepted("Add a feature")
	wt := w.f.worktree(id)
	w.repo.addWorktree(wt, id)
	w.repo.setDirty(wt, "?? notes.txt")

	w.poll()
	w.poll()
	if got := w.lines("warning: " + id + " is Accepted but its worktree " + wt + " has uncommitted changes: left in place"); got != 1 {
		t.Errorf("want one warning over two polls, got %d in %q", got, w.out)
	}
	if !w.repo.hasWorktree(wt) {
		t.Error("the dirty worktree was removed")
	}

	// Cleaned, it is removed, and warned of again should it get dirty.
	w.repo.setDirty(wt, "")
	w.poll()
	if w.repo.hasWorktree(wt) {
		t.Error("the clean worktree of an Accepted Ticket is still there under Branch")
	}
}

func TestAChangeOfLandingMethodAppliesFromTheNextPoll(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.setLanding(store.LandingBranch, false, "")
	first, _ := w.accepted("Left as a branch")
	base := w.repo.tip("refs/heads/main")

	w.poll()
	if s := w.ticket(first).State; s != store.Accepted || w.repo.tip("refs/heads/main") != base {
		t.Fatalf("%s is %s and main moved; want it left under Branch", first, s)
	}
	if got := w.lines(first + ": Accepted under the Branch Landing Method: merge " + first + " into main and the Factory lands it"); got != 1 {
		t.Errorf("want the Branch line once, got %q", w.out)
	}

	// Branch to Merge: the Accepted Ticket not merged by hand is merged.
	w.setLanding(store.LandingMerge, false, "")
	w.poll()
	if s := w.ticket(first).State; s != store.Done {
		t.Fatalf("%s is %s, want it merged and landed under Merge", first, s)
	}
	if got := w.repo.messageOf(w.repo.tip("refs/heads/main")); got != "Merge "+first+": Left as a branch" {
		t.Errorf("main's tip is %q, want the Factory's merge of %s", got, first)
	}

	// Merge to Branch: the next accept is left alone.
	w.setLanding(store.LandingBranch, false, "")
	second, _ := w.accepted("Left as a branch too")
	base = w.repo.tip("refs/heads/main")
	w.poll()
	w.poll()
	if s := w.ticket(second).State; s != store.Accepted || w.repo.tip("refs/heads/main") != base {
		t.Errorf("%s is %s and main moved; want it left under Branch", second, s)
	}
}

// With Push.

func TestUnderMergeAndPushAFailedPushIsCommentedOnceAndRetried(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.repo.setBroken("origin", true)
	w.setLanding(store.LandingMerge, true, "origin")
	id, _ := w.accepted("Add a feature")

	w.poll()
	w.poll()
	tk := w.ticket(id)
	if tk.State != store.Accepted {
		t.Fatalf("%s is %s, want it held Accepted until the push succeeds", id, tk.State)
	}
	merge := w.repo.tip("refs/heads/main")
	if got := w.repo.messageOf(merge); got != "Merge "+id+": Add a feature" {
		t.Errorf("main's tip is %q, want the Factory's merge", got)
	}
	if n := commentsSaying(tk, cantLandYet); n != 1 || commentsSaying(tk, "couldn't push main to origin", "does not appear to be a git repository", "It stays Accepted") != 1 {
		t.Errorf("want one comment quoting the failed push, got %+v", tk.Comments)
	}
	if got := w.lines(id + ": can't land yet: couldn't push main to origin"); got != 1 {
		t.Errorf("want one can't-land line over two polls, got %d in %q", got, w.out)
	}

	w.repo.setBroken("origin", false)
	w.poll()
	tk = w.ticket(id)
	if tk.State != store.Done {
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
	w.setLanding(store.LandingMerge, true, "origin")
	id, _ := w.reviewed("Merged and pushed by hand")
	w.repo.mergeByHand(id)
	if _, err := w.repo.push(context.Background(), "origin", "main"); err != nil {
		t.Fatal(err)
	}

	w.poll()
	if s := w.ticket(id).State; s != store.Done {
		t.Fatalf("%s is %s, want it landed", id, s)
	}
	if w.repo.pushes != 1 || w.lines("pushed main") != 0 {
		t.Errorf("pushed %d times (%q); want the base left as the remote has it", w.repo.pushes, w.out)
	}
}

func TestUnderMergeAndPushAnInReviewTicketMergedByHandLandsOnceTheBaseIsPushed(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.repo.setBroken("origin", true)
	w.setLanding(store.LandingMerge, true, "origin")
	id, commit := w.reviewed("Merged by hand")
	w.repo.mergeByHand(id)

	w.poll()
	tk := w.ticket(id)
	if tk.State != store.InReview {
		t.Fatalf("%s is %s, want it held In Review until the base is pushed", id, tk.State)
	}
	if commentsSaying(tk, "couldn't push main to origin", "It stays In Review") != 1 {
		t.Errorf("want a comment on the failed push, got %+v", tk.Comments)
	}

	w.repo.setBroken("origin", false)
	w.poll()
	tk = w.ticket(id)
	if m := lastMove(tk); tk.State != store.Done || m.From != store.InReview || m.Commit != commit {
		t.Errorf("%s is %s with move %+v; want it landed from In Review with the finish commit %s", id, tk.State, m, commit)
	}
	if got, want := w.repo.remoteTip("origin", "main"), w.repo.tip("refs/heads/main"); got != want {
		t.Errorf("the remote's main is at %q, want %s", got, want)
	}
	if got := w.lines(id + ": pushed main to origin"); got != 1 {
		t.Errorf("want one pushed line, got %q", w.out)
	}
}

func TestAPushRemoteThatGoesMissingHoldsLandingWithAWarningUntilFixed(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.setLanding(store.LandingMerge, true, "nowhere")
	id, _ := w.accepted("Add a feature")
	reviewed, _ := w.reviewed("Merged by hand")
	base := w.repo.mergeByHand(reviewed)

	w.poll()
	w.poll()
	if got := w.lines(`warning: nothing lands until this is fixed: Push names the remote "nowhere", which this repository doesn't have`); got != 1 {
		t.Errorf("want one warning over two polls, got %d in %q", got, w.out)
	}
	if s := w.ticket(id).State; s != store.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, s)
	}
	if s := w.ticket(reviewed).State; s != store.InReview {
		t.Errorf("%s is %s, want the hand merge held In Review", reviewed, s)
	}
	if w.repo.tip("refs/heads/main") != base {
		t.Error("main moved while the Push remote was missing")
	}

	w.setLanding(store.LandingMerge, true, "origin")
	w.poll()
	for _, tk := range []string{id, reviewed} {
		if s := w.ticket(tk).State; s != store.Done {
			t.Errorf("%s is %s, want it landed once the remote is back", tk, s)
		}
	}

	// Missing again after being back: warned of again.
	w.repo.removeRemote("origin")
	w.poll()
	if got := w.lines(`Push names the remote "origin"`); got != 1 {
		t.Errorf("want a warning once origin goes missing after being back, got %d in %q", got, w.out)
	}
}

func TestUnderBranchAndPushTheBranchReachesTheRemoteOnAcceptAndNotWhileInReview(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.setLanding(store.LandingBranch, true, "origin")
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
	if s := w.ticket(id).State; s != store.Accepted {
		t.Errorf("%s is %s, want it left Accepted", id, s)
	}
	if w.repo.tip("refs/heads/main") != base {
		t.Error("main moved under the Branch Landing Method")
	}
	if got := w.repo.remoteTip("origin", "main"); got != "" {
		t.Errorf("the base was pushed under Branch: %s", got)
	}

	w.poll()
	if got := w.lines(id + ": pushed " + id + " to origin"); got != 1 || w.repo.pushes != 1 {
		t.Errorf("want the branch pushed once, got %d lines and %d pushes in %q", got, w.repo.pushes, w.out)
	}

	w.repo.mergeByHand(id)
	w.poll()
	if s := w.ticket(id).State; s != store.Done {
		t.Errorf("%s is %s, want it landed once merged by hand", id, s)
	}
}

func TestUnderBranchAndPushAFailedPushIsCommentedOnceAndRetried(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.repo.setBroken("origin", true)
	w.setLanding(store.LandingBranch, true, "origin")
	id, commit := w.accepted("Add a feature")

	w.poll()
	w.poll()
	tk := w.ticket(id)
	if tk.State != store.Accepted {
		t.Fatalf("%s is %s, want it left Accepted", id, tk.State)
	}
	if n := commentsSaying(tk, cantPushYet); n != 1 || commentsSaying(tk, "couldn't push "+id+" to origin", "It stays Accepted") != 1 {
		t.Errorf("want one comment quoting the failed push, got %+v", tk.Comments)
	}
	if got := w.lines(id + ": can't push yet: couldn't push " + id + " to origin"); got != 1 {
		t.Errorf("want one can't-push line over two polls, got %d in %q", got, w.out)
	}

	w.repo.setBroken("origin", false)
	w.poll()
	if got := w.repo.remoteTip("origin", id); got != commit {
		t.Errorf("the remote's %s is at %q, want the finish commit %s", id, got, commit)
	}
	tk = w.ticket(id)
	if tk.State != store.Accepted || commentsSaying(tk, cantPushYet) != 1 {
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
		w.repo.addWorktree(w.f.worktree(id), id)
	}
	stray := filepath.Join(fakeTop+".worktrees", "acme-999")
	w.repo.addWorktree(stray, "acme-999")
	elsewhere := "/somewhere/else"
	w.repo.addWorktree(elsewhere, "other")

	w.poll()
	if w.repo.hasWorktree(w.f.worktree(landed)) {
		t.Errorf("the clean worktree of the landed %s is still there", landed)
	}
	for _, kept := range []string{w.f.worktree(review), stray, elsewhere} {
		if !w.repo.hasWorktree(kept) {
			t.Errorf("the worktree %s was removed", kept)
		}
	}
	if got := w.lines(landed + ": Done: removed its worktree"); got != 1 {
		t.Errorf("want one removed line, got %q", w.out)
	}
}
