package factory

import (
	"context"
	"fmt"
	"strings"

	"github.com/tomoram/code-central/core"
)

// Under Merge + Push, when the local base and the remote's each hold
// commits the other lacks, the Factory moves the local base onto the
// remote's tip and then pushes (ADR 0033). Its own unpushed merges are
// rebuilt the way they were made, with the Ticket's branch or the Spec
// Branch untouched as the second parent, so the head of every Ticket
// merged into the base is still an ancestor of it; the human's unpushed
// commits are replayed on top in order. The move is all or nothing: the
// base moves only once every commit applied cleanly. Nothing pushed is
// ever rewritten.

// moveBase moves the local base, at local, onto the remote's base at
// theirs, and reports whether the base is level with the remote's after
// it. merged is the Accepted Tickets, whose work marks the merges as the
// Factory's. A merge that isn't the Factory's, or a conflict, leaves the
// base as it was and reports it diverged; uncommitted changes in the way
// of the move leave it for the next poll.
func (f *factory) moveBase(ctx context.Context, repo *repository, remote, local, theirs string, merged []core.TicketView) bool {
	base, s := f.repos.baseOf(repo), f.stateOf(repo)
	heads, err := f.mergeHeads(repo, theirs, merged)
	cant := ""
	var r rebuilt
	if err != nil {
		cant = err.Error()
	} else {
		r, cant = f.rebuildOnto(repo, theirs, local, heads)
	}
	if cant != "" {
		if !s.diverged {
			f.printf("warning: can't move %s onto %s/%s%s: %s", base, remote, base, repo.where(), cant)
		}
		f.noteBase(ctx, repo, true, local, theirs)
		return false
	}
	reset := func(dir, commit string) (string, error) { return repo.git.reset(dir, commit, local) }
	if why := f.setBaseTo(repo, local, r.tip, reset); why != "" {
		f.noteBase(ctx, repo, false, local, theirs)
		f.printf("warning: can't move %s onto %s/%s%s yet: %s", base, remote, base, repo.where(), firstLine(why))
		return false
	}
	for id, m := range s.mergedOn {
		if made, ok := r.rewritten[m.commit]; ok {
			s.mergedOn[id] = mergeOn{m.base, made}
		}
	}
	f.noteBase(ctx, repo, false, r.tip, theirs)
	commits := "commits"
	if r.count == 1 {
		commits = "commit"
	}
	f.printf("%s: moved %d unpushed %s onto %s/%s, now at %s%s", base, r.count, commits, remote, base, short(r.tip), repo.where())
	return true
}

// rebuilt is a base's history rebuilt onto another tip: the new tip, how
// many commits were rebuilt, and each old commit's rebuilt one.
type rebuilt struct {
	tip       string
	count     int
	rewritten map[string]string
}

// rebuildOnto rebuilds the commits on from's first-parent line that onto
// doesn't reach, oldest first, onto onto: a merge whose second parent is
// one of heads is remade as a merge of the new tip and that same second
// parent, keeping its message, and any other non-merge commit is replayed
// as a cherry-pick. Nothing is moved: the commits are only made. It
// returns why it can't, or "": a conflict, or a merge that isn't the
// Factory's.
func (f *factory) rebuildOnto(repo *repository, onto, from string, heads map[string]bool) (rebuilt, string) {
	g := repo.git
	chain, err := g.firstParents(from, onto)
	if err != nil {
		return rebuilt{}, err.Error()
	}
	r := rebuilt{tip: onto, rewritten: map[string]string{}}
	for _, c := range chain {
		parents, err := g.parentsOf(c)
		if err != nil {
			return rebuilt{}, err.Error()
		}
		var tree string
		var conflicts []string
		switch len(parents) {
		case 1:
			tree, conflicts, err = g.replayTree(r.tip, c)
		case 2:
			if !heads[parents[1]] {
				return rebuilt{}, fmt.Sprintf("%s is a merge the Factory didn't make, so it can't be rebuilt safely", short(c))
			}
			tree, conflicts, err = g.mergeTree(r.tip, parents[1])
		default:
			return rebuilt{}, fmt.Sprintf("%s has %d parents, so it can't be rebuilt safely", short(c), len(parents))
		}
		if err != nil {
			return rebuilt{}, err.Error()
		}
		if len(conflicts) > 0 {
			return rebuilt{}, fmt.Sprintf("%s conflicts in %s", short(c), strings.Join(conflicts, ", "))
		}
		made, err := g.commitTreeLike(tree, c, append([]string{r.tip}, parents[1:]...)...)
		if err != nil {
			return rebuilt{}, err.Error()
		}
		r.rewritten[c] = made
		r.tip = made
		r.count++
	}
	return r, ""
}

// mergeHeads is the commits a merge the Factory made into repo's base can
// have as its second parent: the work there of each Accepted Ticket, and
// the commits on each Spec Branch's first-parent line that the remote's
// base at theirs doesn't hold, since a Spec Branch merged into the base
// may have gone on since. A Ticket whose work isn't in the repository has
// no merge to find.
func (f *factory) mergeHeads(repo *repository, theirs string, merged []core.TicketView) (map[string]bool, error) {
	heads := map[string]bool{}
	for _, tk := range merged {
		if head, _, err := f.workIn(repo, tk); err == nil {
			heads[head] = true
		}
	}
	specs, err := repo.git.specBranches()
	if err != nil {
		return nil, err
	}
	for _, s := range specs {
		line, err := repo.git.firstParents("refs/heads/"+s, theirs)
		if err != nil {
			return nil, err
		}
		for _, c := range line {
			heads[c] = true
		}
	}
	return heads, nil
}
