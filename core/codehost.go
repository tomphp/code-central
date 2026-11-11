package core

import (
	"regexp"
	"strings"
)

// The Code Host a Pull Request lands through (code-central-387): worked
// out from the Push remote URL the Factory reported, never configured.

// The Code Hosts.
const (
	CodeHostGitHub     = "github"
	CodeHostAzureRepos = "azure_repos"
)

var (
	githubRemote = regexp.MustCompile(`^(?:(?:https?|ssh|git)://(?:[^@/]+@)?github\.com(?::\d+)?/|[^@/\s]+@github\.com:)([A-Za-z0-9-]+)/([A-Za-z0-9._-]+?)(?:\.git)?/?$`)
	// azureSSH is ssh.dev.azure.com's and vs-ssh.visualstudio.com's
	// v3/<organization>/<project>/<repository>.
	azureSSH = regexp.MustCompile(`^[^@/\s]+@(?:ssh\.dev\.azure\.com|vs-ssh\.visualstudio\.com):v3/([^/\s]+)/([^/\s]+)/([^/\s]+?)/?$`)
	// azureHTTP is https://[login@]host/<path>/_git/<repository>.
	azureHTTP = regexp.MustCompile(`^https?://(?:[^@/\s]+@)?([^/@\s]+)(/.+?)/_git/([^/\s]+?)/?$`)
)

// codeHostOf works out the Code Host and repository a Push remote URL
// names, or "" when it is none Code Central supports. An Azure Repos
// repository reads <organization>/<project>/<repository>.
func codeHostOf(url string) (host, repository string) {
	url = strings.TrimSpace(url)
	if m := githubRemote.FindStringSubmatch(url); m != nil {
		return CodeHostGitHub, m[1] + "/" + m[2]
	}
	if m := azureSSH.FindStringSubmatch(url); m != nil {
		return CodeHostAzureRepos, m[1] + "/" + m[2] + "/" + m[3]
	}
	m := azureHTTP.FindStringSubmatch(url)
	if m == nil {
		return "", ""
	}
	hostname, segs := strings.ToLower(m[1]), strings.Split(strings.Trim(m[2], "/"), "/")
	switch {
	case hostname == "dev.azure.com" && len(segs) == 2:
		return CodeHostAzureRepos, segs[0] + "/" + segs[1] + "/" + m[3]
	case strings.HasSuffix(hostname, ".visualstudio.com"):
		if len(segs) == 2 && strings.EqualFold(segs[0], "DefaultCollection") {
			segs = segs[1:]
		}
		if len(segs) == 1 {
			return CodeHostAzureRepos, strings.TrimSuffix(hostname, ".visualstudio.com") + "/" + segs[0] + "/" + m[3]
		}
	}
	return "", ""
}

// checkPullRequestHost refuses turning Pull Request on while no Push
// remote URL has been reported or it names no supported Code Host. The
// Pull Request Watcher's own checks (gh, the PAT) come with the watcher.
func checkPullRequestHost(url string) error {
	if strings.TrimSpace(url) == "" {
		return &Error{Code: Invalid, Field: "method", Message: "Pull Request needs the Push remote's URL to work out the Code Host, and the Factory hasn't reported one",
			Fix: "start the Factory once so it reports the Push remote's URL, then turn Pull Request on"}
	}
	if host, _ := codeHostOf(url); host == "" {
		return &Error{Code: Invalid, Field: "method", Message: "the Push remote " + url + " is on a host Pull Request doesn't support; it supports GitHub (github.com) and Azure Repos (dev.azure.com, ssh.dev.azure.com, *.visualstudio.com)",
			Fix: "use Merge or Branch, or point the Push remote at a supported host"}
	}
	return nil
}
