package core

import (
	"cmp"
	"context"
	"strings"

	"github.com/tomoram/code-central/store"
)

// A Project's GitHub Issues settings (Spec code-central-279): the human
// turns GitHub Issues on and off with set_github_issues, which first runs
// the Issue Watcher's check mode, and creates the Project's Issues Spec
// the first time it is turned on.

// GitHubIssuesView is a Project's GitHub Issues settings as every Project
// read returns them.
type GitHubIssuesView struct {
	On         bool   `json:"on"`
	Repository string `json:"repository"`
	// Label is the filter label: only issues carrying it come in; empty
	// lets every issue in.
	Label           string `json:"label"`
	NewIssuesNeedMe bool   `json:"new_issues_need_me"`
	// Spec is the Issues Spec's ID, left out until GitHub Issues is first
	// turned on.
	Spec string `json:"spec,omitempty"`
	// Watcher is the Issue Watcher's status, as it last reported it.
	Watcher WatcherView `json:"watcher"`
}

func githubIssuesView(key string, g store.GitHubIssues, watcher WatcherView) GitHubIssuesView {
	v := GitHubIssuesView{On: g.On, Repository: g.Repository, Label: g.Label, NewIssuesNeedMe: g.NewIssuesNeedMe, Watcher: watcher}
	if g.Spec != 0 {
		v.Spec = DisplayID(key, g.Spec)
	}
	return v
}

