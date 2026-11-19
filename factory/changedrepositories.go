package factory

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// A Ticket's Changed Repositories (Spec code-central-848, ADR 0046): the
// Repositories whose branch for the Ticket holds commits in neither their
// Base nor, under Land per Spec, their Spec Branch. With several
// Repositories the Factory reports each held In Progress Ticket's live on
// every poll, and checks each finish against them: after the turn that
// finished it, and again before landing it, so a restart or an
// Auto-accept can't skip the check. With the Home Repository alone it
// does neither, as before. The reports are the session part's; the check
// is both parts', as each reads only the registry.

// changedIn is tk's Changed Repositories as git has them, in landing
// order, each with its branch's head. A Stage, being detached, changes
// none.
func (rs *repositories) changedIn(tk core.TicketView) []core.RepositoryWork {
	branch := workBranch(core.GuideName(tk.Labels), tk.ID)
	if branch == "" {
		return nil
	}
	var out []core.RepositoryWork
	for _, r := range rs.ready() {
		if head, ok := rs.changedHead(r, tk, branch); ok {
			out = append(out, core.RepositoryWork{Name: r.name, Branch: branch, Commit: head})
		}
	}
	return out
}

// changedHead is the head of tk's branch in r, when it holds commits
// beyond r's Base and, for a Spec-landed Ticket, its Spec Branch there.
// While r's Base is missing nothing can be said to be beyond it, so r
// counts as unchanged until it is back (Base missing).
func (rs *repositories) changedHead(r *repository, tk core.TicketView, branch string) (string, bool) {
	head, err := r.git.commit("refs/heads/" + branch)
	if err != nil || rs.inBaseOf(r, head) {
		return "", false
	}
	if _, err := r.git.commit("refs/heads/" + rs.baseOf(r)); err != nil {
		return "", false
	}
	if tk.SpecLanded && r.git.isAncestor(head, "refs/heads/"+tk.Spec.Branch) {
		return "", false
	}
	return head, true
}

// reportChanged reports the live Changed Repositories of tk, held In
// Progress under c, when the set or a head has changed since the last
// report for c.
func (f *factory) reportChanged(ctx context.Context, c claim, tk core.TicketView) {
	if !f.repos.several() || tk.State != domain.InProgress {
		return
	}
	work := f.repos.changedIn(tk)
	said := fmt.Sprint(work)
	if last, ok := f.reportedChanged[c.ClaimID]; ok && last == said {
		return
	}
	in := core.ReportChangedRepositoriesInput{Ticket: core.Ref(c.Ticket), ClaimID: c.ClaimID, Repositories: work}
	if _, err := f.client.ReportChangedRepositories(ctx, f.project, in); err != nil {
		if ctx.Err() == nil {
			f.claimf(c, "%s: report_changed_repositories: %v", c.Ticket, err)
		}
		return
	}
	f.reportedChanged[c.ClaimID] = said
}

// checkFinish runs the finish check on tk, finished: with several
// Repositories, a finish that leaves out a Repository whose branch holds
// work, or names a commit that isn't its branch's head, fails with a
// Failure Report (fail_finish). It reports whether the finish passed, and
// whether failing it moved tk to Failed; one that couldn't be failed yet
// is failed by the check before it lands. It runs against the registry
// rs, fails the finish through c in project, and prints to rs's output:
// the session part runs it after a turn, the landing part before landing.
func checkFinish(ctx context.Context, rs *repositories, c *client.Client, project string, tk core.TicketView) (passed, failed bool) {
	if !rs.several() {
		return true, false
	}
	problems, stands := rs.finishProblems(tk)
	if len(problems) == 0 {
		return true, false
	}
	rs.printf("%s: the finish check failed: %s", tk.ID, strings.Join(problems, "; "))
	in := core.FailFinishInput{
		Ticket:    core.Ref(tk.ID),
		WhatBroke: "The finish's record of the Repositories this Ticket changed doesn't match their branches:\n\n- " + strings.Join(problems, "\n- "),
		Why: "With several Repositories, a finish records every Repository whose branch holds the work, each at its branch's head commit: `branch` and `commit` for the Home Repository, and `repositories` for each other one. " +
			"The Factory checks the record against every Repository's branch after the finish and before landing, so that no work is dropped and none lands at the wrong commit.",
		WhatToFix:       "Send the Ticket back. Its next Claim finishes again, recording each Repository below that holds work at its branch's head commit.",
		WhereWorkStands: "The Ticket's branches, as the finish check found them:\n\n- " + strings.Join(stands, "\n- "),
	}
	if _, err := c.FailFinish(ctx, project, in); err != nil {
		if ctx.Err() == nil {
			rs.printf("%s: fail_finish: %v", tk.ID, err)
		}
		return false, false
	}
	return false, true
}

