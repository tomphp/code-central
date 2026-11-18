package core

import (
	"context"
	neturl "net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/tomoram/code-central/azdourl"
	"github.com/tomoram/code-central/codehost"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// A Project's CI settings (Spec code-central-247): the human turns CI
// Tracking on and off with set_ci_tracking, and the Factory reports its
// Push remote's URL with report_push_remote, so the UI can prefill the
// GitHub repository, or from an Azure Repos Push remote the Azure DevOps
// project URL.

// CIView is a Project's CI settings as every Project read returns them.
type CIView struct {
	Tracking bool `json:"tracking"`
	// Provider, Repository and ProjectURL are the Home Repository's CI
	// entry as stored, for an older client, which set it here: GitHub
	// Actions naming nothing while none is. Its Repository view's CI is
	// its CI choice, filled in from its Clone URL while none is stored.
	Provider domain.CIProvider `json:"provider"`
	// Repository is the GitHub repository GitHub Actions watches.
	Repository string `json:"repository"`
	// ProjectURL is the Azure DevOps project Azure Pipelines watches.
	ProjectURL string `json:"project_url"`
	// PATEnv is the name of the environment variable holding the Azure
	// DevOps PAT, ADO_PAT unless the Project names another. The PAT itself
	// is never kept.
	PATEnv string `json:"pat_env"`
	// IgnoredWorkflows are workflow names, such as the workflow file
	// ci.yml or a pipeline's name, whose runs never count towards a CI
	// result.
	IgnoredWorkflows []string `json:"ignored_workflows"`
	// PushRemoteURL is the URL of the Push remote the Factory last
	// reported, empty until it has.
	PushRemoteURL string `json:"push_remote_url"`
	// Rules are the CI Rules, in the order they were added.
	Rules []CIRuleView `json:"rules"`
	// Watcher is the CI Watcher's status, as it last reported it.
	Watcher WatcherView `json:"watcher"`
	// Repositories are the CI entries of the Repositories other than the
	// Home, in landing order; Provider, Repository and ProjectURL are the
	// Home Repository's, and the PAT environment variable, Ignored
	// workflows and CI Rules are every Repository's. Left out while the
	// Project has the Home Repository alone.
	Repositories []CIRepositoryView `json:"repositories,omitempty"`
}

// CIRuleView is one CI Rule: when Workflow fails on Branch, open a Ticket
// to fix it, in State, on the rule's own Spec, whose ID names the rule.
type CIRuleView struct {
	Spec string `json:"spec"`
	// Workflow is a workflow file's base name or a pipeline's name, or any.
	Workflow string       `json:"workflow"`
	Branch   string       `json:"branch"`
	State    domain.State `json:"state"`
	Created  time.Time    `json:"created"`
}

// CIRepositoryView is the CI entry of one Repository: its CI Provider and
// what that watches, or Not watched.
type CIRepositoryView struct {
	Name       string            `json:"name"`
	NotWatched bool              `json:"not_watched"`
	Provider   domain.CIProvider `json:"provider,omitempty"`
	Repository string            `json:"repository,omitempty"`
	ProjectURL string            `json:"project_url,omitempty"`
}

func ciView(p domain.Project, watcher WatcherView) CIView {
	key, c := p.Key, p.CI
	v := CIView{Tracking: c.Tracking, PATEnv: patEnvOf(c),
		IgnoredWorkflows: orEmpty(c.IgnoredWorkflows), PushRemoteURL: c.PushRemoteURL, Rules: []CIRuleView{}, Watcher: watcher}
	for _, r := range c.Rules {
		v.Rules = append(v.Rules, CIRuleView{Spec: DisplayID(key, r.Spec), Workflow: r.Workflow, Branch: r.Branch, State: r.State, Created: r.Created})
	}
	home, _ := storedCIEntry(p, key)
	v.Provider, v.Repository, v.ProjectURL = providerOrDefault(home.Provider), home.Repository, home.ProjectURL
	for _, r := range p.RepositoryList() {
		if !r.Home {
			v.Repositories = append(v.Repositories, ciEntryView(ciEntryOfRepository(p, r)))
		}
	}
	return v
}

// providerOrDefault is p, or GitHub Actions for none.
func providerOrDefault(p domain.CIProvider) domain.CIProvider {
	if p == "" {
		return domain.CIGitHubActions
	}
	return p
}

// patEnvOf is c's PAT environment variable, ADO_PAT when it names none.
func patEnvOf(c domain.CI) string {
	if c.PATEnv == "" {
		return domain.DefaultPATEnv
	}
	return c.PATEnv
}

// SetCITrackingInput is the human's set_ci_tracking(tracking, pat_env,
// ignored_workflows, full?), and an older client's provider, repository
// and project_url.
type SetCITrackingInput struct {
	Tracking bool `json:"tracking"`
	// Provider, Repository and ProjectURL are the Home Repository's CI
	// entry as an older client sets it here (Spec code-central-890), as
	// set_repository_ci on the Home would, when they name what to watch:
	// Repository for GitHub Actions, ProjectURL for Azure Pipelines.
	// Otherwise they are ignored, and the Home's entry kept. Provider is
	// github_actions when left out.
	Provider   domain.CIProvider `json:"provider,omitempty"`
	Repository string            `json:"repository,omitempty"`
	// ProjectURL is the Azure DevOps project, as its URL.
	ProjectURL string `json:"project_url,omitempty"`
	// PATEnv names the environment variable holding the Azure DevOps PAT;
	// ADO_PAT when left out.
	PATEnv           string   `json:"pat_env"`
	IgnoredWorkflows []string `json:"ignored_workflows"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// patEnvPattern is an environment variable's name.
var patEnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// checkPATEnv refuses a PAT environment variable that isn't a name, on
// pat_env; empty, which reads as the default, passes.
func checkPATEnv(name string) error {
	if name != "" && !patEnvPattern.MatchString(name) {
		return invalid("pat_env", "%q isn't an environment variable name: a letter or underscore, then letters, digits and underscores, such as %s", name, domain.DefaultPATEnv)
	}
	return nil
}

// checkWorkflow reports whether name is a workflow name for any of
// providers, with an example of one, for the first, for messages. GitHub
// Actions names workflow files. A pipeline's name is any non-empty name
// without a folder separator or a glob, spaces allowed.
func checkWorkflow(providers []domain.CIProvider, name string) (example string, ok bool) {
	for i, provider := range providers {
		e, ok := "a workflow file name, such as ci.yml", codehost.ValidGitHubWorkflow(name)
		if provider == domain.CIAzurePipelines {
			e, ok = "a pipeline's name, such as Build", name != "" && !strings.ContainsAny(name, `/\*?[]`)
		}
		if i == 0 {
			example = e
		}
		if ok {
			return example, true
		}
	}
	return example, false
}

// SetCITracking is the human's set_ci_tracking: CI Tracking on or off,
// with the PAT environment variable and the Ignored workflows, and from
// an older client the Home Repository's CI entry. Turning it on checks
// each watched Repository's CI entry. The Push remote URL the Factory
// reported, the CI Rules and the other CI entries are kept.
func (c *Core) SetCITracking(ctx context.Context, key string, in SetCITrackingInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_ci_tracking"); err != nil {
		return Written[ProjectView]{}, err
	}
	home, err := olderClientsHomeCI(key, in)
	if err != nil {
		return Written[ProjectView]{}, err
	}
	patEnv := strings.TrimSpace(in.PATEnv)
	if err := checkPATEnv(patEnv); err != nil {
		return Written[ProjectView]{}, err
	}
	// change is the write, on p's CI settings ci.
	change := func(p domain.Project, ci *domain.CI) error {
		ci.Tracking, ci.PATEnv, ci.IgnoredWorkflows = in.Tracking, patEnv, nil
		if home != nil && !keepsHomeCI(p, *home) {
			ci.Repositories = withCIEntry(ci.Repositories, *home)
		}
		next := p
		next.CI = *ci
		for _, w := range in.IgnoredWorkflows {
			w = strings.TrimSpace(w)
			switch example, ok := checkWorkflow(ciProviders(next), w); {
			case w == "" || slices.Contains(ci.IgnoredWorkflows, w):
			case !ok:
				return invalid("ignored_workflows", "%q isn't %s", w, example)
			default:
				ci.IgnoredWorkflows = append(ci.IgnoredWorkflows, w)
			}
		}
		return nil
	}
	var p domain.Project
	var opts domain.Options
	err = c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ = r.Project(key)
		opts = r.Options()
		return nil
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	next := p
	if err := change(p, &next.CI); err != nil {
		return Written[ProjectView]{}, err
	}
	if err := c.checkCI(ctx, p, next, opts); err != nil {
		return Written[ProjectView]{}, err
	}
	return c.updateCIResetting(key, in.Full, func(p domain.Project, ci *domain.CI) (bool, error) {
		next := p
		next.CI = *ci
		if err := change(p, &next.CI); err != nil {
			return false, err
		}
		*ci = next.CI
		// What the CI Watcher said was about the old settings.
		return !next.CI.Tracking || watchesAnother(p, next), nil
	})
}

// olderClientsHomeCI is the Home Repository's CI entry, key's, as an
// older client's set_ci_tracking names it, checked as it was before the
// Home had a CI entry: what to watch is needed only to turn CI Tracking
// on. It is nil when in names no provider, repository or project URL.
func olderClientsHomeCI(key string, in SetCITrackingInput) (*domain.CIRepository, error) {
	e := domain.CIRepository{Name: key, Provider: in.Provider, Repository: strings.TrimSpace(in.Repository), ProjectURL: strings.TrimSpace(in.ProjectURL)}
	if e.Provider == "" && e.Repository == "" && e.ProjectURL == "" {
		return nil, nil
	}
	e.Provider = providerOrDefault(e.Provider)
	if !e.Provider.Valid() {
		return nil, invalid("provider", "CI Provider %q isn't %q or %q", e.Provider, domain.CIGitHubActions, domain.CIAzurePipelines)
	}
	checked := e
	checked.NotWatched = !in.Tracking
	if err := checkCITarget(&checked); err != nil {
		return nil, err
	}
	e.ProjectURL = checked.ProjectURL
	return &e, nil
}

// keepsHomeCI reports whether e, an older client's Home Repository entry
// naming nothing to watch on GitHub Actions, the default it sends, leaves
// the Home's entry as p stores it: none, so filled in from its Clone URL,
// or Not watched, a choice an older client can't show.
func keepsHomeCI(p domain.Project, e domain.CIRepository) bool {
	if e != (domain.CIRepository{Name: e.Name, Provider: domain.CIGitHubActions}) {
		return false
	}
	stored, ok := storedCIEntry(p, e.Name)
	return !ok || stored.NotWatched
}

// checkCITarget validates what e, a CI entry, watches: the repository
// for GitHub Actions, the project URL for Azure Pipelines, each needed
// unless e is Not watched. Each provider's other field is kept as given,
// unchecked, until that provider is chosen. It replaces the project URL
// with the one it parses to.
func checkCITarget(e *domain.CIRepository) error {
	switch e.Provider {
	case domain.CIAzurePipelines:
		if e.ProjectURL == "" && !e.NotWatched {
			return invalid("project_url", "CI Tracking needs the Azure DevOps project to watch, as its URL")
		}
		if e.ProjectURL != "" {
			u, err := azdourl.ParseProjectURL(e.ProjectURL)
			if err != nil {
				return invalid("project_url", "%v", err)
			}
			e.ProjectURL = u
		}
	default:
		switch {
		case e.Repository == "" && !e.NotWatched:
			return invalid("repository", "CI Tracking needs the GitHub repository to watch, as owner/name")
		case e.Repository != "" && !codehost.ValidGitHubRepository(e.Repository):
			return invalid("repository", "%q isn't a GitHub repository as owner/name, such as acme/web", e.Repository)
		}
	}
	return nil
}

// watchesAnother reports whether next, p with other CI settings, watches
// something p didn't: another CI target in a Repository, or another PAT
// environment variable.
func watchesAnother(p, next domain.Project) bool {
	return patEnvOf(p.CI) != patEnvOf(next.CI) || !slices.Equal(ciTargets(p), ciTargets(next))
}

// CICheckInput is what the CI Watcher's check mode checks: that it can
// watch what the Project names through its CI Provider: the repository
// through gh, found with the gh path Option, or the Azure DevOps project
// through its API with the PAT the environment variable PATEnv names.
type CICheckInput struct {
	Provider   domain.CIProvider `json:"provider"`
	Repository string            `json:"repository"`
	ProjectURL string            `json:"project_url,omitempty"`
	PATEnv     string            `json:"pat_env,omitempty"`
	GHPath     string            `json:"gh_path,omitempty"`
}

// CICheck runs the CI Watcher once in its check mode. The service makes
// no external calls itself (ADR 0002, ADR 0004), so the check runs in the
// CI Watcher's process.
type CICheck func(ctx context.Context, in CICheckInput) WatcherCheckResult

// SetCICheck makes c run check when CI Tracking is turned on. Call it
// before serving; without it, nothing is checked.
func (c *Core) SetCICheck(check CICheck) { c.ciCheck = check }

// checkCI runs the CI check for the watched Repositories' CI entries in
// next, p with other CI settings, while next has CI Tracking on: each one
// when it turns CI Tracking on, else each that watches something new, or
// on Azure Pipelines with a new PAT environment variable. It refuses with
// the check's reason and fix: on repository when gh can't read it, on
// project_url when the Azure DevOps project can't be read, else on
// tracking for the Home Repository, as before it had a CI entry, and on
// provider for another, which is named, under repositories.
// It runs outside the store's lock, as the check calls GitHub.
func (c *Core) checkCI(ctx context.Context, p, next domain.Project, opts domain.Options) error {
	if !next.CI.Tracking {
		return nil
	}
	was := map[string]ciTarget{}
	if p.CI.Tracking {
		for _, t := range ciTargets(p) {
			was[t.name] = t
		}
	}
	newPAT := patEnvOf(p.CI) != patEnvOf(next.CI)
	for _, t := range ciTargets(next) {
		if w, ok := was[t.name]; ok && w == t && (!newPAT || t.provider != domain.CIAzurePipelines) {
			continue
		}
		e := domain.CIRepository{Name: t.name, Provider: t.provider, Repository: t.repository, ProjectURL: t.projectURL}
		check := func() error { return c.checkCIEntry(ctx, next, opts, e, "repositories.") }
		if t.name == p.Key {
			check = func() error { return c.checkCIEntryOn(ctx, next, opts, e, "", "tracking", "") }
		}
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// ReportPushRemoteInput is the Factory's report_push_remote(url, full?).
type ReportPushRemoteInput struct {
	// URL is the Home Repository's Push remote's URL, empty when its
	// clone has no Push remote.
	URL string `json:"url"`
	// Base is ignored: the base is a Project setting, and an older Factory
	// still sends it.
	Base string `json:"base,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportPushRemote is the Factory's report_push_remote: the URL of the
// Home Repository's Push remote, which it reports when it starts, or
// none (Spec code-central-890). None clears the Home's Clone URL and
// makes it local-only; a Home that wasn't local-only starts Not watched.
// A URL makes it pushed again, and leaves its CI entry as it was.
func (c *Core) ReportPushRemote(_ context.Context, key string, in ReportPushRemoteInput) (Written[ProjectView], error) {
	url := strings.TrimSpace(in.URL)
	return c.updateCIResetting(key, in.Full, func(_ domain.Project, p *domain.CI) (bool, error) {
		// What the CI Watcher said may have been about the Home.
		reset := url == "" && !p.NoPushRemote
		if reset {
			p.Repositories = withCIEntry(p.Repositories, domain.CIRepository{Name: key, NotWatched: true})
		}
		p.PushRemoteURL, p.NoPushRemote = withoutLogin(url), url == ""
		return reset, nil
	})
}

// withoutLogin is an HTTP(S) remote URL without the login it may carry,
// such as a token, which Code Central never keeps. An SSH URL's user
// (git@) is no secret, and stays.
func withoutLogin(url string) string {
	u, err := neturl.Parse(url)
	if err != nil || u.User == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return url
	}
	u.User = nil
	return u.String()
}

// updateCI changes a Project's CI settings with change, writing nothing
// when they come out the same.
func (c *Core) updateCI(key string, full bool, change func(*domain.CI) error) (Written[ProjectView], error) {
	return c.updateCIIn(key, full, func(_ domain.Project, ci *domain.CI) error { return change(ci) })
}

// updateCIIn is updateCI with the Project as the write reads it.
func (c *Core) updateCIIn(key string, full bool, change func(domain.Project, *domain.CI) error) (Written[ProjectView], error) {
	return c.updateCIResetting(key, full, func(p domain.Project, ci *domain.CI) (bool, error) { return false, change(p, ci) })
}

// updateCIResetting is updateCIIn whose change also says whether what the
// CI Watcher said was about the old settings: its status is then reset
// once the write commits, as the result shows.
func (c *Core) updateCIResetting(key string, full bool, change func(domain.Project, *domain.CI) (bool, error)) (Written[ProjectView], error) {
	var before ProjectView
	var p domain.Project
	var reset bool
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ = w.Project(key)
		before = c.projectView(p)
		was, ci := p.CI, p.CI
		ci.IgnoredWorkflows = slices.Clone(ci.IgnoredWorkflows)
		ci.Rules = slices.Clone(ci.Rules)
		ci.Repositories = slices.Clone(ci.Repositories)
		var err error
		if reset, err = change(p, &ci); err != nil {
			return err
		}
		p.CI = ci
		if ci.Equal(was) {
			return nil
		}
		return w.PutProject(p)
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	if reset {
		c.watchers.reset(watcherKey{WatcherCI, key})
	}
	after := c.projectView(p)
	// A CI write never changes the Landing or the Repositories; only what
	// is worked out from the Push remote URL, the Code Host and the Home
	// Repository's Clone URL and Base, can differ, and that isn't the
	// write's change.
	before.Landing, before.Repositories = after.Landing, after.Repositories
	return acknowledgeProject(before, after, full, c.now())
}

// CIRuleInput is the human's add_ci_rule(workflow, branch, state, full?)
// and, with the rule's Spec, edit_ci_rule.
type CIRuleInput struct {
	// Spec names the rule to edit; add_ci_rule leaves it out.
	Spec Ref `json:"spec,omitempty"`
	// Workflow is a workflow file's base name, such as ci.yml, or any.
	Workflow string `json:"workflow"`
	// Branch is an exact branch name, with no globs.
	Branch string `json:"branch"`
	// State is Draft or Ready: Draft when add_ci_rule leaves it out, and
	// the rule's own when edit_ci_rule does.
	State domain.State `json:"state,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// RemoveCIRuleInput is the human's remove_ci_rule(spec, full?).
type RemoveCIRuleInput struct {
	Spec Ref `json:"spec"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// globChars are what would make a branch a pattern: a CI Rule's branch
// is an exact name.
const globChars = "*?[]"

// AddCIRule is the human's add_ci_rule: "when this workflow fails on
// this branch, open a Ticket to fix it". It creates the rule's Spec, on
// which its Tickets will open, titled like "CI: ci.yml on main". An
// update writes one record, so the Spec is written first and the rule
// second: a rule never names a missing Spec.
func (c *Core) AddCIRule(ctx context.Context, key string, in CIRuleInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "add_ci_rule"); err != nil {
		return Written[ProjectView]{}, err
	}
	if in.State == "" {
		in.State = domain.Draft
	}
	var r domain.CIRule
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		var err error
		if r, err = checkCIRule(ciProviders(p), in); err != nil {
			return err
		}
		if err := ruleIsNew(key, p.CI.Rules, r, 0); err != nil {
			return err
		}
		r.Created = c.now()
		r.Spec = w.NextNumber(key)
		return w.PutSpec(key, domain.Spec{Number: r.Spec, Title: ciRuleTitle(r), Body: ciRuleSpecBody, Labels: []string{LongLivedLabel}, Created: r.Created, Updated: r.Created})
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	return c.updateCI(key, in.Full, func(ci *domain.CI) error {
		if err := ruleIsNew(key, ci.Rules, r, 0); err != nil {
			return err
		}
		ci.Rules = append(ci.Rules, r)
		return nil
	})
}

// EditCIRule is the human's edit_ci_rule: the rule keeps its Spec and
// when it was created. The Spec's title follows the rule unless the human
// has changed it, in a second write after the rule's.
func (c *Core) EditCIRule(ctx context.Context, key string, in CIRuleInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "edit_ci_rule"); err != nil {
		return Written[ProjectView]{}, err
	}
	n, err := in.Spec.number(key, "spec")
	if err != nil {
		return Written[ProjectView]{}, err
	}
	var old, r domain.CIRule
	v, err := c.updateCIIn(key, in.Full, func(p domain.Project, ci *domain.CI) error {
		i, err := ruleOn(key, ci.Rules, n)
		if err != nil {
			return err
		}
		old = ci.Rules[i]
		// A copy, so a retry still sees whether in named a State.
		edit := in
		if edit.State == "" {
			edit.State = old.State
		}
		if r, err = checkCIRule(ciProviders(p), edit); err != nil {
			return err
		}
		if err := ruleIsNew(key, ci.Rules, r, n); err != nil {
			return err
		}
		r.Spec, r.Created = old.Spec, old.Created
		ci.Rules[i] = r
		return nil
	})
	if err != nil || old.Equal(r) {
		return v, err
	}
	return v, c.followRule(ctx, key, old, r)
}

