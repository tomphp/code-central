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
	"github.com/tomoram/code-central/store"
)

// A Project's CI settings (Spec code-central-247): the human turns CI
// Tracking on and off with set_ci_tracking, and the Factory reports its
// Push remote's URL with report_push_remote, so the UI can prefill the
// GitHub repository, or from an Azure Repos Push remote the Azure DevOps
// project URL.

// CIView is a Project's CI settings as every Project read returns them.
type CIView struct {
	Tracking bool             `json:"tracking"`
	Provider store.CIProvider `json:"provider"`
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
}

// CIRuleView is one CI Rule: when Workflow fails on Branch, open a Ticket
// to fix it, in State, on the rule's own Spec, whose ID names the rule.
type CIRuleView struct {
	Spec string `json:"spec"`
	// Workflow is a workflow file's base name or a pipeline's name, or any.
	Workflow string      `json:"workflow"`
	Branch   string      `json:"branch"`
	State    store.State `json:"state"`
	Created  time.Time   `json:"created"`
}

func ciView(key string, c store.CI, watcher WatcherView) CIView {
	v := CIView{Tracking: c.Tracking, Provider: c.Provider, Repository: c.Repository,
		ProjectURL: c.ProjectURL, PATEnv: patEnvOf(c),
		IgnoredWorkflows: orEmpty(c.IgnoredWorkflows), PushRemoteURL: c.PushRemoteURL, Rules: []CIRuleView{}, Watcher: watcher}
	for _, r := range c.Rules {
		v.Rules = append(v.Rules, CIRuleView{Spec: DisplayID(key, r.Spec), Workflow: r.Workflow, Branch: r.Branch, State: r.State, Created: r.Created})
	}
	return v
}

// patEnvOf is c's PAT environment variable, ADO_PAT when it names none.
func patEnvOf(c store.CI) string {
	if c.PATEnv == "" {
		return store.DefaultPATEnv
	}
	return c.PATEnv
}