// finishProblems compares tk's recorded Changed Repositories with its
// branch in every Repository with a clone: what is wrong with the record,
// and where each Repository's work stands.
func (rs *repositories) finishProblems(tk core.TicketView) (problems, stands []string) {
	work := rs.changedIn(tk)
	for _, r := range rs.ready() {
		i := slices.IndexFunc(tk.Repositories, func(v core.ChangedRepositoryView) bool { return v.Name == r.name })
		j := slices.IndexFunc(work, func(w core.RepositoryWork) bool { return w.Name == r.name })
		if j >= 0 {
			stands = append(stands, fmt.Sprintf("%s: branch %s at %s", r.name, work[j].Branch, short(work[j].Commit)))
		}
		switch {
		case i >= 0:
			if p := rs.recordProblem(r, tk, tk.Repositories[i], j >= 0); p != "" {
				problems = append(problems, p)
			}
		case j >= 0:
			problems = append(problems, fmt.Sprintf("%s is left out, but its branch %s holds commits beyond %s, up to %s", r.name, work[j].Branch, rs.baseOf(r), short(work[j].Commit)))
		}
	}
	if len(stands) == 0 {
		stands = append(stands, "no Repository's branch holds commits beyond its Base")
	}
	return problems, stands
}

// recordProblem says what is wrong with rec, tk's record for r: its
// commit isn't its branch's head there. A record with no commit, or with
// no branch and not the Ticket's own, names nothing to check. Nor does one
// whose commit is in r's base already while the Ticket's branch holds no
// more work (holdsWork), as when the human merged it and deleted the
// branch.
func (rs *repositories) recordProblem(r *repository, tk core.TicketView, rec core.ChangedRepositoryView, holdsWork bool) string {
	branch := rec.Branch
	if branch == "" {
		branch = workBranch(core.GuideName(tk.Labels), tk.ID)
	}
	if rec.Commit == "" || branch == "" {
		return ""
	}
	commit, err := r.git.commit(rec.Commit)
	if err == nil && !holdsWork && rs.inBaseOf(r, commit) {
		return ""
	}
	if rec.Landed != nil && !holdsWork {
		// Its work landed, as by a pull request merged with a squash, and
		// its branch, moved to the base's tip, holds nothing new.
		return ""
	}
	if err == nil {
		if _, ok := replayedTip(r, tk.ID, branch); ok {
			// Its work was replayed onto its Spec Branch or the base
			// under Rebase, and its branch moved to the replayed commits.
			return ""
		}
	}
	head, herr := r.git.commit("refs/heads/" + branch)
	switch {
	case herr != nil:
		return fmt.Sprintf("%s's branch %s isn't in the Repository", r.name, branch)
	case err != nil || commit != head:
		return fmt.Sprintf("%s is recorded at %s, which isn't its branch %s's head, %s", r.name, rec.Commit, branch, short(head))
	}
	return ""
}

// checksFinish reports whether a turn that left tk in state, its Claim
// ended by by, finished it and left it where its finish can still fail.
func checksFinish(by string, state domain.State) bool {
	return by == "finish" && (state == domain.InReview || state == domain.Accepted)
}
