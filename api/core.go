package api

import (
	"github.com/tomoram/code-central/domain"
)

// LandingView is a Project's Landing Method with its Push option and the
// remote it uses.
type LandingView struct {
	Method domain.LandingMethod `json:"method"`
	Push   bool                 `json:"push"`
	Remote string               `json:"remote"`
	// LandPerSpec is the Merge option to land a Spec's work together,
	// shown whatever the method.
	LandPerSpec bool `json:"land_per_spec"`
	// Rebase is the Merge and Pull Request option to replay a Ticket's
	// commits onto its target instead of merging its branch, shown
	// whatever the method.
	Rebase bool `json:"rebase"`
	// CodeHost is the Code Host the reported Push remote URL names,
	// github or azure_repos, empty while none is reported or it is
	// unsupported. Repository is the host's name for the repository.
	CodeHost   CodeHost `json:"code_host"`
	Repository string   `json:"repository"`
	// PATEnv is the environment variable holding the Azure Repos PAT,
	// ADO_PAT unless the Project names another.
	PATEnv string `json:"pat_env"`
	// UsesAzureReposPAT is whether the PAT variable is used: any Azure
	// Repos Repository is under Pull Request.
	UsesAzureReposPAT bool `json:"uses_azure_repos_pat"`
	// Base is the branch the Project's work is cut from and lands on: the
	// stored one, else the reported fallback, else main.
	Base string `json:"base"`
	// Methods is the Landing capabilities of each Landing Method, as if
	// it were the Default Landing: what the Options form offers while the
	// human drafts it.
	Methods LandingMethodsCapabilities `json:"methods"`
	// RunsPullRequestWatcher is whether the Pull Request Watcher runs, as
	// saved: any Repository is under Pull Request.
	RunsPullRequestWatcher bool `json:"runs_pull_request_watcher"`
	// RepositoryLanding is the Default Landing named as a Repository's
	// Landing names it.
	RepositoryLanding RepositoryLanding `json:"repository_landing"`
	// PushesNothing is whether Push is on in some Landing while every
	// Repository is local-only, so it pushes nothing.
	PushesNothing bool `json:"pushes_nothing"`
	// Watcher is the Pull Request Watcher's status, not_reported unless the
	// method is Pull Request.
	Watcher WatcherView `json:"watcher"`
	// OpenPullRequests are the pull requests left open on the Code Host
	// after a switch away from Pull Request, one per Ticket's, until the
	// Ticket leaves Accepted. Always empty under Pull Request.
	OpenPullRequests []OpenPullRequest `json:"open_pull_requests"`
}

// OpenPullRequest is a pull request left open for an Accepted Ticket.
type OpenPullRequest struct {
	Ticket string `json:"ticket"`
	Link   string `json:"link"`
}