// SetGitHubIssuesInput is the human's set_github_issues(on, repository,
// label, new_issues_need_me, full?).
type SetGitHubIssuesInput struct {
	On         bool   `json:"on"`
	Repository string `json:"repository"`
	// Label is the filter label; empty lets every issue in.
	Label           string `json:"label"`
	NewIssuesNeedMe bool   `json:"new_issues_need_me"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetGitHubIssues is the human's set_github_issues: GitHub Issues on or
// off, with the repository its issues come from, the filter label and New
// issues need me. Turning it on needs a repository and a passing check.
// The first time it is turned on, it creates the Issues Spec, written
// before the settings so they never name a missing Spec. Changing the
// repository retitles the Issues Spec unless the human has changed its
// title. Turning it off touches no Link.
func (c *Core) SetGitHubIssues(ctx context.Context, key string, in SetGitHubIssuesInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_github_issues"); err != nil {
		return Written[ProjectView]{}, err
	}
	g := store.GitHubIssues{On: in.On, Repository: strings.TrimSpace(in.Repository), Label: strings.TrimSpace(in.Label), NewIssuesNeedMe: in.NewIssuesNeedMe}
	switch {
	case g.Repository == "" && g.On:
		return Written[ProjectView]{}, invalid("repository", "GitHub Issues needs the GitHub repository its issues come from, as owner/name")
	case g.Repository != "" && !repositoryPattern.MatchString(g.Repository):
		return Written[ProjectView]{}, invalid("repository", "%q isn't a GitHub repository as owner/name, such as acme/web", g.Repository)
	}
	if err := c.checkIssues(ctx, key, g); err != nil {
		return Written[ProjectView]{}, err
	}
	created := 0
	if g.On {
		var err error
		if created, err = c.createIssuesSpec(key, g.Repository); err != nil {
			return Written[ProjectView]{}, err
		}
	}
	var was store.GitHubIssues
	var before, after ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		before, was = c.projectView(p), p.GitHubIssues
		g.Spec = cmp.Or(was.Spec, created)
		p.GitHubIssues = g
		after = c.projectView(p)
		if g == was {
			return nil
		}
		// What the Issue Watcher said was about the old settings.
		if !g.On || g.Repository != was.Repository {
			c.watchers.reset(watcherKey{WatcherIssues, key})
		}
		return w.PutProject(p)
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	if was.Spec != 0 && was.Repository != g.Repository && g.Repository != "" {
		if err := c.retitleIssuesSpec(ctx, key, was.Spec, g.Repository); err != nil {
			return Written[ProjectView]{}, err
		}
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// createIssuesSpec creates key's Issues Spec for repository unless it has
// one, and returns its number; 0 when it already had one.
func (c *Core) createIssuesSpec(key, repository string) (int, error) {
	n := 0
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		if p, _ := w.Project(key); p.GitHubIssues.Spec != 0 {
			return nil
		}
		now := c.now()
		n = w.NextNumber(key)
		return w.PutSpec(key, store.Spec{Number: n, Title: issuesSpecTitle(repository), Body: issuesSpecBody, Created: now, Updated: now})
	})
	return n, err
}

// retitleIssuesSpec retitles Issues Spec n after repository, unless the
// human has changed its title: it still has the title Code Central gave
// it for some repository, which may since have been cleared.
func (c *Core) retitleIssuesSpec(ctx context.Context, key string, n int, repository string) error {
	return c.store.Update(func(w store.Writer) error {
		s, ok := w.Spec(key, n)
		named, given := strings.CutPrefix(s.Title, issuesSpecTitle(""))
		if !ok || !given || !repositoryPattern.MatchString(named) || s.Title == issuesSpecTitle(repository) {
			return nil
		}
		s.Updated = c.now()
		s.Edits = append(s.Edits, store.Edit{Number: len(s.Edits) + 1, Actor: string(ActorOf(ctx)), Time: s.Updated, Previous: map[string]string{"title": s.Title}})
		s.Title = issuesSpecTitle(repository)
		return w.PutSpec(key, s)
	})
}

// issuesSpecTitle is the Issues Spec's title: "GitHub Issues: acme/web".
func issuesSpecTitle(repository string) string { return "GitHub Issues: " + repository }

// issuesSpecBody is the Issues Spec's body. It names no repository, which
// the title carries, so changing the repository leaves it as it is.
const issuesSpecBody = "This Spec holds the Project's Incoming Issues: each open GitHub issue arrives on it as a linked Draft Ticket, to triage with Keep as Draft, To Requirement or Won't fix. " +
	"GitHub Issues is in the GitHub Issues section of Project Options; turning it off leaves this Spec and its Tickets as they are.\n"

// IssuesCheckInput is what the Issue Watcher's check mode checks: that it
// can work the repository's issues, finding gh with the gh path Option.
type IssuesCheckInput struct {
	Repository string `json:"repository"`
	GHPath     string `json:"gh_path,omitempty"`
}

// IssuesCheck runs the Issue Watcher once in its check mode. The service
// makes no external calls itself (ADR 0002, ADR 0004), so the check runs
// in the Issue Watcher's process.
type IssuesCheck func(ctx context.Context, in IssuesCheckInput) WatcherCheckResult

// SetIssuesCheck makes c run check when GitHub Issues is turned on. Call
// it before serving; without it, nothing is checked.
func (c *Core) SetIssuesCheck(check IssuesCheck) { c.issuesCheck = check }

// checkIssues runs the Issues check when g turns GitHub Issues on, or
// changes its repository while on, and refuses with the reason and fix
// when it fails: on the repository when the problem is the repository's,
// else on on. It runs outside the store's lock, as the check calls GitHub.
func (c *Core) checkIssues(ctx context.Context, key string, g store.GitHubIssues) error {
	if !g.On || c.issuesCheck == nil {
		return nil
	}
	var was store.GitHubIssues
	var opts store.Options
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		was, opts = p.GitHubIssues, r.Options()
		return nil
	})
	if err != nil || (was.On && was.Repository == g.Repository) {
		return err
	}
	res := c.issuesCheck(ctx, IssuesCheckInput{Repository: g.Repository, GHPath: opts.GHPath})
	if res.Problem == "" {
		return nil
	}
	field := "on"
	switch res.Problem {
	case ProblemRepositoryUnreadable, ProblemIssuesDisabled, ProblemNoTriage:
		field = "repository"
	}
	return &Error{Code: Invalid, Field: field, Message: res.Reason, Fix: res.Fix}
}
