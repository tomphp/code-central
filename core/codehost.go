package core

import (
	"strings"

	"github.com/tomoram/code-central/codehost"
)

// The Code Host a Pull Request lands through (code-central-387): worked
// out from the Push remote URL the Factory reported, never configured.

// codeHostOf works out the Code Host and repository a Push remote URL
// names, or "" when it is none Code Central supports.
func codeHostOf(url string) (CodeHost, string) {
	host, repository := codehost.Of(url)
	return CodeHost(host), repository
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
