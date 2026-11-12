package core

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// Tracking a pushed Ticket's CI (Spec code-central-247): the Factory
// reports each pushed commit on its Ticket with report_pushed_commit, the
// CI Watcher reports the runs GitHub started for it with report_ci_runs,
// and Code Central works the Ticket's CI result out on read.

// CIResult is a Ticket's CI result, from the runs of its pushed commits
// that aren't ignored: under the CI gate passed once any pushed since it
// last moved back to Ready passed, otherwise its latest pushed commit's.
type CIResult string

// The CI results.
const (
	// CIRunning is a pushed commit with a run still going, or none
	// reported yet.
	CIRunning CIResult = "running"
	// CIPassed is a pushed commit whose runs all finished, none failing.
	CIPassed CIResult = "passed"
	// CIFailed is a pushed commit with a run that concluded failure or
	// timed out.
	CIFailed CIResult = "failed"
	// CICancelled is a pushed commit with a cancelled run and none
	// failed.
	CICancelled CIResult = "cancelled"
)

// ciNewlyPushed is how long after its push a pushed commit is still
// watched once its runs have all finished: GitHub may start a workflow
// after another has already finished.
const ciNewlyPushed = 5 * time.Minute

// ciWaitLimit is how long after its push a pushed commit is watched at
// most: one GitHub never starts a run for, say in a repository with no
// workflows, would otherwise be watched every 30 seconds for good.
const ciWaitLimit = 24 * time.Hour

// ciKeep is how long after it finished a CI Run is kept: older ones are
// pruned from the store on the CI Watcher's next report, and every read
// leaves them out from the moment they are due.
const ciKeep = 30 * 24 * time.Hour

// keptCIRuns is runs without those finished more than ciKeep before now.
func keptCIRuns(runs []store.CIRun, now time.Time) []store.CIRun {
	return slices.DeleteFunc(runs, func(run store.CIRun) bool {
		ended := ciRunEnded(run)
		return !ended.IsZero() && now.Sub(ended) > ciKeep
	})
}

// ciRunEnded is when a completed run finished, or, when the CI Provider
// gave no finish time, when it started; zero for an unfinished run.
func ciRunEnded(run store.CIRun) time.Time {
	if run.Status != store.CICompleted {
		return time.Time{}
	}
	if !run.Finished.IsZero() {
		return run.Finished
	}
	return run.Started
}

// PushedCommitView is a commit the Factory pushed for a Ticket, and when
// Code Central heard of it.
type PushedCommitView struct {
	Commit string    `json:"commit"`
	Pushed time.Time `json:"pushed"`
}

// CIJobView is one job of a CI Run; its conclusion is empty until it
// ends.
type CIJobView struct {
	Name       string             `json:"name"`
	Conclusion store.CIConclusion `json:"conclusion,omitempty"`
	URL        string             `json:"url"`
}

// CIRunView is one CI Run as reads return it, and as report_ci_runs
// takes it: the CI Watcher names what it is tracked against, a Ticket or
// a CI Rule's Spec; Code Central marks it ignored.
type CIRunView struct {
	// ID and Attempt are the CI Provider's run ID and attempt, which key
	// the run.
	ID      int64 `json:"id"`
	Attempt int   `json:"attempt"`
	// Workflow is the workflow file's base name, such as ci.yml; Name is
	// the name it shows.
	Workflow   string             `json:"workflow"`
	Name       string             `json:"name"`
	Branch     string             `json:"branch"`
	Commit     string             `json:"commit"`
	Status     store.CIRunStatus  `json:"status"`
	Conclusion store.CIConclusion `json:"conclusion,omitempty"`
	Started    *time.Time         `json:"started,omitempty"`
	Finished   *time.Time         `json:"finished,omitempty"`
	URL        string             `json:"url"`
	Jobs       []CIJobView        `json:"jobs"`
	// Ticket or Rule is what the run is tracked against: a Ticket's ID,
	// or the ID of a CI Rule's Spec.
	Ticket string `json:"ticket,omitempty"`
	Rule   string `json:"rule,omitempty"`
	// Ignored marks a run of an Ignored workflow, which never counts
	// towards a CI result; Code Central sets it.
	Ignored bool `json:"ignored"`
}