// RemoveCIRule is the human's remove_ci_rule. The rule's Spec stays, as
// the record of its Tickets.
func (c *Core) RemoveCIRule(ctx context.Context, key string, in RemoveCIRuleInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "remove_ci_rule"); err != nil {
		return Written[ProjectView]{}, err
	}
	n, err := in.Spec.number(key, "spec")
	if err != nil {
		return Written[ProjectView]{}, err
	}
	return c.updateCI(key, in.Full, func(ci *domain.CI) error {
		i, err := ruleOn(key, ci.Rules, n)
		if err != nil {
			return err
		}
		ci.Rules = slices.Delete(ci.Rules, i, i+1)
		return nil
	})
}

// checkCIRule is in's rule for a Project on providers, trimmed, or why it
// isn't one.
func checkCIRule(providers []domain.CIProvider, in CIRuleInput) (domain.CIRule, error) {
	r := domain.CIRule{Workflow: strings.TrimSpace(in.Workflow), Branch: strings.TrimSpace(in.Branch), State: in.State}
	if example, ok := checkWorkflow(providers, r.Workflow); r.Workflow != domain.AnyWorkflow && !ok {
		return r, invalid("workflow", "%q isn't %s, or %s for any workflow", r.Workflow, example, domain.AnyWorkflow)
	}
	if r.Branch == "" || strings.ContainsAny(r.Branch, globChars) || strings.IndexFunc(r.Branch, unicode.IsSpace) >= 0 {
		return r, invalid("branch", "%q isn't an exact branch name, such as main: a CI Rule's branch takes no globs", r.Branch)
	}
	if r.State != domain.Draft && r.State != domain.Ready {
		return r, invalid("state", "a CI Rule's Tickets open as %s or %s, not %q", domain.Draft, domain.Ready, r.State)
	}
	return r, nil
}

