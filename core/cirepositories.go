package core

import (
	"context"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// CI Tracking across Repositories (Spec code-central-848): CI Tracking
// stays one switch, and each Repository other than the Home has a CI
// entry, its CI Provider and what that watches, or Not watched. The Home
// Repository's entry is the CI settings' own Provider and target. A
// Ticket has a CI result per watched Repository it pushed in, combined
// into one; a Not watched Repository never gates, raises a CI item or
// counts towards that result.

// ciEntryOf is the CI entry of p's Repository named name, which isn't the
// Home: the one stored, or with none stored, one filled in from its
// Clone URL. ok is false when p has no such Repository.
func ciEntryOf(p store.Project, name string) (entry store.CIRepository, ok bool) {
	if i := slices.IndexFunc(p.CI.Repositories, func(e store.CIRepository) bool { return e.Name == name }); i >= 0 {
		return p.CI.Repositories[i], true
	}
	for _, r := range p.RepositoryList() {
		if r.Name == name && !r.Home {
			return ciEntryFromCloneURL(name, r.CloneURL), true
		}
	}
	return store.CIRepository{}, false
}

// ciEntryFromCloneURL is the CI entry a Repository's Clone URL fills in:
// GitHub Actions on a GitHub repository, Azure Pipelines on the Azure
// DevOps project of an Azure Repos one, or Not watched for any other.
func ciEntryFromCloneURL(name, url string) store.CIRepository {
	switch host, repo := codeHostOf(url); host {
	case CodeHostGitHub:
		return store.CIRepository{Name: name, Provider: store.CIGitHubActions, Repository: repo}
	case CodeHostAzureRepos:
		parts := strings.Split(repo, "/")
		return store.CIRepository{Name: name, Provider: store.CIAzurePipelines, ProjectURL: "https://dev.azure.com/" + parts[0] + "/" + parts[1]}
	}
	return store.CIRepository{Name: name, NotWatched: true}
}

// watchesCI reports whether CI Tracking watches p's Repository name: the
// Home Repository always, another while it is listed and its CI entry
// isn't Not watched.
func watchesCI(p store.Project, name string) bool {
	if name == p.Key {
		return true
	}
	entry, ok := ciEntryOf(p, name)
	return ok && !entry.NotWatched
}

// ciTarget is one watched Repository's CI: its Name and what its CI
// Provider watches.
type ciTarget struct {
	name     string
	provider store.CIProvider
	// repository is the GitHub repository, projectURL the Azure DevOps
	// project.
	repository, projectURL string
}

// ciTargets are the CI targets of p's watched Repositories other than
// the Home, in landing order.
func ciTargets(p store.Project) []ciTarget {
	var out []ciTarget
	for _, r := range p.RepositoryList() {
		if r.Home {
			continue
		}
		if e, _ := ciEntryOf(p, r.Name); !e.NotWatched {
			out = append(out, ciTarget{name: r.Name, provider: e.Provider, repository: e.Repository, projectURL: e.ProjectURL})
		}
	}
	return out
}

// recordName is the Repository Name name as records keep it: "" for the
// Home Repository, named key. recordRepository is its inverse.
func recordName(key, name string) string {
	if name == key {
		return ""
	}
	return name
}

// pushedIn are the commits t pushed in the Repository name, oldest first.
func pushedIn(key string, t store.Ticket, name string) []store.PushedCommit {
	var out []store.PushedCommit
	for _, pc := range t.PushedCommits {
		if recordRepository(key, pc.Repository) == name {
			out = append(out, pc)
		}
	}
	return out
}

// runsIn are the Ticket-tracked runs, in the Repository record names, of
// pushed, whichever Ticket they were tracked against: Tickets sharing a
// pushed commit share its runs. Runs tracked against a CI Rule are left
// out.
func runsIn(runs []store.CIRun, record string, pushed []store.PushedCommit) []store.CIRun {
	var mine []store.CIRun
	for _, run := range runs {
		if run.Ticket != 0 && run.Repository == record && slices.ContainsFunc(pushed, func(p store.PushedCommit) bool { return p.Commit == run.Commit }) {
			mine = append(mine, run)
		}
	}
	return mine
}

// landedIn reports whether t's work has landed in the Repository name
// while other Repositories still wait: a Partly landed Ticket's landed
// Repositories.
func landedIn(t store.Ticket, name string) bool {
	return slices.ContainsFunc(t.Repositories, func(r store.ChangedRepository) bool { return r.Name == name && r.Landed != nil })
}

// RepositoryCI is a Repository's CI entry as add_repository and
// set_repository_ci take it: Not watched, or a CI Provider, GitHub
// Actions when left out, with what it watches.
type RepositoryCI struct {
	NotWatched bool             `json:"not_watched,omitempty"`
	Provider   store.CIProvider `json:"provider,omitempty"`
	// Repository is the GitHub repository GitHub Actions watches, as
	// owner/name.
	Repository string `json:"repository,omitempty"`
	// ProjectURL is the Azure DevOps project Azure Pipelines watches, as
	// its URL.
	ProjectURL string `json:"project_url,omitempty"`
}

// SetRepositoryCIInput is the human's set_repository_ci(name,
// not_watched?, provider?, repository?, project_url?, full?).
type SetRepositoryCIInput struct {
	// Name is the Repository's, which can't be the Home Repository's: its
	// CI is the CI settings' own.
	Name string `json:"name"`
	RepositoryCI
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ciEntry is in as the CI entry of the Repository name, checked: a
// watched one needs a CI Provider and what it watches. field prefixes
// the field a refusal names.
func ciEntry(name string, in RepositoryCI, field string) (store.CIRepository, error) {
	e := store.CIRepository{Name: name, NotWatched: in.NotWatched, Provider: in.Provider,
		Repository: strings.TrimSpace(in.Repository), ProjectURL: strings.TrimSpace(in.ProjectURL)}
	if e.NotWatched && e.Provider == "" {
		return e, nil
	}
	if e.Provider == "" {
		e.Provider = store.CIGitHubActions
	}
	if !e.Provider.Valid() {
		return e, invalid(field+"provider", "CI Provider %q isn't %q or %q", e.Provider, store.CIGitHubActions, store.CIAzurePipelines)
	}
	ci := store.CI{Tracking: !e.NotWatched, Provider: e.Provider, Repository: e.Repository, ProjectURL: e.ProjectURL}
	if err := checkCITarget(&ci); err != nil {
		if ce, ok := err.(*Error); ok {
			ce.Field = field + ce.Field
		}
		return e, err
	}
	e.Repository, e.ProjectURL = ci.Repository, ci.ProjectURL
	return e, nil
}

// checkCIEntry runs the CI check for e, the CI entry of one of p's
// Repositories, when e is watched, and refuses with its reason and fix,
// naming e's Repository: on field+repository when gh can't read it, on
// field+project_url when the Azure DevOps project can't be read, else on
// field+provider. It runs outside the store's lock, as the check calls
// GitHub.
func (c *Core) checkCIEntry(ctx context.Context, p store.Project, opts store.Options, e store.CIRepository, field string) error {
	if e.NotWatched || c.ciCheck == nil {
		return nil
	}
	in := CICheckInput{Provider: e.Provider, Repository: e.Repository, GHPath: opts.GHPath}
	if e.Provider == store.CIAzurePipelines {
		in.ProjectURL, in.PATEnv = e.ProjectURL, patEnvOf(p.CI)
	}
	res := c.ciCheck(ctx, in)
	if res.Problem == "" {
		return nil
	}
	f := "provider"
	switch res.Problem {
	case ProblemRepositoryUnreadable:
		f = "repository"
	case ProblemProjectUnreadable:
		f = "project_url"
	}
	return &Error{Code: Invalid, Field: field + f, Message: e.Name + ": " + res.Reason, Fix: res.Fix}
}

// SetRepositoryCI is the human's set_repository_ci: the CI entry of a
// Repository other than the Home, its CI Provider and what it watches,
// or Not watched. While CI Tracking is on, a watched entry that watches
// something new must pass the CI Watcher's check first.
func (c *Core) SetRepositoryCI(ctx context.Context, key string, in SetRepositoryCIInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_repository_ci"); err != nil {
		return Written[ProjectView]{}, err
	}
	name := strings.TrimSpace(in.Name)
	e, err := ciEntry(name, in.RepositoryCI, "")
	if err != nil {
		return Written[ProjectView]{}, err
	}
	var p store.Project
	var opts store.Options
	err = c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ = r.Project(key)
		opts = r.Options()
		_, err := ciEntryNamed(p, name)
		return err
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	if was, _ := ciEntryOf(p, name); landingPolicyOf(p).RunsCIWatcher() && (was.NotWatched || !sameCITarget(was, e)) {
		if err := c.checkCIEntry(ctx, p, opts, e, ""); err != nil {
			return Written[ProjectView]{}, err
		}
	}
	return c.updateCIIn(key, in.Full, func(p store.Project, ci *store.CI) error {
		if _, err := ciEntryNamed(p, name); err != nil {
			return err
		}
		ci.Repositories = withCIEntry(ci.Repositories, e)
		return nil
	})
}

// ciEntryNamed is the CI entry of p's Repository name, refusing the Home
// Repository and a name p has no Repository by.
func ciEntryNamed(p store.Project, name string) (store.CIRepository, error) {
	if name == p.Key {
		return store.CIRepository{}, &Error{Code: Invalid, Field: "name", Message: name + " is the Home Repository, whose CI is the CI settings' own CI Provider and target",
			Fix: "change the Home Repository's CI with set_ci_tracking"}
	}
	e, ok := ciEntryOf(p, name)
	if !ok {
		return e, notARepository(p, "name", name)
	}
	return e, nil
}

// sameCITarget reports whether a and b, both watched, watch the same.
func sameCITarget(a, b store.CIRepository) bool {
	return a.Provider == b.Provider && a.Repository == b.Repository && a.ProjectURL == b.ProjectURL
}

// withCIEntry is es with e in place of the entry of its name, or added,
// on a copy of the list.
func withCIEntry(es []store.CIRepository, e store.CIRepository) []store.CIRepository {
	out := slices.Clone(es)
	if i := slices.IndexFunc(out, func(o store.CIRepository) bool { return o.Name == e.Name }); i >= 0 {
		out[i] = e
		return out
	}
	return append(out, e)
}
