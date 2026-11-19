package core

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/tomoram/code-central/domain"
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

// severalRepositories reports whether p has more than one Repository:
// only then do reads name Repositories (the visibility rule).
func severalRepositories(p domain.Project) bool { return len(p.RepositoryList()) > 1 }

// changedRepositories is t's Changed Repositories as recorded, or with
// none recorded, the Home Repository's, named home, when t has a branch
// or a commit.
func changedRepositories(home string, t domain.Ticket) []domain.ChangedRepository {
	if t.Repositories != nil {
		return t.Repositories
	}
	if t.Branch != "" || t.Commit != "" {
		return []domain.ChangedRepository{{Name: home, Branch: t.Branch, Commit: t.Commit}}
	}
	return nil
}

// changedRepositoryNames is the Names of t's Changed Repositories.
func changedRepositoryNames(home string, t domain.Ticket) []string {
	var names []string
	for _, r := range changedRepositories(home, t) {
		names = append(names, r.Name)
	}
	return names
}

// hasBranchToLand reports whether accepting t moves it to Accepted to
// wait to land rather than to Done: one of its Changed Repositories has a
// branch, as a finish naming a branch always gave the Home Repository.
func hasBranchToLand(home string, t domain.Ticket) bool {
	return slices.ContainsFunc(changedRepositories(home, t), func(r domain.ChangedRepository) bool { return r.Branch != "" })
}

// landedSomewhere reports whether t's work has landed in any of its
// Changed Repositories.
func landedSomewhere(home string, t domain.Ticket) bool {
	return slices.ContainsFunc(changedRepositories(home, t), func(r domain.ChangedRepository) bool { return r.Landed != nil })
}

// landedNames is the Names of the Changed Repositories t has landed in.
func landedNames(home string, t domain.Ticket) []string {
	var out []string
	for _, r := range changedRepositories(home, t) {
		if r.Landed != nil {
			out = append(out, r.Name)
		}
	}
	return out
}

// partlyLanded reports whether t is Partly landed: Accepted, and landed
// in some of its Changed Repositories but not all.
func partlyLanded(home string, t domain.Ticket) bool {
	return t.State == domain.Accepted && landedSomewhere(home, t) &&
		slices.ContainsFunc(changedRepositories(home, t), func(r domain.ChangedRepository) bool { return r.Landed == nil })
}

// landingRepository is the Repository a land of t, p's, names: name, or
// the Home Repository for none. It refuses a name that isn't one of p's
// Repositories, or isn't one of t's Changed Repositories while t has any.
func landingRepository(p domain.Project, t domain.Ticket, name string) (string, error) {
	r, err := repositoryField(p, name)
	if err != nil {
		return "", err
	}
	changed := changedRepositoryNames(p.Key, t)
	if len(changed) > 0 && !slices.Contains(changed, r.Name) {
		return "", invalid("repository", "%s is not one of %s's Changed Repositories, which are %s: land the work in each of those, naming it",
			r.Name, DisplayID(p.Key, t.Number), strings.Join(namesInLandingOrder(p, changed), ", "))
	}
	return r.Name, nil
}

// landsLast reports whether landing in the Repository name lands the
// last of rs, a Ticket's Changed Repositories: every other one has landed.
func landsLast(rs []domain.ChangedRepository, name string) bool {
	return !slices.ContainsFunc(rs, func(r domain.ChangedRepository) bool { return r.Name != name && r.Landed == nil })
}

// landingRank orders Repository Names by p's landing order, with any
// name no longer listed after the rest.
func landingRank(p domain.Project) func(name string) int {
	names := repositoryNames(p.RepositoryList())
	return func(name string) int {
		if i := slices.Index(names, name); i >= 0 {
			return i
		}
		return len(names)
	}
}

// inLandingOrder sorts xs, each named by name, by p's landing order.
func inLandingOrder[X any](p domain.Project, xs []X, name func(X) string) {
	rank := landingRank(p)
	slices.SortStableFunc(xs, func(a, b X) int { return rank(name(a)) - rank(name(b)) })
}

// namesInLandingOrder is names sorted by p's landing order.
func namesInLandingOrder(p domain.Project, names []string) []string {
	out := slices.Clone(names)
	inLandingOrder(p, out, func(n string) string { return n })
	return out
}

// repositoryName is r's Repository Name, for inLandingOrder.
func repositoryName(r domain.ChangedRepository) string { return r.Name }

// checkRepositoryWork is ws as Changed Repositories of p, refusing a
// name that isn't one of p's Repositories, a name given twice and an
// entry without its branch or commit. field names the list.
func checkRepositoryWork(p domain.Project, field string, ws []RepositoryWork) ([]domain.ChangedRepository, error) {
	names := repositoryNames(p.RepositoryList())
	var out []domain.ChangedRepository
	for i, w := range ws {
		at := fmt.Sprintf("%s[%d]", field, i)
		r := domain.ChangedRepository{Name: strings.TrimSpace(w.Name), Branch: strings.TrimSpace(w.Branch), Commit: strings.TrimSpace(w.Commit)}
		switch {
		case !slices.Contains(names, r.Name):
			return nil, invalid(at+".name", "%q is not one of this Project's Repositories, which are %s: name each as your first prompt's Repositories list does", w.Name, strings.Join(names, ", "))
		case slices.ContainsFunc(out, func(o domain.ChangedRepository) bool { return o.Name == r.Name }):
			return nil, invalid(at+".name", "%s is listed twice: give each Repository once, with its branch's head commit", r.Name)
		case r.Branch == "" || r.Commit == "":
			return nil, invalid(at, "%s needs both its branch and its branch's head commit", r.Name)
		}
		out = append(out, r)
	}
	return out, nil
}

