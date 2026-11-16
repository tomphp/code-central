package core

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// Changed Repositories (Spec code-central-848, ADR 0046): the
// Repositories in which a Ticket's branch holds work. The Factory
// reports them live while the Ticket is In Progress
// (report_changed_repositories), each finish settles them, and the
// Factory's finish check fails a finish that leaves one out or names a
// commit that isn't its branch's head (fail_finish, in failure.go). A
// Ticket stored without them, with a branch or a commit, reads as the
// Home Repository's, so there is no migration. Reads show them only while
// the Project has more than one Repository.

// RepositoryWork is a Ticket's branch and its head commit in one
// Repository, as finish and report_changed_repositories take them.
type RepositoryWork struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
}

// ChangedRepositoryView is one of a Ticket's Changed Repositories as
// reads show it, in landing order.
type ChangedRepositoryView struct {
	Name   string `json:"name"`
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Landed is the commit that put the work into the Repository's base;
	// left out until it lands.
	Landed *LandedView `json:"landed,omitempty"`
	// PushedCommit is the latest commit the Factory pushed for the Ticket
	// in the Repository; left out while it has pushed none.
	PushedCommit *PushedCommitView `json:"pushed_commit,omitempty"`
	// PullRequest is the Ticket's pull request in the Repository; left
	// out until the Pull Request Watcher reports one.
	PullRequest *PullRequestView `json:"pull_request,omitempty"`
	// CIResult is the Repository's CI result, from the pushed commit
	// CICommit; both are left out while it has none.
	CIResult CIResult `json:"ci_result,omitempty"`
	CICommit string   `json:"ci_commit,omitempty"`
}

// LandedView is the commit that put a Ticket's work into a Repository's
// base, and when Code Central heard of it.
type LandedView struct {
	Commit string    `json:"commit"`
	Time   time.Time `json:"time"`
}

// severalRepositories reports whether p has more than one Repository:
// only then do reads name Repositories (the visibility rule).
func severalRepositories(p store.Project) bool { return len(p.RepositoryList()) > 1 }

// changedRepositories is t's Changed Repositories as recorded, or with
// none recorded, the Home Repository's, named home, when t has a branch
// or a commit.
func changedRepositories(home string, t store.Ticket) []store.ChangedRepository {
	if t.Repositories != nil {
		return t.Repositories
	}
	if t.Branch != "" || t.Commit != "" {
		return []store.ChangedRepository{{Name: home, Branch: t.Branch, Commit: t.Commit}}
	}
	return nil
}

// changedRepositoryNames is the Names of t's Changed Repositories.
func changedRepositoryNames(home string, t store.Ticket) []string {
	var names []string
	for _, r := range changedRepositories(home, t) {
		names = append(names, r.Name)
	}
	return names
}

// hasBranchToLand reports whether accepting t moves it to Accepted to
// wait to land rather than to Done: one of its Changed Repositories has a
// branch, as a finish naming a branch always gave the Home Repository.
func hasBranchToLand(home string, t store.Ticket) bool {
	return slices.ContainsFunc(changedRepositories(home, t), func(r store.ChangedRepository) bool { return r.Branch != "" })
}

// landedSomewhere reports whether t's work has landed in any of its
// Changed Repositories.
func landedSomewhere(home string, t store.Ticket) bool {
	return slices.ContainsFunc(changedRepositories(home, t), func(r store.ChangedRepository) bool { return r.Landed != nil })
}

// partlyLanded reports whether t is Partly landed: Accepted, and landed
// in some of its Changed Repositories but not all.
func partlyLanded(home string, t store.Ticket) bool {
	return t.State == store.Accepted && landedSomewhere(home, t) &&
		slices.ContainsFunc(changedRepositories(home, t), func(r store.ChangedRepository) bool { return r.Landed == nil })
}

// landingRank orders Repository Names by p's landing order, with any
// name no longer listed after the rest.
func landingRank(p store.Project) func(name string) int {
	names := repositoryNames(p.RepositoryList())
	return func(name string) int {
		if i := slices.Index(names, name); i >= 0 {
			return i
		}
		return len(names)
	}
}

// inLandingOrder sorts xs, each named by name, by p's landing order.
func inLandingOrder[X any](p store.Project, xs []X, name func(X) string) {
	rank := landingRank(p)
	slices.SortStableFunc(xs, func(a, b X) int { return rank(name(a)) - rank(name(b)) })
}

