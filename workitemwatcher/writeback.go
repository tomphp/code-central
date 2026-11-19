// Write-backs: each poll, the Work Item Watcher makes each linked work
// item what get_work_item_watch says it should be, writing each comment
// once, then the status tag. It changes only what differs, and never a
// work item's State, which is the human's to move.

package workitemwatcher

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ratelimit"
)

// writeBack carries out each of watch's due writes. It returns Azure
// DevOps' rate limit, if it refused a call for it, or an error wrapping
// azdo.ErrMissingScope when the PAT can't write work items; anything else
// that goes wrong for a work item is logged, and its writes are tried
// again on the next poll.
func (cfg Config) writeBack(ctx context.Context, q queryRun, watch api.WorkItemWatch) error {
	for _, w := range watch.Writes {
		id, ours := workItemID(q.org, w.Link.ID)
		if !ours {
			continue
		}
		err := cfg.writeItem(ctx, q.c, id, w)
		var limit *ratelimit.Error
		switch {
		case ctx.Err() != nil:
			return nil
		case errors.As(err, &limit), errors.Is(err, azdo.ErrMissingScope):
			return err
		case err != nil:
			cfg.logf("writing to %s for %s: %v", w.Link.ID, w.Target, err)
		}
	}
	return nil
}

// writeItem carries out w's due writes on work item id: its comments,
// then its tag.
func (cfg Config) writeItem(ctx context.Context, c *azdo.Client, id int, w api.WorkItemWrites) error {
	item, err := c.WorkItemToWrite(ctx, id)
	if err != nil {
		return err
	}
	posted, err := cfg.writeComments(ctx, c, item, w)
	if err != nil {
		return err
	}
	if w.Tag == "" {
		return nil
	}
	if posted {
		// Posting a comment is an edit: the revision read is stale.
		if item, err = c.WorkItemToWrite(ctx, id); err != nil {
			return err
		}
	}
	tags, changed := withStatusTag(item.Tags(), w.Tag)
	if !changed {
		return nil
	}
	return c.SetTags(ctx, id, item.Rev, tags)
}

// writeComments posts each of w's comments due on item, unless its
// branded or opaque marker shows it was posted before a crash, and
// reports each written. It reports whether it posted any.
func (cfg Config) writeComments(ctx context.Context, c *azdo.Client, item azdo.WorkItem, w api.WorkItemWrites) (posted bool, err error) {
	if len(w.Comments) == 0 {
		return false, nil
	}
	existing, err := c.Comments(ctx, item.Fields.TeamProject, item.ID)
	if err != nil {
		return false, err
	}
	for _, cm := range w.Comments {
		// Either marker, whatever the setting: it may have changed between
		// posting a comment and reporting it written.
		branded, opaque := core.IssueMarker(w.Target, cm.Key), core.OpaqueMarker(w.Target, cm.Key)
		if !slices.ContainsFunc(existing, func(text string) bool { return strings.Contains(text, branded) || strings.Contains(text, opaque) }) {
			if err := c.AddComment(ctx, item.Fields.TeamProject, item.ID, cm.Body); err != nil {
				return posted, fmt.Errorf("posting the %s comment: %w", cm.Key, err)
			}
			posted = true
		}
		if _, err := cfg.Client.ReportIssueWritten(ctx, cfg.Project, api.ReportIssueWrittenInput{Target: api.Ref(w.Target), Key: cm.Key}); err != nil {
			return posted, fmt.Errorf("report_issue_written %s: %w", cm.Key, err)
		}
	}
	return posted, nil
}

// withStatusTag is tags with tag the only status tag: every other tag
// that starts with core.StatusLabelPrefix is removed, and tag is added if
// it isn't there. Tags are the same whatever their case, as Azure DevOps
// takes them. changed is false when tags already are so.
func withStatusTag(tags []string, tag string) (out []string, changed bool) {
	has := false
	for _, t := range tags {
		switch {
		case strings.EqualFold(t, tag):
			has = true
			out = append(out, t)
		case strings.HasPrefix(strings.ToLower(t), core.StatusLabelPrefix):
			changed = true
		default:
			out = append(out, t)
		}
	}
	if !has {
		out, changed = append(out, tag), true
	}
	return out, changed
}

// writeScopeProblem is the PAT refused for writing work items in the
// project of q.
func writeScopeProblem(watch api.WorkItemWatch, q queryRun) api.WatcherCheckResult {
	return api.WatcherCheckResult{Problem: api.ProblemPATScope,
		Reason: fmt.Sprintf("The PAT in %s can read work items but Azure DevOps refused it writing a comment or tag to one in %s.", watch.PATEnv, q.c.ProjectURL()),
		Fix:    fmt.Sprintf("The PAT needs %s. Make a new PAT with that scope, and set it in %s for the service, then restart the Code Central service.", scope, watch.PATEnv)}
}