// setChangedRepositories stores rs as t's Changed Repositories, in p's
// landing order, each keeping the landed commit t had for it while its
// commit is the same: new work in a Repository needs landing again. A
// Repository t's work has landed in stays one of them though rs leaves it
// out, as when t is sent back after a sibling pull request merged and its
// branch there holds nothing new (ADR 0050). They are stored as none
// while they are just what t's branch and commit read as, so a
// single-repository Ticket's file is as it always was.
func setChangedRepositories(p domain.Project, t *domain.Ticket, rs []domain.ChangedRepository) {
	was := changedRepositories(p.Key, *t)
	for i := range rs {
		if j := slices.IndexFunc(was, func(w domain.ChangedRepository) bool { return w.Name == rs[i].Name }); j >= 0 && rs[i].Landed == nil && rs[i].Commit == was[j].Commit {
			rs[i].Landed = was[j].Landed
		}
	}
	for _, w := range was {
		if w.Landed != nil && !slices.ContainsFunc(rs, func(r domain.ChangedRepository) bool { return r.Name == w.Name }) {
			rs = append(rs, w)
		}
	}
	inLandingOrder(p, rs, repositoryName)
	if len(rs) == 0 || len(rs) == 1 && reflect.DeepEqual(rs, changedRepositories(p.Key, domain.Ticket{Branch: t.Branch, Commit: t.Commit})) {
		rs = nil
	}
	t.Repositories = rs
}

// settleFinish applies a finish's branch, commit and repositories to t,
// which is p's. The Home Repository's work comes in either form, and
// branch and commit keep meaning it: one left out keeps its stored value.
// The finish then replaces t's Changed Repositories with the Home
// Repository's, when it has a branch or commit, and the others it names.
func settleFinish(p domain.Project, t *domain.Ticket, in FinishInput) error {
	rs, err := checkRepositoryWork(p, "repositories", in.Repositories)
	if err != nil {
		return err
	}
	branch, commit := strings.TrimSpace(in.Branch), strings.TrimSpace(in.Commit)
	home := slices.IndexFunc(rs, func(r domain.ChangedRepository) bool { return r.Name == p.Key })
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
		rs = append(rs, domain.ChangedRepository{Name: p.Key, Branch: t.Branch, Commit: t.Commit})
	}
	setChangedRepositories(p, t, rs)
	return nil
}

// dropsRepositories reports whether t's Changed Repositories, as a finish
// settled them, leave out any of was, those it had before. Such a finish
// isn't auto-accepted: it waits In Review for the Factory's finish check,
// which an Auto-accept to Done would otherwise skip.
func dropsRepositories(home string, t domain.Ticket, was []string) bool {
	now := changedRepositoryNames(home, t)
	return slices.ContainsFunc(was, func(name string) bool { return !slices.Contains(now, name) })
}

// changedRepositoryViews is t's Changed Repositories as t's view in p
// shows them: each with its landed commit, latest pushed commit, pull
// request, and from ci, p's CI state, its CI result there.
func changedRepositoryViews(p domain.Project, t domain.Ticket, s *ciState) []ChangedRepositoryView {
	ci := s.ticket(t)
	rs := slices.Clone(changedRepositories(p.Key, t))
	inLandingOrder(p, rs, repositoryName)
	out := []ChangedRepositoryView{}
	for _, r := range rs {
		rv := ChangedRepositoryView{Name: r.Name, Branch: r.Branch, Commit: r.Commit, PushedCommit: latestPushedIn(p.Key, t, r.Name)}
		if r.Landed != nil {
			rv.Landed = &LandedView{Commit: r.Landed.Commit, Time: r.Landed.Time}
		}
		if rc := ci.in(r.Name); rc != nil {
			rv.CIResult, rv.CICommit, rv.LandAnyway = rc.result, rc.commit, s.landAnyway(rc)
		} else if landingPolicies(p).RunsCIWatcher() && !watchesCI(p, r.Name) {
			rv.CINotWatched = true
		}
		if r.Name == p.Key {
			if rv.Landed == nil {
				rv.Landed = landedByMove(t)
			}
		}
		rv.PullRequest = pullRequestView(pullRequestIn(p.Key, t, r.Name))
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
func latestPushedIn(home string, t domain.Ticket, name string) *PushedCommitView {
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
func landedByMove(t domain.Ticket) *LandedView {
	if t.State != domain.Done || len(t.Moves) == 0 {
		return nil
	}
	if m := t.Moves[len(t.Moves)-1]; m.Via == "merge" && m.Commit != "" {
		return &LandedView{Commit: m.Commit, Time: m.Time}
	}
	return nil
}

// specRepositories is the Changed Repositories of tickets, a Spec's, by
// Name in p's landing order: the union of theirs.
func specRepositories(p domain.Project, tickets []domain.Ticket) []string {
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

// ReportChangedRepositories is the Factory's report_changed_repositories:
// from the live Claim, while the Ticket is In Progress, it records the
// Repositories as the Ticket's live Changed Repositories, replacing the
// last report's. A report the same as the record writes nothing.
func (c *Core) ReportChangedRepositories(ctx context.Context, key string, in ReportChangedRepositoriesInput) (Written[TicketView], error) {
	return c.updateTicketIn(key, "report_changed_repositories", in.Ticket, in.Full, func(r store.Reader, t *domain.Ticket) (bool, error) {
		if _, err := holder(t, key, in.ClaimID); err != nil {
			return false, err
		}
		if t.State != domain.InProgress {
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
