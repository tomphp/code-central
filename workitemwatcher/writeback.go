// Write-backs: each poll, the Work Item Watcher makes each linked work
// item what get_work_item_watch says it should be, writing each comment
// once, then the status tag, then the State Work Item States maps, once
// for each phase. It changes only what differs.

package workitemwatcher

import (
	"context"
	"errors"
	"fmt"
	"github.com/tomoram/code-central/domain"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/azdo"
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
// then its tag, then its State.
func (cfg Config) writeItem(ctx context.Context, c *azdo.Client, id int, w api.WorkItemWrites) error {
	item, err := c.WorkItemToWrite(ctx, id)
	if err != nil {
		return err
	}
	edited, err := cfg.writeComments(ctx, c, item, w)
	if err != nil {
		return err
	}
	if w.Tag != "" {
		if item, err = reread(ctx, c, item, edited); err != nil {
			return err
		}
		if edited, err = writeTag(ctx, c, item, w.Tag); err != nil {
			return err
		}
	}
	if w.State == nil {
		return nil
	}
	if item, err = reread(ctx, c, item, edited); err != nil {
		return err
	}
	return cfg.writeState(ctx, c, item, w)
}

// reread is item read again when it was edited since it was read, as each
// edit makes its revision stale.
func reread(ctx context.Context, c *azdo.Client, item azdo.WorkItem, edited bool) (azdo.WorkItem, error) {
	if !edited {
		return item, nil
	}
	return c.WorkItemToWrite(ctx, item.ID)
}

// writeTag makes tag item's status tag, reporting whether it changed it.
func writeTag(ctx context.Context, c *azdo.Client, item azdo.WorkItem, tag string) (edited bool, err error) {
	tags, changed := withStatusTag(item.Tags(), tag)
	if !changed {
		return false, nil
	}
	return true, c.SetTags(ctx, item.ID, item.Rev, tags)
}

// writeState moves item to w's State, unless it is in it already, as
// compared ignoring case, then reports its key and its nearer phases'
// keys written.
func (cfg Config) writeState(ctx context.Context, c *azdo.Client, item azdo.WorkItem, w api.WorkItemWrites) error {
	if !strings.EqualFold(item.Fields.State, w.State.Name) {
		if err := c.SetState(ctx, item.ID, item.Rev, w.State.Name); err != nil {
			return fmt.Errorf("setting the State %s: %w", w.State.Name, err)
		}
	}
	for _, key := range append([]string{w.State.Key}, w.State.Nearer...) {
		if _, err := cfg.Client.ReportIssueWritten(ctx, cfg.Project, api.ReportIssueWrittenInput{Target: api.Ref(w.Target), Key: key}); err != nil {
			return fmt.Errorf("report_issue_written %s: %w", key, err)
		}
	}
	return nil
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
		branded, opaque := domain.IssueMarker(w.Target, cm.Key), domain.OpaqueMarker(w.Target, cm.Key)
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
// that starts with domain.StatusLabelPrefix is removed, and tag is added if
// it isn't there. Tags are the same whatever their case, as Azure DevOps
// takes them. changed is false when tags already are so.
func withStatusTag(tags []string, tag string) (out []string, changed bool) {
	has := false
	for _, t := range tags {
		switch {
		case strings.EqualFold(t, tag):
			has = true
			out = append(out, t)
		case strings.HasPrefix(strings.ToLower(t), domain.StatusLabelPrefix):
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