// ciRunView is r as reads return it.
func ciRunView(key string, r store.CIRun) CIRunView {
	v := CIRunView{ID: r.ID, Attempt: r.Attempt, Workflow: r.Workflow, Name: r.Name, Branch: r.Branch, Commit: r.Commit,
		Status: r.Status, Conclusion: r.Conclusion, URL: r.URL, Jobs: []CIJobView{}, Ignored: r.Ignored}
	if !r.Started.IsZero() {
		at := r.Started
		v.Started = &at
	}
	if !r.Finished.IsZero() {
		at := r.Finished
		v.Finished = &at
	}
	for _, j := range r.Jobs {
		v.Jobs = append(v.Jobs, CIJobView{Name: j.Name, Conclusion: j.Conclusion, URL: j.URL})
	}
	if r.Ticket != 0 {
		v.Ticket = DisplayID(key, r.Ticket)
	}
	if r.Rule != 0 {
		v.Rule = DisplayID(key, r.Rule)
	}
	return v
}

// ticketCI is a Ticket's latest pushed commit, its CI result, the pushed
// commit the result comes from and every CI Run tracked against it, newest
// pushed commit's first; all empty for a Ticket never pushed while CI
// Tracking was on.
func ticketCI(r store.Reader, key string, t store.Ticket, now time.Time) (*PushedCommitView, CIResult, string, []CIRunView) {
	mine, ok := ticketRuns(r, key, t, now)
	if !ok {
		return nil, "", "", nil
	}
	latest := t.PushedCommits[len(t.PushedCommits)-1]
	// The latest pushed commit's runs first, then each older one's.
	order := func(commit string) int {
		return len(t.PushedCommits) - 1 - slices.IndexFunc(t.PushedCommits, func(p store.PushedCommit) bool { return p.Commit == commit })
	}
	slices.SortStableFunc(mine, func(a, b store.CIRun) int { return order(a.Commit) - order(b.Commit) })
	var runs []CIRunView
	for _, run := range mine {
		runs = append(runs, ciRunView(key, run))
	}
	from, result := ticketResult(r, key, t, mine, now)
	return &PushedCommitView{Commit: latest.Commit, Pushed: latest.Pushed}, result, from, runs
}

// ticketCIResult is t's CI result alone, for its rows' badges: "" for a
// Ticket never pushed while CI Tracking was on.
func ticketCIResult(r store.Reader, key string, t store.Ticket, now time.Time) CIResult {
	mine, ok := ticketRuns(r, key, t, now)
	if !ok {
		return ""
	}
	_, result := ticketResult(r, key, t, mine, now)
	return result
}

// ticketResult is t's CI result from its runs at now, and the commit it
// comes from: the newest of its result commits that passed, if any has,
// otherwise its latest pushed commit (code-central-430); no commit while
// it has no CI result. t must have a pushed commit.
func ticketResult(r store.Reader, key string, t store.Ticket, runs []store.CIRun, now time.Time) (commit string, result CIResult) {
	p, _ := r.Project(key)
	for _, pc := range resultCommits(p.LandingPolicy().CIGateHolds(), t) {
		if pushedCIResult(pc, runs, now) == CIPassed {
			return pc.Commit, CIPassed
		}
	}
	latest := latestPushed(t)
	if result = latestCIResult(latest, runs, now); result == "" {
		return "", ""
	}
	return latest.Commit, result
}

// resultCommits are the pushed commits t's CI result reads, newest first:
// under the CI gate those pushed since it last moved back to Ready, any
// of which passing passes t; otherwise, or when it has pushed none since,
// its latest. t must have a pushed commit.
func resultCommits(gate bool, t store.Ticket) []store.PushedCommit {
	var out []store.PushedCommit
	if gate {
		since := backToReady(t)
		for _, pc := range slices.Backward(t.PushedCommits) {
			if !pc.Pushed.After(since) {
				break
			}
			out = append(out, pc)
		}
	}
	if len(out) == 0 {
		out = []store.PushedCommit{latestPushed(t)}
	}
	return out
}

// readsCommit reports whether t's CI result reads commit: whether it is
// one of t's result commits.
func readsCommit(gate bool, t store.Ticket, commit string) bool {
	return len(t.PushedCommits) > 0 && slices.ContainsFunc(resultCommits(gate, t), func(p store.PushedCommit) bool { return p.Commit == commit })
}

