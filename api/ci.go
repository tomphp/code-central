package api

import (
	"context"
	"time"

	"github.com/tomoram/code-central/domain"
)

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
	// AutoFix is the Auto-fix setting, kept while CI Tracking is off.
	AutoFix AutoFixView `json:"auto_fix"`
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

// CICheck runs the CI Watcher once in its check mode. The service makes
// no external calls itself (ADR 0002, ADR 0004), so the check runs in the
// CI Watcher's process.
type CICheck func(ctx context.Context, in CICheckInput) WatcherCheckResult
