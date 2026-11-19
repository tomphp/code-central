package api

// PullRequestCheckInput is what the Pull Request Watcher's check mode
// checks: that it can open and follow pull requests on the Code Host's
// repository, or with Repositories, on each of theirs. On GitHub it goes
// through gh, found with the gh path Option.
type PullRequestCheckInput struct {
	CodeHost   CodeHost `json:"code_host"`
	Repository string   `json:"repository"`
	// Repositories, when given, are the Repositories to check in place
	// of CodeHost and Repository, each answered on its own (Spec
	// code-central-848).
	Repositories []PullRequestCheckRepository `json:"repositories,omitempty"`
	GHPath       string                       `json:"gh_path,omitempty"`
	PATEnv       string                       `json:"pat_env,omitempty"`
	// ProjectURL, when set, is the Azure DevOps project URL to call instead
	// of the one worked out from Repository, for tests.
	ProjectURL string `json:"project_url,omitempty"`
}

// PullRequestCheckRepository is one Repository for the check: its
// Repository Name, and the Code Host and repository there its Clone URL
// names.
type PullRequestCheckRepository struct {
	Name       string   `json:"name"`
	CodeHost   CodeHost `json:"code_host"`
	Repository string   `json:"repository"`
}
