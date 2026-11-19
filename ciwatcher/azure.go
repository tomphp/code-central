// Azure Pipelines: the CI Watcher reaches Azure DevOps over its REST API
// (package azdo), with a PAT it reads from its own environment by the
// variable's name, so Code Central stores no PAT (ADR 0027).

package ciwatcher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/ratelimit"
)

// AzurePipelines is the Azure Pipelines CI Provider for one Azure DevOps
// project.
type AzurePipelines struct {
	// ProjectURL is the project URL, as the Project stores it.
	ProjectURL string
	// PATEnv names the environment variable holding the PAT.
	PATEnv string
	// Env is the CI Watcher's environment, which holds the PAT.
	Env gh.Env

	mu        sync.Mutex
	c         *azdo.Client
	timelines map[int]azureTimeline
}

// CheckAccess checks the PAT variable is set, reads the project, and
// lists one build. Azure DevOps throttling it is a *ratelimit.Error,
// not a problem. The execute scope can't be confirmed without retrying a
// run, so it isn't checked.
func (a *AzurePipelines) CheckAccess(ctx context.Context) (api.WatcherCheckResult, error) {
	c, r, err := a.access().Ready(a.ProjectURL)
	switch {
	case err != nil:
		return unreadable(a.ProjectURL, err), nil
	case r.Problem != "":
		return r, nil
	}
	if _, err := c.Project(ctx); err != nil {
		return a.refused(err)
	}
	if _, err := c.Builds(ctx, azdo.BuildQuery{Top: 1}); err != nil {
		return a.refused(err)
	}
	return api.WatcherCheckResult{}, nil
}

// refused is the check's answer to err from a call.
func (a *AzurePipelines) refused(err error) (api.WatcherCheckResult, error) {
	var limit *ratelimit.Error
	switch {
	case errors.As(err, &limit):
		return api.WatcherCheckResult{}, limit
	case errors.Is(err, azdo.ErrPATRejected):
		return a.access().Rejected(), nil
	case errors.Is(err, azdo.ErrMissingScope):
		return api.WatcherCheckResult{Problem: api.ProblemPATScope,
			Reason: fmt.Sprintf("The PAT in %s can't read builds in %s.", a.access().PATVariable(), a.ProjectURL),
			Fix:    "The PAT needs " + buildScope + ". Make a new PAT with that scope, and set it for the service."}, nil
	}
	return unreadable(a.ProjectURL, err), nil
}

// buildScope is the PAT scope the CI Watcher needs, as Azure DevOps
// names it.
const buildScope = "Build: Read & execute"

// access is how the CI Watcher reaches Azure DevOps.
func (a *AzurePipelines) access() azdo.Access {
	return azdo.Access{Environ: a.Env.Environ, PATEnv: a.PATEnv, Watcher: "CI Watcher", Scope: buildScope}
}

// unreadable is the project not found, or its host not reached.
func unreadable(projectURL string, err error) api.WatcherCheckResult {
	return api.WatcherCheckResult{Problem: api.ProblemProjectUnreadable,
		Reason: fmt.Sprintf("Code Central can't read the Azure DevOps project %s: %v", projectURL, err),
		Fix:    "Check the project URL is the one the browser shows, such as https://dev.azure.com/acme/web, and that the PAT's organization can see it."}
}

// azureRunLimit is how many of the project's latest builds RunsForCommit
// reads. The Builds API has no commit filter, and waiting commits are
// recent, so one page is enough: a build beyond it is missed, as GitHub
// Actions' 100-run limit misses one.
const azureRunLimit = 200

// azureTimeline is a build's timeline as last read, and when the build
// last changed, so an unchanged build costs no further timeline call.
type azureTimeline struct {
	changed time.Time
	// url is the build's web link.
	url     string
	records []azdo.TimelineRecord
}

// RunsForCommit lists the project's latest builds, newest queued first,
// and keeps those built from commit, whatever repository they built, and
// those of pull request pullRequest's merge ref, which count as built
// from commit.
func (a *AzurePipelines) RunsForCommit(ctx context.Context, commit string, pullRequest int) ([]Run, error) {
	mergeRef := ""
	if pullRequest > 0 {
		mergeRef = fmt.Sprintf("refs/pull/%d/merge", pullRequest)
	}
	builds, err := a.builds(ctx, azdo.BuildQuery{Top: azureRunLimit, Order: "queueTimeDescending"})
	if err != nil {
		return nil, err
	}
	var runs []Run
	for _, b := range builds {
		viaPR := mergeRef != "" && b.SourceBranch == mergeRef
		if b.SourceVersion != commit && !viaPR {
			continue
		}
		run, err := a.run(ctx, b)
		if err != nil {
			return nil, err
		}
		run.Commit = commit
		runs = append(runs, run)
	}
	return runs, nil
}

