// GitHub Actions, the only CI Provider: the CI Watcher reaches GitHub
// through gh, the GitHub CLI, and its own login (package gh), so Code
// Central stores no GitHub token.

package ciwatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/store"
)

// GitHubActions is the GitHub Actions CI Provider for one repository.
type GitHubActions struct {
	// Repository is owner/name.
	Repository string
	// GHPath is the gh path Option: gh itself, or the folder it is in;
	// "" finds it on PATH, then in the install places.
	GHPath string
	Env    gh.Env
}

// CheckAccess finds gh, checks its login with gh auth status, and reads
// the repository. GitHub rate-limiting the read is a *gh.RateLimitError,
// not a problem.
func (g *GitHubActions) CheckAccess(ctx context.Context) (core.WatcherCheckResult, error) {
	prog, missing := gh.Find(g.GHPath, g.Env)
	if missing != nil {
		return *missing, nil
	}
	if r, err := gh.CheckLogin(ctx, g.Env, prog); r.Problem != "" || err != nil {
		return r, err
	}
	if _, msg, err := gh.API(ctx, g.Env, prog, "repos/"+g.Repository, "--jq", ".full_name"); err != nil {
		var limit *gh.RateLimitError
		if errors.As(err, &limit) {
			return core.WatcherCheckResult{}, limit
		}
		if r, ran := gh.DidntRun(prog, err); !ran {
			return r, nil
		}
		return core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable, Reason: fmt.Sprintf("gh can't read the repository %s: %s", g.Repository, msg),
			Fix: "Check the repository is the owner/name GitHub shows, and that the account gh is logged in as can see it."}, nil
	}
	return core.WatcherCheckResult{}, nil
}

// ghRun is a workflow run as GitHub's REST API gives it.
type ghRun struct {
	ID         int64     `json:"id"`
	Attempt    int       `json:"run_attempt"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Branch     string    `json:"head_branch"`
	Commit     string    `json:"head_sha"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Started    time.Time `json:"run_started_at"`
	Updated    time.Time `json:"updated_at"`
	URL        string    `json:"html_url"`
}

// ghJob is one job of a run as GitHub's REST API gives it.
type ghJob struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"html_url"`
}

// RunsForCommit lists the workflow runs GitHub started for commit.
func (g *GitHubActions) RunsForCommit(ctx context.Context, commit string) ([]Run, error) {
	return g.runs(ctx, fmt.Sprintf("repos/%s/actions/runs?head_sha=%s&per_page=100", g.Repository, url.QueryEscape(commit)))
}

// CompletedRunsOnBranch lists the completed workflow runs on branch that
// finished after since. GitHub lists the newest first, and filters by
// when a run was created, not finished, so this takes the latest 100
// and keeps those that finished after since: more finishing between two
// polls, or while the CI Watcher is down, would be missed.
func (g *GitHubActions) CompletedRunsOnBranch(ctx context.Context, branch string, since time.Time) ([]Run, error) {
	runs, err := g.runs(ctx, fmt.Sprintf("repos/%s/actions/runs?branch=%s&status=completed&per_page=100", g.Repository, url.QueryEscape(branch)))
	if err != nil {
		return nil, err
	}
	var out []Run
	for _, r := range runs {
		if r.Branch == branch && r.Finished != nil && r.Finished.After(since) {
			out = append(out, r)
		}
	}
	return out, nil
}

// runs lists the workflow runs at path, a query of GitHub's REST API.
func (g *GitHubActions) runs(ctx context.Context, path string) ([]Run, error) {
	var out struct {
		Runs []ghRun `json:"workflow_runs"`
	}
	if err := g.api(ctx, path, &out); err != nil {
		return nil, err
	}
	var runs []Run
	for _, r := range out.Runs {
		run := Run{Updated: r.Updated, CIRunView: core.CIRunView{ID: r.ID, Attempt: max(r.Attempt, 1), Workflow: workflowName(r.Path), Name: r.Name,
			Branch: r.Branch, Commit: r.Commit, Status: runStatus(r.Status), URL: r.URL}}
		if run.Status == store.CICompleted {
			run.Conclusion = conclusion(r.Conclusion)
			if !r.Updated.IsZero() {
				at := r.Updated.UTC()
				run.Finished = &at
			}
		}
		if !r.Started.IsZero() {
			at := r.Started.UTC()
			run.Started = &at
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// Jobs lists the jobs of one attempt of a run.
func (g *GitHubActions) Jobs(ctx context.Context, id int64, attempt int) ([]core.CIJobView, error) {
	var out struct {
		Jobs []ghJob `json:"jobs"`
	}
	if err := g.api(ctx, fmt.Sprintf("repos/%s/actions/runs/%d/attempts/%d/jobs?per_page=100", g.Repository, id, attempt), &out); err != nil {
		return nil, err
	}
	jobs := []core.CIJobView{}
	for _, j := range out.Jobs {
		job := core.CIJobView{Name: j.Name, URL: j.URL}
		if runStatus(j.Status) == store.CICompleted {
			job.Conclusion = conclusion(j.Conclusion)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// RerunFailedJobs asks GitHub to re-run the failed jobs of a run, which
// starts its next attempt.
func (g *GitHubActions) RerunFailedJobs(ctx context.Context, id int64) error {
	prog, missing := gh.Find(g.GHPath, g.Env)
	if missing != nil {
		return errors.New(missing.Reason)
	}
	path := fmt.Sprintf("repos/%s/actions/runs/%d/rerun-failed-jobs", g.Repository, id)
	if _, msg, err := gh.API(ctx, g.Env, prog, path, "--method", "POST"); err != nil {
		return fmt.Errorf("gh api %s: %w: %s", path, err, msg)
	}
	return nil
}

// api reads path from GitHub's REST API through gh into out.
func (g *GitHubActions) api(ctx context.Context, path string, out any) error {
	prog, missing := gh.Find(g.GHPath, g.Env)
	if missing != nil {
		return errors.New(missing.Reason)
	}
	body, msg, err := gh.API(ctx, g.Env, prog, path)
	if err != nil {
		return fmt.Errorf("gh api %s: %w: %s", path, err, msg)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("gh api %s: %v", path, err)
	}
	return nil
}

// workflowName is a workflow's file base name from the run's workflow
// path, such as .github/workflows/ci.yml, which may name a ref after @.
func workflowName(p string) string {
	p, _, _ = strings.Cut(p, "@")
	return path.Base(p)
}

// runStatus maps a GitHub run or job status to Code Central's: requested,
// waiting and pending are all still queued.
func runStatus(s string) store.CIRunStatus {
	switch s {
	case "completed":
		return store.CICompleted
	case "in_progress":
		return store.CIInProgress
	default:
		return store.CIQueued
	}
}

// conclusion maps a GitHub conclusion to Code Central's: a run that
// needed an action or failed to start failed, and a stale one was
// cancelled.
func conclusion(c string) store.CIConclusion {
	switch c {
	case "success":
		return store.CISuccess
	case "cancelled", "stale":
		return store.CICancelled
	case "timed_out":
		return store.CITimedOut
	case "skipped":
		return store.CISkipped
	case "neutral":
		return store.CINeutral
	default:
		return store.CIFailure
	}
}