// ruleOn is the index of the rule on Spec n.
func ruleOn(key string, rules []domain.CIRule, n int) (int, error) {
	i := slices.IndexFunc(rules, func(r domain.CIRule) bool { return r.Spec == n })
	if i < 0 {
		return 0, notFound("there is no CI Rule on Spec %s", DisplayID(key, n))
	}
	return i, nil
}

// ruleIsNew refuses r when another rule, not the one on Spec self,
// already watches its workflow on its branch.
func ruleIsNew(key string, rules []domain.CIRule, r domain.CIRule, self int) error {
	for _, o := range rules {
		if o.Spec != self && o.Workflow == r.Workflow && o.Branch == r.Branch {
			return invalid("workflow", "the CI Rule on Spec %s already watches %s", DisplayID(key, o.Spec), ciRuleWatches(r))
		}
	}
	return nil
}

// followRule retitles the Spec of the rule edited from old to r, unless
// the human has changed its title.
func (c *Core) followRule(ctx context.Context, key string, old, r domain.CIRule) error {
	if ciRuleTitle(old) == ciRuleTitle(r) {
		return nil
	}
	return c.store.Update(func(w store.Writer) error {
		s, ok := w.Spec(key, old.Spec)
		if !ok || s.Title != ciRuleTitle(old) {
			return nil
		}
		s.Updated = c.now()
		s.Edits = append(s.Edits, domain.Edit{Number: len(s.Edits) + 1, Actor: string(ActorOf(ctx)), Time: s.Updated, Previous: map[string]string{"title": s.Title}})
		s.Title = ciRuleTitle(r)
		return w.PutSpec(key, s)
	})
}

// ciRuleWatches is what a rule watches: "ci.yml on main" or "any workflow
// on main".
func ciRuleWatches(r domain.CIRule) string {
	w := r.Workflow
	if w == domain.AnyWorkflow {
		w = "any workflow"
	}
	return w + " on " + r.Branch
}

// ciRuleTitle is the title of a rule's Spec: "CI: ci.yml on main".
func ciRuleTitle(r domain.CIRule) string { return "CI: " + ciRuleWatches(r) }

// ciRuleSpecBody is a rule's Spec's body. It names neither workflow nor
// branch, which the title carries, so an edit leaves it as it is.
const ciRuleSpecBody = "This Spec holds the fix Tickets of a CI Rule, which opens a Ticket to fix its workflow when it fails on its branch. " +
	"The rule is in the CI section of Project Options; removing it leaves this Spec as the record of its Tickets.\n"
