package factory

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// landingMethod reads the Project's Landing Method, which the human can
// change at any time, so it is read again on every poll. ok is false when
// it can't be read, and then nothing lands this poll.
func (f *factory) landingMethod(ctx context.Context) (method store.LandingMethod, ok bool) {
	p, err := f.client.GetProject(ctx, f.project)
	if err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't read the Project's Landing Method, so nothing lands this poll: %v", err)
		}
		return "", false
	}
	return p.Landing.Method, true
}

// land lands Accepted Tickets one at a time, oldest accept first (ADR
// 0006), by the Project's Landing Method, and detects merges made by
// hand. Under Merge the Factory merges each into the base; under Branch
// it leaves each for the human to merge, and lands it once merged. The
// Factory never accepts: only the human moves a Ticket to Accepted.
func (f *factory) land(ctx context.Context, method store.LandingMethod) {
	accepted := f.landable(ctx, store.Accepted)
	sort.SliceStable(accepted, func(i, j int) bool { return acceptedAt(accepted[i]).Before(acceptedAt(accepted[j])) })
	for _, tk := range accepted {
		if f.halted(ctx) || f.paused() {
			return
		}
		if method == store.LandingBranch {
			f.landMergedBranch(ctx, tk)
		} else {
			f.landAccepted(ctx, tk)
		}
	}
	f.detectHandMerges(ctx)
}

// landMergedBranch lands an Accepted Ticket under the Branch Landing
// Method once its work is in the base: the human merged it. Until then it
// is left as it is, with a line saying so once: never merged, never sent
// back.
func (f *factory) landMergedBranch(ctx context.Context, tk core.TicketView) {
	work, what := workOf(tk)
	head, err := f.gitOut(f.repo, "rev-parse", "--verify", "-q", work+"^{commit}")
	if err == nil && f.inBase(head) {
		f.printf("%s: merged into %s by hand", tk.ID, f.base)
		f.landAt(ctx, tk, head)
		return
	}
	key := tk.ID + "@" + head
	if f.leftOnBranch[key] {
		return
	}
	f.leftOnBranch[key] = true
	if err != nil {
		f.printf("warning: %s is Accepted under the Branch Landing Method, but %s isn't in this repository", tk.ID, what)
		return
	}
	merge := tk.Branch
	if merge == "" {
		merge = short(head)
	}
	f.printf("%s: Accepted under the Branch Landing Method: merge %s into %s and the Factory lands it", tk.ID, merge, f.base)
}

// workOf is the work the human reviewed: the finish commit, or with none,
// the branch's tip; and how to name it.
func workOf(tk core.TicketView) (ref, what string) {
	if tk.Commit != "" {
		return tk.Commit, "its commit " + tk.Commit
	}
	return "refs/heads/" + tk.Branch, "its branch " + tk.Branch
}

// detectHandMerges lands In Review Tickets whose finish commit is in the
// base: the human merged them. (Accepted ones are land's.) A Ticket merged while in any other State
// gets a warning line, once.
func (f *factory) detectHandMerges(ctx context.Context) {
	for _, tk := range f.landable(ctx, mergeableStates...) {
		if f.halted(ctx) || f.paused() {
			return
		}
		if tk.Commit == "" || !f.inBase(tk.Commit) {
			continue
		}
		if tk.State == store.InReview {
			f.printf("%s: merged into %s by hand", tk.ID, f.base)
			f.landAt(ctx, tk, tk.Commit)
			continue
		}
		key := tk.ID + "@" + tk.Commit
		if !f.warnedMerged[key] {
			f.warnedMerged[key] = true
			f.printf("warning: %s is %s but its finish commit %s is already in %s: only an In Review or Accepted Ticket lands", tk.ID, tk.State, short(tk.Commit), f.base)
		}
	}
}

// mergeableStates are the States a Ticket with a finish commit can be in
// short of Accepted, Done and Cancelled: every State but those and Draft,
// which comes before any work.
var mergeableStates = slices.DeleteFunc(slices.Clone(store.States), func(s store.State) bool {
	return s == store.Draft || s == store.Accepted || s == store.Done || s == store.Cancelled
})

// inBase reports whether commit is reachable from the base.
func (f *factory) inBase(commit string) bool {
	_, err := f.gitOut(f.repo, "merge-base", "--is-ancestor", commit, "refs/heads/"+f.base)
	return err == nil
}

// landable is every Ticket in these States, read in full.
func (f *factory) landable(ctx context.Context, states ...store.State) []core.TicketView {
	rows, err := f.client.FindTickets(ctx, f.project, core.FindTicketsInput{State: states})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't find %v Tickets: %v", states, err)
		}
		return nil
	}
	var out []core.TicketView
	for _, r := range rows {
		tk, err := f.client.GetTicket(ctx, f.project, core.Ref(r.ID))
		if err != nil {
			if ctx.Err() == nil {
				f.printf("%s: get_ticket: %v", r.ID, err)
			}
			continue
		}
		out = append(out, tk)
	}
	return out
}

// acceptedAt is when the Ticket last moved to Accepted.
func acceptedAt(tk core.TicketView) time.Time {
	for i := len(tk.Moves) - 1; i >= 0; i-- {
		if tk.Moves[i].To == store.Accepted {
			return tk.Moves[i].Time
		}
	}
	return tk.Updated
}

