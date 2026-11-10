// GitHub Actions, the only CI Provider: the CI Watcher reaches GitHub
// through gh, the GitHub CLI, and its own login, so Code Central stores
// no GitHub token.

package ciwatcher

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// GitHubActions is the GitHub Actions CI Provider for one repository.
type GitHubActions struct {
	// Repository is owner/name.
	Repository string
	// GHPath is the gh path Option: gh itself, or the folder it is in;
	// "" finds it on PATH, then in the install places.
	GHPath string
	Env    Env
}

// DefaultInstallPlaces are where gh's installers put it: Homebrew on
// Apple silicon and Intel Macs, Homebrew on Linux, and the Windows
// installer under %ProgramFiles%.
func DefaultInstallPlaces(getenv func(string) string) []string {
	places := []string{"/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin"}
	if pf := getenv("ProgramFiles"); pf != "" {
		places = append(places, filepath.Join(pf, "GitHub CLI"))
	}
	return places
}

// ghMissing is the problem of a gh found nowhere it was looked for.
func ghMissing(reason string) core.CICheckResult {
	return core.CICheckResult{Problem: core.CIGHMissing, Reason: reason,
		Fix: "Install gh, the GitHub CLI (https://cli.github.com), or set the gh path Option to where it is."}
}

// FindGH finds gh: the gh path Option if set, else on PATH, else in the
// install places. Not finding it is the gh missing problem.
func (g *GitHubActions) FindGH() (string, *core.CICheckResult) {
	if g.GHPath != "" {
		if gh, ok := isProgram(g.GHPath); ok {
			return gh, nil
		}
		if gh, ok := ghIn(g.GHPath); ok {
			return gh, nil
		}
		r := ghMissing(fmt.Sprintf("The gh path Option is %s, but gh isn't there.", g.GHPath))
		r.Fix = "Set the gh path Option to where gh is, or clear it to find gh on the service's PATH."
		return "", &r
	}
	for _, dir := range filepath.SplitList(g.Env.getenv("PATH")) {
		if gh, ok := ghIn(dir); ok {
			return gh, nil
		}
	}
	places := g.Env.InstallPlaces
	if places == nil {
		places = DefaultInstallPlaces(g.Env.getenv)
	}
	for _, dir := range places {
		if gh, ok := ghIn(dir); ok {
			return gh, nil
		}
	}
	r := ghMissing(fmt.Sprintf("gh, the GitHub CLI, isn't on the service's PATH or in %s.", strings.Join(places, ", ")))
	return "", &r
}

// ghIn is gh in the folder dir, if it is there.
func ghIn(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	return isProgram(filepath.Join(dir, "gh"+exeSuffix))
}

// isProgram reports whether path is a program that can be run.
func isProgram(path string) (string, bool) {
	fi, err := os.Stat(path)
	return path, err == nil && !fi.IsDir() && executable(fi)
}

// CheckAccess finds gh, checks its login with gh auth status, and reads
// the repository. GitHub rate-limiting the read is a *RateLimitError, not
// a problem.
func (g *GitHubActions) CheckAccess(ctx context.Context) (core.CICheckResult, error) {
	gh, missing := g.FindGH()
	if missing != nil {
		return *missing, nil
	}
	if out, err := g.run(ctx, gh, "auth", "status", "--hostname", "github.com"); err != nil {
		if r, ran := didntRun(gh, err); !ran {
			return r, nil
		}
		// gh auth status calls GitHub too, and says so when that is
		// rate-limited, with no reset time.
		if strings.Contains(strings.ToLower(out), "rate limit") {
			return core.CICheckResult{}, &RateLimitError{Message: rateLimitLine(out)}
		}
		return core.CICheckResult{Problem: core.CILoggedOut, Reason: "gh isn't logged in to GitHub: " + firstLine(out),
			Fix: "Run `gh auth login` as the user the service runs as."}, nil
	}
	if _, msg, err := g.callAPI(ctx, gh, "repos/"+g.Repository, "--jq", ".full_name"); err != nil {
		var limit *RateLimitError
		if errors.As(err, &limit) {
			return core.CICheckResult{}, limit
		}
		if r, ran := didntRun(gh, err); !ran {
			return r, nil
		}
		return core.CICheckResult{Problem: core.CIRepositoryUnreadable, Reason: fmt.Sprintf("gh can't read the repository %s: %s", g.Repository, msg),
			Fix: "Check the repository is the owner/name GitHub shows, and that the account gh is logged in as can see it."}, nil
	}
	return core.CICheckResult{}, nil
}

// run runs gh with args, never prompting, and returns its output:
// stderr's when it fails, as gh writes its errors there.
func (g *GitHubActions) run(ctx context.Context, gh string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, gh, args...)
	cmd.Env = append(append([]string{}, g.Env.Environ...), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	out := stdout.String()
	if err != nil && strings.TrimSpace(stderr.String()) != "" {
		out = stderr.String()
	}
	return out, err
}

// rateLimitLine is the line of out that names the rate limit.
func rateLimitLine(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(l), "rate limit") {
			return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-X "))
		}
	}
	return firstLine(out)
}

