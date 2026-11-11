// Write-backs: each poll, the Issue Watcher makes each linked issue what
// get_issue_watch says it should be, writing each comment once, then the
// status label, then closing it. It changes only what differs.

package issuewatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tomoram/code-central/watchers"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/store"
)

// labelColour is the colour a status label is created with.
const labelColour = "ededed"

// ghComment is what the Issue Watcher reads of a comment on an issue.
type ghComment struct {
	Body string `json:"body"`
}

// splitLinkID is the repository and number of the issue the Link ID id
// names, owner/repo#n.
func splitLinkID(id string) (repository string, n int, err error) {
	repository, num, ok := strings.Cut(id, "#")
	if n, err = strconv.Atoi(num); !ok || err != nil || n <= 0 {
		return "", 0, fmt.Errorf("the Link ID %q doesn't name an issue as owner/repo#number", id)
	}
	return repository, n, nil
}

// writeBack carries out each of watch's due writes. listed are the open
// issues this poll listed, so an open issue isn't read again. It
// returns GitHub's rate limit, if it refused a call for it; anything else
// that goes wrong for an issue is logged, and its writes are tried again
// on the next poll.
func (cfg Config) writeBack(ctx context.Context, watch core.IssueWatch, prog string, listed []ghIssue) *watchers.RateLimitError {
	open := map[string]ghIssue{}
	for _, i := range listed {
		open[LinkID(watch.Repository, i.Number)] = i
	}
	for _, w := range watch.Writes {
		err := cfg.writeIssue(ctx, prog, w, open)
		var limit *watchers.RateLimitError
		switch {
		case errors.As(err, &limit):
			return limit
		case ctx.Err() != nil:
			return nil
		case err != nil:
			cfg.logf("writing to %s for %s: %v", w.Link.ID, w.Target, err)
		}
	}
	return nil
}

// writeIssue carries out w's due writes on its issue: its comments, then
// its label, then closing it. open are the repository's open issues, by
// Link ID.
func (cfg Config) writeIssue(ctx context.Context, prog string, w core.IssueWrites, open map[string]ghIssue) error {
	repository, n, err := splitLinkID(w.Link.ID)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("repos/%s/issues/%d", repository, n)
	if err := cfg.writeComments(ctx, prog, path, w); err != nil {
		return err
	}
	if w.Label == "" && w.Close == "" {
		return nil
	}
	issue, ok := open[w.Link.ID]
	if !ok {
		if issue, err = cfg.fetchIssue(ctx, prog, path); err != nil {
			return err
		}
	}
	if w.Label != "" {
		if err := cfg.setLabel(ctx, prog, repository, path, w.Label, issue.Labels); err != nil {
			return err
		}
	}
	if w.Close == "" {
		return nil
	}
	if issue.State == "open" {
		if _, msg, err := gh.API(ctx, cfg.Env, prog, path, "--method", "PATCH", "-f", "state=closed", "-f", "state_reason="+string(w.Close)); err != nil {
			return apiError("closing the issue", msg, err)
		}
	}
	// The issue is closed: its label and closing are no longer due.
	_, err = cfg.Client.SetExternalState(ctx, cfg.Project, core.SetExternalStateInput{Target: core.Ref(w.Target), ExternalState: store.ExternalClosed})
	return err
}

// writeComments posts each of w's comments due on the issue at path, unless
// its marker shows it was posted before a crash, and reports each written.
func (cfg Config) writeComments(ctx context.Context, prog, path string, w core.IssueWrites) error {
	if len(w.Comments) == 0 {
		return nil
	}
	posted, err := cfg.comments(ctx, prog, path)
	if err != nil {
		return err
	}
	for _, c := range w.Comments {
		marker := core.IssueMarker(w.Target, c.Key)
		if !slices.ContainsFunc(posted, func(p ghComment) bool { return strings.Contains(p.Body, marker) }) {
			if _, msg, err := gh.API(ctx, cfg.Env, prog, path+"/comments", "--method", "POST", "-f", "body="+c.Body); err != nil {
				return apiError("posting the "+c.Key+" comment", msg, err)
			}
		}
		if _, err := cfg.Client.ReportIssueWritten(ctx, cfg.Project, core.ReportIssueWrittenInput{Target: core.Ref(w.Target), Key: c.Key}); err != nil {
			return fmt.Errorf("report_issue_written %s: %w", c.Key, err)
		}
	}
	return nil
}