// landAccepted merges an Accepted Ticket's work into the base and lands
// it.
func (f *factory) landAccepted(ctx context.Context, tk core.TicketView) {
	baseTip, err := f.gitOut(f.repo, "rev-parse", "--verify", "-q", "refs/heads/"+f.base)
	if err != nil {
		f.cantLandYet(ctx, tk, fmt.Sprintf("the base, %s, isn't a branch, so the Factory can't move it.", f.base))
		return
	}
	work, what := workOf(tk)
	head, err := f.gitOut(f.repo, "rev-parse", "--verify", "-q", work+"^{commit}")
	if err != nil {
		f.cantLandYet(ctx, tk, what+" isn't in this repository.")
		return
	}
	if f.inBase(head) {
		f.landAt(ctx, tk, head)
		return
	}
	tree, conflicts, err := f.mergeTree(baseTip, head)
	if err != nil {
		f.printf("warning: %s: can't merge: %v", tk.ID, err)
		return
	}
	if len(conflicts) > 0 {
		f.sendBack(ctx, tk, head, baseTip, conflicts)
		return
	}
	msg := fmt.Sprintf("Merge %s: %s", tk.ID, tk.Title)
	merge, err := f.gitOut(f.repo, "commit-tree", tree, "-p", baseTip, "-p", head, "-m", msg)
	if err != nil {
		f.printf("warning: %s: can't make the merge commit: %v", tk.ID, err)
		return
	}
	if why := f.advanceBase(baseTip, merge); why != "" {
		f.cantLandYet(ctx, tk, why)
		return
	}
	f.landAt(ctx, tk, merge)
}

// cantLandYet leaves a Ticket whose clean merge can't land Accepted, to
// be retried next poll. Each distinct reason is commented once.
func (f *factory) cantLandYet(ctx context.Context, tk core.TicketView, why string) {
	body := fmt.Sprintf("The Factory can't land this yet: %s\n\nIt stays Accepted and the Factory tries again on every poll.", why)
	for _, c := range tk.Comments {
		if c.Body == body {
			return
		}
	}
	f.printf("%s: can't land yet: %s", tk.ID, firstLine(why))
	if _, err := f.client.Comment(ctx, f.project, core.CommentInput{Ticket: core.Ref(tk.ID), Body: body}); err != nil && ctx.Err() == nil {
		f.printf("%s: comment: %v", tk.ID, err)
	}
}

// firstLine is s up to its first newline.
func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// mergeTree merges head into base without touching any working tree: the
// merged tree, or the conflicting files.
func (f *factory) mergeTree(base, head string) (tree string, conflicts []string, err error) {
	cmd := exec.Command(f.git, "merge-tree", "--write-tree", "--name-only", "--no-messages", base, head)
	cmd.Dir = f.repo
	cmd.Env = f.e.Environ
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return "", nil, fmt.Errorf("git merge-tree: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	tree = lines[0]
	if err == nil {
		return tree, nil, nil
	}
	for _, l := range lines[1:] {
		if l == "" {
			break
		}
		conflicts = append(conflicts, l)
	}
	return tree, conflicts, nil
}

// sendBack sends a Ticket whose work conflicts with the base back to
// Ready, so a later Claim resolves the conflict and the human reviews it
// again.
func (f *factory) sendBack(ctx context.Context, tk core.TicketView, head, baseTip string, conflicts []string) {
	var b strings.Builder
	fmt.Fprintf(&b, "The Factory couldn't land this: merging %s into %s at %s conflicts in these files:\n\n", short(head), f.base, short(baseTip))
	for _, c := range conflicts {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	fmt.Fprintf(&b, "\nMerge %s into the branch, resolve the conflicts, and finish again.", f.base)
	if _, err := f.client.SendBack(ctx, f.project, core.SendBackInput{Ticket: core.Ref(tk.ID), Note: b.String()}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: send_back: %v", tk.ID, err)
		}
		return
	}
	f.printf("%s: conflicts with %s at %s in %s: sent back to Ready", tk.ID, f.base, short(baseTip), strings.Join(conflicts, ", "))
}

// advanceBase fast-forwards the base from old to merge: in the checkout
// where it is checked out, or by ref with old as a guard. It returns why
// it couldn't, or "".
func (f *factory) advanceBase(old, merge string) string {
	list, err := f.worktreeList()
	if err != nil {
		return err.Error()
	}
	for _, w := range list {
		if w.branch != f.base {
			continue
		}
		if moved := f.baseMoved(old); moved != "" {
			return moved
		}
		cmd := exec.Command(f.git, "merge", "--ff-only", "-q", merge)
		cmd.Dir = w.path
		cmd.Env = f.e.Environ
		out, err := cmd.CombinedOutput()
		if err == nil {
			return ""
		}
		if moved := f.baseMoved(old); moved != "" {
			return moved
		}
		return fmt.Sprintf("uncommitted changes in %s, where %s is checked out, are in the way:\n\n%s", w.path, f.base, strings.TrimSpace(string(out)))
	}
	if _, err := f.gitOut(f.repo, "update-ref", "refs/heads/"+f.base, merge, old); err != nil {
		if moved := f.baseMoved(old); moved != "" {
			return moved
		}
		return err.Error()
	}
	return ""
}

// baseMoved says so when the base is no longer at old.
func (f *factory) baseMoved(old string) string {
	if tip, err := f.gitOut(f.repo, "rev-parse", "--verify", "-q", "refs/heads/"+f.base); err == nil && tip == old {
		return ""
	}
	return fmt.Sprintf("%s moved while the Factory was merging.", f.base)
}

// landAt calls land with the commit that put tk's work into the base.
func (f *factory) landAt(ctx context.Context, tk core.TicketView, commit string) {
	if _, err := f.client.Land(ctx, f.project, core.LandInput{Ticket: core.Ref(tk.ID), Commit: commit}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: land: %v", tk.ID, err)
		}
		return
	}
	f.printf("%s: landed on %s at %s", tk.ID, f.base, short(commit))
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
