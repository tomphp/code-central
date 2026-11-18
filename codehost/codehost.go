// Package codehost works out the Code Host a remote URL names and checks
// the repository and workflow names a Code Host uses. It imports nothing
// of Code Central, so core and the watchers can both use it.
package codehost

import (
	"regexp"
	"strings"
)

// Host is a Code Host a Pull Request is opened on, worked out from a Push
// remote URL, or none.
type Host string

// The Code Hosts.
const (
	GitHub     Host = "github"
	AzureRepos Host = "azure_repos"
	// None is no remote URL reported, or one naming a Code Host Code
	// Central doesn't support.
	None Host = ""
)

var (
	githubRemote = regexp.MustCompile(`^(?:(?:https?|ssh|git)://(?:[^@/]+@)?github\.com(?::\d+)?/|[^@/\s]+@github\.com:)([A-Za-z0-9-]+)/([A-Za-z0-9._-]+?)(?:\.git)?/?$`)
	// azureSSH is ssh.dev.azure.com's and vs-ssh.visualstudio.com's
	// v3/<organization>/<project>/<repository>.
	azureSSH = regexp.MustCompile(`^[^@/\s]+@(?:ssh\.dev\.azure\.com|vs-ssh\.visualstudio\.com):v3/([^/\s]+)/([^/\s]+)/([^/\s]+?)/?$`)
	// azureHTTP is https://[login@]host/<path>/_git/<repository>.
	azureHTTP = regexp.MustCompile(`^https?://(?:[^@/\s]+@)?([^/@\s]+)(/.+?)/_git/([^/\s]+?)/?$`)

	// githubRepository is a GitHub repository's owner/name.
	githubRepository = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)
	// githubWorkflow is a workflow file's base name: GitHub Actions keeps
	// every workflow in one folder, so the base name names it. It is a
	// name, not a pattern, so it holds no glob.
	githubWorkflow = regexp.MustCompile(`^[^/\\\s*?\[\]]+\.ya?ml$`)
)

// Of works out the Code Host and repository a Push remote URL names, or
// None when it is none Code Central supports. An Azure Repos repository
// reads <organization>/<project>/<repository>.
func Of(remoteURL string) (host Host, repository string) {
	remoteURL = strings.TrimSpace(remoteURL)
	if m := githubRemote.FindStringSubmatch(remoteURL); m != nil {
		return GitHub, m[1] + "/" + m[2]
	}
	if m := azureSSH.FindStringSubmatch(remoteURL); m != nil {
		return AzureRepos, m[1] + "/" + m[2] + "/" + m[3]
	}
	m := azureHTTP.FindStringSubmatch(remoteURL)
	if m == nil {
		return None, ""
	}
	hostname, segs := strings.ToLower(m[1]), strings.Split(strings.Trim(m[2], "/"), "/")
	switch {
	case hostname == "dev.azure.com" && len(segs) == 2:
		return AzureRepos, segs[0] + "/" + segs[1] + "/" + m[3]
	case strings.HasSuffix(hostname, ".visualstudio.com"):
		if len(segs) == 2 && strings.EqualFold(segs[0], "DefaultCollection") {
			segs = segs[1:]
		}
		if len(segs) == 1 {
			return AzureRepos, strings.TrimSuffix(hostname, ".visualstudio.com") + "/" + segs[0] + "/" + m[3]
		}
	}
	return None, ""
}

// ValidGitHubRepository reports whether s is a GitHub repository's
// owner/name.
func ValidGitHubRepository(s string) bool { return githubRepository.MatchString(s) }

// ValidGitHubWorkflow reports whether name is a GitHub Actions workflow
// file's base name.
func ValidGitHubWorkflow(name string) bool { return githubWorkflow.MatchString(name) }
