package core

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// A Project's GitHub Issues settings (Spec code-central-279): the human
// turns GitHub Issues on and off with set_github_issues, which first runs
// the Issue Watcher's check mode, and creates the Project's Issues Spec
// the first time it is turned on. It watches a list of repositories
// (Spec code-central-848): the filter label and New issues need me apply
// to every one of them.

// GitHubIssuesView is a Project's GitHub Issues settings as every Project
// read returns them.
type GitHubIssuesView struct {
	On bool `json:"on"`
	// Repositories are the GitHub repositories (owner/name) its issues
	// come from, in the order the human listed them.
	Repositories []string `json:"repositories"`
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
	v := GitHubIssuesView{On: g.On, Repositories: slices.Clone(g.Repositories), Label: g.Label, NewIssuesNeedMe: g.NewIssuesNeedMe, Watcher: watcher}
	if v.Repositories == nil {
		v.Repositories = []string{}
	}
	if g.Spec != 0 {
		v.Spec = DisplayID(key, g.Spec)
	}
	return v
}

// SetGitHubIssuesInput is the human's set_github_issues(on, repositories,
// label, new_issues_need_me, full?).
type SetGitHubIssuesInput struct {
	On           bool     `json:"on"`
	Repositories []string `json:"repositories"`
	// Label is the filter label; empty lets every issue in.
	Label           string `json:"label"`
	NewIssuesNeedMe bool   `json:"new_issues_need_me"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetGitHubIssues is the human's set_github_issues: GitHub Issues on or
// off, with the repositories its issues come from, the filter label and
// New issues need me. Turning it on needs a repository, and each
// repository added passes the check. The first time it is turned on, it
// creates the Issues Spec, written before the settings so they never name
// a missing Spec. Changing the repositories retitles the Issues Spec
// unless the human has changed its title. Turning it off touches no Link.
func (c *Core) SetGitHubIssues(ctx context.Context, key string, in SetGitHubIssuesInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_github_issues"); err != nil {
		return Written[ProjectView]{}, err
	}
	g := store.GitHubIssues{On: in.On, Label: strings.TrimSpace(in.Label), NewIssuesNeedMe: in.NewIssuesNeedMe}
	for _, repository := range in.Repositories {
		repository = strings.TrimSpace(repository)
		switch {
		case repository == "":
			continue
		case !repositoryPattern.MatchString(repository):
			return Written[ProjectView]{}, invalid("repositories", "%q isn't a GitHub repository as owner/name, such as acme/web", repository)
		case slices.ContainsFunc(g.Repositories, func(r string) bool { return strings.EqualFold(r, repository) }):
			return Written[ProjectView]{}, invalid("repositories", "%s is listed twice: list each repository once", repository)
		}
		g.Repositories = append(g.Repositories, repository)
	}
	if len(g.Repositories) == 0 && g.On {
		return Written[ProjectView]{}, invalid("repositories", "GitHub Issues needs the GitHub repositories its issues come from, as owner/name")
	}
	if err := c.checkIssues(ctx, key, g); err != nil {
		return Written[ProjectView]{}, err
	}
	created := 0
	if g.On {
		var err error
		created, err = c.createIntegrationSpec(key, func(p store.Project) int { return p.GitHubIssues.Spec },
			store.Spec{Title: issuesSpecTitle(g.Repositories), Body: issuesSpecBody})
		if err != nil {
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
		if sameGitHubIssues(g, was) {
			return nil
		}
		// What the Issue Watcher said was about the old settings.
		if !g.On || !slices.Equal(g.Repositories, was.Repositories) {
			c.watchers.reset(watcherKey{WatcherIssues, key})
		}
		return w.PutProject(p)
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	if was.Spec != 0 && len(g.Repositories) > 0 && !slices.Equal(was.Repositories, g.Repositories) {
		if err := c.retitleIntegrationSpec(ctx, key, was.Spec, givenIssuesSpecTitle, issuesSpecTitle(g.Repositories)); err != nil {
			return Written[ProjectView]{}, err
		}
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// sameGitHubIssues reports whether a and b are the same settings.
func sameGitHubIssues(a, b store.GitHubIssues) bool {
	return a.On == b.On && slices.Equal(a.Repositories, b.Repositories) && a.Label == b.Label &&
		a.NewIssuesNeedMe == b.NewIssuesNeedMe && a.Spec == b.Spec
}

// createIntegrationSpec creates the Spec an Integration's Incoming Issues
// arrive on, the Issues Spec or the Azure Boards Spec: a Long-lived Spec
// with spec's title and body. specOf reads the Spec's number from the
// Project's settings; when it already has one, nothing is created. It
// returns the new Spec's number, else 0.
func (c *Core) createIntegrationSpec(key string, specOf func(store.Project) int, spec store.Spec) (int, error) {
	n := 0
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		if p, _ := w.Project(key); specOf(p) != 0 {
			return nil
		}
		now := c.now()
		n = w.NextNumber(key)
		spec.Number, spec.Labels, spec.Created, spec.Updated = n, []string{LongLivedLabel}, now, now
		return w.PutSpec(key, spec)
	})
	return n, err
}

// retitleIntegrationSpec retitles an Integration's Spec n to title,
// unless the human has changed its title: it still has a title Code
// Central gave it, as given reports, for settings which may since have
// been cleared.
func (c *Core) retitleIntegrationSpec(ctx context.Context, key string, n int, given func(string) bool, title string) error {
	return c.store.Update(func(w store.Writer) error {
		s, ok := w.Spec(key, n)
		if !ok || !given(s.Title) || s.Title == title {
			return nil
		}
		s.Updated = c.now()
		s.Edits = append(s.Edits, store.Edit{Number: len(s.Edits) + 1, Actor: string(ActorOf(ctx)), Time: s.Updated, Previous: map[string]string{"title": s.Title}})
		s.Title = title
		return w.PutSpec(key, s)
	})
}

// issuesSpecTitle is the Issues Spec's title: "GitHub Issues: acme/web"
// for one repository, and "GitHub Issues" once more than one is listed.
func issuesSpecTitle(repositories []string) string {
	if len(repositories) > 1 {
		return issuesSpecName
	}
	return issuesSpecName + ": " + strings.Join(repositories, "")
}

// issuesSpecName is what every Issues Spec's title starts with.
const issuesSpecName = "GitHub Issues"

// givenIssuesSpecTitle reports whether title is one Code Central gave an
// Issues Spec, for some repositories.
func givenIssuesSpecTitle(title string) bool {
	named, given := strings.CutPrefix(title, issuesSpecName+": ")
	return title == issuesSpecName || given && repositoryPattern.MatchString(named)
}

// issuesSpecBody is the Issues Spec's body. It names no repository, which
// the title carries, so changing the repositories leaves it as it is.
const issuesSpecBody = "This Spec holds the Project's Incoming Issues: each open GitHub issue arrives on it as a linked Draft Ticket, to triage with Keep as Draft, To Requirement or Won't fix. " +
	"GitHub Issues is in the GitHub Issues section of Project Options; turning it off leaves this Spec and its Tickets as they are.\n"

// IssuesCheckInput is what the Issue Watcher's check mode checks: that it
// can work each of the repositories' issues, finding gh with the gh path
// Option.
type IssuesCheckInput struct {
	Repositories []string `json:"repositories"`
	GHPath       string   `json:"gh_path,omitempty"`
}

// IssuesCheck runs the Issue Watcher once in its check mode. The service
// makes no external calls itself (ADR 0002, ADR 0004), so the check runs
// in the Issue Watcher's process.
type IssuesCheck func(ctx context.Context, in IssuesCheckInput) WatcherCheckResult

// SetIssuesCheck makes c run check when GitHub Issues is turned on. Call
// it before serving; without it, nothing is checked.
func (c *Core) SetIssuesCheck(check IssuesCheck) { c.issuesCheck = check }

// checkIssues runs the Issues check when g turns GitHub Issues on, or
// adds a repository while on, on each repository that is new, and refuses
// with the reason and fix when it fails: on the repositories when the
// problem is theirs, else on on. It runs outside the store's lock, as the
// check calls GitHub.
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
	if err != nil {
		return err
	}
	var added []string
	for _, repository := range g.Repositories {
		if !was.On || !slices.ContainsFunc(was.Repositories, func(r string) bool { return strings.EqualFold(r, repository) }) {
			added = append(added, repository)
		}
	}
	if len(added) == 0 {
		return nil
	}
	res := c.issuesCheck(ctx, IssuesCheckInput{Repositories: added, GHPath: opts.GHPath})
	if res.Problem == "" {
		return nil
	}
	field := "on"
	if len(res.Repositories) > 0 {
		field = "repositories"
	} else if len(added) == 1 {
		switch res.Problem {
		case ProblemRepositoryUnreadable, ProblemIssuesDisabled, ProblemNoTriage:
			field = "repositories"
		}
	}
	return &Error{Code: Invalid, Field: field, Message: res.Reason, Fix: res.Fix}
}