// comments are every comment on the issue at path, oldest first.
func (cfg Config) comments(ctx context.Context, prog, path string) ([]ghComment, error) {
	var out []ghComment
	for page := 1; ; page++ {
		q := url.Values{"per_page": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(page)}}
		body, msg, err := gh.API(ctx, cfg.Env, prog, path+"/comments?"+q.Encode())
		if err != nil {
			return nil, apiError("listing the issue's comments", msg, err)
		}
		var items []ghComment
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("GitHub's list of the issue's comments can't be read: %w", err)
		}
		out = append(out, items...)
		if len(items) < perPage {
			return out, nil
		}
	}
}

// fetchIssue reads the issue at path, for one this poll didn't list.
func (cfg Config) fetchIssue(ctx context.Context, prog, path string) (ghIssue, error) {
	body, msg, err := gh.API(ctx, cfg.Env, prog, path)
	if err != nil {
		return ghIssue{}, apiError("reading the issue", msg, err)
	}
	var i ghIssue
	if err := json.Unmarshal(body, &i); err != nil {
		return ghIssue{}, fmt.Errorf("GitHub's answer for the issue can't be read: %w", err)
	}
	return i, nil
}

// setLabel makes label the only status label on the issue at path, which
// has labels: it creates label in repository if it is missing, applies
// it, and removes the other status labels.
func (cfg Config) setLabel(ctx context.Context, prog, repository, path, label string, labels []ghLabel) error {
	if !slices.ContainsFunc(labels, func(l ghLabel) bool { return l.Name == label }) {
		if err := cfg.createLabel(ctx, prog, repository, label); err != nil {
			return err
		}
		if _, msg, err := gh.API(ctx, cfg.Env, prog, path+"/labels", "--method", "POST", "-f", "labels[]="+label); err != nil {
			return apiError("labelling the issue "+label, msg, err)
		}
	}
	for _, l := range labels {
		if l.Name != label && strings.HasPrefix(l.Name, core.StatusLabelPrefix) {
			if _, msg, err := gh.API(ctx, cfg.Env, prog, path+"/labels/"+url.PathEscape(l.Name), "--method", "DELETE"); err != nil && !notFound(err) {
				return apiError("removing the label "+l.Name, msg, err)
			}
		}
	}
	return nil
}

// createLabel creates label in repository if it isn't there.
func (cfg Config) createLabel(ctx context.Context, prog, repository, label string) error {
	_, msg, err := gh.API(ctx, cfg.Env, prog, "repos/"+repository+"/labels/"+url.PathEscape(label))
	if err == nil {
		return nil
	}
	if !notFound(err) {
		return apiError("reading the label "+label, msg, err)
	}
	if _, msg, err := gh.API(ctx, cfg.Env, prog, "repos/"+repository+"/labels", "--method", "POST",
		"-f", "name="+label, "-f", "color="+labelColour, "-f", "description=Where Code Central's work on this issue stands"); err != nil {
		return apiError("creating the label "+label, msg, err)
	}
	return nil
}

// apiError is a failed gh api call doing what, with what gh said, keeping
// a rate limit recognisable.
func apiError(what, msg string, err error) error {
	if msg != "" {
		return fmt.Errorf("%s: %w: %s", what, err, msg)
	}
	return fmt.Errorf("%s: %w", what, err)
}

// notFound reports whether err is GitHub answering 404 Not Found.
func notFound(err error) bool {
	var status *gh.StatusError
	return errors.As(err, &status) && status.Status == http.StatusNotFound
}
