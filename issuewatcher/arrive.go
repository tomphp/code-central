// Issues arriving: each poll, every open issue in the repository with no
// Link in the Project comes in as an Incoming Issue on the Issues Spec.

package issuewatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/store"
)

// Integration is the name every Link the Issue Watcher makes carries.
const Integration = "github"

// LinkID is the Link ID of repository's issue n: owner/repo#n, so it
// stays unique if the configured repository changes.
func LinkID(repository string, n int) string { return fmt.Sprintf("%s#%d", repository, n) }

// perPage is how many issues the Issue Watcher asks GitHub for a page.
const perPage = 100

// ghIssue is what the Issue Watcher reads of an issue, alone or in
// GitHub's list of a repository's issues, which has its pull requests
// too.
type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	// State is open or closed.
	State         string          `json:"state"`
	HTMLURL       string          `json:"html_url"`
	RepositoryURL string          `json:"repository_url"`
	Labels        []ghLabel       `json:"labels"`
	PullRequest   json.RawMessage `json:"pull_request"`
}

// ghLabel is a label on an issue.
type ghLabel struct {
	Name string `json:"name"`
}

// openIssues lists repository's open issues through prog, gh, oldest
// first, leaving out pull requests and, if label isn't empty, issues
// without it. GitHub rate-limiting a call is a *watchers.RateLimitError.
func openIssues(ctx context.Context, env gh.Env, prog, repository, label string) ([]ghIssue, error) {
	var out []ghIssue
	for page := 1; ; page++ {
		q := url.Values{"state": {"open"}, "sort": {"created"}, "direction": {"asc"}, "per_page": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(page)}}
		if label != "" {
			q.Set("labels", label)
		}
		body, msg, err := gh.API(ctx, env, prog, "repos/"+repository+"/issues?"+q.Encode())
		if err != nil {
			if msg != "" {
				err = fmt.Errorf("%w: %s", err, msg)
			}
			return nil, fmt.Errorf("listing the open issues of %s: %w", repository, err)
		}
		var items []ghIssue
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("GitHub's list of the open issues of %s can't be read: %w", repository, err)
		}
		for _, i := range items {
			if i.PullRequest == nil {
				out = append(out, i)
			}
		}
		if len(items) < perPage {
			return out, nil
		}
	}
}

// arrive brings in each of issues, watch's repository's open issues,
// that has no Link in the Project, detached ones included, so an
// unlinked or triaged issue never comes in again, as a Draft Ticket on
// the Issues Spec. What goes wrong is logged and tried again on the next
// poll.
func (cfg Config) arrive(ctx context.Context, watch core.IssueWatch, issues []ghIssue) {
	linked, err := cfg.Client.FindLinked(ctx, cfg.Project, core.FindLinkedInput{Integration: Integration})
	if err != nil {
		if ctx.Err() == nil {
			cfg.logf("find_linked: %v", err)
		}
		return
	}
	have := map[string]bool{}
	for _, row := range linked {
		have[row.Link.ID] = true
	}
	for _, i := range issues {
		id := LinkID(watch.Repository, i.Number)
		if have[id] {
			continue
		}
		_, err := cfg.Client.CreateLinkedTicket(ctx, cfg.Project, core.CreateLinkedTicketInput{
			Spec: core.Ref(watch.Spec), Title: i.Title, Description: i.Body, State: store.Draft,
			Link: core.LinkInput{Integration: Integration, ID: id, URL: i.HTMLURL},
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			cfg.logf("create_linked_ticket for %s: %v", id, err)
		}
	}
}