// didntRun is the check failing because gh didn't run at all, rather
// than running and saying no; ran is true when it did run.
func didntRun(gh string, err error) (r core.CICheckResult, ran bool) {
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.Exited() {
		return r, true
	}
	return core.CICheckResult{Problem: core.CICheckFailed, Reason: fmt.Sprintf("gh at %s didn't run: %v", gh, err),
		Fix: "Check gh runs for the user the service runs as, or set the gh path Option to one that does."}, false
}

// firstLine is s's first line with text on it, trimmed.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
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
	var out struct {
		Runs []ghRun `json:"workflow_runs"`
	}
	if err := g.api(ctx, fmt.Sprintf("repos/%s/actions/runs?head_sha=%s&per_page=100", g.Repository, url.QueryEscape(commit)), &out); err != nil {
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
	gh, missing := g.FindGH()
	if missing != nil {
		return errors.New(missing.Reason)
	}
	path := fmt.Sprintf("repos/%s/actions/runs/%d/rerun-failed-jobs", g.Repository, id)
	if _, msg, err := g.callAPI(ctx, gh, path, "--method", "POST"); err != nil {
		return fmt.Errorf("gh api %s: %w: %s", path, err, msg)
	}
	return nil
}

// api reads path from GitHub's REST API through gh into out.
func (g *GitHubActions) api(ctx context.Context, path string, out any) error {
	gh, missing := g.FindGH()
	if missing != nil {
		return errors.New(missing.Reason)
	}
	body, msg, err := g.callAPI(ctx, gh, path)
	if err != nil {
		return fmt.Errorf("gh api %s: %w: %s", path, err, msg)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("gh api %s: %v", path, err)
	}
	return nil
}

// callAPI runs gh api path with args, never prompting, and with --include,
// so the response's headers say when a rate limit ends. It returns the
// response's body and, when it fails, the first line gh wrote to stderr;
// GitHub refusing it for a rate limit is a *RateLimitError.
func (g *GitHubActions) callAPI(ctx context.Context, gh, path string, args ...string) (body []byte, msg string, err error) {
	cmd := exec.CommandContext(ctx, gh, append(append([]string{"api", path}, args...), "--include")...)
	cmd.Env = append(append([]string{}, g.Env.Environ...), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	status, header, body := response(stdout.Bytes())
	if err == nil {
		return body, "", nil
	}
	msg = firstLine(stderr.String())
	if limit := rateLimit(status, header, msg); limit != nil {
		return nil, msg, limit
	}
	return nil, msg, err
}

// response splits what gh api --include prints into the status, the
// headers and the body. Output with no status line is all body.
func response(out []byte) (status int, header http.Header, body []byte) {
	if !bytes.HasPrefix(out, []byte("HTTP/")) {
		return 0, http.Header{}, out
	}
	r := bufio.NewReader(bytes.NewReader(out))
	tp := textproto.NewReader(r)
	line, err := tp.ReadLine()
	if err != nil {
		return 0, http.Header{}, out
	}
	if _, code, ok := strings.Cut(line, " "); ok {
		code, _, _ = strings.Cut(code, " ")
		status, _ = strconv.Atoi(code)
	}
	h, err := tp.ReadMIMEHeader()
	if err != nil && !errors.Is(err, io.EOF) {
		return status, http.Header{}, out
	}
	body, _ = io.ReadAll(r)
	return status, http.Header(h), body
}

// RateLimitError is GitHub refusing a call for its rate limit.
type RateLimitError struct {
	// Reset is when GitHub said the limit ends, and RetryAfter how long
	// it said to wait; both are zero when it said neither.
	Reset      time.Time
	RetryAfter time.Duration
	// Message is what gh said.
	Message string
}

func (e *RateLimitError) Error() string { return "GitHub is rate-limiting gh: " + e.Message }

// Until is when the CI Watcher may call GitHub again, from now; zero
// when GitHub didn't say.
func (e *RateLimitError) Until(now time.Time) time.Time {
	if e.RetryAfter > 0 {
		return now.Add(e.RetryAfter)
	}
	return e.Reset
}

// rateLimit is the rate limit a failed call's answer says GitHub refused
// it for, or nil: a 403 or 429 with no requests remaining, or one saying
// how long to wait, or a message naming a rate limit, as GitHub's
// secondary rate limits may give with neither.
func rateLimit(status int, header http.Header, msg string) *RateLimitError {
	if status != 0 && status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return nil
	}
	limit := &RateLimitError{Message: msg}
	limited := strings.Contains(strings.ToLower(msg), "rate limit")
	if secs, err := strconv.Atoi(header.Get("Retry-After")); err == nil && secs > 0 {
		limit.RetryAfter, limited = time.Duration(secs)*time.Second, true
	}
	if header.Get("X-Ratelimit-Remaining") == "0" {
		limited = true
		if reset, err := strconv.ParseInt(header.Get("X-Ratelimit-Reset"), 10, 64); err == nil && reset > 0 {
			limit.Reset = time.Unix(reset, 0).UTC()
		}
	}
	if !limited {
		return nil
	}
	return limit
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