// latestCIResult is the CI result of a Ticket's latest pushed commit from
// its runs. A pushed commit whose runs have all been pruned has no CI
// result: none is running.
func latestCIResult(latest store.PushedCommit, runs []store.CIRun, now time.Time) CIResult {
	if now.Sub(latest.Pushed) > ciKeep && !slices.ContainsFunc(runs, func(run store.CIRun) bool { return run.Commit == latest.Commit }) {
		return ""
	}
	return pushedCIResult(latest, runs, now)
}

// pushedCIResult is the CI result of a pushed commit from runs at now:
// running while a Re-run request waits on the CI Watcher, else its runs'.
func pushedCIResult(p store.PushedCommit, runs []store.CIRun, now time.Time) CIResult {
	if len(pendingReruns(p, runs, now)) > 0 {
		return CIRunning
	}
	return ciResultOf(p.Commit, runs)
}

// ciRerunLapse is how long a Re-run request waits for the run's new
// attempt: one GitHub refuses, say for a run too old to re-run, lapses,
// and the Ticket is CI failed again, with its failure options.
const ciRerunLapse = 10 * time.Minute

// pendingReruns are p's Re-run requests still waiting on the CI Watcher
// at now: those whose run has no later attempt in runs, until the
// request lapses. A request whose run has been pruned waits on nothing.
func pendingReruns(p store.PushedCommit, runs []store.CIRun, now time.Time) []store.CIRunAttempt {
	if now.Sub(p.Rerun) >= ciRerunLapse {
		return nil
	}
	var out []store.CIRunAttempt
	for _, a := range p.Reruns {
		kept, answered := false, false
		for _, run := range runs {
			if run.ID == a.ID && run.Commit == p.Commit {
				kept = kept || run.Attempt == a.Attempt
				answered = answered || run.Attempt > a.Attempt
			}
		}
		if kept && !answered {
			out = append(out, a)
		}
	}
	return out
}

// ticketRuns is every CI Run tracked against t and kept at now, and
// whether t has a pushed commit at all; the Project's runs are read only
// when it has.
func ticketRuns(r store.Reader, key string, t store.Ticket, now time.Time) ([]store.CIRun, bool) {
	if len(t.PushedCommits) == 0 {
		return nil, false
	}
	// Under Pull Request without CI Tracking a push is recorded for its
	// pull request alone: it has no CI result.
	if p, _ := r.Project(key); !p.LandingPolicy().TracksCIResults() {
		return nil, false
	}
	return runsOf(keptCIRuns(r.CIRuns(key), now), t), true
}

// runsOf are the Ticket-tracked runs of the commits t pushed, whichever
// Ticket they were tracked against: Tickets sharing a pushed commit share
// its runs. Runs tracked against a CI Rule are left out.
func runsOf(runs []store.CIRun, t store.Ticket) []store.CIRun {
	var mine []store.CIRun
	for _, run := range runs {
		if run.Ticket != 0 && slices.ContainsFunc(t.PushedCommits, func(p store.PushedCommit) bool { return p.Commit == run.Commit }) {
			mine = append(mine, run)
		}
	}
	return mine
}

// ticketsReading are the Tickets whose CI result reads commit: those whose
// latest pushed commit it is, and under the CI gate those that pushed it
// since they last moved back to Ready.
func ticketsReading(r store.Reader, key, commit string) []store.Ticket {
	p, _ := r.Project(key)
	var out []store.Ticket
	for _, t := range r.Tickets(key) {
		if readsCommit(p.LandingPolicy().CIGateHolds(), t, commit) {
			out = append(out, t)
		}
	}
	return out
}

// countsFor reports whether run counts towards commit's CI result: one
// of commit's runs that isn't ignored.
func countsFor(run store.CIRun, commit string) bool { return run.Commit == commit && !run.Ignored }

// ciResultOf is the CI result of commit from runs: running while any of
// its runs that aren't ignored is unfinished, or none is reported; failed
// if any concluded failure or timed out; else cancelled if any was
// cancelled, a lone cancelled run included; passed otherwise,
// skipped and neutral counting as passed. A re-run's latest attempt
// replaces its earlier ones.
func ciResultOf(commit string, runs []store.CIRun) CIResult {
	latest := countedRuns(commit, runs)
	if len(latest) == 0 {
		return CIRunning
	}
	failed, cancelled := false, false
	for _, run := range latest {
		switch {
		case run.Status != store.CICompleted:
			return CIRunning
		case run.Conclusion.Failed():
			failed = true
		case run.Conclusion.Cancelled():
			cancelled = true
		}
	}
	if failed {
		return CIFailed
	}
	if cancelled {
		return CICancelled
	}
	return CIPassed
}

