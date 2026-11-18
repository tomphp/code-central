package factory

import (
	"fmt"
)

// Replaying an accepted Ticket onto its Spec Branch under Rebase (Spec
// code-central-1063, ADR 0067): instead of a `Merge <id>` commit, the
// Ticket's own commits are made again on the Spec Branch's tip, which
// then fast-forwards to them, and its branch moves to them too, so that
// the "head is an ancestor of the Spec Branch" check still answers
// "already merged". Where its own commits start is its fork point, kept
// per Ticket per Repository in a Factory-private fork ref.

// forkRefs is the namespace of the Factory's fork refs.
const forkRefs = "refs/code-central/fork/"

// forkRef is ticket's fork ref in a Repository: the Spec Branch tip its
// branch was cut from or last replayed onto, where its own commits start.
// It is written at the cut, moved whenever the Factory moves the branch,
// and deleted when the Ticket's branch is cleaned up.
func forkRef(ticket string) string { return forkRefs + ticket }

// setForkPoint points ticket's fork ref in r at commit, whatever it held.
func setForkPoint(r *repository, ticket, commit string) error {
	old, _ := r.git.commit(forkRef(ticket))
	return r.git.updateRef(forkRef(ticket), commit, old)
}

// moveForkPoint moves ticket's fork ref in r to commit, as its branch
// moves there, when it has one.
func (rs *repositories) moveForkPoint(r *repository, ticket, commit string) {
	old, err := r.git.commit(forkRef(ticket))
	if err != nil || old == commit {
		return
	}
	if err := r.git.updateRef(forkRef(ticket), commit, old); err != nil {
		rs.printf("warning: %s: can't move its fork point%s: %v", ticket, r.where(), err)
	}
}

// deleteForkPoint deletes ticket's fork ref in r, when it has one.
func (rs *repositories) deleteForkPoint(r *repository, ticket string) {
	old, err := r.git.commit(forkRef(ticket))
	if err != nil {
		return
	}
	if err := r.git.deleteRef(forkRef(ticket), old); err != nil {
		rs.printf("warning: %s: can't delete its fork point%s: %v", ticket, r.where(), err)
	}
}

// forkPoint is where ticket's own commits on head start, for replaying
// them onto onto: the later of its fork ref and head's merge base with
// onto. The ref is newer when onto was rewritten since the cut; the merge
// base is newer when the branch was rebased onto onto since, to resolve
// a conflict. A ref head doesn't hold, or none, leaves the merge base.
func forkPoint(r *repository, ticket, head, onto string) (string, error) {
	base, err := r.git.mergeBase(head, onto)
	if err != nil {
		return "", err
	}
	if ref, err := r.git.commit(forkRef(ticket)); err == nil && ref != base && r.git.isAncestor(base, ref) && r.git.isAncestor(ref, head) {
		return ref, nil
	}
	return base, nil
}

// replayedTip is ticket's branch's tip in r when the Factory has replayed
// the work finished at head: the branch no longer holds head, and holds
// commits beyond its fork ref, which the replay set to the Spec Branch
// tip it replayed onto. Those commits are the Ticket's work from then on,
// whether or not the Spec Branch was rewritten since.
func replayedTip(r *repository, ticket, branch, head string) (string, bool) {
	if branch == "" {
		return "", false
	}
	tip, err := r.git.commit("refs/heads/" + branch)
	if err != nil || tip == head || r.git.isAncestor(head, tip) {
		return "", false
	}
	ref, err := r.git.commit(forkRef(ticket))
	if err != nil || ref == tip || !r.git.isAncestor(ref, tip) {
		return "", false
	}
	return tip, true
}

// replay is a Ticket's work in a Repository replayed onto its Spec
// Branch: the commits made, ending at tip, and the Ticket's branch to move
// there from head, by a reset in worktree when it is checked out there.
// Nothing is moved until it is applied.
type replay struct {
	branch, head, worktree, tip string
}

// planReplay replays tk's work in r, head, onto onto, the Spec Branch's
// tip: the non-merge commits on head's first-parent line after its fork
// point, oldest first, each with its author, author date and message, as
// `git rebase` drops merges. It makes the commits and moves nothing. It
// returns the files the first commit that doesn't apply cleanly conflicts
// in; why it can't move tk's branch yet, as when uncommitted changes in
// its worktree are in the way; or a nil replay when there is nothing to
// replay it by, so the work is merged instead: no branch, a branch moved
// on from head, or no commit of its own but merges.
func (l *lander) planReplay(r *repository, ticket, branch, head, onto string) (rp *replay, conflicts []string, why string, err error) {
	if branch == "" {
		return nil, nil, "", nil
	}
	if tip, err := r.git.commit("refs/heads/" + branch); err != nil || tip != head {
		l.printf("%s: its branch %s%s isn't at its finish commit %s, so it is merged into its Spec Branch, not replayed", ticket, branch, r.where(), short(head))
		return nil, nil, "", nil
	}
	fork, err := forkPoint(r, ticket, head, onto)
	if err != nil {
		return nil, nil, "", err
	}
	chain, err := r.git.firstParents(head, fork)
	if err != nil {
		return nil, nil, "", err
	}
	tip := onto
	for _, c := range chain {
		parents, err := r.git.parentsOf(c)
		if err != nil {
			return nil, nil, "", err
		}
		if len(parents) != 1 {
			continue
		}
		tree, files, err := r.git.replayTree(tip, c)
		if err != nil {
			return nil, nil, "", err
		}
		if len(files) > 0 {
			return nil, files, "", nil
		}
		if tip, err = r.git.commitTreeLike(tree, c, tip); err != nil {
			return nil, nil, "", err
		}
	}
	if tip == onto {
		return nil, nil, "", nil
	}
	rp = &replay{branch: branch, head: head, tip: tip}
	list, err := r.git.worktrees()
	if err != nil {
		return nil, nil, "", err
	}
	for _, w := range list {
		if w.branch != branch {
			continue
		}
		status, err := r.git.status(w.path)
		if err != nil {
			return nil, nil, "", err
		}
		if status != "" {
			return nil, nil, fmt.Sprintf("uncommitted changes in %s, where %s is checked out, are in the way of moving it to its commits replayed onto its Spec Branch:\n\n%s", w.path, branch, status), nil
		}
		rp.worktree = w.path
	}
	return rp, nil, "", nil
}

// apply moves the Ticket's branch in r to the replayed commits, and its
// fork ref to onto, the Spec Branch tip they were replayed onto, first,
// so that a branch found moved never replays twice.
func (rp *replay) apply(r *repository, ticket, onto string) error {
	if err := setForkPoint(r, ticket, onto); err != nil {
		return fmt.Errorf("can't record its fork point: %v", err)
	}
	if rp.worktree != "" {
		if out, err := r.git.reset(rp.worktree, rp.tip, rp.head); err != nil {
			return fmt.Errorf("can't move %s, checked out in %s, to its replayed commits: %v %s", rp.branch, rp.worktree, err, out)
		}
		return nil
	}
	if err := r.git.updateRef("refs/heads/"+rp.branch, rp.tip, rp.head); err != nil {
		return fmt.Errorf("can't move %s to its replayed commits: %v", rp.branch, err)
	}
	return nil
}