// CompletedRunsOnBranch lists the completed builds on refs/heads/branch
// that finished after since, newest first. Builds of other refs, such as
// pull requests and tags, never match.
func (a *AzurePipelines) CompletedRunsOnBranch(ctx context.Context, branch string, since time.Time) ([]Run, error) {
	builds, err := a.builds(ctx, azdo.BuildQuery{Order: "finishTimeDescending", Branch: azureBranchPrefix + branch, Status: "completed", MinTime: since})
	if err != nil {
		return nil, err
	}
	var runs []Run
	for _, b := range builds {
		if b.SourceBranch != azureBranchPrefix+branch || !b.FinishTime.After(since) {
			continue
		}
		run, err := a.run(ctx, b)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

const azureBranchPrefix = "refs/heads/"

// builds lists builds as q asks, through the client.
func (a *AzurePipelines) builds(ctx context.Context, q azdo.BuildQuery) ([]azdo.Build, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	return c.Builds(ctx, q)
}

// client is the Azure DevOps client, made once.
func (a *AzurePipelines) client() (*azdo.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.c == nil {
		c, err := azdo.New(a.Env.Environ, a.access().PATVariable(), a.ProjectURL, nil)
		if err != nil {
			return nil, err
		}
		a.c = c
	}
	return a.c, nil
}

// run maps build b to a CI Run, its attempt the highest among its
// timeline's Job records.
func (a *AzurePipelines) run(ctx context.Context, b azdo.Build) (Run, error) {
	records, err := a.timeline(ctx, b)
	if err != nil {
		return Run{}, err
	}
	attempt := 1
	for _, r := range records {
		if r.Type == "Job" {
			attempt = max(attempt, r.Attempt)
		}
	}
	run := Run{Updated: b.LastChanged, CIRunView: api.CIRunView{ID: int64(b.ID), Attempt: attempt, Workflow: b.Definition.Name, Name: b.BuildNumber,
		Branch: strings.TrimPrefix(b.SourceBranch, azureBranchPrefix), Commit: b.SourceVersion, Status: azureStatus(b.Status), URL: b.Links.Web.Href}}
	if run.Status == domain.CICompleted {
		run.Conclusion = azureConclusion(b.Result)
		if !b.FinishTime.IsZero() {
			at := b.FinishTime.UTC()
			run.Finished = &at
		}
	}
	if !b.StartTime.IsZero() {
		at := b.StartTime.UTC()
		run.Started = &at
	}
	return run, nil
}

// timeline is b's timeline records, read again only when the build has
// changed since they were.
func (a *AzurePipelines) timeline(ctx context.Context, b azdo.Build) ([]azdo.TimelineRecord, error) {
	a.mu.Lock()
	t, ok := a.timelines[b.ID]
	a.mu.Unlock()
	if ok && t.changed.Equal(b.LastChanged) {
		return t.records, nil
	}
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	records, err := c.Timeline(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.timelines == nil {
		a.timelines = map[int]azureTimeline{}
	}
	a.timelines[b.ID] = azureTimeline{changed: b.LastChanged, url: b.Links.Web.Href, records: records}
	return records, nil
}

// Jobs lists the build's Job records, each at its latest attempt, from
// the timeline run read, with the job's logs link.
func (a *AzurePipelines) Jobs(ctx context.Context, id int64, attempt int) ([]api.CIJobView, error) {
	a.mu.Lock()
	t, ok := a.timelines[int(id)]
	a.mu.Unlock()
	if !ok {
		c, err := a.client()
		if err != nil {
			return nil, err
		}
		records, err := c.Timeline(ctx, int(id))
		if err != nil {
			return nil, err
		}
		t = azureTimeline{records: records}
	}
	base := fmt.Sprintf("%s/_build/results?buildId=%d", a.ProjectURL, id)
	if t.url != "" {
		base = t.url
	}
	jobs := []api.CIJobView{}
	for _, r := range t.records {
		if r.Type != "Job" {
			continue
		}
		job := api.CIJobView{Name: r.Name, URL: base + "&view=logs&j=" + r.ID}
		if r.State == "completed" {
			job.Conclusion = azureJobConclusion(r.Result)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// azureStatus maps a build's status to Code Central's.
func azureStatus(s string) domain.CIRunStatus {
	switch s {
	case "completed":
		return domain.CICompleted
	case "inProgress", "cancelling":
		return domain.CIInProgress
	default:
		return domain.CIQueued
	}
}

// azureConclusion maps a build's result to Code Central's. Azure DevOps
// has no timed-out result: it reports one as failed or cancelled.
func azureConclusion(r string) domain.CIConclusion {
	switch r {
	case "succeeded":
		return domain.CISuccess
	case "partiallySucceeded":
		return domain.CIPartiallySucceeded
	case "canceled":
		return domain.CICancelled
	default:
		return domain.CIFailure
	}
}

// azureJobConclusion maps a job's result: a build's, plus the job-only
// ones.
func azureJobConclusion(r string) domain.CIConclusion {
	switch r {
	case "succeededWithIssues":
		return domain.CIPartiallySucceeded
	case "skipped":
		return domain.CISkipped
	case "abandoned":
		return domain.CICancelled
	default:
		return azureConclusion(r)
	}
}

// RerunFailedJobs asks Azure DevOps to retry the build, as a new attempt,
// which the next poll reports. Azure DevOps refusing it, a 403 when the PAT
// lacks execute among them, is returned for the CI Watcher to log and try
// again until the request lapses.
func (a *AzurePipelines) RerunFailedJobs(ctx context.Context, id int64) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	return c.Retry(ctx, int(id))
}