// failedRuns are the runs of commit, counted towards its CI result, that
// concluded failure or timed out.
func failedRuns(commit string, runs []store.CIRun) []store.CIRun {
	var out []store.CIRun
	for _, run := range countedRuns(commit, runs) {
		if run.Status == store.CICompleted && run.Conclusion.Failed() {
			out = append(out, run)
		}
	}
	return out
}

// rerunnableRuns are the runs of commit, counted towards its CI result,
// that failed or were cancelled: what a Re-run request covers.
func rerunnableRuns(commit string, runs []store.CIRun) []store.CIRun {
	var out []store.CIRun
	for _, run := range countedRuns(commit, runs) {
		if run.Status == store.CICompleted && (run.Conclusion.Failed() || run.Conclusion.Cancelled()) {
			out = append(out, run)
		}
	}
	return out
}

// countedRuns are the runs of commit that count towards its CI result,
// by run ID: those that aren't ignored, each at its latest attempt.
func countedRuns(commit string, runs []store.CIRun) map[int64]store.CIRun {
	latest := map[int64]store.CIRun{}
	for _, run := range runs {
		if !countsFor(run, commit) {
			continue
		}
		if have, ok := latest[run.ID]; !ok || run.Attempt > have.Attempt {
			latest[run.ID] = run
		}
	}
	return latest
}

