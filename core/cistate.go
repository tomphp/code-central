package core

import (
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// The CI state: one Project's Tickets' CI, worked out once per read, so
// every reader gets the same answer. It is the Ticket CI state of Spec
// code-central-568, an implementation name kept out of the glossary.

// ciState is a Project's CI state, worked out from the Project's CI
// Runs, its CI responses and every Ticket's pushed commits as one read
// has them. For each Ticket and each watched Repository it pushed in, it
// answers the Repository's CI result and the commit the result comes
// from, whether it is CI failed, CI cancelled or CI unavailable there,
// whether the CI gate holds it there waiting on a running result and its
// pending Re-run requests; the Ticket's own answers combine them, and the
// Project's evaluation groups the Tickets waiting on CI into CI items,
// one per Repository and pushed commit. Each Ticket's answers are worked
// out the first time they are asked for and kept for the rest of the
// read, as are the Project's runs, so a reader of one Ticket pays for
// that Ticket, and a reader of every Ticket pays once. It is built
// inside one store read and never kept past it.
type ciState struct {
	c   *Core
	r   store.Reader
	p   store.Project
	now time.Time
	// runs are the Project's CI Runs kept at now, read on first use.
	runs     []store.CIRun
	runsRead bool
	tickets  map[int]*ticketCI
}

// ticketCI is one Ticket's answers in its Project's CI state: one per
// watched Repository it pushed in, and what they come to.
type ticketCI struct {
	// repos are its answers in each watched Repository it pushed in, in
	// landing order; none for a Ticket with no CI result.
	repos []*repoCI
	// result is its combined CI result: running while any Repository's
	// is, else failed, else cancelled, else passed; "" while none has
	// one.
	result CIResult
	// failed is set while it is CI failed in some Repository, cancelled
	// while it is CI cancelled in one and failed in none, each since the
	// earliest of those Repositories'.
	failed    bool
	cancelled bool
	since     time.Time
	// waiting says the CI gate holds it on a running result in some
	// Repository; unavailable is the earliest pushed of those that are CI
	// unavailable.
	waiting     bool
	unavailable *CIUnavailableView
}

// repoCI is one Ticket's answers in one watched Repository.
type repoCI struct {
	// name is the Repository Name, the Project Key for the Home
	// Repository; record is the name as records keep it, "" for the Home.
	name, record string
	// pushed are the commits the Ticket pushed there, oldest first, and
	// latest the last of them.
	pushed []store.PushedCommit
	latest store.PushedCommit
	// runs are the Ticket-tracked runs of every commit it pushed there.
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

// response is the Project's response to commit in the Repository record
// names, zero when it has none.
func (s *ciState) response(record, commit string) store.CIResponse {
	return responseOf(s.p, record, commit)
}

// ticket is t's answers, worked out on the first ask.
func (s *ciState) ticket(t store.Ticket) *ticketCI {
	if a, ok := s.tickets[t.Number]; ok {
		return a
	}
	a := &ticketCI{}
	s.tickets[t.Number] = a
	if len(t.PushedCommits) == 0 {
		return a
	}
	for _, name := range namesInLandingOrder(s.p, pushedRepositories(s.p.Key, t)) {
		// Under Pull Request without CI Tracking a push is recorded for
		// its pull request alone: it has no CI result.
		if watchesCI(s.p, name) && landingPolicyIn(s.p, name).TracksCIResults() {
			a.repos = append(a.repos, s.repo(t, name))
		}
	}
	a.combine()
	return a
}

// repo is t's answers in the watched Repository name, where it pushed.
func (s *ciState) repo(t store.Ticket, name string) *repoCI {
	rc := &repoCI{name: name, record: recordName(s.p.Key, name), pushed: pushedIn(s.p.Key, t, name)}
	rc.latest = rc.pushed[len(rc.pushed)-1]
	rc.runs = runsIn(s.keptRuns(), rc.record, rc.pushed)
	rc.commit, rc.result = s.result(t, rc)
	rc.reruns = s.pendingReruns(rc.record, rc.latest.Commit, rc.runs)
	// Work that has landed in the Repository stays landed: its CI no
	// longer waits on anything.
	if t.State != store.Accepted || landedIn(t, name) {
		return rc
	}
	s.unhandled(rc)
	if landingPolicyIn(s.p, name).CIGateHolds() && rc.result == CIRunning {
		rc.waiting, rc.unavailable = true, s.unavailable(rc)
	}
	return rc
}

// combine works a's own answers out from its Repositories'.
func (a *ticketCI) combine() {
	has := map[CIResult]bool{}
	for _, rc := range a.repos {
		has[rc.result] = true
		a.waiting = a.waiting || rc.waiting
		if u := rc.unavailable; u != nil && (a.unavailable == nil || u.Pushed.Before(a.unavailable.Pushed)) {
			a.unavailable = u
		}
	}
	for _, r := range []CIResult{CIRunning, CIFailed, CICancelled, CIPassed} {
		if has[r] {
			a.result = r
			break
		}
	}
	for _, rc := range a.repos {
		if rc.failure != nil && (!a.failed || rc.since.Before(a.since)) {
			a.failed, a.since = true, rc.since
		}
	}
	if a.failed {
		return
	}
	for _, rc := range a.repos {
		if rc.cancelled && (!a.cancelled || rc.since.Before(a.since)) {
			a.cancelled, a.since = true, rc.since
		}
	}
}

// in is a's answers in the Repository name, nil when it has no CI
// result there.
func (a *ticketCI) in(name string) *repoCI {
	for _, rc := range a.repos {
		if rc.name == name {
			return rc
		}
	}
	return nil
}

// result is t's CI result in rc's Repository from its runs, and the
// commit it comes from: the newest of its result commits there that
// passed, if any has, otherwise its latest pushed commit there
// (code-central-430); no commit while it has no CI result. A latest
// pushed commit whose runs have all been pruned has no CI result: none is
// running.
func (s *ciState) result(t store.Ticket, rc *repoCI) (commit string, result CIResult) {
	for _, pc := range s.resultCommits(t, rc.name, rc.pushed) {
		if s.commitResult(rc.record, pc.Commit, rc.runs) == CIPassed {
			return pc.Commit, CIPassed
		}
	}
	latest := rc.latest
	if s.now.Sub(latest.Pushed) > ciKeep && !slices.ContainsFunc(rc.runs, func(run store.CIRun) bool { return run.Commit == latest.Commit }) {
		return "", ""
	}
	return latest.Commit, s.commitResult(rc.record, latest.Commit, rc.runs)
}

// unhandled marks rc CI failed or CI cancelled when the human hasn't
// dealt with its latest pushed commit's result: chosen Land anyway, or,
// for a failure, opened a fix Ticket that is still open.
func (s *ciState) unhandled(rc *repoCI) {
	commit := rc.latest.Commit
	if !s.response(rc.record, commit).LandAnyway.IsZero() {
		return
	}
	var finished []store.CIRun
	switch rc.result {
	case CIFailed:
		if fixTicketOpen(s.r, s.p, rc.record, commit) {
			return
		}
		finished = failedRuns(commit, rc.runs)
		rc.failure = &CIFailureSummary{Commit: commit, Workflows: []string{}}
		for _, run := range finished {
			rc.failure.Workflows = append(rc.failure.Workflows, run.Workflow)
		}
		slices.Sort(rc.failure.Workflows)
		rc.failure.Workflows = slices.Compact(rc.failure.Workflows)
	case CICancelled:
		rc.cancelled = true
		finished = rerunnableRuns(commit, rc.runs)
	default:
		return
	}
	for _, run := range finished {
		if at := run.Finished; !at.IsZero() && (rc.since.IsZero() || at.Before(rc.since)) {
			rc.since = at
		}
	}
	if rc.since.IsZero() {
		rc.since = rc.latest.Pushed
	}
}

// unavailable is why rc, which the CI gate holds on a running result, is
// CI unavailable, or nil: it has gone CIUnavailableAfter since its push
// while the CI Watcher isn't Watching its Repository, or with no runs
// reported for its latest pushed commit.
func (s *ciState) unavailable(rc *repoCI) *CIUnavailableView {
	if s.now.Sub(rc.latest.Pushed) < CIUnavailableAfter {
		return nil
	}
	view := &CIUnavailableView{Commit: rc.latest.Commit, Pushed: rc.latest.Pushed}
	if w := s.c.watchers.view(watcherKey{WatcherCI, s.p.Key}); !w.watches(rc.name) {
		view.Cause, view.Message = CIWatcherNotWatching, w.messageFor(rc.name)
		return view
	}
	if slices.ContainsFunc(rc.runs, func(run store.CIRun) bool { return countsFor(run, rc.latest.Commit) }) {
		return nil
	}
	view.Cause = CINoRuns
	return view
}

// resultCommits are the commits of pushed, a Ticket's pushed commits in
// the Repository name, that its CI result there reads, newest first:
// under the CI gate there those pushed since it last moved back to Ready,
// any of which passing passes it; otherwise, or when it has pushed none
// since, its latest. pushed must hold one.
func (s *ciState) resultCommits(t store.Ticket, name string, pushed []store.PushedCommit) []store.PushedCommit {
	var out []store.PushedCommit
	if landingPolicyIn(s.p, name).CIGateHolds() {
		since := backToReady(t)
		for _, pc := range slices.Backward(pushed) {
			if !pc.Pushed.After(since) {
				break
			}
			out = append(out, pc)
		}
	}
	if len(out) == 0 {
		out = []store.PushedCommit{pushed[len(pushed)-1]}
	}
	return out
}

// reads reports whether some Ticket's CI result reads commit in the
// Repository record names: whether it is one of that Ticket's result
// commits there.
func (s *ciState) reads(record, commit string) bool { return len(s.ticketsReading(record, commit)) > 0 }

// ticketsReading are the Tickets whose CI result reads commit in the
// Repository record names: those whose latest pushed commit there it is,
// and under the CI gate those that pushed it there since they last moved
// back to Ready.
func (s *ciState) ticketsReading(record, commit string) []store.Ticket {
	name := recordRepository(s.p.Key, record)
	var out []store.Ticket
	for _, t := range s.r.Tickets(s.p.Key) {
		pushed := pushedIn(s.p.Key, t, name)
		if len(pushed) > 0 && slices.ContainsFunc(s.resultCommits(t, name, pushed), func(pc store.PushedCommit) bool { return pc.Commit == commit }) {
			out = append(out, t)
		}
	}
	return out
}

// commitResult is the CI result of commit, in the Repository record
// names, from runs: running while a Re-run request waits on the CI
// Watcher, else its runs'.
func (s *ciState) commitResult(record, commit string, runs []store.CIRun) CIResult {
	if len(s.pendingReruns(record, commit, runs)) > 0 {
		return CIRunning
	}
	return ciResultOf(commit, runs)
}

// pendingReruns are the Project's Re-run requests for commit, in the
// Repository record names, still waiting on the CI Watcher: those whose
// run has no later attempt in runs, until the request lapses. A request
// whose run has been pruned waits on nothing.
func (s *ciState) pendingReruns(record, commit string, runs []store.CIRun) []store.CIRunAttempt {
	res := s.response(record, commit)
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

// watched reports whether the CI Watcher watches pc, one of rc's pushed
// commits, from rc's runs: running, or newly pushed or asked to be
// re-run, and within ciWaitLimit of either.
func (s *ciState) watched(rc *repoCI, pc store.PushedCommit) bool {
	// A Re-run request starts the wait again.
	since := pc.Pushed
	if rerun := s.response(rc.record, pc.Commit).Rerun; rerun.After(since) {
		since = rerun
	}
	if s.now.Sub(since) >= ciWaitLimit {
		return false
	}
	return s.commitResult(rc.record, pc.Commit, rc.runs) == CIRunning || s.now.Sub(since) < ciNewlyPushed
}

// landAnyway reports whether the human chose Land anyway for rc's latest
// pushed commit; false for a nil rc.
func (s *ciState) landAnyway(rc *repoCI) bool {
	return rc != nil && !s.response(rc.record, rc.latest.Commit).LandAnyway.IsZero()
}

// view is t's latest pushed commit in the Home Repository, its combined
// CI result, the Home Repository's pushed commit that result comes from
// and every CI Run tracked against it, the Home Repository's first and
// each newest pushed commit's first; all empty for a Ticket never pushed
// while CI Tracking was on.
func (s *ciState) view(t store.Ticket) (*PushedCommitView, CIResult, string, []CIRunView) {
	a := s.ticket(t)
	if len(a.repos) == 0 {
		return nil, "", "", nil
	}
	var runs []CIRunView
	for _, rc := range a.repos {
		runs = append(runs, s.runViews(rc)...)
	}
	var pushed *PushedCommitView
	var commit string
	if home := a.in(s.p.Key); home != nil {
		pushed, commit = &PushedCommitView{Commit: home.latest.Commit, Pushed: home.latest.Pushed}, home.commit
	}
	return pushed, a.result, commit, runs
}

// runViews are rc's runs, its latest pushed commit's first, then each
// older one's.
func (s *ciState) runViews(rc *repoCI) []CIRunView {
	mine := slices.Clone(rc.runs)
	order := func(commit string) int {
		return len(rc.pushed) - 1 - slices.IndexFunc(rc.pushed, func(p store.PushedCommit) bool { return p.Commit == commit })
	}
	slices.SortStableFunc(mine, func(x, y store.CIRun) int { return order(x.Commit) - order(y.Commit) })
	var out []CIRunView
	for _, run := range mine {
		out = append(out, ciRunView(s.p.Key, run))
	}
	return out
}