// SetCITrackingInput is the human's set_ci_tracking(tracking, provider,
// repository, project_url, pat_env, ignored_workflows,
// full?).
type SetCITrackingInput struct {
	Tracking bool `json:"tracking"`
	// Provider is github_actions when left out.
	Provider   store.CIProvider `json:"provider"`
	Repository string           `json:"repository"`
	// ProjectURL is the Azure DevOps project, as its URL.
	ProjectURL string `json:"project_url"`
	// PATEnv names the environment variable holding the Azure DevOps PAT;
	// ADO_PAT when left out.
	PATEnv           string   `json:"pat_env"`
	IgnoredWorkflows []string `json:"ignored_workflows"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// repositoryPattern is a GitHub repository's owner/name.
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)

// workflowPattern is a workflow file's base name: GitHub Actions keeps
// every workflow in one folder, so the base name names it. It is a name,
// not a pattern, so it holds no glob.
var workflowPattern = regexp.MustCompile(`^[^/\\\s*?\[\]]+\.ya?ml$`)

// patEnvPattern is an environment variable's name.
var patEnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// checkWorkflow reports whether name is a workflow name for provider, with
// an example of one for messages. GitHub Actions names workflow files. A pipeline's name is any
// non-empty name without a folder separator or a glob, spaces allowed.
func checkWorkflow(provider store.CIProvider, name string) (example string, ok bool) {
	if provider == store.CIAzurePipelines {
		return "a pipeline's name, such as Build", name != "" && !strings.ContainsAny(name, `/\*?[]`)
	}
	return "a workflow file name, such as ci.yml", workflowPattern.MatchString(name)
}

// SetCITracking is the human's set_ci_tracking: CI Tracking on or off,
// with the CI Provider, the repository it watches and the Ignored
// workflows. Turning it on needs a repository. The Push remote URL the
// Factory reported is kept.
func (c *Core) SetCITracking(ctx context.Context, key string, in SetCITrackingInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_ci_tracking"); err != nil {
		return Written[ProjectView]{}, err
	}
	if in.Provider == "" {
		in.Provider = store.CIGitHubActions
	}
	if !in.Provider.Valid() {
		return Written[ProjectView]{}, invalid("provider", "CI Provider %q isn't %q or %q", in.Provider, store.CIGitHubActions, store.CIAzurePipelines)
	}
	ci := store.CI{Tracking: in.Tracking, Provider: in.Provider, Repository: strings.TrimSpace(in.Repository),
		ProjectURL: strings.TrimSpace(in.ProjectURL), PATEnv: strings.TrimSpace(in.PATEnv)}
	if err := checkCITarget(&ci); err != nil {
		return Written[ProjectView]{}, err
	}
	for _, w := range in.IgnoredWorkflows {
		w = strings.TrimSpace(w)
		switch example, ok := checkWorkflow(ci.Provider, w); {
		case w == "" || slices.Contains(ci.IgnoredWorkflows, w):
		case !ok:
			return Written[ProjectView]{}, invalid("ignored_workflows", "%q isn't %s", w, example)
		default:
			ci.IgnoredWorkflows = append(ci.IgnoredWorkflows, w)
		}
	}
	if err := c.checkCI(ctx, key, ci); err != nil {
		return Written[ProjectView]{}, err
	}
	return c.updateCI(key, in.Full, func(p *store.CI) error {
		ci.PushRemoteURL, ci.Rules = p.PushRemoteURL, p.Rules
		// What the CI Watcher said was about the old settings.
		if !ci.Tracking || watchesAnother(*p, ci) {
			c.watchers.reset(watcherKey{WatcherCI, key})
		}
		*p = ci
		return nil
	})
}

// checkCITarget validates what the CI Provider ci names watches: the
// repository for GitHub Actions, the project URL and PAT environment
// variable for Azure Pipelines. Each provider's other fields are kept as
// given, unchecked, until that provider is chosen. It replaces the
// project URL with the one it parses to.
func checkCITarget(ci *store.CI) error {
	switch ci.Provider {
	case store.CIAzurePipelines:
		if ci.ProjectURL == "" && ci.Tracking {
			return invalid("project_url", "CI Tracking needs the Azure DevOps project to watch, as its URL")
		}
		if ci.ProjectURL != "" {
			u, err := azdourl.ParseProjectURL(ci.ProjectURL)
			if err != nil {
				return invalid("project_url", "%v", err)
			}
			ci.ProjectURL = u
		}
		if ci.PATEnv != "" && !patEnvPattern.MatchString(ci.PATEnv) {
			return invalid("pat_env", "%q isn't an environment variable name: a letter or underscore, then letters, digits and underscores, such as %s", ci.PATEnv, store.DefaultPATEnv)
		}
	default:
		switch {
		case ci.Repository == "" && ci.Tracking:
			return invalid("repository", "CI Tracking needs the GitHub repository to watch, as owner/name")
		case ci.Repository != "" && !repositoryPattern.MatchString(ci.Repository):
			return invalid("repository", "%q isn't a GitHub repository as owner/name, such as acme/web", ci.Repository)
		}
	}
	return nil
}

// watchesAnother reports whether now watches something other than was
// did: another CI Provider, repository, project URL or PAT environment
// variable.
func watchesAnother(was, now store.CI) bool {
	return was.Provider != now.Provider || was.Repository != now.Repository || was.ProjectURL != now.ProjectURL || patEnvOf(was) != patEnvOf(now)
}

// CICheckInput is what the CI Watcher's check mode checks: that it can
// watch what the Project names through its CI Provider: the repository
// through gh, found with the gh path Option, or the Azure DevOps project
// through its API with the PAT the environment variable PATEnv names.
type CICheckInput struct {
	Provider   store.CIProvider `json:"provider"`
	Repository string           `json:"repository"`
	ProjectURL string           `json:"project_url,omitempty"`
	PATEnv     string           `json:"pat_env,omitempty"`
	GHPath     string           `json:"gh_path,omitempty"`
}

// CICheck runs the CI Watcher once in its check mode. The service makes
// no external calls itself (ADR 0002, ADR 0004), so the check runs in the
// CI Watcher's process.
type CICheck func(ctx context.Context, in CICheckInput) WatcherCheckResult

// SetCICheck makes c run check when CI Tracking is turned on. Call it
// before serving; without it, nothing is checked.
func (c *Core) SetCICheck(check CICheck) { c.ciCheck = check }

// checkCI runs the CI check when ci turns CI Tracking on, or changes what
// it watches while on, and refuses with the reason and fix when it fails:
// on the repository when gh can't read it, on the project URL when the
// Azure DevOps project can't be read, else on tracking.
// It runs outside the store's lock, as the check calls GitHub.
func (c *Core) checkCI(ctx context.Context, key string, ci store.CI) error {
	if !ci.Tracking || c.ciCheck == nil {
		return nil
	}
	var was store.CI
	var opts store.Options
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		was, opts = p.CI, r.Options()
		return nil
	})
	if err != nil || (was.Tracking && !watchesAnother(was, ci)) {
		return err
	}
	in := CICheckInput{Provider: ci.Provider, Repository: ci.Repository, GHPath: opts.GHPath}
	if ci.Provider == store.CIAzurePipelines {
		in.ProjectURL, in.PATEnv = ci.ProjectURL, patEnvOf(ci)
	}
	res := c.ciCheck(ctx, in)
	if res.Problem == "" {
		return nil
	}
	field := "tracking"
	switch res.Problem {
	case ProblemRepositoryUnreadable:
		field = "repository"
	case ProblemProjectUnreadable:
		field = "project_url"
	}
	return &Error{Code: Invalid, Field: field, Message: res.Reason, Fix: res.Fix}
}

// ReportPushRemoteInput is the Factory's report_push_remote(url, full?).
type ReportPushRemoteInput struct {
	URL string `json:"url"`
	// Base is the base branch the Factory lands into, which pull requests
	// open into; left out by a Factory that doesn't report it.
	Base string `json:"base,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportPushRemote is the Factory's report_push_remote: the URL of the
// Push remote, which it reports when it starts.
func (c *Core) ReportPushRemote(_ context.Context, key string, in ReportPushRemoteInput) (Written[ProjectView], error) {
	url := strings.TrimSpace(in.URL)
	if url == "" {
		return Written[ProjectView]{}, invalid("url", "report_push_remote needs the Push remote's URL")
	}
	return c.updateCI(key, in.Full, func(p *store.CI) error {
		p.PushRemoteURL = withoutLogin(url)
		if b := strings.TrimSpace(in.Base); b != "" {
			p.PushRemoteBase = b
		}
		return nil
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
func (c *Core) updateCI(key string, full bool, change func(*store.CI) error) (Written[ProjectView], error) {
	var before, after ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = c.projectView(p)
		was, ci := p.CI, p.CI
		ci.IgnoredWorkflows = slices.Clone(ci.IgnoredWorkflows)
		ci.Rules = slices.Clone(ci.Rules)
		if err := change(&ci); err != nil {
			return err
		}
		p.CI = ci
		after = c.projectView(p)
		// A CI write never changes the Landing; only its Code Host, worked
		// out from the Push remote URL, can differ, and that isn't the
		// write's change.
		before.Landing = after.Landing
		if ci.Equal(was) {
			return nil
		}
		return w.PutProject(p)
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
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
	State store.State `json:"state,omitempty"`
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
		in.State = store.Draft
	}
	var r store.CIRule
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		var err error
		if r, err = checkCIRule(p.CI.Provider, in); err != nil {
			return err
		}
		if err := ruleIsNew(key, p.CI.Rules, r, 0); err != nil {
			return err
		}
		r.Created = c.now()
		r.Spec = w.NextNumber(key)
		return w.PutSpec(key, store.Spec{Number: r.Spec, Title: ciRuleTitle(r), Body: ciRuleSpecBody, Labels: []string{LongLivedLabel}, Created: r.Created, Updated: r.Created})
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	return c.updateCI(key, in.Full, func(ci *store.CI) error {
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
	var old, r store.CIRule
	v, err := c.updateCI(key, in.Full, func(ci *store.CI) error {
		i, err := ruleOn(key, ci.Rules, n)
		if err != nil {
			return err
		}
		old = ci.Rules[i]
		if in.State == "" {
			in.State = old.State
		}
		if r, err = checkCIRule(ci.Provider, in); err != nil {
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
	return c.updateCI(key, in.Full, func(ci *store.CI) error {
		i, err := ruleOn(key, ci.Rules, n)
		if err != nil {
			return err
		}
		ci.Rules = slices.Delete(ci.Rules, i, i+1)
		return nil
	})
}

// checkCIRule is in's rule for a Project on provider, trimmed, or why it isn't one.
func checkCIRule(provider store.CIProvider, in CIRuleInput) (store.CIRule, error) {
	r := store.CIRule{Workflow: strings.TrimSpace(in.Workflow), Branch: strings.TrimSpace(in.Branch), State: in.State}
	if example, ok := checkWorkflow(provider, r.Workflow); r.Workflow != store.AnyWorkflow && !ok {
		return r, invalid("workflow", "%q isn't %s, or %s for any workflow", r.Workflow, example, store.AnyWorkflow)
	}
	if r.Branch == "" || strings.ContainsAny(r.Branch, globChars) || strings.IndexFunc(r.Branch, unicode.IsSpace) >= 0 {
		return r, invalid("branch", "%q isn't an exact branch name, such as main: a CI Rule's branch takes no globs", r.Branch)
	}
	if r.State != store.Draft && r.State != store.Ready {
		return r, invalid("state", "a CI Rule's Tickets open as %s or %s, not %q", store.Draft, store.Ready, r.State)
	}
	return r, nil
}

// ruleOn is the index of the rule on Spec n.
func ruleOn(key string, rules []store.CIRule, n int) (int, error) {
	i := slices.IndexFunc(rules, func(r store.CIRule) bool { return r.Spec == n })
	if i < 0 {
		return 0, notFound("there is no CI Rule on Spec %s", DisplayID(key, n))
	}
	return i, nil
}

// ruleIsNew refuses r when another rule, not the one on Spec self,
// already watches its workflow on its branch.
func ruleIsNew(key string, rules []store.CIRule, r store.CIRule, self int) error {
	for _, o := range rules {
		if o.Spec != self && o.Workflow == r.Workflow && o.Branch == r.Branch {
			return invalid("workflow", "the CI Rule on Spec %s already watches %s", DisplayID(key, o.Spec), ciRuleWatches(r))
		}
	}
	return nil
}

// followRule retitles the Spec of the rule edited from old to r, unless
// the human has changed its title.
func (c *Core) followRule(ctx context.Context, key string, old, r store.CIRule) error {
	if ciRuleTitle(old) == ciRuleTitle(r) {
		return nil
	}
	return c.store.Update(func(w store.Writer) error {
		s, ok := w.Spec(key, old.Spec)
		if !ok || s.Title != ciRuleTitle(old) {
			return nil
		}
		s.Updated = c.now()
		s.Edits = append(s.Edits, store.Edit{Number: len(s.Edits) + 1, Actor: string(ActorOf(ctx)), Time: s.Updated, Previous: map[string]string{"title": s.Title}})
		s.Title = ciRuleTitle(r)
		return w.PutSpec(key, s)
	})
}

// ciRuleWatches is what a rule watches: "ci.yml on main" or "any workflow
// on main".
func ciRuleWatches(r store.CIRule) string {
	w := r.Workflow
	if w == store.AnyWorkflow {
		w = "any workflow"
	}
	return w + " on " + r.Branch
}

// ciRuleTitle is the title of a rule's Spec: "CI: ci.yml on main".
func ciRuleTitle(r store.CIRule) string { return "CI: " + ciRuleWatches(r) }

// ciRuleSpecBody is a rule's Spec's body. It names neither workflow nor
// branch, which the title carries, so an edit leaves it as it is.
const ciRuleSpecBody = "This Spec holds the fix Tickets of a CI Rule, which opens a Ticket to fix its workflow when it fails on its branch. " +
	"The rule is in the CI section of Project Options; removing it leaves this Spec as the record of its Tickets.\n"