// ReportPushedCommitInput is the Factory's report_pushed_commit(ticket,
// commit, full?).
type ReportPushedCommitInput struct {
	Ticket Ref    `json:"ticket"`
	Commit string `json:"commit"`
	// Branch is the branch the Factory pushed the commit as, so that the
	// Pull Request Watcher knows the pull request's head branch.
	Branch string `json:"branch,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportPushedCommit is the Factory's report_pushed_commit: the commit it
// pushed for an Accepted Ticket (or an In Review one, merged by hand),
// recorded with when. It is recorded only while CI Tracking and Push are
// on, so turning CI Tracking on tracks only pushes from then on; with
// either off, nothing changes; under Pull Request, which implies Push, it
// is recorded whether or not CI Tracking is on. A newer pushed commit replaces the old one
// as the one the CI result comes from.
func (c *Core) ReportPushedCommit(_ context.Context, key string, in ReportPushedCommitInput) (Written[TicketView], error) {
	commit := strings.TrimSpace(in.Commit)
	if commit == "" {
		return Written[TicketView]{}, invalid("commit", "report_pushed_commit needs the commit the Factory pushed")
	}
	return c.updateTicketIn(key, "report_pushed_commit", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		if t.State != store.Accepted && t.State != store.InReview {
			return false, &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is %s: only an Accepted or In Review Ticket is pushed", DisplayID(key, t.Number), t.State)}
		}
		p, _ := r.Project(key)
		if !p.LandingPolicy().RecordsPushedCommit() {
			return false, nil
		}
		if n := len(t.PushedCommits); n > 0 && t.PushedCommits[n-1].Commit == commit {
			return false, nil
		}
		t.PushedCommits = append(t.PushedCommits, store.PushedCommit{Commit: commit, Branch: strings.TrimSpace(in.Branch), Pushed: c.now()})
		return true, nil
	})
}

// ReportCIRunsInput is the CI Watcher's report_ci_runs(runs, full?).
type ReportCIRunsInput struct {
	Runs []CIRunView `json:"runs"`
	// Full asks for the runs as stored in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// CIRunsView is report_ci_runs' record: the runs it reported, as stored.
type CIRunsView struct {
	Runs []CIRunView `json:"runs"`
}

// ReportCIRuns is the CI Watcher's report_ci_runs: it upserts each run,
// keyed by its run ID and attempt, marking runs of Ignored workflows
// ignored, and prunes the runs finished more than ciKeep ago. It needs
// CI Tracking on. Every Ticket whose runs changed gets a change notice,
// as the Project gets ci.changed, so the UI updates live.
func (c *Core) ReportCIRuns(_ context.Context, key string, in ReportCIRunsInput) (Written[CIRunsView], error) {
	var reported CIRunsView
	changed := map[int]bool{}
	wrote := false
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		if !p.CI.Tracking {
			return &Error{Code: ActionNotAllowed, Message: "CI Tracking is off on " + key + ", so it tracks no CI Runs"}
		}
		stored := w.CIRuns(key)
		kept := keptCIRuns(slices.Clone(stored), c.now())
		runs := slices.Clone(kept)
		pruned := len(kept) < len(stored)
		for i, v := range in.Runs {
			run, err := c.ciRunOf(w, key, p.CI, v, fmt.Sprintf("runs[%d]", i))
			if err != nil {
				return err
			}
			at := slices.IndexFunc(runs, func(r store.CIRun) bool { return r.ID == run.ID && r.Attempt == run.Attempt })
			switch {
			case at < 0:
				runs = append(runs, run)
			case reflect.DeepEqual(runs[at], run):
				reported.Runs = append(reported.Runs, ciRunView(key, run))
				continue
			default:
				if runs[at].Ticket != 0 {
					changed[runs[at].Ticket] = true
				}
				runs[at] = run
			}
			wrote = true
			c.markCIRunChanged(w, key, changed, run)
			reported.Runs = append(reported.Runs, ciRunView(key, run))
		}
		if !wrote && !pruned {
			return nil
		}
		return w.PutCIRuns(key, runs)
	})
	if err != nil {
		return Written[CIRunsView]{}, err
	}
	for n := range changed {
		c.events.Publish(events.Event{Type: events.TicketChanged, Project: key, ID: DisplayID(key, n)})
	}
	if reported.Runs == nil {
		reported.Runs = []CIRunView{}
	}
	if in.Full {
		return Written[CIRunsView]{Record: &reported}, nil
	}
	ack := Acknowledgement{ID: key, Updated: c.now(), Changed: []string{}}
	if wrote {
		ack.Changed = []string{"runs"}
	}
	return Written[CIRunsView]{Acknowledgement: ack}, nil
}

// markCIRunChanged adds to changed every Ticket that reads run: those
// whose CI result reads its commit. A run tracked against a Ticket that
// has since pushed another still marks that Ticket.
func (c *Core) markCIRunChanged(r store.Reader, key string, changed map[int]bool, run store.CIRun) {
	if run.Ticket == 0 {
		return
	}
	changed[run.Ticket] = true
	for _, t := range ticketsReading(r, key, run.Commit) {
		changed[t.Number] = true
	}
}

// ciRunOf is the run v reports, checked, as stored: tracked against an
// existing Ticket or Spec, and ignored when its workflow is.
func (c *Core) ciRunOf(r store.Reader, key string, ci store.CI, v CIRunView, field string) (store.CIRun, error) {
	run := store.CIRun{ID: v.ID, Attempt: v.Attempt, Workflow: strings.TrimSpace(v.Workflow), Name: v.Name, Branch: v.Branch,
		Commit: strings.TrimSpace(v.Commit), Status: v.Status, Conclusion: v.Conclusion, URL: v.URL}
	switch {
	case run.ID <= 0:
		return run, invalid(field+".id", "a CI Run needs its CI Provider's run ID")
	case run.Attempt <= 0:
		return run, invalid(field+".attempt", "a CI Run's attempt counts from 1, not %d", run.Attempt)
	case run.Workflow == "" || path.Base(run.Workflow) != run.Workflow:
		return run, invalid(field+".workflow", "%q isn't a workflow file's base name, such as ci.yml", run.Workflow)
	case run.Commit == "":
		return run, invalid(field+".commit", "a CI Run needs the commit it ran on")
	case !run.Status.Valid():
		return run, invalid(field+".status", "%q isn't a CI Run status (%s)", run.Status, joinNames(store.CIRunStatuses))
	case !run.Conclusion.Valid():
		return run, invalid(field+".conclusion", "%q isn't a CI conclusion (%s)", run.Conclusion, joinNames(store.CIConclusions))
	case (run.Status == store.CICompleted) != (run.Conclusion != ""):
		return run, invalid(field+".conclusion", "a completed CI Run has a conclusion, and an unfinished one has none")
	}
	if v.Started != nil {
		run.Started = v.Started.UTC()
	}
	if v.Finished != nil {
		run.Finished = v.Finished.UTC()
	}
	for j, job := range v.Jobs {
		if !job.Conclusion.Valid() {
			return run, invalid(fmt.Sprintf("%s.jobs[%d].conclusion", field, j), "%q isn't a CI conclusion (%s)", job.Conclusion, joinNames(store.CIConclusions))
		}
		run.Jobs = append(run.Jobs, store.CIJob{Name: job.Name, Conclusion: job.Conclusion, URL: job.URL})
	}
	switch {
	case (v.Ticket == "") == (v.Rule == ""):
		return run, invalid(field+".ticket", "a CI Run is tracked against one Ticket or one CI Rule")
	case v.Ticket != "":
		t, err := resolveTicket(r, key, "report_ci_runs", field+".ticket", Ref(v.Ticket))
		if err != nil {
			return run, err
		}
		run.Ticket = t.Number
	default:
		n, err := Ref(v.Rule).number(key, field+".rule")
		if err != nil {
			return run, err
		}
		if _, ok := r.Spec(key, n); !ok {
			return run, notFound("there is no CI Rule on Spec %s", DisplayID(key, n))
		}
		run.Rule = n
	}
	run.Ignored = slices.Contains(ci.IgnoredWorkflows, run.Workflow)
	return run, nil
}

// joinNames is a closed set of values, for a message.
func joinNames[T ~string](vs []T) string {
	var out []string
	for _, v := range vs {
		out = append(out, string(v))
	}
	return strings.Join(out, ", ")
}

// CIWaitingCommit is a pushed commit the CI Watcher watches: one still
// waiting on a final CI result, or newly pushed.
type CIWaitingCommit struct {
	Ticket string    `json:"ticket"`
	Commit string    `json:"commit"`
	Pushed time.Time `json:"pushed"`
	// PullRequest is the number of the Ticket's pull request, when it has
	// one: Azure Pipelines builds on refs/pull/<n>/merge count towards
	// the commit's CI result.
	PullRequest int `json:"pull_request,omitempty"`
}

// ciWaiting is the pushed commits the CI Watcher watches, once per
// commit, oldest push first: each Ticket's latest pushed commit and,
// under the CI gate, every other it pushed since it last moved back to
// Ready, whose CI result is still running, or that was pushed, or asked
// to be re-run, in the last ciNewlyPushed. A Cancelled Ticket's are left
// out, as is one pushed, or last asked to be re-run, more than
// ciWaitLimit ago.
func ciWaiting(r store.Reader, key string, now time.Time) []CIWaitingCommit {
	out := []CIWaitingCommit{}
	runs := keptCIRuns(r.CIRuns(key), now)
	p, _ := r.Project(key)
	for _, t := range r.Tickets(key) {
		if len(t.PushedCommits) == 0 || t.State == store.Cancelled {
			continue
		}
		mine := runsOf(runs, t)
		for _, pc := range resultCommits(p.LandingPolicy().CIGateHolds(), t) {
			if !ciWatched(pc, mine, now) {
				continue
			}
			w := CIWaitingCommit{Ticket: DisplayID(key, t.Number), Commit: pc.Commit, Pushed: pc.Pushed}
			if t.PullRequest != nil {
				w.PullRequest = t.PullRequest.Number
			}
			out = append(out, w)
		}
	}
	slices.SortStableFunc(out, func(a, b CIWaitingCommit) int { return a.Pushed.Compare(b.Pushed) })
	seen := map[string]bool{}
	return slices.DeleteFunc(out, func(w CIWaitingCommit) bool {
		dup := seen[w.Commit]
		seen[w.Commit] = true
		return dup
	})
}

// ciWatched reports whether the CI Watcher watches pushed commit p, from
// its runs at now: running, or newly pushed or asked to be re-run, and
// within ciWaitLimit of either.
func ciWatched(p store.PushedCommit, runs []store.CIRun, now time.Time) bool {
	// A Re-run request starts the wait again.
	since := p.Pushed
	if p.Rerun.After(since) {
		since = p.Rerun
	}
	if now.Sub(since) >= ciWaitLimit {
		return false
	}
	return pushedCIResult(p, runs, now) == CIRunning || now.Sub(since) < ciNewlyPushed
}
