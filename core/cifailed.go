package core

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// CI failed (code-central-261): an Accepted Ticket whose CI result is
// failed waits on the human in Needs Me until they choose one of the
// failure options. Land anyway, the human's land_anyway, accepts the
// failed CI result, and the Factory lands the Ticket on its next poll.
// Re-run failed jobs, the human's rerun_ci (code-central-265), asks the
// CI Watcher to re-run the failed runs, and the Ticket waits on CI again.

// The failure options, each named for its operation.
const (
	// OptionRerunCI is rerun_ci, offered under both Push Landing Methods.
	OptionRerunCI = "rerun_ci"
	// OptionLandAnyway is land_anyway, offered only under Merge: under
	// Branch the human's merge decides.
	OptionLandAnyway = "land_anyway"
)

// CIFailureSummary is what failed on a CI failed Ticket's pushed commit:
// the commit and its failed runs' workflow file names.
type CIFailureSummary struct {
	Commit    string   `json:"commit"`
	Workflows []string `json:"workflows"`
}

// ciFailure is t's failure while it is CI failed: an Accepted Ticket
// whose latest pushed commit's CI result is failed and that the human
// hasn't dealt with, from its runs kept at now. since is when its first
// failed run finished.
func ciFailure(r store.Reader, key string, t store.Ticket, now time.Time) (s CIFailureSummary, since time.Time, ok bool) {
	if t.State != store.Accepted || len(t.PushedCommits) == 0 {
		return CIFailureSummary{}, time.Time{}, false
	}
	latest := latestPushed(t)
	if !latest.LandAnyway.IsZero() {
		return CIFailureSummary{}, time.Time{}, false
	}
	mine, _ := ticketRuns(r, key, t, now)
	if latestCIResult(latest, mine, now) != CIFailed {
		return CIFailureSummary{}, time.Time{}, false
	}
	s = CIFailureSummary{Commit: latest.Commit, Workflows: []string{}}
	for _, run := range failedRuns(latest.Commit, mine) {
		s.Workflows = append(s.Workflows, run.Workflow)
		if at := run.Finished; !at.IsZero() && (since.IsZero() || at.Before(since)) {
			since = at
		}
	}
	slices.Sort(s.Workflows)
	s.Workflows = slices.Compact(s.Workflows)
	if since.IsZero() {
		since = latest.Pushed
	}
	return s, since, true
}

// ciFailureOptions are the failure options the human may choose for t
// while it is CI failed, nil otherwise: Re-run failed jobs, and Land
// anyway under Merge only, as under Branch the human's merge decides.
func ciFailureOptions(r store.Reader, key string, t store.Ticket, now time.Time) []string {
	if _, _, ok := ciFailure(r, key, t, now); !ok {
		return nil
	}
	p, _ := r.Project(key)
	if p.Landing.Method == store.LandingBranch {
		return []string{OptionRerunCI}
	}
	return []string{OptionRerunCI, OptionLandAnyway}
}

// latestPushed is t's latest pushed commit, the one its CI result comes
// from; t must have one.
func latestPushed(t store.Ticket) store.PushedCommit {
	return t.PushedCommits[len(t.PushedCommits)-1]
}

// choseLandAnyway reports whether the human chose Land anyway for t's
// latest pushed commit.
func choseLandAnyway(t store.Ticket) bool {
	return len(t.PushedCommits) > 0 && !latestPushed(t).LandAnyway.IsZero()
}

// LandAnyway is the human's land_anyway(ticket): it accepts the failed CI
// result of an Accepted Ticket's latest pushed commit, recorded on that
// commit, so the Factory lands the Ticket on its next poll. A newer
// pushed commit has its own CI result, which it doesn't cover. Choosing
// it again changes nothing.
func (c *Core) LandAnyway(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "land_anyway"); err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, "land_anyway", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		if choseLandAnyway(*t) && t.State == store.Accepted {
			return false, nil
		}
		if !slices.Contains(ciFailureOptions(r, key, *t, c.now()), OptionLandAnyway) {
			return false, &Error{Code: ActionNotAllowed, Message: whyNotLandAnyway(r, key, *t, c.now())}
		}
		t.PushedCommits[len(t.PushedCommits)-1].LandAnyway = c.now()
		return true, nil
	})
}

// whyNotLandAnyway says why t isn't offered Land anyway.
func whyNotLandAnyway(r store.Reader, key string, t store.Ticket, now time.Time) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed lands anyway", id, t.State)
	}
	if _, _, ok := ciFailure(r, key, t, now); !ok {
		return fmt.Sprintf("%s has no failed CI result to land anyway", id)
	}
	return fmt.Sprintf("%s's Project lands by Branch, so your merge decides whether it lands, whatever its CI result", id)
}

// RerunCI is the human's rerun_ci(ticket): a Re-run request for each run
// that failed on a CI failed Ticket's latest pushed commit, at its latest
// attempt, recorded on that commit in place of any earlier request. The
// Ticket's CI result reads running until the CI Watcher, which re-runs
// the runs' failed jobs, reports each run's new attempt, which answers
// the request, or the request lapses. Asking again while it waits
// changes nothing.
func (c *Core) RerunCI(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "rerun_ci"); err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, "rerun_ci", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		mine, _ := ticketRuns(r, key, *t, c.now())
		if rerunPending(*t, mine, c.now()) && t.State == store.Accepted {
			return false, nil
		}
		if !slices.Contains(ciFailureOptions(r, key, *t, c.now()), OptionRerunCI) {
			return false, &Error{Code: ActionNotAllowed, Message: whyNotRerun(r, key, *t, c.now())}
		}
		latest := &t.PushedCommits[len(t.PushedCommits)-1]
		latest.Reruns = nil
		for _, run := range failedRuns(latest.Commit, mine) {
			latest.Reruns = append(latest.Reruns, store.CIRunAttempt{ID: run.ID, Attempt: run.Attempt})
		}
		slices.SortFunc(latest.Reruns, func(a, b store.CIRunAttempt) int { return cmp.Compare(a.ID, b.ID) })
		latest.Rerun = c.now()
		return true, nil
	})
}

// rerunPending reports whether a Re-run request for t's latest pushed
// commit waits on the CI Watcher, among t's runs at now.
func rerunPending(t store.Ticket, runs []store.CIRun, now time.Time) bool {
	return len(t.PushedCommits) > 0 && len(pendingReruns(latestPushed(t), runs, now)) > 0
}

// whyNotRerun says why t isn't offered Re-run failed jobs.
func whyNotRerun(r store.Reader, key string, t store.Ticket, now time.Time) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed has failed jobs to re-run", id, t.State)
	}
	return fmt.Sprintf("%s has no failed CI result to re-run", id)
}
