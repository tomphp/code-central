package core

import (
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// ciState is a Project's Ticket CI state (Spec code-central-568): its
// Tickets' CI as every reader sees it, worked out from the Project's CI
// Runs, its CI responses and every Ticket's pushed commits as one read
// has them. For each Ticket it answers its CI result and the commit the
// result comes from, whether it is CI failed, CI cancelled or CI
// unavailable, whether the CI gate holds it waiting on a running result,
// its pending Re-run requests and the CI item that lists it. Each
// Ticket's answers are worked out the first time they are asked for and
// kept for the rest of the read, as are the Project's runs and CI items,
// so a reader of one Ticket pays for that Ticket and a reader of every
// Ticket pays once. It is built inside one store read and never kept
// past it.
type ciState struct {
	c   *Core
	r   store.Reader
	p   store.Project
	now time.Time
	// runs are the Project's CI Runs kept at now, read on first use.
	runs     []store.CIRun
	runsRead bool
	tickets  map[int]*ticketCI
	// ciItems are the Project's CI items, built on first use.
	ciItems []ciItem
	built   bool
}

// ticketCI is one Ticket's answers in its Project's CI state.
type ticketCI struct {
	// tracked says the Ticket has a pushed commit with CI results; every
	// other answer is empty when it hasn't.
	tracked bool
	latest  store.PushedCommit
	// runs are the Ticket-tracked runs of every commit it pushed.
	runs []store.CIRun
	// commit is the pushed commit its result comes from, "" while it has
	// no result.
	commit string
	result CIResult
	// failure is set while it is CI failed, cancelled while it is CI
	// cancelled, each since its first failed or cancelled run finished.
	failure   *CIFailureSummary
	cancelled bool
	since     time.Time
	// waiting says the CI gate holds it on a running result; unavailable
	// is why it is CI unavailable, once it has also waited too long.
	waiting     bool
	unavailable *CIUnavailableView
	// reruns are the Re-run requests for its latest pushed commit still
	// waiting on the CI Watcher.
	reruns []store.CIRunAttempt
}

// ciStateOf is p's CI state as r has it at now.
func (c *Core) ciStateOf(r store.Reader, p store.Project) *ciState {
	return &ciState{c: c, r: r, p: p, now: c.now(), tickets: map[int]*ticketCI{}}
}

// keptRuns are the Project's CI Runs kept at now.
func (s *ciState) keptRuns() []store.CIRun {
	if !s.runsRead {
		s.runs, s.runsRead = keptCIRuns(s.r.CIRuns(s.p.Key), s.now), true
	}
	return s.runs
}

// response is the Project's response to commit, zero when it has none.
func (s *ciState) response(commit string) store.CIResponse {
	return responseOf(s.p, commit)
}

// of is t's answers.
func (s *ciState) of(t store.Ticket) *ticketCI {
	if a, ok := s.tickets[t.Number]; ok {
		return a
	}
	a := &ticketCI{}
	s.tickets[t.Number] = a
	// Under Pull Request without CI Tracking a push is recorded for its
	// pull request alone: it has no CI result.
	if len(t.PushedCommits) == 0 || !s.p.LandingPolicy().TracksCIResults() {
		return a
	}
	a.tracked, a.latest = true, latestPushed(t)
	a.runs = runsOf(s.keptRuns(), t)
	a.commit, a.result = s.result(t, a.runs)
	a.reruns = s.pendingReruns(a.latest.Commit, a.runs)
	if t.State != store.Accepted {
		return a
	}
	s.unhandled(a)
	if s.p.LandingPolicy().CIGateHolds() && a.result == CIRunning {
		a.waiting, a.unavailable = true, s.unavailable(a)
	}
	return a
}

// result is t's CI result from its runs, and the commit it comes from:
// the newest of its result commits that passed, if any has, otherwise
// its latest pushed commit (code-central-430); no commit while it has no
// CI result. A latest pushed commit whose runs have all been pruned has
// no CI result: none is running.
func (s *ciState) result(t store.Ticket, runs []store.CIRun) (commit string, result CIResult) {
	for _, pc := range s.resultCommits(t) {
		if s.commitResult(pc.Commit, runs) == CIPassed {
			return pc.Commit, CIPassed
		}
	}
	latest := latestPushed(t)
	if s.now.Sub(latest.Pushed) > ciKeep && !slices.ContainsFunc(runs, func(run store.CIRun) bool { return run.Commit == latest.Commit }) {
		return "", ""
	}
	return latest.Commit, s.commitResult(latest.Commit, runs)
}

// unhandled marks a CI failed or CI cancelled when the human hasn't
// dealt with its latest pushed commit's result: chosen Land anyway, or,
// for a failure, opened a fix Ticket that is still open.
func (s *ciState) unhandled(a *ticketCI) {
	commit := a.latest.Commit
	if !s.response(commit).LandAnyway.IsZero() {
		return
	}
	var finished []store.CIRun
	switch a.result {
	case CIFailed:
		if fixTicketOpen(s.r, s.p, commit) {
			return
		}
		finished = failedRuns(commit, a.runs)
		a.failure = &CIFailureSummary{Commit: commit, Workflows: []string{}}
		for _, run := range finished {
			a.failure.Workflows = append(a.failure.Workflows, run.Workflow)
		}
		slices.Sort(a.failure.Workflows)
		a.failure.Workflows = slices.Compact(a.failure.Workflows)
	case CICancelled:
		a.cancelled = true
		finished = rerunnableRuns(commit, a.runs)
	default:
		return
	}
	for _, run := range finished {
		if at := run.Finished; !at.IsZero() && (a.since.IsZero() || at.Before(a.since)) {
			a.since = at
		}
	}
	if a.since.IsZero() {
		a.since = a.latest.Pushed
	}
}

// unavailable is why a, which the CI gate holds on a running result, is
// CI unavailable, or nil: it has gone CIUnavailableAfter since its push
// while the CI Watcher isn't Watching, or with no runs reported for its
// latest pushed commit.
func (s *ciState) unavailable(a *ticketCI) *CIUnavailableView {
	if s.now.Sub(a.latest.Pushed) < CIUnavailableAfter {
		return nil
	}
	view := &CIUnavailableView{Commit: a.latest.Commit, Pushed: a.latest.Pushed}
	if w := s.c.watchers.view(watcherKey{WatcherCI, s.p.Key}); w.State != WatcherWatching {
		view.Cause, view.Message = CIWatcherNotWatching, w.Message
		return view
	}
	if slices.ContainsFunc(a.runs, func(run store.CIRun) bool { return countsFor(run, a.latest.Commit) }) {
		return nil
	}
	view.Cause = CINoRuns
	return view
}

// reason is t's CI reason, since when, and its detail, while a CI item
// lists t: CI failed, else CI cancelled, else CI unavailable.
func (s *ciState) reason(t store.Ticket) (reason NeedsMeReason, since time.Time, failure *CIFailureSummary, unavailable *CIUnavailableView) {
	a := s.of(t)
	switch {
	case a.failure != nil:
		return ReasonCIFailed, a.since, a.failure, nil
	case a.cancelled:
		return ReasonCICancelled, a.since, nil, nil
	case a.unavailable != nil:
		return ReasonCIUnavailable, a.unavailable.Pushed, nil, a.unavailable
	}
	return "", time.Time{}, nil, nil
}

// resultCommits are the pushed commits t's CI result reads, newest
// first: under the CI gate those pushed since it last moved back to
// Ready, any of which passing passes t; otherwise, or when it has pushed
// none since, its latest. t must have a pushed commit.
func (s *ciState) resultCommits(t store.Ticket) []store.PushedCommit {
	var out []store.PushedCommit
	if s.p.LandingPolicy().CIGateHolds() {
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

// reads reports whether some Ticket's CI result reads commit: whether it
// is one of that Ticket's result commits.
func (s *ciState) reads(commit string) bool { return len(s.ticketsReading(commit)) > 0 }

// ticketsReading are the Tickets whose CI result reads commit: those whose
// latest pushed commit it is, and under the CI gate those that pushed it
// since they last moved back to Ready.
func (s *ciState) ticketsReading(commit string) []store.Ticket {
	var out []store.Ticket
	for _, t := range s.r.Tickets(s.p.Key) {
		if len(t.PushedCommits) > 0 && slices.ContainsFunc(s.resultCommits(t), func(pc store.PushedCommit) bool { return pc.Commit == commit }) {
			out = append(out, t)
		}
	}
	return out
}

// commitResult is the CI result of commit from runs: running while a
// Re-run request waits on the CI Watcher, else its runs'.
func (s *ciState) commitResult(commit string, runs []store.CIRun) CIResult {
	if len(s.pendingReruns(commit, runs)) > 0 {
		return CIRunning
	}
	return ciResultOf(commit, runs)
}

// pendingReruns are the Project's Re-run requests for commit still
// waiting on the CI Watcher: those whose run has no later attempt in
// runs, until the request lapses. A request whose run has been pruned
// waits on nothing.
func (s *ciState) pendingReruns(commit string, runs []store.CIRun) []store.CIRunAttempt {
	res := s.response(commit)
	if s.now.Sub(res.Rerun) >= ciRerunLapse {
		return nil
	}
	var out []store.CIRunAttempt
	for _, a := range res.Reruns {
		kept, answered := false, false
		for _, run := range runs {
			if run.ID == a.ID && run.Commit == commit {
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

// watched reports whether the CI Watcher watches pushed commit pc, from
// its runs: running, or newly pushed or asked to be re-run, and within
// ciWaitLimit of either.
func (s *ciState) watched(pc store.PushedCommit, runs []store.CIRun) bool {
	// A Re-run request starts the wait again.
	since := pc.Pushed
	if rerun := s.response(pc.Commit).Rerun; rerun.After(since) {
		since = rerun
	}
	if s.now.Sub(since) >= ciWaitLimit {
		return false
	}
	return s.commitResult(pc.Commit, runs) == CIRunning || s.now.Sub(since) < ciNewlyPushed
}

// landAnyway reports whether the human chose Land anyway for t's latest
// pushed commit.
func (s *ciState) landAnyway(t store.Ticket) bool {
	return len(t.PushedCommits) > 0 && !s.response(latestPushed(t).Commit).LandAnyway.IsZero()
}

// view is t's latest pushed commit, its CI result, the pushed commit the
// result comes from and every CI Run tracked against it, newest pushed
// commit's first; all empty for a Ticket never pushed while CI Tracking
// was on.
func (s *ciState) view(t store.Ticket) (*PushedCommitView, CIResult, string, []CIRunView) {
	a := s.of(t)
	if !a.tracked {
		return nil, "", "", nil
	}
	mine := slices.Clone(a.runs)
	// The latest pushed commit's runs first, then each older one's.
	order := func(commit string) int {
		return len(t.PushedCommits) - 1 - slices.IndexFunc(t.PushedCommits, func(p store.PushedCommit) bool { return p.Commit == commit })
	}
	slices.SortStableFunc(mine, func(a, b store.CIRun) int { return order(a.Commit) - order(b.Commit) })
	var runs []CIRunView
	for _, run := range mine {
		runs = append(runs, ciRunView(s.p.Key, run))
	}
	return &PushedCommitView{Commit: a.latest.Commit, Pushed: a.latest.Pushed}, a.result, a.commit, runs
}

// failureOptions are the failure options the human may choose on t's CI
// item while t is CI failed or CI cancelled, nil otherwise; see
// ciItemOptions.
func (s *ciState) failureOptions(t store.Ticket) []string {
	it, ok := s.itemOf(t)
	if !ok || it.reason == ReasonCIUnavailable {
		return nil
	}
	return ciItemOptions(s.r, s.p, it)
}
