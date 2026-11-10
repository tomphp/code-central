package core

import (
	"context"
	neturl "net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// A Project's CI settings (Spec code-central-247): the human turns CI
// Tracking on and off with set_ci_tracking, and the Factory reports its
// Push remote's URL with report_push_remote, so the UI can prefill the
// GitHub repository.

// CIView is a Project's CI settings as every Project read returns them.
type CIView struct {
	Tracking   bool             `json:"tracking"`
	Provider   store.CIProvider `json:"provider"`
	Repository string           `json:"repository"`
	// IgnoredWorkflows are workflow file base names, such as ci.yml,
	// whose runs never count towards a CI result.
	IgnoredWorkflows []string `json:"ignored_workflows"`
	// PushRemoteURL is the URL of the Push remote the Factory last
	// reported, empty until it has.
	PushRemoteURL string `json:"push_remote_url"`
}

func ciView(c store.CI) CIView {
	return CIView{Tracking: c.Tracking, Provider: c.Provider, Repository: c.Repository,
		IgnoredWorkflows: orEmpty(c.IgnoredWorkflows), PushRemoteURL: c.PushRemoteURL}
}

// SetCITrackingInput is the human's set_ci_tracking(tracking, provider,
// repository, ignored_workflows, full?).
type SetCITrackingInput struct {
	Tracking bool `json:"tracking"`
	// Provider is github_actions, the only one, when left out.
	Provider         store.CIProvider `json:"provider"`
	Repository       string           `json:"repository"`
	IgnoredWorkflows []string         `json:"ignored_workflows"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// repositoryPattern is a GitHub repository's owner/name.
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)

// workflowPattern is a workflow file's base name: GitHub Actions keeps
// every workflow in one folder, so the base name names it.
var workflowPattern = regexp.MustCompile(`^[^/\\\s]+\.ya?ml$`)

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
		return Written[ProjectView]{}, invalid("provider", "CI Provider %q isn't %q, the only one", in.Provider, store.CIGitHubActions)
	}
	ci := store.CI{Tracking: in.Tracking, Provider: in.Provider, Repository: strings.TrimSpace(in.Repository)}
	switch {
	case ci.Repository == "" && ci.Tracking:
		return Written[ProjectView]{}, invalid("repository", "CI Tracking needs the GitHub repository to watch, as owner/name")
	case ci.Repository != "" && !repositoryPattern.MatchString(ci.Repository):
		return Written[ProjectView]{}, invalid("repository", "%q isn't a GitHub repository as owner/name, such as acme/web", ci.Repository)
	}
	for _, w := range in.IgnoredWorkflows {
		w = strings.TrimSpace(w)
		switch {
		case w == "" || slices.Contains(ci.IgnoredWorkflows, w):
		case !workflowPattern.MatchString(w):
			return Written[ProjectView]{}, invalid("ignored_workflows", "%q isn't a workflow file name, such as ci.yml", w)
		default:
			ci.IgnoredWorkflows = append(ci.IgnoredWorkflows, w)
		}
	}
	return c.updateCI(key, in.Full, func(p *store.CI) {
		ci.PushRemoteURL = p.PushRemoteURL
		*p = ci
	})
}

// ReportPushRemoteInput is the Factory's report_push_remote(url, full?).
type ReportPushRemoteInput struct {
	URL string `json:"url"`
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
	return c.updateCI(key, in.Full, func(p *store.CI) { p.PushRemoteURL = withoutLogin(url) })
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
func (c *Core) updateCI(key string, full bool, change func(*store.CI)) (Written[ProjectView], error) {
	var before, after ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = projectView(p)
		ci := p.CI
		ci.IgnoredWorkflows = slices.Clone(ci.IgnoredWorkflows)
		change(&ci)
		if ci.Equal(p.CI) {
			after = before
			return nil
		}
		p.CI = ci
		after = projectView(p)
		return w.PutProject(p)
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	return acknowledgeProject(before, after, full, c.now())
}
