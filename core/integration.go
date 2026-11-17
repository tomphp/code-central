package core

import "github.com/tomoram/code-central/store"

// The Integrations (Spec code-central-894): GitHub Issues and Azure
// Boards. Each has its own inbox Spec, the Issues Spec or the Azure
// Boards Spec, that its Incoming Issues arrive on, its own New issues
// need me, and its own Watcher, which writes back only to its own
// Integration's Links.

// The Integrations' names, as a Link names them.
const (
	IntegrationGitHub      = "github"
	IntegrationAzureBoards = "azure-boards"
)

// inbox is an Integration's inbox Spec in a Project.
type inbox struct {
	// integration is the Integration whose inbox it is.
	integration string
	// spec is the Spec's number, 0 until the Integration is first turned
	// on.
	spec int
	// newIssuesNeedMe is the Integration's New issues need me.
	newIssuesNeedMe bool
}

// inboxes are p's Integrations' inboxes, GitHub Issues' then Azure
// Boards'.
func inboxes(p store.Project) []inbox {
	return []inbox{
		{IntegrationGitHub, p.GitHubIssues.Spec, p.GitHubIssues.NewIssuesNeedMe},
		{IntegrationAzureBoards, p.AzureBoards.Spec, p.AzureBoards.NewIssuesNeedMe},
	}
}

// inboxOn is the inbox whose Spec is spec, in key, if spec is an
// Integration's inbox Spec.
func inboxOn(r store.Reader, key string, spec int) (inbox, bool) {
	p, _ := r.Project(key)
	for _, in := range inboxes(p) {
		if in.spec != 0 && in.spec == spec {
			return in, true
		}
	}
	return inbox{}, false
}

// inboxOf is integration's inbox in key.
func inboxOf(r store.Reader, key, integration string) inbox {
	p, _ := r.Project(key)
	for _, in := range inboxes(p) {
		if in.integration == integration {
			return in
		}
	}
	return inbox{integration: integration}
}