// namesInLandingOrder is names sorted by p's landing order.
func namesInLandingOrder(p store.Project, names []string) []string {
	out := slices.Clone(names)
	inLandingOrder(p, out, func(n string) string { return n })
	return out
}

// repositoryName is r's Repository Name, for inLandingOrder.
func repositoryName(r store.ChangedRepository) string { return r.Name }

// checkRepositoryWork is ws as Changed Repositories of p, refusing a
// name that isn't one of p's Repositories, a name given twice and an
// entry without its branch or commit. field names the list.
func checkRepositoryWork(p store.Project, field string, ws []RepositoryWork) ([]store.ChangedRepository, error) {
	names := repositoryNames(p.RepositoryList())
	var out []store.ChangedRepository
	for i, w := range ws {
		at := fmt.Sprintf("%s[%d]", field, i)
		r := store.ChangedRepository{Name: strings.TrimSpace(w.Name), Branch: strings.TrimSpace(w.Branch), Commit: strings.TrimSpace(w.Commit)}
		switch {
		case !slices.Contains(names, r.Name):
			return nil, invalid(at+".name", "%q is not one of this Project's Repositories, which are %s: name each as your first prompt's Repositories list does", w.Name, strings.Join(names, ", "))
		case slices.ContainsFunc(out, func(o store.ChangedRepository) bool { return o.Name == r.Name }):
			return nil, invalid(at+".name", "%s is listed twice: give each Repository once, with its branch's head commit", r.Name)
		case r.Branch == "" || r.Commit == "":
			return nil, invalid(at, "%s needs both its branch and its branch's head commit", r.Name)
		}
		out = append(out, r)
	}
	return out, nil
}

// setChangedRepositories stores rs as t's Changed Repositories, in p's
// landing order, each keeping the landed commit t had for it. They are
// stored as none while they are just what t's branch and commit read as,
// so a single-repository Ticket's file is as it always was.
func setChangedRepositories(p store.Project, t *store.Ticket, rs []store.ChangedRepository) {
	was := changedRepositories(p.Key, *t)
	for i := range rs {
		if j := slices.IndexFunc(was, func(w store.ChangedRepository) bool { return w.Name == rs[i].Name }); j >= 0 && rs[i].Landed == nil {
			rs[i].Landed = was[j].Landed
		}
	}
	inLandingOrder(p, rs, repositoryName)
	if len(rs) == 0 || len(rs) == 1 && reflect.DeepEqual(rs, changedRepositories(p.Key, store.Ticket{Branch: t.Branch, Commit: t.Commit})) {
		rs = nil
	}
	t.Repositories = rs
}

// settleFinish applies a finish's branch, commit and repositories to t,
// which is p's. The Home Repository's work comes in either form, and
// branch and commit keep meaning it: one left out keeps its stored value.
// The finish then replaces t's Changed Repositories with the Home
// Repository's, when it has a branch or commit, and the others it names.
func settleFinish(p store.Project, t *store.Ticket, in FinishInput) error {
	rs, err := checkRepositoryWork(p, "repositories", in.Repositories)
	if err != nil {
		return err
	}
	branch, commit := strings.TrimSpace(in.Branch), strings.TrimSpace(in.Commit)
	home := slices.IndexFunc(rs, func(r store.ChangedRepository) bool { return r.Name == p.Key })
	switch {
	case home >= 0 && (branch != "" || commit != ""):
		return invalid("repositories", "%s is the Home Repository, given both in branch and commit and in repositories: give it in one form or the other", p.Key)
	case home >= 0:
		t.Branch, t.Commit = rs[home].Branch, rs[home].Commit
		rs = slices.Delete(rs, home, home+1)
	default:
		if branch != "" {
			t.Branch = branch
		}
		if commit != "" {
			t.Commit = commit
		}
	}
	if t.Branch != "" || t.Commit != "" {
		rs = append(rs, store.ChangedRepository{Name: p.Key, Branch: t.Branch, Commit: t.Commit})
	}
	setChangedRepositories(p, t, rs)
	return nil
}

// dropsRepositories reports whether t's Changed Repositories, as a finish
// settled them, leave out any of was, those it had before. Such a finish
// isn't auto-accepted: it waits In Review for the Factory's finish check,
// which an Auto-accept to Done would otherwise skip.
func dropsRepositories(home string, t store.Ticket, was []string) bool {
	now := changedRepositoryNames(home, t)
	return slices.ContainsFunc(was, func(name string) bool { return !slices.Contains(now, name) })
}

