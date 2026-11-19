package api

// CodeHost is the Code Host a Pull Request is opened on, worked out from
// the Push remote URL, or none.
type CodeHost string

// CodeHosts are the Code Hosts, then none.
func CodeHosts() []CodeHost { return []CodeHost{CodeHostGitHub, CodeHostAzureRepos, CodeHostNone} }

// The Code Hosts.
const (
	CodeHostGitHub     CodeHost = "github"
	CodeHostAzureRepos CodeHost = "azure_repos"
	// CodeHostNone is no Push remote URL reported, or one naming a Code
	// Host Code Central doesn't support.
	CodeHostNone CodeHost = ""
)
