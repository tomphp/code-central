package azdo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Azure Repos: the Pull Request Watcher's calls (Spec code-central-383),
// under the project's _apis/git/repositories/<repository>.

// ErrRefused is Azure DevOps refusing a request it understood but won't act
// on (400 or 409), such as a pull request from a branch that doesn't exist.
var ErrRefused = errors.New("the request was refused")

// MaxPullRequestDescription is the longest description Azure Repos accepts.
const MaxPullRequestDescription = 4000

// Repository is a Git repository's resource.
type Repository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PullRequest is one pull request. Status is active, abandoned or
// completed, and MergeStatus is succeeded, conflicts or others while Azure
// DevOps works it out.
type PullRequest struct {
	ID              int       `json:"pullRequestId"`
	Status          string    `json:"status"`
	MergeStatus     string    `json:"mergeStatus"`
	Created         time.Time `json:"creationDate"`
	Closed          time.Time `json:"closedDate"`
	LastMergeCommit struct {
		CommitID string `json:"commitId"`
	} `json:"lastMergeCommit"`
}

func (c *Client) repoURL(repository string) string {
	return c.baseURL + "/_apis/git/repositories/" + url.PathEscape(repository)
}

// WebURL is the link to a pull request in a browser.
func (c *Client) WebURL(repository string, id int) string {
	return fmt.Sprintf("%s/_git/%s/pullrequest/%d", c.baseURL, url.PathEscape(repository), id)
}

// Repository reads the repository.
func (c *Client) Repository(ctx context.Context, repository string) (Repository, error) {
	var r Repository
	err := c.do(ctx, http.MethodGet, c.repoURL(repository), nil, nil, &r)
	return r, err
}

// PullRequests lists the repository's pull requests of every status from
// source into target, both full ref names.
func (c *Client) PullRequests(ctx context.Context, repository, source, target string) ([]PullRequest, error) {
	v := url.Values{"searchCriteria.status": {"all"}, "$top": {"100"}}
	if source != "" {
		v.Set("searchCriteria.sourceRefName", source)
	}
	if target != "" {
		v.Set("searchCriteria.targetRefName", target)
	}
	var list struct {
		Value []PullRequest `json:"value"`
	}
	if err := c.do(ctx, http.MethodGet, c.repoURL(repository)+"/pullrequests", v, nil, &list); err != nil {
		return nil, err
	}
	return list.Value, nil
}

// PullRequest reads one pull request.
func (c *Client) PullRequest(ctx context.Context, repository string, id int) (PullRequest, error) {
	var p PullRequest
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/pullrequests/%d", c.repoURL(repository), id), nil, nil, &p)
	return p, err
}

// OpenPullRequest opens a pull request, ready for review, from source into
// target, both full ref names. The description is cut to what Azure Repos
// accepts.
func (c *Client) OpenPullRequest(ctx context.Context, repository, source, target, title, description string) (PullRequest, error) {
	if r := []rune(description); len(r) > MaxPullRequestDescription {
		description = string(r[:MaxPullRequestDescription-1]) + "…"
	}
	var p PullRequest
	err := c.do(ctx, http.MethodPost, c.repoURL(repository)+"/pullrequests", nil, map[string]any{
		"sourceRefName": source, "targetRefName": target, "title": title, "description": description, "isDraft": false,
	}, &p)
	return p, err
}

// CanContribute reports whether the PAT may contribute to pull requests
// (Code: Read & write), without creating one: it asks for a pull request
// from a branch that doesn't exist, which Azure DevOps refuses after
// checking the PAT's scope. An answer that opened one would not be
// expected, and counts as able.
func (c *Client) CanContribute(ctx context.Context, repository string) error {
	const missing = "refs/heads/code-central-check-no-such-branch"
	_, err := c.OpenPullRequest(ctx, repository, missing, missing, "Code Central check", "")
	if errors.Is(err, ErrRefused) || errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// SplitRepository splits an Azure Repos repository, written
// <organization>/<project>/<repository>.
func SplitRepository(s string) (organization, project, repository string, ok bool) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