// changedRepositoryViews is t's Changed Repositories as v, t's view in
// p, shows them: the Home Repository's with the CI result v gives, and
// any Repository's landed commit, latest pushed commit and pull request.
func changedRepositoryViews(p store.Project, t store.Ticket, v TicketView) []ChangedRepositoryView {
	rs := slices.Clone(changedRepositories(p.Key, t))
	inLandingOrder(p, rs, repositoryName)
	out := []ChangedRepositoryView{}
	for _, r := range rs {
		rv := ChangedRepositoryView{Name: r.Name, Branch: r.Branch, Commit: r.Commit, PushedCommit: latestPushedIn(p.Key, t, r.Name)}
		if r.Landed != nil {
			rv.Landed = &LandedView{Commit: r.Landed.Commit, Time: r.Landed.Time}
		}
		if r.Name == p.Key {
			rv.CIResult, rv.CICommit = v.CIResult, v.CICommit
			if rv.Landed == nil {
				rv.Landed = landedByMove(t)
			}
		}
		if t.PullRequest != nil && recordRepository(p.Key, t.PullRequest.Repository) == r.Name {
			rv.PullRequest = pullRequestView(t.PullRequest)
		}
		out = append(out, rv)
	}
	return out
}

// recordRepository is the Repository Name a record names, home for one
// naming none, as one written before Repositories existed.
func recordRepository(home, name string) string {
	if name == "" {
		return home
	}
	return name
}

// latestPushedIn is the latest commit the Factory pushed for t in the
// Repository name; nil when it pushed none there.
func latestPushedIn(home string, t store.Ticket, name string) *PushedCommitView {
	for i := len(t.PushedCommits) - 1; i >= 0; i-- {
		if pc := t.PushedCommits[i]; recordRepository(home, pc.Repository) == name {
			return &PushedCommitView{Commit: pc.Commit, Pushed: pc.Pushed}
		}
	}
	return nil
}

// landedByMove is the commit a Done t landed at, from its land, for a
// Ticket that landed before landed commits were recorded per Repository;
// nil when its last Move isn't a land with a commit.
func landedByMove(t store.Ticket) *LandedView {
	if t.State != store.Done || len(t.Moves) == 0 {
		return nil
	}
	if m := t.Moves[len(t.Moves)-1]; m.Via == "merge" && m.Commit != "" {
		return &LandedView{Commit: m.Commit, Time: m.Time}
	}
	return nil
}

// specRepositories is the Changed Repositories of tickets, a Spec's, by
// Name in p's landing order: the union of theirs.
func specRepositories(p store.Project, tickets []store.Ticket) []string {
	var names []string
	for _, t := range tickets {
		for _, name := range changedRepositoryNames(p.Key, t) {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	return namesInLandingOrder(p, names)
}

// ReportChangedRepositoriesInput is the Factory's
// report_changed_repositories(ticket, claim_id, repositories, full?): the
// Repositories whose branch holds the Ticket's work so far, each with its
// branch and head commit.
type ReportChangedRepositoriesInput struct {
	Ticket       Ref              `json:"ticket"`
	ClaimID      string           `json:"claim_id"`
	Repositories []RepositoryWork `json:"repositories"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportChangedRepositories is the Factory's report_changed_repositories:
// from the live Claim, while the Ticket is In Progress, it records the
// Repositories as the Ticket's live Changed Repositories, replacing the
// last report's. A report the same as the record writes nothing.
func (c *Core) ReportChangedRepositories(ctx context.Context, key string, in ReportChangedRepositoriesInput) (Written[TicketView], error) {
	return c.updateTicketIn(key, "report_changed_repositories", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		if _, err := holder(t, key, in.ClaimID); err != nil {
			return false, err
		}
		if t.State != store.InProgress {
			return false, &Error{Code: ActionNotAllowed, AllowedActions: allowedActions(t.State, ActorOf(ctx)), Message: fmt.Sprintf(
				"%s is %s: its Changed Repositories are reported only while it is In Progress, and its finish settles them", DisplayID(key, t.Number), t.State)}
		}
		p, _ := r.Project(key)
		rs, err := checkRepositoryWork(p, "repositories", in.Repositories)
		if err != nil {
			return false, err
		}
		was := t.Repositories
		setChangedRepositories(p, t, rs)
		return !reflect.DeepEqual(was, t.Repositories), nil
	})
}
