package api

import (
	"github.com/tomoram/code-central/domain"
)

// RepositoryCI is a Repository's CI entry as add_repository and
// set_repository_ci take it: Not watched, or a CI Provider, GitHub
// Actions when left out, with what it watches.
type RepositoryCI struct {
	NotWatched bool              `json:"not_watched,omitempty"`
	Provider   domain.CIProvider `json:"provider,omitempty"`
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
	// Name is the Repository's, the Home Repository's included.
	Name string `json:"name"`
	RepositoryCI
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
