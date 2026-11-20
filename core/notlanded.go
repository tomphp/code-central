package core

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Why an Accepted Ticket hasn't landed in each of its Changed
// Repositories (Spec code-central-848, ADR 0049): a Repository whose base
// is missing or diverged waits for it, and otherwise the Factory says why
// in a comment naming the Repository (api.CantLandComment), once per reason.
// Accepted but not landed names the Repositories that have landed and
// those that haven't with these reasons, and the Ticket's Repositories
// section shows them, while the Project has several Repositories.

// notLandedReason is why t, p's, hasn't landed in the Repository name:
// its base is missing or diverged; else what its work there waits on,
// from ci, p's CI state (waitingIn); else the first line of what the
// Factory last commented about it since t moved to its State; "" when
// there's none of these.
func (c *Core) notLandedReason(p domain.Project, t domain.Ticket, name string, ci *ciState) string {
	if why := c.brokenBase(p, name); why != "" {
		return why
	}
	if why := waitingIn(p, t, name, ci); why != "" {
		return why
	}
	lead, since := api.CantLandLead+name+" yet: ", entered(t)
	for i := len(t.Comments) - 1; i >= 0 && !t.Comments[i].Time.Before(since); i-- {
		if why, ok := strings.CutPrefix(t.Comments[i].Body, lead); ok && t.Comments[i].Actor != string(api.Human) {
			return strings.TrimSuffix(firstLine(why), ":")
		}
	}
	return ""
}

// waitingIn is what t's work in the Repository name, p's, waits on to
// land there: under Pull Request, its open pull request there to merge;
// under the CI gate there, once pushed there since t moved to its State,
// its CI result there, from ci, to pass. It is "" when it waits on
// neither, as in a Repository Not watched or a local-only one.
func waitingIn(p domain.Project, t domain.Ticket, name string, ci *ciState) string {
	policy := landingPolicyIn(p, name)
	if pr := pullRequestIn(p.Key, t, name); policy.LandsByPullRequest() && pr != nil && pr.State == domain.PROpen {
		return fmt.Sprintf("its pull request %d is open", pr.Number)
	}
	rc := ci.ticket(t).in(name)
	if !policy.CIGateHolds() || rc == nil || rc.latest.Pushed.Before(entered(t)) {
		return ""
	}
	switch rc.result {
	case api.CIPassed:
		return ""
	case api.CIFailed, api.CICancelled:
		return fmt.Sprintf("CI %s at %s", rc.result, shortCommit(rc.commit))
	}
	commit := rc.commit
	if commit == "" {
		commit = rc.latest.Commit
	}
	return "waiting on CI at " + shortCommit(commit)
}

// waitsOnCIAlone reports whether the CI gate holds t, p's, on a running
// CI result, from a, in each of its Changed Repositories it hasn't landed
// in: then its landing waits on CI alone. One stuck in a Repository
// meanwhile, as on a refused push, waits on more than CI.
func waitsOnCIAlone(p domain.Project, t domain.Ticket, a *ticketCI) bool {
	if !a.waiting {
		return false
	}
	return !slices.ContainsFunc(changedRepositories(p.Key, t), func(r domain.ChangedRepository) bool {
		rc := a.in(r.Name)
		return r.Landed == nil && (rc == nil || !rc.waiting)
	})
}

// notLandedView is t's NotLandedView, t being p's and Accepted but not
// landed, with ci p's CI state; nil while p has the Home Repository
// alone, or t has no Changed Repositories.
func (c *Core) notLandedView(p domain.Project, t domain.Ticket, ci *ciState) *api.NotLandedView {
	rs := changedRepositories(p.Key, t)
	if !severalRepositories(p) || len(rs) == 0 {
		return nil
	}
	rs = slices.Clone(rs)
	inLandingOrder(p, rs, repositoryName)
	v := &api.NotLandedView{Landed: []string{}, NotLanded: []api.NotLandedRepositoryView{}}
	for _, r := range rs {
		if r.Landed != nil {
			v.Landed = append(v.Landed, r.Name)
		} else {
			v.NotLanded = append(v.NotLanded, api.NotLandedRepositoryView{Name: r.Name, Reason: c.notLandedReason(p, t, r.Name, ci)})
		}
	}
	return v
}
