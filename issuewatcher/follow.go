// Following GitHub: while an Incoming Issue waits for triage, GitHub
// owns its text and its external state, so each poll it takes its
// issue's edits, and whether the issue is still open in the repository.

package issuewatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/store"
)

// follow has each of watch's Incoming Issues from its repository follow
// its issue. One among listed, the repository's open issues, takes the
// issue's title and body when they differ, and is open. One that wasn't
// listed has its issue read: closed is closed, deleted is deleted, and
// transferred away or without the filter label is out of scope, with the
// title and body followed while the issue is still in the repository.
// Nothing is deleted. Incoming Issues from a repository no longer
// configured are left as they are. It returns GitHub's rate limit, if it
// refused a call for it; anything else that goes wrong is logged and
// tried again on the next poll.
func (cfg Config) follow(ctx context.Context, watch core.IssueWatch, prog string, listed []ghIssue) *gh.RateLimitError {
	open := map[int]ghIssue{}
	for _, i := range listed {
		open[i.Number] = i
	}
	for _, in := range watch.Issues {
		n, ok := issueNumber(watch.Repository, in.Link.ID)
		if !ok {
			continue
		}
		i, ok := open[n]
		issue, state := &i, store.ExternalOpen
		if !ok {
			var err error
			issue, state, err = readIssue(ctx, cfg.Env, prog, watch.Repository, watch.Label, n)
			if err != nil {
				var limit *gh.RateLimitError
				if errors.As(err, &limit) {
					return limit
				}
				if ctx.Err() != nil {
					return nil
				}
				cfg.logf("%v", err)
				continue
			}
		}
		if issue != nil {
			cfg.followText(ctx, in, *issue)
		}
		if state != in.Link.ExternalState {
			_, err := cfg.Client.SetExternalState(ctx, cfg.Project, core.SetExternalStateInput{Target: core.Ref(in.Ticket), ExternalState: state})
			if err != nil && ctx.Err() == nil {
				cfg.logf("set_external_state for %s: %v", in.Link.ID, err)
			}
		}
		if ctx.Err() != nil {
			return nil
		}
	}
	return nil
}

// followText updates in to its issue's title and body when either
// differs, and does nothing when neither does.
func (cfg Config) followText(ctx context.Context, in core.IncomingIssue, issue ghIssue) {
	up := core.UpdateLinkedTicketInput{Ticket: core.Ref(in.Ticket)}
	if issue.Title != in.Title {
		up.Title = &issue.Title
	}
	if issue.Body != in.Description {
		up.Description = &issue.Body
	}
	if up.Title == nil && up.Description == nil {
		return
	}
	if _, err := cfg.Client.UpdateLinkedTicket(ctx, cfg.Project, up); err != nil && ctx.Err() == nil {
		cfg.logf("update_linked_ticket for %s: %v", in.Link.ID, err)
	}
}

// issueNumber is the number of the issue Link ID id names, if it is one
// of repository's: LinkID's owner/repo#n, the repository matching
// whatever its case, as GitHub's names do.
func issueNumber(repository, id string) (int, bool) {
	repo, num, ok := strings.Cut(id, "#")
	if !ok || !strings.EqualFold(repo, repository) {
		return 0, false
	}
	n, err := strconv.Atoi(num)
	return n, err == nil
}

// readIssue reads repository's issue n through prog, gh, and its
// external state: closed, deleted (GitHub answers 410 Gone), out of
// scope if it was transferred to another repository (gh follows
// GitHub's redirect there, or it answers 404 Not Found where the login
// can't follow) or lacks label, when label isn't empty, and open
// otherwise. The issue is nil once it is deleted or transferred, as its
// text is no longer the repository's. GitHub rate-limiting the read is
// a *gh.RateLimitError.
func readIssue(ctx context.Context, env gh.Env, prog, repository, label string, n int) (*ghIssue, store.ExternalState, error) {
	body, msg, err := gh.API(ctx, env, prog, fmt.Sprintf("repos/%s/issues/%d", repository, n))
	if err != nil {
		var status *gh.StatusError
		if errors.As(err, &status) {
			switch status.Status {
			case http.StatusGone:
				return nil, store.ExternalDeleted, nil
			case http.StatusNotFound:
				return nil, store.ExternalOutOfScope, nil
			}
		}
		if msg != "" {
			err = fmt.Errorf("%w: %s", err, msg)
		}
		return nil, "", fmt.Errorf("reading issue %s: %w", LinkID(repository, n), err)
	}
	var i ghIssue
	if err := json.Unmarshal(body, &i); err != nil {
		return nil, "", fmt.Errorf("GitHub's issue %s can't be read: %w", LinkID(repository, n), err)
	}
	if !strings.EqualFold(repositoryOf(i.RepositoryURL), repository) {
		return nil, store.ExternalOutOfScope, nil
	}
	switch {
	case i.State == "closed":
		return &i, store.ExternalClosed, nil
	case label != "" && !i.hasLabel(label):
		return &i, store.ExternalOutOfScope, nil
	}
	return &i, store.ExternalOpen, nil
}

// hasLabel reports whether i has label, whatever its case, as GitHub's
// label names match.
func (i ghIssue) hasLabel(label string) bool {
	for _, l := range i.Labels {
		if strings.EqualFold(l.Name, label) {
			return true
		}
	}
	return false
}

// repositoryOf is the owner/repo of a repository's API URL, such as
// https://api.github.com/repos/acme/web.
func repositoryOf(apiURL string) string {
	_, repo, _ := strings.Cut(apiURL, "/repos/")
	return repo
}
