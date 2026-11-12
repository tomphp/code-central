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
func (f *factory) moveBase(ctx context.Context, remote, local, theirs string, merged []core.TicketView) bool {
	base := f.baseBranch()
	heads, err := f.mergeHeads(theirs, merged)
	cant := ""
	var r rebuilt
	if err != nil {
		cant = err.Error()
	} else {
		r, cant = f.rebuildOnto(theirs, local, heads)
	}
	if cant != "" {
		if !f.baseDiverged {
			f.printf("warning: can't move %s onto %s/%s: %s", base, remote, base, cant)
		}
		f.noteBase(ctx, true, local, theirs)
		return false
	}
	reset := func(dir, commit string) (string, error) { return f.repository.reset(dir, commit, local) }
	if why := f.setBaseTo(local, r.tip, reset); why != "" {
		f.noteBase(ctx, false, local, theirs)
		f.printf("warning: can't move %s onto %s/%s yet: %s", base, remote, base, firstLine(why))
		return false
	}
	for id, m := range f.mergedOn {
		if made, ok := r.rewritten[m.commit]; ok {
			f.mergedOn[id] = mergeOn{m.base, made}
		}
	}
	f.noteBase(ctx, false, r.tip, theirs)
	commits := "commits"
	if r.count == 1 {
		commits = "commit"
	}
	f.printf("%s: moved %d unpushed %s onto %s/%s, now at %s", base, r.count, commits, remote, base, short(r.tip))
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
func (f *factory) rebuildOnto(onto, from string, heads map[string]bool) (rebuilt, string) {
	chain, err := f.repository.firstParents(from, onto)
	if err != nil {
		return rebuilt{}, err.Error()
	}
	r := rebuilt{tip: onto, rewritten: map[string]string{}}
	for _, c := range chain {
		parents, err := f.repository.parentsOf(c)
		if err != nil {
			return rebuilt{}, err.Error()
		}
		var tree string
		var conflicts []string
		switch len(parents) {
		case 1:
			tree, conflicts, err = f.repository.replayTree(r.tip, c)
		case 2:
			if !heads[parents[1]] {
				return rebuilt{}, fmt.Sprintf("%s is a merge the Factory didn't make, so it can't be rebuilt safely", short(c))
			}
			tree, conflicts, err = f.repository.mergeTree(r.tip, parents[1])
		default:
			return rebuilt{}, fmt.Sprintf("%s has %d parents, so it can't be rebuilt safely", short(c), len(parents))
		}
		if err != nil {
			return rebuilt{}, err.Error()
		}
		if len(conflicts) > 0 {
			return rebuilt{}, fmt.Sprintf("%s conflicts in %s", short(c), strings.Join(conflicts, ", "))
		}
		made, err := f.repository.commitTreeLike(tree, c, append([]string{r.tip}, parents[1:]...)...)
		if err != nil {
			return rebuilt{}, err.Error()
		}
		r.rewritten[c] = made
		r.tip = made
		r.count++
	}
	return r, ""
}

// mergeHeads is the commits a merge the Factory made into the base can
// have as its second parent: the work of each Accepted Ticket, and the
// commits on each Spec Branch's first-parent line that the remote's base
// at theirs doesn't hold, since a Spec Branch merged into the base may
// have gone on since. A Ticket whose work isn't in the repository has no
// merge to find.
func (f *factory) mergeHeads(theirs string, merged []core.TicketView) (map[string]bool, error) {
	heads := map[string]bool{}
	for _, tk := range merged {
		if head, _, err := f.workOf(tk); err == nil {
			heads[head] = true
		}
	}
	specs, err := f.repository.specBranches()
	if err != nil {
		return nil, err
	}
	for _, s := range specs {
		line, err := f.repository.firstParents("refs/heads/"+s, theirs)
		if err != nil {
			return nil, err
		}
		for _, c := range line {
			heads[c] = true
		}
	}
	return heads, nil
}
